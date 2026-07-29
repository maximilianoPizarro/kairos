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

package controller

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	kairosv1alpha1 "github.com/maximilianoPizarro/kairos/api/v1alpha1"
)

func TestNormalizePrometheusEndpoint(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"", ""},
		{"http://prom:9090", "http://prom:9090"},
		{"https://thanos:9091/", "https://thanos:9091"},
		{"prometheus.default.svc:9090", "http://prometheus.default.svc:9090"},
		{"thanos-querier.openshift-monitoring.svc:9091", "https://thanos-querier.openshift-monitoring.svc:9091"},
		{"prometheus-k8s.openshift-monitoring.svc.cluster.local:9090", "https://prometheus-k8s.openshift-monitoring.svc.cluster.local:9090"},
	}
	for _, tc := range cases {
		if got := normalizePrometheusEndpoint(tc.in); got != tc.want {
			t.Fatalf("normalizePrometheusEndpoint(%q)=%q want %q", tc.in, got, tc.want)
		}
	}
}

func TestQueryPrometheusSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/query" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if r.URL.Query().Get("query") != "up" {
			t.Fatalf("unexpected query %q", r.URL.Query().Get("query"))
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"status": "success",
			"data": map[string]interface{}{
				"resultType": "vector",
				"result": []map[string]interface{}{
					{"value": []interface{}{1.0, "0.75"}},
				},
			},
		})
	}))
	defer srv.Close()

	val, err := queryPrometheus(context.Background(), srv.URL, "up")
	if err != nil {
		t.Fatalf("queryPrometheus error: %v", err)
	}
	if val != 0.75 {
		t.Fatalf("got %v want 0.75", val)
	}
}

func TestQueryPrometheusEmptyResult(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"status": "success",
			"data": map[string]interface{}{
				"resultType": "vector",
				"result":     []interface{}{},
			},
		})
	}))
	defer srv.Close()

	_, err := queryPrometheus(context.Background(), srv.URL, "up")
	if err == nil {
		t.Fatal("expected error for empty result")
	}
}

func TestInCooldownPolicyWide(t *testing.T) {
	r := &SmartScalingPolicyReconciler{}
	policy := &kairosv1alpha1.SmartScalingPolicy{
		Status: kairosv1alpha1.SmartScalingPolicyStatus{
			LastScalingEvent: &kairosv1alpha1.ScalingEvent{
				Rule:      "rule-a",
				Action:    kairosv1alpha1.ActionAddReplicas,
				Timestamp: metav1.NewTime(time.Now().Add(-30 * time.Second)),
			},
		},
	}
	actionB := kairosv1alpha1.ScalingAction{
		Type:     kairosv1alpha1.ActionRemoveReplicas,
		Cooldown: "2m",
	}
	if !r.inCooldown(policy, actionB) {
		t.Fatal("expected cooldown to block a different rule within cooldown window")
	}

	policy.Status.LastScalingEvent.Timestamp = metav1.NewTime(time.Now().Add(-5 * time.Minute))
	if r.inCooldown(policy, actionB) {
		t.Fatal("expected cooldown to expire")
	}
}

func TestConditionHeldFor(t *testing.T) {
	key := "ns/policy/rule-for"
	ruleConditionSince.Delete(key)
	now := time.Now()

	if conditionHeldFor(key, "1m", now, false) {
		t.Fatal("false match should not hold")
	}
	if conditionHeldFor(key, "1m", now, true) {
		t.Fatal("first true match should wait for duration")
	}
	// Simulate condition already true for 2 minutes.
	ruleConditionSince.Store(key, now.Add(-2*time.Minute))
	if !conditionHeldFor(key, "1m", now, true) {
		t.Fatal("expected for-duration to be satisfied")
	}
	ruleConditionSince.Delete(key)
}
