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

// approvalStore holds pending approval requests (thread-safe)
var approvalStore = struct {
	sync.RWMutex
	items []map[string]interface{}
}{
	items: []map[string]interface{}{
		{
			"id": "appr-001", "resource": "ie-anomaly-alerter",
			"namespace": "industrial-edge-tst-all", "cluster": "east", "agent": "east-agent",
			"proposedCPU": "250m", "proposedMemory": "512Mi",
			"currentCPU": "100m", "currentMemory": "256Mi",
			"reason": "CPU utilization averaging 92% over last 30 minutes; AI recommends scaling up to avoid throttling",
			"status": "pending",
		},
		{
			"id": "appr-002", "resource": "line-dashboard",
			"namespace": "industrial-edge-tst-all", "cluster": "west", "agent": "west-agent",
			"proposedCPU": "150m", "proposedMemory": "384Mi",
			"currentCPU": "200m", "currentMemory": "512Mi",
			"reason": "Resource over-provisioned: memory usage below 40% for 2 hours; AI recommends downsizing",
			"status": "pending",
		},
		{
			"id": "appr-003", "resource": "minio",
			"namespace": "industrial-edge-ml-workspace", "cluster": "east", "agent": "east-agent",
			"proposedCPU": "750m", "proposedMemory": "2Gi",
			"currentCPU": "500m", "currentMemory": "1Gi",
			"reason": "Storage I/O contention detected; scaling CPU and memory to match workload pattern",
			"status": "pending",
		},
		{
			"id": "appr-004", "resource": "machine-sensor-1",
			"namespace": "industrial-edge-tst-all", "cluster": "west", "agent": "west-agent",
			"proposedCPU": "100m", "proposedMemory": "256Mi",
			"currentCPU": "50m", "currentMemory": "128Mi",
			"reason": "Pod restarts detected (OOMKilled x2 in last hour); AI recommends doubling memory allocation",
			"status": "pending",
		},
	},
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

func main() {
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

	// Check Thanos connectivity
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

	// Check OTel endpoint
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

	// Local agent from this cluster (hub)
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

	// Merge in agents reported by spoke clusters
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

	// If no spoke agents have reported, show placeholder entries
	if len(agents) == 1 {
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
	// Store events from the report
	if len(report.Events) > 0 {
		agentStore.events = append(agentStore.events, report.Events...)
		// Keep last 100 events
		if len(agentStore.events) > 100 {
			agentStore.events = agentStore.events[len(agentStore.events)-100:]
		}
	}
	// Store managed resources from the report (replace per-cluster)
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

func handlePolicies(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	policies := []map[string]interface{}{
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
	}
	json.NewEncoder(w).Encode(policies)
}

func handleEvents(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Base events (local)
	events := []map[string]interface{}{
		{
			"timestamp": time.Now().Add(-2 * time.Minute),
			"type":      "ScalingEvaluated",
			"resource":  "kairos-console",
			"namespace": "kairos-system",
			"action":    "NoAction",
			"detail":    "All metrics within threshold",
			"cluster":   "hub",
		},
	}

	// Merge events reported by spoke agents
	agentStore.RLock()
	events = append(events, agentStore.events...)
	agentStore.RUnlock()

	// If no remote events, show example entries
	if len(events) == 1 {
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

	// Pagination support
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
	status := map[string]interface{}{
		"operatorVersion": "2.0.1",
		"totalAgents":     3,
		"totalPolicies":   1,
		"totalEvents":     3,
		"totalApprovals":  4,
		"totalHistory":    9,
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

	// When behind oauth-proxy, user info is passed via X-Forwarded-Email or X-Forwarded-User headers
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

func handleApprovals(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	approvalStore.RLock()
	pending := make([]map[string]interface{}, 0)
	for _, a := range approvalStore.items {
		if a["status"] == "pending" {
			pending = append(pending, a)
		}
	}
	approvalStore.RUnlock()

	json.NewEncoder(w).Encode(pending)
}

func handleApprovalAction(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Parse path: /api/v1/approvals/{id}/{action}
	path := r.URL.Path
	parts := splitPath(path)
	// Expected: ["api","v1","approvals","{id}","{action}"]
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

	approvalStore.Lock()
	found := false
	for i, a := range approvalStore.items {
		if a["id"] == id {
			found = true
			if action == "approve" {
				approvalStore.items[i]["status"] = "approved"
			} else {
				approvalStore.items[i]["status"] = "rejected"
			}
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

func splitPath(path string) []string {
	parts := make([]string, 0)
	for _, p := range strings.Split(path, "/") {
		if p != "" {
			parts = append(parts, p)
		}
	}
	return parts
}

func handleHistory(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	history := []map[string]interface{}{
		{
			"timestamp":    time.Now().Add(-1 * time.Hour),
			"agent":        "east-agent",
			"resource":     "ie-anomaly-alerter",
			"namespace":    "industrial-edge-tst-all",
			"cluster":      "east",
			"action":       "ScaleUp",
			"beforeCPU":    "50m",
			"beforeMemory": "128Mi",
			"afterCPU":     "100m",
			"afterMemory":  "256Mi",
			"status":       "applied",
			"aiResponse":   "CPU throttling observed at 95th percentile. Doubling requests to stabilize latency.",
		},
		{
			"timestamp":    time.Now().Add(-2 * time.Hour),
			"agent":        "west-agent",
			"resource":     "line-dashboard",
			"namespace":    "industrial-edge-tst-all",
			"cluster":      "west",
			"action":       "ScaleDown",
			"beforeCPU":    "400m",
			"beforeMemory": "1Gi",
			"afterCPU":     "200m",
			"afterMemory":  "512Mi",
			"status":       "applied",
			"aiResponse":   "Resource utilization consistently below 30% for 4 hours. Safe to reduce allocation.",
		},
		{
			"timestamp":    time.Now().Add(-4 * time.Hour),
			"agent":        "east-agent",
			"resource":     "minio",
			"namespace":    "industrial-edge-ml-workspace",
			"cluster":      "east",
			"action":       "ScaleUp",
			"beforeCPU":    "250m",
			"beforeMemory": "512Mi",
			"afterCPU":     "500m",
			"afterMemory":  "1Gi",
			"status":       "applied",
			"aiResponse":   "High I/O wait times correlated with training job schedule. Proactive scaling recommended.",
		},
		{
			"timestamp":    time.Now().Add(-6 * time.Hour),
			"agent":        "hub-agent",
			"resource":     "kairos-console",
			"namespace":    "kairos-system",
			"cluster":      "hub",
			"action":       "ScaleUp",
			"beforeCPU":    "100m",
			"beforeMemory": "128Mi",
			"afterCPU":     "200m",
			"afterMemory":  "256Mi",
			"status":       "dry-run",
			"aiResponse":   "Moderate increase suggested due to WebSocket connection growth. Dry-run mode active.",
		},
		{
			"timestamp":    time.Now().Add(-8 * time.Hour),
			"agent":        "west-agent",
			"resource":     "machine-sensor-1",
			"namespace":    "industrial-edge-tst-all",
			"cluster":      "west",
			"action":       "ScaleUp",
			"beforeCPU":    "25m",
			"beforeMemory": "64Mi",
			"afterCPU":     "50m",
			"afterMemory":  "128Mi",
			"status":       "applied",
			"aiResponse":   "OOMKilled events detected. Memory allocation insufficient for sensor data buffering.",
		},
		{
			"timestamp":    time.Now().Add(-10 * time.Hour),
			"agent":        "east-agent",
			"resource":     "machine-sensor-2",
			"namespace":    "industrial-edge-tst-all",
			"cluster":      "east",
			"action":       "ScaleDown",
			"beforeCPU":    "100m",
			"beforeMemory": "256Mi",
			"afterCPU":     "50m",
			"afterMemory":  "128Mi",
			"status":       "rejected",
			"aiResponse":   "Scaling down recommended but rejected by policy: minimum resource floor violated.",
		},
		{
			"timestamp":    time.Now().Add(-14 * time.Hour),
			"agent":        "west-agent",
			"resource":     "ie-anomaly-alerter",
			"namespace":    "industrial-edge-tst-all",
			"cluster":      "west",
			"action":       "ScaleUp",
			"beforeCPU":    "75m",
			"beforeMemory": "192Mi",
			"afterCPU":     "100m",
			"afterMemory":  "256Mi",
			"status":       "applied",
			"aiResponse":   "Anomaly detection pipeline experiencing backpressure. Increasing resources for timely alerting.",
		},
		{
			"timestamp":    time.Now().Add(-18 * time.Hour),
			"agent":        "hub-agent",
			"resource":     "kairos-console",
			"namespace":    "kairos-system",
			"cluster":      "hub",
			"action":       "NoAction",
			"beforeCPU":    "100m",
			"beforeMemory": "128Mi",
			"afterCPU":     "100m",
			"afterMemory":  "128Mi",
			"status":       "dry-run",
			"aiResponse":   "Current allocation matches demand. No changes required at this time.",
		},
		{
			"timestamp":    time.Now().Add(-22 * time.Hour),
			"agent":        "east-agent",
			"resource":     "line-dashboard",
			"namespace":    "industrial-edge-tst-all",
			"cluster":      "east",
			"action":       "ScaleUp",
			"beforeCPU":    "100m",
			"beforeMemory": "256Mi",
			"afterCPU":     "200m",
			"afterMemory":  "512Mi",
			"status":       "applied",
			"aiResponse":   "Shift change traffic spike anticipated based on historical patterns. Pre-scaling applied.",
		},
	}
	json.NewEncoder(w).Encode(history)
}

func handleNotifications(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
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
}

func handleManagedResources(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var allResources []ManagedResource

	// 1. Get resources from spoke clusters (reported via agent-report)
	agentStore.RLock()
	allResources = append(allResources, agentStore.resources...)
	spokeCount := len(agentStore.resources)
	agentStore.RUnlock()

	// If no spoke resources reported yet, generate from known agents mock data
	if spokeCount == 0 {
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

	// 2. Get local (hub) resources from Kubernetes API
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
