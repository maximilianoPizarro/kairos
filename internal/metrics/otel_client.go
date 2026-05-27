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

package metrics

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"
)

const (
	defaultOTelPort        = "4317"
	defaultPrometheusPort  = "9090"
	defaultOTelEndpoint    = "otel-collector.observability:4317"
	defaultPrometheusBase  = "http://prometheus:9090"
	defaultRequestTimeout  = 10 * time.Second
	defaultDialTimeout     = 5 * time.Second
)

// MetricValue represents a single metric sample.
type MetricValue struct {
	Value     float64
	Timestamp time.Time
	Labels    map[string]string
}

// MetricsClient queries metrics from an observability backend.
type MetricsClient interface {
	// QueryMetric queries a specific metric for a resource.
	QueryMetric(ctx context.Context, metric string, namespace string, podSelector map[string]string) (*MetricValue, error)
	// IsHealthy checks if the metrics backend is reachable.
	IsHealthy(ctx context.Context) bool
	// Close closes the connection.
	Close() error
}

// NewOTelClient connects to an OpenTelemetry Collector via gRPC. When the
// collector is unavailable or metric queries are not supported, it falls back
// to a Prometheus HTTP client derived from the same host.
func NewOTelClient(endpoint string) (MetricsClient, error) {
	if endpoint == "" {
		endpoint = defaultOTelEndpoint
	}

	otel, otelErr := newOTelClient(endpoint)
	prom, promErr := NewPrometheusClient(derivePrometheusEndpoint(endpoint))

	if otelErr != nil {
		if promErr != nil {
			return nil, fmt.Errorf("otel connection failed: %w; prometheus fallback failed: %v", otelErr, promErr)
		}
		return prom, nil
	}
	if promErr != nil {
		return otel, nil
	}
	return &fallbackClient{primary: otel, fallback: prom}, nil
}

// NewPrometheusClient creates a client that queries metrics via the Prometheus HTTP API v1.
func NewPrometheusClient(endpoint string) (MetricsClient, error) {
	if endpoint == "" {
		endpoint = defaultPrometheusBase
	}

	baseURL, err := normalizePrometheusEndpoint(endpoint)
	if err != nil {
		return nil, err
	}

	return &prometheusClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: defaultRequestTimeout,
		},
	}, nil
}

// NewStubClient returns a MetricsClient with fixed values for testing.
func NewStubClient() MetricsClient {
	return &stubClient{
		healthy: true,
		values: map[string]float64{
			"cpu_usage":    0.75,
			"memory_usage": 0.60,
			"gpu_usage":    0.40,
		},
	}
}

type otelClient struct {
	endpoint string
	conn     *grpc.ClientConn
}

func newOTelClient(endpoint string) (*otelClient, error) {
	normalized := normalizeOTelEndpoint(endpoint)

	ctx, cancel := context.WithTimeout(context.Background(), defaultDialTimeout)
	defer cancel()

	conn, err := grpc.DialContext(
		ctx,
		normalized,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, fmt.Errorf("dial otel collector at %s: %w", normalized, err)
	}

	return &otelClient{
		endpoint: normalized,
		conn:     conn,
	}, nil
}

func (c *otelClient) QueryMetric(_ context.Context, _ string, _ string, _ map[string]string) (*MetricValue, error) {
	if c.conn == nil {
		return nil, fmt.Errorf("otel client is closed")
	}
	// Placeholder: OTLP metrics query over gRPC is not implemented yet.
	return nil, fmt.Errorf("otel metrics query not implemented for %s", c.endpoint)
}

func (c *otelClient) IsHealthy(ctx context.Context) bool {
	if c.conn == nil {
		return false
	}

	state := c.conn.GetState()
	if state == connectivity.Ready || state == connectivity.Idle {
		return true
	}

	waitCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	if !c.conn.WaitForStateChange(waitCtx, state) {
		return false
	}

	finalState := c.conn.GetState()
	return finalState == connectivity.Ready || finalState == connectivity.Idle
}

func (c *otelClient) Close() error {
	if c.conn == nil {
		return nil
	}
	err := c.conn.Close()
	c.conn = nil
	return err
}

type prometheusClient struct {
	baseURL    string
	httpClient *http.Client
}

func (c *prometheusClient) QueryMetric(ctx context.Context, metric string, namespace string, podSelector map[string]string) (*MetricValue, error) {
	if metric == "" {
		return nil, fmt.Errorf("metric name is required")
	}

	query := buildPromQL(metric, namespace, podSelector)
	endpoint := c.baseURL + "/api/v1/query?" + url.Values{"query": {query}}.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("create prometheus query request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("query prometheus at %s: %w", c.baseURL, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read prometheus response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("prometheus query failed with status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	return parsePrometheusQueryResult(body)
}

func (c *prometheusClient) IsHealthy(ctx context.Context) bool {
	if c.isHealthyEndpoint(ctx, c.baseURL+"/-/healthy") {
		return true
	}
	return c.isHealthyEndpoint(ctx, c.baseURL+"/api/v1/query?query=up")
}

func (c *prometheusClient) isHealthyEndpoint(ctx context.Context, endpoint string) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return false
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)

	return resp.StatusCode == http.StatusOK
}

func (c *prometheusClient) Close() error {
	return nil
}

type fallbackClient struct {
	primary  MetricsClient
	fallback MetricsClient
}

func (c *fallbackClient) QueryMetric(ctx context.Context, metric string, namespace string, podSelector map[string]string) (*MetricValue, error) {
	if c.primary != nil && c.primary.IsHealthy(ctx) {
		value, err := c.primary.QueryMetric(ctx, metric, namespace, podSelector)
		if err == nil {
			return value, nil
		}
	}
	if c.fallback == nil {
		return nil, fmt.Errorf("metrics backends unavailable")
	}
	return c.fallback.QueryMetric(ctx, metric, namespace, podSelector)
}

func (c *fallbackClient) IsHealthy(ctx context.Context) bool {
	if c.primary != nil && c.primary.IsHealthy(ctx) {
		return true
	}
	return c.fallback != nil && c.fallback.IsHealthy(ctx)
}

func (c *fallbackClient) Close() error {
	var errs []error
	if c.primary != nil {
		if err := c.primary.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if c.fallback != nil {
		if err := c.fallback.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("close fallback client: %v", errs)
	}
	return nil
}

type stubClient struct {
	mu      sync.RWMutex
	healthy bool
	values  map[string]float64
}

func (c *stubClient) QueryMetric(_ context.Context, metric string, namespace string, podSelector map[string]string) (*MetricValue, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	value, ok := c.values[metric]
	if !ok {
		value = 0.5
	}

	labels := map[string]string{
		"__name__":  metric,
		"namespace": namespace,
	}
	for key, val := range podSelector {
		labels[key] = val
	}

	return &MetricValue{
		Value:     value,
		Timestamp: time.Now().UTC(),
		Labels:    labels,
	}, nil
}

func (c *stubClient) IsHealthy(_ context.Context) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.healthy
}

func (c *stubClient) Close() error {
	return nil
}

type promQueryResponse struct {
	Status    string `json:"status"`
	ErrorType string `json:"errorType"`
	Error     string `json:"error"`
	Data      struct {
		ResultType string `json:"resultType"`
		Result     []struct {
			Metric map[string]string `json:"metric"`
			Value  []json.RawMessage   `json:"value"`
		} `json:"result"`
	} `json:"data"`
}

func parsePrometheusQueryResult(body []byte) (*MetricValue, error) {
	var response promQueryResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("decode prometheus response: %w", err)
	}
	if response.Status != "success" {
		if response.Error != "" {
			return nil, fmt.Errorf("prometheus query error: %s", response.Error)
		}
		return nil, fmt.Errorf("prometheus query failed with status %q", response.Status)
	}
	if len(response.Data.Result) == 0 {
		return nil, fmt.Errorf("prometheus query returned no data")
	}

	sample := response.Data.Result[0]
	if len(sample.Value) != 2 {
		return nil, fmt.Errorf("unexpected prometheus sample format")
	}

	var timestampRaw float64
	if err := json.Unmarshal(sample.Value[0], &timestampRaw); err != nil {
		return nil, fmt.Errorf("parse prometheus timestamp: %w", err)
	}

	var valueString string
	if err := json.Unmarshal(sample.Value[1], &valueString); err != nil {
		return nil, fmt.Errorf("parse prometheus value: %w", err)
	}

	value, err := strconv.ParseFloat(valueString, 64)
	if err != nil {
		return nil, fmt.Errorf("convert prometheus value %q: %w", valueString, err)
	}

	labels := make(map[string]string, len(sample.Metric))
	for key, val := range sample.Metric {
		labels[key] = val
	}

	return &MetricValue{
		Value:     value,
		Timestamp: time.Unix(int64(timestampRaw), 0).UTC(),
		Labels:    labels,
	}, nil
}

func buildPromQL(metric, namespace string, podSelector map[string]string) string {
	labels := make([]string, 0, len(podSelector)+1)
	if namespace != "" {
		labels = append(labels, fmt.Sprintf(`namespace="%s"`, escapePrometheusLabelValue(namespace)))
	}

	keys := make([]string, 0, len(podSelector))
	for key := range podSelector {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		labels = append(labels, fmt.Sprintf(`%s="%s"`, key, escapePrometheusLabelValue(podSelector[key])))
	}

	if len(labels) == 0 {
		return metric
	}
	return fmt.Sprintf("%s{%s}", metric, strings.Join(labels, ","))
}

func escapePrometheusLabelValue(value string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `"`, `\"`)
	return replacer.Replace(value)
}

func normalizeOTelEndpoint(endpoint string) string {
	endpoint = strings.TrimSpace(endpoint)
	endpoint = strings.TrimPrefix(endpoint, "http://")
	endpoint = strings.TrimPrefix(endpoint, "https://")
	endpoint = strings.TrimPrefix(endpoint, "grpc://")

	if !strings.Contains(endpoint, ":") {
		endpoint = endpoint + ":" + defaultOTelPort
	}
	return endpoint
}

func normalizePrometheusEndpoint(endpoint string) (string, error) {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return "", fmt.Errorf("prometheus endpoint is required")
	}

	if !strings.HasPrefix(endpoint, "http://") && !strings.HasPrefix(endpoint, "https://") {
		endpoint = "http://" + endpoint
	}

	parsed, err := url.Parse(endpoint)
	if err != nil {
		return "", fmt.Errorf("parse prometheus endpoint %q: %w", endpoint, err)
	}
	if parsed.Host == "" {
		return "", fmt.Errorf("invalid prometheus endpoint %q", endpoint)
	}

	parsed.Path = strings.TrimSuffix(parsed.Path, "/")
	return strings.TrimSuffix(parsed.String(), "/"), nil
}

func derivePrometheusEndpoint(otelEndpoint string) string {
	host := normalizeOTelEndpoint(otelEndpoint)
	if idx := strings.LastIndex(host, ":"); idx != -1 {
		host = host[:idx]
	}
	return fmt.Sprintf("http://%s:%s", host, defaultPrometheusPort)
}
