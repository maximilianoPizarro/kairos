/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	defaultTimeoutSeconds = 30
	systemPrompt          = `You are a Kubernetes resource optimizer agent. Analyze workload metrics and current resource allocation, then recommend a scaling action.

Respond with a single JSON object only (no markdown fences) using this schema:
{
  "action": "increase_cpu" | "increase_memory" | "add_replicas" | "decrease_resources" | "no_action",
  "reason": "brief explanation",
  "confidence": 0.0-1.0,
  "details": {"key": "value"}
}

Prefer vertical scaling (CPU/memory) before adding replicas. Use "no_action" when metrics are healthy or changes are unnecessary.`
)

var (
	ErrAIDisabled     = errors.New("AI client is disabled")
	ErrEmptyResponse  = errors.New("AI returned an empty response")
	ErrInvalidPayload = errors.New("AI returned an invalid recommendation payload")
)

// ScalingRecommendation describes an AI-generated scaling decision.
type ScalingRecommendation struct {
	Action     string
	Reason     string
	Confidence float64
	Details    map[string]string
}

// RecommendationRequest carries workload context for the AI model.
type RecommendationRequest struct {
	ResourceName    string
	Namespace       string
	CurrentCPU      string
	CurrentMemory   string
	CurrentReplicas int32
	MetricName      string
	MetricValue     float64
	Threshold       string
	History         []string
}

// AIClient requests scaling recommendations from an OpenAI-compatible endpoint.
type AIClient interface {
	GetScalingRecommendation(ctx context.Context, request RecommendationRequest) (*ScalingRecommendation, error)
	IsAvailable(ctx context.Context) bool
}

type openAIClient struct {
	apiURL  string
	model   string
	apiKey  string
	timeout time.Duration
	client  *http.Client
}

type noopClient struct{}

type chatCompletionRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Temperature float64       `json:"temperature"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatCompletionResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error,omitempty"`
}

type recommendationPayload struct {
	Action     string            `json:"action"`
	Reason     string            `json:"reason"`
	Confidence float64           `json:"confidence"`
	Details    map[string]string `json:"details"`
}

// NewAIClient creates an OpenAI-compatible client. When apiURL or model is empty, a no-op
// client is returned that always recommends no_action.
func NewAIClient(apiURL, model, apiKey string, timeoutSeconds int) AIClient {
	apiURL = strings.TrimSpace(apiURL)
	model = strings.TrimSpace(model)
	if apiURL == "" || model == "" {
		return &noopClient{}
	}

	if timeoutSeconds <= 0 {
		timeoutSeconds = defaultTimeoutSeconds
	}

	return &openAIClient{
		apiURL:  strings.TrimRight(apiURL, "/"),
		model:   model,
		apiKey:  strings.TrimSpace(apiKey),
		timeout: time.Duration(timeoutSeconds) * time.Second,
		client:  &http.Client{},
	}
}

func (c *noopClient) GetScalingRecommendation(_ context.Context, _ RecommendationRequest) (*ScalingRecommendation, error) {
	return &ScalingRecommendation{
		Action:     "no_action",
		Reason:     "AI scaling is disabled",
		Confidence: 1.0,
		Details:    map[string]string{"source": "noop-client"},
	}, nil
}

func (c *noopClient) IsAvailable(_ context.Context) bool {
	return false
}

func (c *openAIClient) GetScalingRecommendation(ctx context.Context, request RecommendationRequest) (*ScalingRecommendation, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	body, err := json.Marshal(chatCompletionRequest{
		Model: c.model,
		Messages: []chatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: formatRecommendationContext(request)},
		},
		Temperature: 0.2,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal chat completion request: %w", err)
	}

	endpoint := c.chatCompletionsURL()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create chat completion request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call AI endpoint: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("read AI response: %w", err)
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("AI endpoint returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var completion chatCompletionResponse
	if err := json.Unmarshal(respBody, &completion); err != nil {
		return nil, fmt.Errorf("decode AI response: %w", err)
	}
	if completion.Error != nil {
		return nil, fmt.Errorf("AI endpoint error: %s", completion.Error.Message)
	}
	if len(completion.Choices) == 0 || strings.TrimSpace(completion.Choices[0].Message.Content) == "" {
		return nil, ErrEmptyResponse
	}

	recommendation, err := parseRecommendationContent(completion.Choices[0].Message.Content)
	if err != nil {
		return nil, err
	}

	return recommendation, nil
}

func (c *openAIClient) IsAvailable(ctx context.Context) bool {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.apiURL, nil)
	if err != nil {
		return false
	}
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return false
	}
	defer func() { _ = resp.Body.Close() }()

	return resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusMultipleChoices
}

func (c *openAIClient) chatCompletionsURL() string {
	if strings.HasSuffix(c.apiURL, "/chat/completions") {
		return c.apiURL
	}
	return c.apiURL + "/chat/completions"
}

func formatRecommendationContext(request RecommendationRequest) string {
	var builder strings.Builder
	builder.WriteString("Analyze this Kubernetes workload and recommend a scaling action.\n\n")
	builder.WriteString(fmt.Sprintf("Resource: %s/%s\n", request.Namespace, request.ResourceName))
	builder.WriteString(fmt.Sprintf("Current CPU: %s\n", request.CurrentCPU))
	builder.WriteString(fmt.Sprintf("Current Memory: %s\n", request.CurrentMemory))
	builder.WriteString(fmt.Sprintf("Current Replicas: %d\n", request.CurrentReplicas))
	builder.WriteString(fmt.Sprintf("Metric: %s = %.4f (threshold: %s)\n", request.MetricName, request.MetricValue, request.Threshold))

	if len(request.History) > 0 {
		builder.WriteString("\nRecent events:\n")
		for _, event := range request.History {
			builder.WriteString("- ")
			builder.WriteString(event)
			builder.WriteByte('\n')
		}
	}

	return builder.String()
}

func parseRecommendationContent(content string) (*ScalingRecommendation, error) {
	content = strings.TrimSpace(content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)

	var payload recommendationPayload
	if err := json.Unmarshal([]byte(content), &payload); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidPayload, err)
	}

	action := strings.TrimSpace(payload.Action)
	if action == "" {
		action = "no_action"
	}

	recommendation := &ScalingRecommendation{
		Action:     action,
		Reason:     strings.TrimSpace(payload.Reason),
		Confidence: payload.Confidence,
		Details:    payload.Details,
	}
	if recommendation.Details == nil {
		recommendation.Details = map[string]string{}
	}
	if recommendation.Reason == "" {
		recommendation.Reason = "AI recommendation"
	}
	if recommendation.Confidence < 0 || recommendation.Confidence > 1 {
		recommendation.Confidence = 0.5
	}

	return recommendation, nil
}
