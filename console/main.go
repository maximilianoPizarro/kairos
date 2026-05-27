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
	"embed"
	"encoding/json"
	"fmt"
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

	// Health endpoints
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "ok")
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "ok")
	})

	// API endpoints
	mux.HandleFunc("/api/v1/agents", handleAgents)
	mux.HandleFunc("/api/v1/policies", handlePolicies)
	mux.HandleFunc("/api/v1/events", handleEvents)
	mux.HandleFunc("/api/v1/clusters", handleClusters)
	mux.HandleFunc("/api/v1/status", handleStatus)

	// WebSocket endpoint
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

	// Serve static frontend files - prefer filesystem over embedded
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

func handleAgents(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	agents := []map[string]interface{}{
		{
			"name":            "agent-east-edge",
			"namespace":       "kairos-system",
			"mode":            "autopilot",
			"phase":           "Active",
			"watchedResources": 12,
			"totalCorrections": 3,
			"lastCheck":       time.Now().Add(-30 * time.Second),
		},
	}
	json.NewEncoder(w).Encode(agents)
}

func handlePolicies(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	policies := []map[string]interface{}{
		{
			"name":       "kafka-scaling",
			"namespace":  "industrial-edge",
			"target":     "factory-kafka",
			"rules":      2,
			"paused":     false,
			"lastAction": time.Now().Add(-5 * time.Minute),
		},
	}
	json.NewEncoder(w).Encode(policies)
}

func handleEvents(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	events := []map[string]interface{}{
		{
			"timestamp": time.Now().Add(-2 * time.Minute),
			"type":      "ScalingApplied",
			"resource":  "factory-kafka",
			"namespace": "industrial-edge",
			"action":    "IncreaseResources",
			"detail":    "Memory increased from 2Gi to 2.5Gi",
			"cluster":   "east",
		},
	}
	json.NewEncoder(w).Encode(events)
}

func handleClusters(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	clusters := []map[string]interface{}{
		{"name": "hub", "region": "central", "status": "healthy", "agents": 1, "policies": 3},
		{"name": "east", "region": "east", "status": "healthy", "agents": 1, "policies": 2},
		{"name": "west", "region": "west", "status": "healthy", "agents": 1, "policies": 2},
	}
	json.NewEncoder(w).Encode(clusters)
}

func handleStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	status := map[string]interface{}{
		"operatorVersion": "0.1.0",
		"totalAgents":     3,
		"totalPolicies":   7,
		"totalEvents":     42,
		"uptime":          "2h34m",
	}
	json.NewEncoder(w).Encode(status)
}
