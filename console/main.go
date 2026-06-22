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

package main

import (
	"crypto/tls"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

//go:embed static/*
var staticFiles embed.FS

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

type Event struct {
	Type      string      `json:"type"`
	Timestamp time.Time   `json:"timestamp"`
	Data      interface{} `json:"data"`
}

// AgentReport is what spoke agents push to the hub console
type AgentReport struct {
	Name             string                   `json:"name"`
	Namespace        string                   `json:"namespace"`
	Cluster          string                   `json:"cluster"`
	Mode             string                   `json:"mode"`
	Phase            string                   `json:"phase"`
	WatchedResources int                      `json:"watchedResources"`
	TotalCorrections int                      `json:"totalCorrections"`
	LastCheck        time.Time                `json:"lastCheck"`
	AIModel          string                   `json:"aiModel"`
	Events           []map[string]interface{} `json:"events,omitempty"`
	ManagedResources []ManagedResource        `json:"managedResources,omitempty"`
}

// ManagedResource represents a workload managed by Kairos
type ManagedResource struct {
	Name          string `json:"name"`
	Namespace     string `json:"namespace"`
	Kind          string `json:"kind"`
	Cluster       string `json:"cluster"`
	Policy        string `json:"policy"`
	Agent         string `json:"agent"`
	CurrentCPU    string `json:"currentCPU"`
	CurrentMemory string `json:"currentMemory"`
	Status        string `json:"status"`
}

// agentStore holds agent reports from spoke clusters (thread-safe)
var agentStore = struct {
	sync.RWMutex
	agents    map[string]*AgentReport
	events    []map[string]interface{}
	resources []ManagedResource
}{
	agents:    make(map[string]*AgentReport),
	events:    make([]map[string]interface{}, 0),
	resources: make([]ManagedResource, 0),
}

// approvalStore holds pending approval requests (thread-safe, used only in demo mode)
var approvalStore = struct {
	sync.RWMutex
	items []map[string]interface{}
}{
	items: []map[string]interface{}{},
}

func initDemoApprovals() {
	approvalStore.Lock()
	defer approvalStore.Unlock()
	approvalStore.items = []map[string]interface{}{
		{
			"id": "appr-001", "resource": "ie-anomaly-alerter",
			"namespace": "industrial-edge-tst-all", "cluster": "east", "agent": "east-agent",
			"proposedCPU": "250m", "proposedMemory": "512Mi",
			"currentCPU": "100m", "currentMemory": "256Mi",
			"reason":  "CPU utilization averaging 92% over last 30 minutes; AI recommends scaling up to avoid throttling",
			"status":  "pending",
			"before":  map[string]string{"cpu": "100m", "memory": "256Mi"},
			"after":   map[string]string{"cpu": "250m", "memory": "512Mi"},
			"dryRun":  false,
			"action":  "ScaleUp",
			"policyName": "auto-scale",
		},
		{
			"id": "appr-002", "resource": "line-dashboard",
			"namespace": "industrial-edge-tst-all", "cluster": "west", "agent": "west-agent",
			"proposedCPU": "150m", "proposedMemory": "384Mi",
			"currentCPU": "200m", "currentMemory": "512Mi",
			"reason":  "Resource over-provisioned: memory usage below 40% for 2 hours; AI recommends downsizing",
			"status":  "pending",
			"before":  map[string]string{"cpu": "200m", "memory": "512Mi"},
			"after":   map[string]string{"cpu": "150m", "memory": "384Mi"},
			"dryRun":  false,
			"action":  "ScaleDown",
			"policyName": "auto-scale",
		},
		{
			"id": "appr-003", "resource": "minio",
			"namespace": "industrial-edge-ml-workspace", "cluster": "east", "agent": "east-agent",
			"proposedCPU": "750m", "proposedMemory": "2Gi",
			"currentCPU": "500m", "currentMemory": "1Gi",
			"reason":  "Storage I/O contention detected; scaling CPU and memory to match workload pattern",
			"status":  "pending",
			"before":  map[string]string{"cpu": "500m", "memory": "1Gi"},
			"after":   map[string]string{"cpu": "750m", "memory": "2Gi"},
			"dryRun":  false,
			"action":  "ScaleUp",
			"policyName": "auto-scale",
		},
		{
			"id": "appr-004", "resource": "machine-sensor-1",
			"namespace": "industrial-edge-tst-all", "cluster": "west", "agent": "west-agent",
			"proposedCPU": "100m", "proposedMemory": "256Mi",
			"currentCPU": "50m", "currentMemory": "128Mi",
			"reason":  "Pod restarts detected (OOMKilled x2 in last hour); AI recommends doubling memory allocation",
			"status":  "pending",
			"before":  map[string]string{"cpu": "50m", "memory": "128Mi"},
			"after":   map[string]string{"cpu": "100m", "memory": "256Mi"},
			"dryRun":  false,
			"action":  "ScaleUp",
			"policyName": "auto-scale",
		},
	}
}

type Hub struct {
	clients    map[*websocket.Conn]bool
	broadcast  chan Event
	register   chan *websocket.Conn
	unregister chan *websocket.Conn
	mu         sync.RWMutex
}

func newHub() *Hub {
	return &Hub{
		clients:    make(map[*websocket.Conn]bool),
		broadcast:  make(chan Event, 256),
		register:   make(chan *websocket.Conn),
		unregister: make(chan *websocket.Conn),
	}
}

func (h *Hub) run() {
	for {
		select {
		case conn := <-h.register:
			h.mu.Lock()
			h.clients[conn] = true
			h.mu.Unlock()
		case conn := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[conn]; ok {
				delete(h.clients, conn)
				conn.Close()
			}
			h.mu.Unlock()
		case event := <-h.broadcast:
			h.mu.RLock()
			for conn := range h.clients {
				if err := conn.WriteJSON(event); err != nil {
					conn.Close()
					delete(h.clients, conn)
				}
			}
			h.mu.RUnlock()
		}
	}
}

func isDemoMode() bool {
	return os.Getenv("KAIROS_CONSOLE_DEMO_DATA") == "true"
}

// --- Kubernetes API helpers ---

const crdGroup = "kairos.maximilianopizarro.github.io"
const crdVersion = "v1alpha1"

func k8sClient() *http.Client {
	return &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}
}

func k8sAPIBase() string {
	host := os.Getenv("KUBERNETES_SERVICE_HOST")
	port := os.Getenv("KUBERNETES_SERVICE_PORT")
	if host == "" || port == "" {
		return ""
	}
	return fmt.Sprintf("https://%s:%s", host, port)
}

func k8sGet(path string) ([]byte, error) {
	base := k8sAPIBase()
	if base == "" {
		return nil, fmt.Errorf("not running in-cluster")
	}
	token := getServiceAccountToken()
	if token == "" {
		return nil, fmt.Errorf("no service account token")
	}

	req, err := http.NewRequest("GET", base+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := k8sClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	return io.ReadAll(resp.Body)
}

func k8sPut(path string, body []byte) ([]byte, error) {
	base := k8sAPIBase()
	if base == "" {
		return nil, fmt.Errorf("not running in-cluster")
	}
	token := getServiceAccountToken()
	if token == "" {
		return nil, fmt.Errorf("no service account token")
	}

	req, err := http.NewRequest("PUT", base+path, strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := k8sClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	return io.ReadAll(resp.Body)
}

// KairosEvent JSON structures for K8s API responses
type k8sKairosEventList struct {
	Items []k8sKairosEvent `json:"items"`
}

type k8sKairosEvent struct {
	Metadata struct {
		Name              string `json:"name"`
		Namespace         string `json:"namespace"`
		UID               string `json:"uid"`
		CreationTimestamp string `json:"creationTimestamp"`
		ResourceVersion   string `json:"resourceVersion"`
	} `json:"metadata"`
	Spec struct {
		AgentName  string            `json:"agentName"`
		Cluster    string            `json:"cluster"`
		Action     string            `json:"action"`
		Resource   string            `json:"resource"`
		Namespace  string            `json:"namespace"`
		Before     map[string]string `json:"before,omitempty"`
		After      map[string]string `json:"after,omitempty"`
		Reason     string            `json:"reason,omitempty"`
		AIResponse string            `json:"aiResponse,omitempty"`
		DryRun     bool              `json:"dryRun,omitempty"`
		Status     string            `json:"status,omitempty"`
		PolicyName string            `json:"policyName,omitempty"`
	} `json:"spec"`
}

type k8sSmartScalingPolicyList struct {
	Items []k8sSmartScalingPolicy `json:"items"`
}

type k8sSmartScalingPolicy struct {
	Metadata struct {
		Name              string `json:"name"`
		Namespace         string `json:"namespace"`
		CreationTimestamp string `json:"creationTimestamp"`
	} `json:"metadata"`
	Spec struct {
		Target struct {
			Name string `json:"name"`
			Kind string `json:"kind"`
		} `json:"target"`
		Rules              []interface{} `json:"rules,omitempty"`
		Paused             bool          `json:"paused,omitempty"`
		PrometheusEndpoint string        `json:"prometheusEndpoint,omitempty"`
		OtelEndpoint       string        `json:"otelEndpoint,omitempty"`
	} `json:"spec"`
}

func listKairosEvents() ([]k8sKairosEvent, error) {
	path := fmt.Sprintf("/apis/%s/%s/kairosevents", crdGroup, crdVersion)
	data, err := k8sGet(path)
	if err != nil {
		return nil, err
	}
	var list k8sKairosEventList
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, err
	}
	return list.Items, nil
}

func listSmartScalingPolicies() ([]k8sSmartScalingPolicy, error) {
	path := fmt.Sprintf("/apis/%s/%s/smartscalingpolicies", crdGroup, crdVersion)
	data, err := k8sGet(path)
	if err != nil {
		return nil, err
	}
	var list k8sSmartScalingPolicyList
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, err
	}
	return list.Items, nil
}

func main() {
	if isDemoMode() {
		initDemoApprovals()
		log.Println("Demo mode enabled (KAIROS_CONSOLE_DEMO_DATA=true)")
	}

	hub := newHub()
	go hub.run()

	mux := http.NewServeMux()

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "ok")
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "ok")
	})

	mux.HandleFunc("/api/v1/agents", handleAgents)
	mux.HandleFunc("/api/v1/policies", handlePolicies)
	mux.HandleFunc("/api/v1/events", handleEvents)
	mux.HandleFunc("/api/v1/diffs", handleDiffs)
	mux.HandleFunc("/api/v1/clusters", handleClusters)
	mux.HandleFunc("/api/v1/status", handleStatus)
	mux.HandleFunc("/api/v1/observability", handleObservability)
	mux.HandleFunc("/api/v1/metrics/query", handleMetricsQuery)
	mux.HandleFunc("/api/v1/agent-report", handleAgentReport)
	mux.HandleFunc("/api/v1/managed-resources", handleManagedResources)
	mux.HandleFunc("/api/v1/user", handleUser)
	mux.HandleFunc("/api/v1/approvals", handleApprovals)
	mux.HandleFunc("/api/v1/approvals/", handleApprovalAction)
	mux.HandleFunc("/api/v1/history", handleHistory)
	mux.HandleFunc("/api/v1/notifications", handleNotifications)

	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Printf("WebSocket upgrade error: %v", err)
			return
		}
		hub.register <- conn

		go func() {
			defer func() { hub.unregister <- conn }()
			for {
				_, _, err := conn.ReadMessage()
				if err != nil {
					break
				}
			}
		}()
	})

	var fileHandler http.Handler
	if _, statErr := os.Stat("/static/index.html"); statErr == nil {
		fileHandler = http.FileServer(http.Dir("/static"))
	} else {
		staticFS, fsErr := fs.Sub(staticFiles, "static")
		if fsErr != nil {
			log.Fatal("Failed to create static filesystem:", fsErr)
		}
		fileHandler = http.FileServer(http.FS(staticFS))
	}
	mux.Handle("/", fileHandler)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Kairos Console starting on :%s", port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatal("Server failed:", err)
	}
}

func getServiceAccountToken() string {
	data, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/token")
	if err != nil {
		return ""
	}
	return string(data)
}

func getThanosEndpoint() string {
	ep := os.Getenv("THANOS_ENDPOINT")
	if ep != "" {
		return ep
	}
	return "https://thanos-querier.openshift-monitoring.svc:9091"
}

func getOTelEndpoint() string {
	ep := os.Getenv("OTEL_ENDPOINT")
	if ep != "" {
		return ep
	}
	return "kairos-otel-collector.kairos-system.svc:4317"
}

func queryThanos(query string) ([]byte, error) {
	token := getServiceAccountToken()
	endpoint := getThanosEndpoint()

	skipVerify := os.Getenv("THANOS_INSECURE_SKIP_VERIFY") != "false"

	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: skipVerify},
		},
	}

	url := fmt.Sprintf("%s/api/v1/query?query=%s", endpoint, query)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	return io.ReadAll(resp.Body)
}

func handleObservability(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	thanosStatus := "disconnected"
	thanosMetrics := 0
	otelStatus := "not configured"

	data, err := queryThanos("up")
	if err == nil {
		var result map[string]interface{}
		if json.Unmarshal(data, &result) == nil {
			if status, ok := result["status"].(string); ok && status == "success" {
				thanosStatus = "connected"
				if d, ok := result["data"].(map[string]interface{}); ok {
					if results, ok := d["result"].([]interface{}); ok {
						thanosMetrics = len(results)
					}
				}
			}
		}
	}

	otelEp := getOTelEndpoint()
	if otelEp != "" {
		otelStatus = "configured"
	}

	observability := map[string]interface{}{
		"thanos": map[string]interface{}{
			"status":        thanosStatus,
			"endpoint":      getThanosEndpoint(),
			"activeTargets": thanosMetrics,
		},
		"opentelemetry": map[string]interface{}{
			"status":   otelStatus,
			"endpoint": otelEp,
			"protocol": "gRPC/OTLP",
			"port":     4317,
		},
		"metricsSource": func() string {
			if otelStatus == "connected" {
				return "OpenTelemetry"
			}
			if thanosStatus == "connected" {
				return "Thanos"
			}
			return "none"
		}(),
		"pipelines": []map[string]interface{}{
			{
				"name":      "metrics",
				"receivers": []string{"otlp", "prometheus"},
				"exporters": []string{"debug"},
				"status":    "active",
			},
		},
	}

	json.NewEncoder(w).Encode(observability)
}

func handleMetricsQuery(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	query := r.URL.Query().Get("query")
	if query == "" {
		query = "up{job=\"kubelet\"}"
	}

	data, err := queryThanos(query)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status": "error",
			"error":  err.Error(),
		})
		return
	}

	w.Write(data)
}

func handleAgents(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	agents := []map[string]interface{}{
		{
			"name":             "hub-agent",
			"namespace":        "kairos-system",
			"cluster":          "hub",
			"mode":             "supervised",
			"phase":            "Active",
			"watchedResources": 8,
			"totalCorrections": 0,
			"lastCheck":        time.Now().Add(-30 * time.Second),
			"aiModel":          "deepseek-r1-distill-qwen-14b",
		},
	}

	agentStore.RLock()
	for _, report := range agentStore.agents {
		agents = append(agents, map[string]interface{}{
			"name":             report.Name,
			"namespace":        report.Namespace,
			"cluster":          report.Cluster,
			"mode":             report.Mode,
			"phase":            report.Phase,
			"watchedResources": report.WatchedResources,
			"totalCorrections": report.TotalCorrections,
			"lastCheck":        report.LastCheck,
			"aiModel":          report.AIModel,
		})
	}
	agentStore.RUnlock()

	if len(agents) == 1 && isDemoMode() {
		agents = append(agents,
			map[string]interface{}{
				"name": "east-agent", "namespace": "kairos-system", "cluster": "east",
				"mode": "autopilot", "phase": "Active", "watchedResources": 12,
				"totalCorrections": 3, "lastCheck": time.Now().Add(-45 * time.Second),
				"aiModel": "deepseek-r1-distill-qwen-14b",
			},
			map[string]interface{}{
				"name": "west-agent", "namespace": "kairos-system", "cluster": "west",
				"mode": "autopilot", "phase": "Active", "watchedResources": 10,
				"totalCorrections": 1, "lastCheck": time.Now().Add(-20 * time.Second),
				"aiModel": "deepseek-r1-distill-qwen-14b",
			},
		)
	}

	json.NewEncoder(w).Encode(agents)
}

func handleAgentReport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}

	var report AgentReport
	if err := json.NewDecoder(r.Body).Decode(&report); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	if report.Name == "" || report.Cluster == "" {
		http.Error(w, "name and cluster are required", http.StatusBadRequest)
		return
	}

	report.LastCheck = time.Now()
	key := report.Cluster + "/" + report.Name

	agentStore.Lock()
	agentStore.agents[key] = &report
	if len(report.Events) > 0 {
		agentStore.events = append(agentStore.events, report.Events...)
		if len(agentStore.events) > 100 {
			agentStore.events = agentStore.events[len(agentStore.events)-100:]
		}
	}
	if len(report.ManagedResources) > 0 {
		filtered := make([]ManagedResource, 0)
		for _, r := range agentStore.resources {
			if r.Cluster != report.Cluster {
				filtered = append(filtered, r)
			}
		}
		agentStore.resources = append(filtered, report.ManagedResources...)
	}
	agentStore.Unlock()

	log.Printf("Agent report received: %s/%s (cluster=%s, mode=%s, watched=%d)",
		report.Namespace, report.Name, report.Cluster, report.Mode, report.WatchedResources)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "accepted"})
}

func handleDiffs(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	events, err := listKairosEvents()
	if err != nil {
		log.Printf("Failed to list KairosEvents for diffs: %v", err)
		if isDemoMode() {
			json.NewEncoder(w).Encode(demoModeHistory())
			return
		}
		json.NewEncoder(w).Encode([]interface{}{})
		return
	}

	clusterFilter := r.URL.Query().Get("cluster")
	nsFilter := r.URL.Query().Get("namespace")
	agentFilter := r.URL.Query().Get("agent")

	var result []map[string]interface{}
	for _, ev := range events {
		if clusterFilter != "" && ev.Spec.Cluster != clusterFilter {
			continue
		}
		if nsFilter != "" && ev.Spec.Namespace != nsFilter {
			continue
		}
		if agentFilter != "" && ev.Spec.AgentName != agentFilter {
			continue
		}

		result = append(result, map[string]interface{}{
			"id":         ev.Metadata.UID,
			"timestamp":  ev.Metadata.CreationTimestamp,
			"resource":   ev.Spec.Resource,
			"namespace":  ev.Spec.Namespace,
			"cluster":    ev.Spec.Cluster,
			"action":     ev.Spec.Action,
			"agentName":  ev.Spec.AgentName,
			"policyName": ev.Spec.PolicyName,
			"before":     ev.Spec.Before,
			"after":      ev.Spec.After,
			"status":     ev.Spec.Status,
			"dryRun":     ev.Spec.DryRun,
			"reason":     ev.Spec.Reason,
			"aiResponse": ev.Spec.AIResponse,
		})
	}

	if result == nil {
		result = []map[string]interface{}{}
	}

	json.NewEncoder(w).Encode(result)
}

// --- 2.2: /api/v1/approvals - KairosEvent CRs with status pending-approval ---

func handleApprovals(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	events, err := listKairosEvents()
	if err != nil {
		log.Printf("Failed to list KairosEvents for approvals: %v", err)
		if isDemoMode() {
			approvalStore.RLock()
			pending := make([]map[string]interface{}, 0)
			for _, a := range approvalStore.items {
				if a["status"] == "pending" {
					pending = append(pending, a)
				}
			}
			approvalStore.RUnlock()
			json.NewEncoder(w).Encode(pending)
			return
		}
		json.NewEncoder(w).Encode([]interface{}{})
		return
	}

	var pending []map[string]interface{}
	for _, ev := range events {
		if ev.Spec.Status != "pending-approval" {
			continue
		}
		pending = append(pending, map[string]interface{}{
			"id":         ev.Metadata.UID,
			"resource":   ev.Spec.Resource,
			"namespace":  ev.Spec.Namespace,
			"cluster":    ev.Spec.Cluster,
			"agent":      ev.Spec.AgentName,
			"reason":     ev.Spec.Reason,
			"status":     "pending",
			"action":     ev.Spec.Action,
			"policyName": ev.Spec.PolicyName,
			"before":     ev.Spec.Before,
			"after":      ev.Spec.After,
			"dryRun":     ev.Spec.DryRun,
			"timestamp":  ev.Metadata.CreationTimestamp,
		})
	}

	if pending == nil {
		pending = []map[string]interface{}{}
	}

	json.NewEncoder(w).Encode(pending)
}

func handleApprovalAction(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	path := r.URL.Path
	parts := splitPath(path)
	if len(parts) < 5 {
		http.Error(w, `{"error":"invalid path"}`, http.StatusBadRequest)
		return
	}

	id := parts[3]
	action := parts[4]

	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"POST required"}`, http.StatusMethodNotAllowed)
		return
	}

	if action != "approve" && action != "reject" {
		http.Error(w, `{"error":"action must be approve or reject"}`, http.StatusBadRequest)
		return
	}

	newStatus := "applied"
	if action == "reject" {
		newStatus = "rejected"
	}

	// Try K8s API first: find the KairosEvent by UID and update its status
	events, err := listKairosEvents()
	if err == nil {
		for _, ev := range events {
			if ev.Metadata.UID == id {
				ev.Spec.Status = newStatus
				evJSON, marshalErr := json.Marshal(map[string]interface{}{
					"apiVersion": crdGroup + "/" + crdVersion,
					"kind":       "KairosEvent",
					"metadata": map[string]interface{}{
						"name":            ev.Metadata.Name,
						"namespace":       ev.Metadata.Namespace,
						"resourceVersion": ev.Metadata.ResourceVersion,
					},
					"spec": ev.Spec,
				})
				if marshalErr == nil {
					putPath := fmt.Sprintf("/apis/%s/%s/namespaces/%s/kairosevents/%s",
						crdGroup, crdVersion, ev.Metadata.Namespace, ev.Metadata.Name)
					_, putErr := k8sPut(putPath, evJSON)
					if putErr != nil {
						log.Printf("Failed to update KairosEvent %s: %v", ev.Metadata.Name, putErr)
					}
				}
				json.NewEncoder(w).Encode(map[string]string{"status": action + "d", "id": id})
				return
			}
		}
	}

	// Fallback: in-memory store (demo mode)
	approvalStore.Lock()
	found := false
	for i, a := range approvalStore.items {
		if a["id"] == id {
			found = true
			approvalStore.items[i]["status"] = newStatus
			break
		}
	}
	approvalStore.Unlock()

	if !found {
		http.Error(w, `{"error":"approval not found"}`, http.StatusNotFound)
		return
	}

	json.NewEncoder(w).Encode(map[string]string{"status": action + "d", "id": id})
}

// --- 2.3: /api/v1/history - KairosEvent CRs with pagination ---

func handleHistory(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	events, err := listKairosEvents()
	if err != nil {
		log.Printf("Failed to list KairosEvents for history: %v", err)
		if isDemoMode() {
			history := demoModeHistory()
			applyHistoryPagination(w, r, history)
			return
		}
		json.NewEncoder(w).Encode([]interface{}{})
		return
	}

	var history []map[string]interface{}
	for _, ev := range events {
		beforeCPU := ev.Spec.Before["cpu"]
		beforeMem := ev.Spec.Before["memory"]
		afterCPU := ev.Spec.After["cpu"]
		afterMem := ev.Spec.After["memory"]

		history = append(history, map[string]interface{}{
			"timestamp":    ev.Metadata.CreationTimestamp,
			"agent":        ev.Spec.AgentName,
			"resource":     ev.Spec.Resource,
			"namespace":    ev.Spec.Namespace,
			"cluster":      ev.Spec.Cluster,
			"action":       ev.Spec.Action,
			"beforeCPU":    beforeCPU,
			"beforeMemory": beforeMem,
			"afterCPU":     afterCPU,
			"afterMemory":  afterMem,
			"status":       ev.Spec.Status,
			"aiResponse":   ev.Spec.AIResponse,
		})
	}

	if history == nil {
		history = []map[string]interface{}{}
	}

	applyHistoryPagination(w, r, history)
}

func applyHistoryPagination(w http.ResponseWriter, r *http.Request, history []map[string]interface{}) {
	limitStr := r.URL.Query().Get("limit")
	offsetStr := r.URL.Query().Get("offset")

	limit := len(history)
	offset := 0

	if limitStr != "" {
		if v, err := strconv.Atoi(limitStr); err == nil && v > 0 {
			limit = v
		}
	}
	if offsetStr != "" {
		if v, err := strconv.Atoi(offsetStr); err == nil && v >= 0 {
			offset = v
		}
	}

	if offset >= len(history) {
		json.NewEncoder(w).Encode([]map[string]interface{}{})
		return
	}

	end := offset + limit
	if end > len(history) {
		end = len(history)
	}

	json.NewEncoder(w).Encode(history[offset:end])
}

func demoModeHistory() []map[string]interface{} {
	return []map[string]interface{}{
		{
			"timestamp": time.Now().Add(-1 * time.Hour), "agent": "east-agent",
			"resource": "ie-anomaly-alerter", "namespace": "industrial-edge-tst-all", "cluster": "east",
			"action": "ScaleUp", "beforeCPU": "50m", "beforeMemory": "128Mi",
			"afterCPU": "100m", "afterMemory": "256Mi", "status": "applied",
			"aiResponse": "CPU throttling observed at 95th percentile. Doubling requests to stabilize latency.",
			"before": map[string]string{"cpu": "50m", "memory": "128Mi"},
			"after":  map[string]string{"cpu": "100m", "memory": "256Mi"},
		},
		{
			"timestamp": time.Now().Add(-2 * time.Hour), "agent": "west-agent",
			"resource": "line-dashboard", "namespace": "industrial-edge-tst-all", "cluster": "west",
			"action": "ScaleDown", "beforeCPU": "400m", "beforeMemory": "1Gi",
			"afterCPU": "200m", "afterMemory": "512Mi", "status": "applied",
			"aiResponse": "Resource utilization consistently below 30% for 4 hours. Safe to reduce allocation.",
			"before": map[string]string{"cpu": "400m", "memory": "1Gi"},
			"after":  map[string]string{"cpu": "200m", "memory": "512Mi"},
		},
		{
			"timestamp": time.Now().Add(-4 * time.Hour), "agent": "east-agent",
			"resource": "minio", "namespace": "industrial-edge-ml-workspace", "cluster": "east",
			"action": "ScaleUp", "beforeCPU": "250m", "beforeMemory": "512Mi",
			"afterCPU": "500m", "afterMemory": "1Gi", "status": "applied",
			"aiResponse": "High I/O wait times correlated with training job schedule. Proactive scaling recommended.",
			"before": map[string]string{"cpu": "250m", "memory": "512Mi"},
			"after":  map[string]string{"cpu": "500m", "memory": "1Gi"},
		},
		{
			"timestamp": time.Now().Add(-6 * time.Hour), "agent": "hub-agent",
			"resource": "kairos-console", "namespace": "kairos-system", "cluster": "hub",
			"action": "ScaleUp", "beforeCPU": "100m", "beforeMemory": "128Mi",
			"afterCPU": "200m", "afterMemory": "256Mi", "status": "dry-run",
			"aiResponse": "Moderate increase suggested due to WebSocket connection growth. Dry-run mode active.",
			"before": map[string]string{"cpu": "100m", "memory": "128Mi"},
			"after":  map[string]string{"cpu": "200m", "memory": "256Mi"},
		},
		{
			"timestamp": time.Now().Add(-8 * time.Hour), "agent": "west-agent",
			"resource": "machine-sensor-1", "namespace": "industrial-edge-tst-all", "cluster": "west",
			"action": "ScaleUp", "beforeCPU": "25m", "beforeMemory": "64Mi",
			"afterCPU": "50m", "afterMemory": "128Mi", "status": "applied",
			"aiResponse": "OOMKilled events detected. Memory allocation insufficient for sensor data buffering.",
			"before": map[string]string{"cpu": "25m", "memory": "64Mi"},
			"after":  map[string]string{"cpu": "50m", "memory": "128Mi"},
		},
		{
			"timestamp": time.Now().Add(-10 * time.Hour), "agent": "east-agent",
			"resource": "machine-sensor-2", "namespace": "industrial-edge-tst-all", "cluster": "east",
			"action": "ScaleDown", "beforeCPU": "100m", "beforeMemory": "256Mi",
			"afterCPU": "50m", "afterMemory": "128Mi", "status": "rejected",
			"aiResponse": "Scaling down recommended but rejected by policy: minimum resource floor violated.",
			"before": map[string]string{"cpu": "100m", "memory": "256Mi"},
			"after":  map[string]string{"cpu": "50m", "memory": "128Mi"},
		},
		{
			"timestamp": time.Now().Add(-14 * time.Hour), "agent": "west-agent",
			"resource": "ie-anomaly-alerter", "namespace": "industrial-edge-tst-all", "cluster": "west",
			"action": "ScaleUp", "beforeCPU": "75m", "beforeMemory": "192Mi",
			"afterCPU": "100m", "afterMemory": "256Mi", "status": "applied",
			"aiResponse": "Anomaly detection pipeline experiencing backpressure. Increasing resources for timely alerting.",
			"before": map[string]string{"cpu": "75m", "memory": "192Mi"},
			"after":  map[string]string{"cpu": "100m", "memory": "256Mi"},
		},
		{
			"timestamp": time.Now().Add(-18 * time.Hour), "agent": "hub-agent",
			"resource": "kairos-console", "namespace": "kairos-system", "cluster": "hub",
			"action": "NoAction", "beforeCPU": "100m", "beforeMemory": "128Mi",
			"afterCPU": "100m", "afterMemory": "128Mi", "status": "dry-run",
			"aiResponse": "Current allocation matches demand. No changes required at this time.",
			"before": map[string]string{"cpu": "100m", "memory": "128Mi"},
			"after":  map[string]string{"cpu": "100m", "memory": "128Mi"},
		},
		{
			"timestamp": time.Now().Add(-22 * time.Hour), "agent": "east-agent",
			"resource": "line-dashboard", "namespace": "industrial-edge-tst-all", "cluster": "east",
			"action": "ScaleUp", "beforeCPU": "100m", "beforeMemory": "256Mi",
			"afterCPU": "200m", "afterMemory": "512Mi", "status": "applied",
			"aiResponse": "Shift change traffic spike anticipated based on historical patterns. Pre-scaling applied.",
			"before": map[string]string{"cpu": "100m", "memory": "256Mi"},
			"after":  map[string]string{"cpu": "200m", "memory": "512Mi"},
		},
	}
}

// --- 2.4: /api/v1/managed-resources ---

func handleManagedResources(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var allResources []ManagedResource

	agentStore.RLock()
	allResources = append(allResources, agentStore.resources...)
	spokeCount := len(agentStore.resources)
	agentStore.RUnlock()

	if spokeCount == 0 && isDemoMode() {
		mockSpokeResources := []ManagedResource{
			{Name: "ie-anomaly-alerter", Namespace: "industrial-edge-tst-all", Kind: "Deployment", Cluster: "east", Policy: "auto-scale", Agent: "east-agent", CurrentCPU: "100m", CurrentMemory: "256Mi", Status: "managed"},
			{Name: "line-dashboard", Namespace: "industrial-edge-tst-all", Kind: "Deployment", Cluster: "east", Policy: "auto-scale", Agent: "east-agent", CurrentCPU: "200m", CurrentMemory: "512Mi", Status: "managed"},
			{Name: "machine-sensor-1", Namespace: "industrial-edge-tst-all", Kind: "Deployment", Cluster: "east", Policy: "auto-scale", Agent: "east-agent", CurrentCPU: "50m", CurrentMemory: "128Mi", Status: "managed"},
			{Name: "machine-sensor-2", Namespace: "industrial-edge-tst-all", Kind: "Deployment", Cluster: "east", Policy: "auto-scale", Agent: "east-agent", CurrentCPU: "50m", CurrentMemory: "128Mi", Status: "managed"},
			{Name: "minio", Namespace: "industrial-edge-ml-workspace", Kind: "Deployment", Cluster: "east", Policy: "auto-scale", Agent: "east-agent", CurrentCPU: "500m", CurrentMemory: "1Gi", Status: "managed"},
			{Name: "ie-anomaly-alerter", Namespace: "industrial-edge-tst-all", Kind: "Deployment", Cluster: "west", Policy: "auto-scale", Agent: "west-agent", CurrentCPU: "100m", CurrentMemory: "256Mi", Status: "managed"},
			{Name: "line-dashboard", Namespace: "industrial-edge-tst-all", Kind: "Deployment", Cluster: "west", Policy: "auto-scale", Agent: "west-agent", CurrentCPU: "200m", CurrentMemory: "512Mi", Status: "managed"},
			{Name: "machine-sensor-1", Namespace: "industrial-edge-tst-all", Kind: "Deployment", Cluster: "west", Policy: "auto-scale", Agent: "west-agent", CurrentCPU: "50m", CurrentMemory: "128Mi", Status: "managed"},
			{Name: "machine-sensor-2", Namespace: "industrial-edge-tst-all", Kind: "Deployment", Cluster: "west", Policy: "auto-scale", Agent: "west-agent", CurrentCPU: "50m", CurrentMemory: "128Mi", Status: "managed"},
			{Name: "minio", Namespace: "industrial-edge-ml-workspace", Kind: "Deployment", Cluster: "west", Policy: "auto-scale", Agent: "west-agent", CurrentCPU: "500m", CurrentMemory: "1Gi", Status: "managed"},
		}
		allResources = append(allResources, mockSpokeResources...)
	}

	// Hub resources from Kubernetes API
	token := getServiceAccountToken()
	kubeHost := os.Getenv("KUBERNETES_SERVICE_HOST")
	kubePort := os.Getenv("KUBERNETES_SERVICE_PORT")

	if kubeHost != "" && token != "" {
		apiBase := fmt.Sprintf("https://%s:%s", kubeHost, kubePort)
		client := &http.Client{
			Timeout: 10 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			},
		}

		for _, kind := range []string{"deployments", "statefulsets"} {
			url := fmt.Sprintf("%s/apis/apps/v1/%s", apiBase, kind)
			req, err := http.NewRequest("GET", url, nil)
			if err != nil {
				continue
			}
			req.Header.Set("Authorization", "Bearer "+token)

			resp, err := client.Do(req)
			if err != nil {
				continue
			}

			body, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()

			var list struct {
				Items []struct {
					Metadata struct {
						Name        string            `json:"name"`
						Namespace   string            `json:"namespace"`
						Annotations map[string]string `json:"annotations"`
					} `json:"metadata"`
					Spec struct {
						Template struct {
							Spec struct {
								Containers []struct {
									Resources struct {
										Requests map[string]string `json:"requests"`
										Limits   map[string]string `json:"limits"`
									} `json:"resources"`
								} `json:"containers"`
							} `json:"spec"`
						} `json:"template"`
					} `json:"spec"`
				} `json:"items"`
			}

			if json.Unmarshal(body, &list) != nil {
				continue
			}

			kindName := "Deployment"
			if kind == "statefulsets" {
				kindName = "StatefulSet"
			}

			for _, item := range list.Items {
				if item.Metadata.Annotations["kairos.io/managed"] == "true" {
					cpu := ""
					mem := ""
					if len(item.Spec.Template.Spec.Containers) > 0 {
						res := item.Spec.Template.Spec.Containers[0].Resources
						cpu = res.Requests["cpu"]
						mem = res.Requests["memory"]
						if cpu == "" {
							cpu = res.Limits["cpu"]
						}
						if mem == "" {
							mem = res.Limits["memory"]
						}
					}
					allResources = append(allResources, ManagedResource{
						Name:          item.Metadata.Name,
						Namespace:     item.Metadata.Namespace,
						Kind:          kindName,
						Cluster:       "hub",
						Policy:        item.Metadata.Annotations["kairos.io/policy"],
						Agent:         item.Metadata.Annotations["kairos.io/agent"],
						CurrentCPU:    cpu,
						CurrentMemory: mem,
						Status:        "managed",
					})
				}
			}
		}
	}

	if allResources == nil {
		allResources = []ManagedResource{}
	}
	json.NewEncoder(w).Encode(allResources)
}

// --- 2.5: /api/v1/notifications ---

func handleNotifications(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if isDemoMode() {
		notifications := map[string]interface{}{
			"configured": true,
			"webhookURL": "https://hooks.slack.com/services/T****/B****/xxxx",
			"lastSent":   time.Now().Add(-15 * time.Minute),
			"totalSent":  47,
			"events": []string{
				"scaling.applied",
				"scaling.rejected",
				"agent.disconnected",
				"policy.violated",
			},
		}
		json.NewEncoder(w).Encode(notifications)
		return
	}

	webhookURL := os.Getenv("KAIROS_NOTIFICATION_WEBHOOK_URL")
	configured := webhookURL != ""

	notifications := map[string]interface{}{
		"configured": configured,
		"webhookURL": webhookURL,
		"events":     []string{},
	}

	if configured {
		notifications["events"] = []string{
			"scaling.applied",
			"scaling.rejected",
			"agent.disconnected",
			"policy.violated",
		}
	}

	json.NewEncoder(w).Encode(notifications)
}

// --- 2.6: /api/v1/policies, /api/v1/events, /api/v1/status ---

func handlePolicies(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	policies, err := listSmartScalingPolicies()
	if err != nil {
		log.Printf("Failed to list SmartScalingPolicies: %v", err)
		if isDemoMode() {
			json.NewEncoder(w).Encode([]map[string]interface{}{
				{
					"name":               "demo-policy",
					"namespace":          "kairos-system",
					"cluster":            "hub",
					"target":             "kairos-console",
					"rules":              2,
					"paused":             false,
					"metricsSource":      "Thanos",
					"prometheusEndpoint": "thanos-querier.openshift-monitoring.svc:9091",
					"lastAction":         time.Now().Add(-5 * time.Minute),
				},
			})
			return
		}
		json.NewEncoder(w).Encode([]interface{}{})
		return
	}

	var result []map[string]interface{}
	for _, p := range policies {
		metricsSource := "none"
		if p.Spec.OtelEndpoint != "" {
			metricsSource = "OpenTelemetry"
		} else if p.Spec.PrometheusEndpoint != "" {
			metricsSource = "Thanos"
		}

		result = append(result, map[string]interface{}{
			"name":               p.Metadata.Name,
			"namespace":          p.Metadata.Namespace,
			"cluster":            "hub",
			"target":             p.Spec.Target.Name,
			"rules":              len(p.Spec.Rules),
			"paused":             p.Spec.Paused,
			"metricsSource":      metricsSource,
			"prometheusEndpoint": p.Spec.PrometheusEndpoint,
			"lastAction":         p.Metadata.CreationTimestamp,
		})
	}

	if result == nil {
		result = []map[string]interface{}{}
	}

	json.NewEncoder(w).Encode(result)
}

func handleEvents(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var events []map[string]interface{}

	kairosEvents, err := listKairosEvents()
	if err == nil && len(kairosEvents) > 0 {
		for _, ev := range kairosEvents {
			entry := map[string]interface{}{
				"timestamp": ev.Metadata.CreationTimestamp,
				"type":      "AgentReconciled",
				"resource":  ev.Spec.Resource,
				"namespace": ev.Spec.Namespace,
				"action":    ev.Spec.Action,
				"detail":    ev.Spec.Reason,
				"cluster":   ev.Spec.Cluster,
			}
			if ev.Spec.Before != nil || ev.Spec.After != nil {
				entry["before"] = ev.Spec.Before
				entry["after"] = ev.Spec.After
			}
			events = append(events, entry)
		}
	}

	// Local base event
	events = append(events, map[string]interface{}{
		"timestamp": time.Now().Add(-2 * time.Minute),
		"type":      "ScalingEvaluated",
		"resource":  "kairos-console",
		"namespace": "kairos-system",
		"action":    "NoAction",
		"detail":    "All metrics within threshold",
		"cluster":   "hub",
	})

	// Merge events reported by spoke agents
	agentStore.RLock()
	events = append(events, agentStore.events...)
	agentStore.RUnlock()

	if len(events) == 1 && isDemoMode() {
		events = append(events,
			map[string]interface{}{
				"timestamp": time.Now().Add(-10 * time.Minute),
				"type":      "AgentReconciled",
				"resource":  "east-agent",
				"namespace": "kairos-system",
				"action":    "ResourceOptimized",
				"detail":    "Memory request adjusted from 256Mi to 384Mi via AI recommendation",
				"cluster":   "east",
			},
			map[string]interface{}{
				"timestamp": time.Now().Add(-25 * time.Minute),
				"type":      "PolicyCreated",
				"resource":  "demo-policy",
				"namespace": "kairos-system",
				"action":    "Created",
				"detail":    "SmartScalingPolicy targeting kairos-console with Thanos metrics",
				"cluster":   "hub",
			},
		)
	}

	page := r.URL.Query().Get("page")
	pageSize := r.URL.Query().Get("pageSize")
	if page != "" && pageSize != "" {
		p := 0
		ps := 20
		fmt.Sscanf(page, "%d", &p)
		fmt.Sscanf(pageSize, "%d", &ps)
		start := p * ps
		if start >= len(events) {
			events = []map[string]interface{}{}
		} else {
			end := start + ps
			if end > len(events) {
				end = len(events)
			}
			events = events[start:end]
		}
	}

	json.NewEncoder(w).Encode(events)
}

func handleClusters(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	clusters := []map[string]interface{}{
		{"name": "hub", "region": "central", "status": "healthy", "agents": 1, "policies": 1, "apiURL": "https://api.cluster-xqg4c.dynamic2.redhatworkshops.io:6443"},
		{"name": "east", "region": "us-east-2", "status": "healthy", "agents": 1, "policies": 0, "apiURL": "https://api.cluster-2847b.dynamic2.redhatworkshops.io:6443"},
		{"name": "west", "region": "us-west-1", "status": "healthy", "agents": 1, "policies": 0, "apiURL": "https://api.cluster-5zjkk.dynamic2.redhatworkshops.io:6443"},
	}
	json.NewEncoder(w).Encode(clusters)
}

func handleStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	totalAgents := 1
	totalPolicies := 0
	totalEvents := 0
	totalApprovals := 0

	agentStore.RLock()
	totalAgents += len(agentStore.agents)
	totalEvents += len(agentStore.events)
	agentStore.RUnlock()

	kairosEvents, err := listKairosEvents()
	if err == nil {
		totalEvents += len(kairosEvents)
		for _, ev := range kairosEvents {
			if ev.Spec.Status == "pending-approval" {
				totalApprovals++
			}
		}
	}

	policies, err := listSmartScalingPolicies()
	if err == nil {
		totalPolicies = len(policies)
	}

	if isDemoMode() {
		if totalAgents <= 1 {
			totalAgents = 3
		}
		if totalPolicies == 0 {
			totalPolicies = 1
		}
		if totalEvents == 0 {
			totalEvents = 3
		}
		if totalApprovals == 0 {
			approvalStore.RLock()
			for _, a := range approvalStore.items {
				if a["status"] == "pending" {
					totalApprovals++
				}
			}
			approvalStore.RUnlock()
		}
	}

	status := map[string]interface{}{
		"operatorVersion": "2.1.0",
		"totalAgents":     totalAgents,
		"totalPolicies":   totalPolicies,
		"totalEvents":     totalEvents,
		"totalApprovals":  totalApprovals,
		"totalHistory":    totalEvents,
		"uptime":          fmt.Sprintf("%dm", int(time.Since(startTime).Minutes())),
		"metricsSource":   "Thanos Querier",
	}
	json.NewEncoder(w).Encode(status)
}

var startTime = time.Now()

func handleUser(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	username := "anonymous"
	authenticated := false

	if user := r.Header.Get("X-Forwarded-User"); user != "" {
		username = user
		authenticated = true
	} else if email := r.Header.Get("X-Forwarded-Email"); email != "" {
		username = email
		authenticated = true
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"username":      username,
		"authenticated": authenticated,
	})
}

func splitPath(path string) []string {
	parts := make([]string, 0)
	for _, p := range strings.Split(path, "/") {
		if p != "" {
			parts = append(parts, p)
		}
	}
	return parts
}
