/*
Copyright 2026.
*/

package controller

import (
	"testing"

	kairosv1alpha1 "github.com/maximilianoPizarro/kairos/api/v1alpha1"
	"github.com/maximilianoPizarro/kairos/internal/ai"
)

func testRec(action string, details map[string]string) *ai.ScalingRecommendation {
	return &ai.ScalingRecommendation{
		Action:  action,
		Details: details,
	}
}

func TestScaleActionFromEventHorizontal(t *testing.T) {
	ev := &kairosv1alpha1.KairosEvent{
		Spec: kairosv1alpha1.KairosEventSpec{
			Action:    "scale_up",
			Resource:  "demo-app",
			Namespace: "kairos-system",
			After: kairosv1alpha1.ResourceSnapshot{
				Replicas: "3",
				CPU:      "100m",
				Memory:   "128Mi",
			},
		},
	}
	target, action, err := scaleActionFromEvent(ev)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if target.Name != "demo-app" || target.Namespace != "kairos-system" {
		t.Fatalf("unexpected target: %+v", target)
	}
	if action.Type != "horizontal" {
		t.Fatalf("expected horizontal for scale_up, got %q", action.Type)
	}
	if action.Replicas == nil || *action.Replicas != 3 {
		t.Fatalf("expected replicas=3, got %+v", action.Replicas)
	}
	if action.Resources != nil {
		t.Fatal("scale_up should not patch resources (SSA template risk)")
	}
}

func TestScaleActionFromEventRequiresReplicas(t *testing.T) {
	ev := &kairosv1alpha1.KairosEvent{
		Spec: kairosv1alpha1.KairosEventSpec{
			Action:    "scale_up",
			Resource:  "demo-app",
			Namespace: "kairos-system",
			After:     kairosv1alpha1.ResourceSnapshot{CPU: "100m"},
		},
	}
	_, _, err := scaleActionFromEvent(ev)
	if err == nil {
		t.Fatal("expected error when scale_up lacks after.replicas")
	}
}

func TestProposedSnapshotScaleUpReplicas(t *testing.T) {
	snap := proposedSnapshot(testRec("scale_up", nil), "100m", "128Mi", 2)
	if snap.Replicas != "3" {
		t.Fatalf("expected replicas 3, got %q", snap.Replicas)
	}
	if snap.CPU != "100m" || snap.Memory != "128Mi" {
		t.Fatalf("expected current resources preserved, got %+v", snap)
	}
}

func TestProposedSnapshotDetailsOverride(t *testing.T) {
	snap := proposedSnapshot(testRec("increase_resources", map[string]string{
		"cpu": "200m", "memory": "256Mi",
	}), "100m", "128Mi", 1)
	if snap.CPU != "200m" || snap.Memory != "256Mi" {
		t.Fatalf("details not applied: %+v", snap)
	}
}
