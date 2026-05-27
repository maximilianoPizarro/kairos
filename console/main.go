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
	Name             string    `json:"name"`
	Namespace        string    `json:"namespace"`
	Cluster          string    `json:"cluster"`
	Mode             string    `json:"mode"`
	Phase            string    `json:"phase"`
	WatchedResources int       `json:"watchedResources"`
	TotalCorrections int       `json:"totalCorrections"`
	LastCheck        time.Time `json:"lastCheck"`
	AIModel          string    `json:"aiModel"`
	Events           []map[string]interface{} `json:"events,omitempty"`
}

// agentStore holds agent reports from spoke clusters (thread-safe)
var agentStore = struct {
	sync.RWMutex
	agents map[string]*AgentReport
	events []map[string]interface{}
}{
	agents: make(map[string]*AgentReport),
	events: make([]map[string]interface{}, 0),
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
			"status":       thanosStatus,
			"endpoint":     getThanosEndpoint(),
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
			"name":             "demo-policy",
			"namespace":        "kairos-system",
			"cluster":          "hub",
			"target":           "kairos-console",
			"rules":            2,
			"paused":           false,
			"metricsSource":    "Thanos",
			"prometheusEndpoint": "thanos-querier.openshift-monitoring.svc:9091",
			"lastAction":       time.Now().Add(-5 * time.Minute),
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
		"operatorVersion": "0.1.0",
		"totalAgents":     3,
		"totalPolicies":   1,
		"totalEvents":     3,
		"uptime":          fmt.Sprintf("%dm", int(time.Since(startTime).Minutes())),
		"metricsSource":   "Thanos Querier",
	}
	json.NewEncoder(w).Encode(status)
}

var startTime = time.Now()
