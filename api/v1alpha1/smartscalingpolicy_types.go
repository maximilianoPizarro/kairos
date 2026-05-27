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

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TargetRef identifies the workload to scale.
type TargetRef struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Name       string `json:"name"`
	Namespace  string `json:"namespace,omitempty"`
}

// ComparisonOperator defines how a metric is evaluated.
// +kubebuilder:validation:Enum=GreaterThan;LessThan;Equal
type ComparisonOperator string

const (
	OperatorGreaterThan ComparisonOperator = "GreaterThan"
	OperatorLessThan    ComparisonOperator = "LessThan"
	OperatorEqual       ComparisonOperator = "Equal"
)

// ActionType defines what scaling action to take.
// +kubebuilder:validation:Enum=IncreaseResources;DecreaseResources;AddReplicas;RemoveReplicas;SetMinReplicas;SetResources
type ActionType string

const (
	ActionIncreaseResources ActionType = "IncreaseResources"
	ActionDecreaseResources ActionType = "DecreaseResources"
	ActionAddReplicas       ActionType = "AddReplicas"
	ActionRemoveReplicas    ActionType = "RemoveReplicas"
	ActionSetMinReplicas    ActionType = "SetMinReplicas"
	ActionSetResources      ActionType = "SetResources"
)

// MetricCondition defines when a rule triggers.
type MetricCondition struct {
	// OTel or Prometheus metric name
	Metric string `json:"metric"`
	// Comparison operator
	Operator ComparisonOperator `json:"operator"`
	// Threshold value (e.g. "200ms", "500", "80%")
	Threshold string `json:"threshold"`
	// Duration the condition must hold before triggering
	// +optional
	For string `json:"for,omitempty"`
}

// ScalingAction defines what to do when a rule triggers.
type ScalingAction struct {
	Type ActionType `json:"type"`
	// +optional
	IncreaseMemoryPercent *int32 `json:"increaseMemoryPercent,omitempty"`
	// +optional
	IncreaseCPUPercent *int32 `json:"increaseCPUPercent,omitempty"`
	// +optional
	MaxMemory *resource.Quantity `json:"maxMemory,omitempty"`
	// +optional
	MaxCPU *resource.Quantity `json:"maxCPU,omitempty"`
	// +optional
	MinMemory *resource.Quantity `json:"minMemory,omitempty"`
	// +optional
	MinCPU *resource.Quantity `json:"minCPU,omitempty"`
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`
	// +optional
	MinReplicas *int32 `json:"minReplicas,omitempty"`
	// +optional
	MaxReplicas *int32 `json:"maxReplicas,omitempty"`
	// +optional
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`
	// Cooldown period between consecutive actions
	// +optional
	Cooldown string `json:"cooldown,omitempty"`
}

// ScalingRule defines a single metric-based rule.
type ScalingRule struct {
	Name   string          `json:"name"`
	When   MetricCondition `json:"when"`
	Action ScalingAction   `json:"action"`
	// +optional
	Enabled *bool `json:"enabled,omitempty"`
}

// ScheduleAction defines a time-based scaling action.
type ScheduleAction struct {
	Name   string        `json:"name"`
	Cron   string        `json:"cron"`
	Action ScalingAction `json:"action"`
	// +optional
	Enabled *bool `json:"enabled,omitempty"`
}

// AIConfig enables optional AI-assisted scaling decisions.
type AIConfig struct {
	Enabled bool `json:"enabled"`
	// +optional
	AgentRef string `json:"agentRef,omitempty"`
}

// SmartScalingPolicySpec defines the desired state of SmartScalingPolicy.
type SmartScalingPolicySpec struct {
	// Target workload to scale
	Target TargetRef `json:"target"`
	// OTel collector endpoint (gRPC)
	// +optional
	OtelEndpoint string `json:"otelEndpoint,omitempty"`
	// Prometheus endpoint (fallback)
	// +optional
	PrometheusEndpoint string `json:"prometheusEndpoint,omitempty"`
	// TLS settings for metrics endpoints (Prometheus/Thanos/OTel)
	// +optional
	MetricsTLS *TLSConfig `json:"metricsTLS,omitempty"`
	// Metric-based scaling rules
	// +optional
	Rules []ScalingRule `json:"rules,omitempty"`
	// Time-based scheduling rules
	// +optional
	Schedule []ScheduleAction `json:"schedule,omitempty"`
	// AI-assisted decisions configuration
	// +optional
	AI *AIConfig `json:"ai,omitempty"`
	// Pause all scaling actions
	// +optional
	Paused bool `json:"paused,omitempty"`
}

// ConditionType for status conditions.
type ConditionType string

const (
	ConditionReady    ConditionType = "Ready"
	ConditionScaling  ConditionType = "Scaling"
	ConditionDegraded ConditionType = "Degraded"
)

// ScalingEvent records a scaling action that was taken.
type ScalingEvent struct {
	Timestamp metav1.Time `json:"timestamp"`
	Rule      string      `json:"rule"`
	Action    ActionType  `json:"action"`
	Detail    string      `json:"detail"`
	Success   bool        `json:"success"`
}

// SmartScalingPolicyStatus defines the observed state of SmartScalingPolicy.
type SmartScalingPolicyStatus struct {
	// Current conditions
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	// Current replica count
	// +optional
	CurrentReplicas *int32 `json:"currentReplicas,omitempty"`
	// Current resource allocation
	// +optional
	CurrentResources *corev1.ResourceRequirements `json:"currentResources,omitempty"`
	// Last scaling event
	// +optional
	LastScalingEvent *ScalingEvent `json:"lastScalingEvent,omitempty"`
	// Recent scaling events (last 10)
	// +optional
	RecentEvents []ScalingEvent `json:"recentEvents,omitempty"`
	// Last time the policy was evaluated
	// +optional
	LastEvaluationTime *metav1.Time `json:"lastEvaluationTime,omitempty"`
	// Active rules currently triggering
	// +optional
	ActiveRules []string `json:"activeRules,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Target",type=string,JSONPath=`.spec.target.name`
// +kubebuilder:printcolumn:name="Rules",type=integer,JSONPath=`.spec.rules`
// +kubebuilder:printcolumn:name="Paused",type=boolean,JSONPath=`.spec.paused`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// SmartScalingPolicy is the Schema for the smartscalingpolicies API.
type SmartScalingPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SmartScalingPolicySpec   `json:"spec,omitempty"`
	Status SmartScalingPolicyStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// SmartScalingPolicyList contains a list of SmartScalingPolicy.
type SmartScalingPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SmartScalingPolicy `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SmartScalingPolicy{}, &SmartScalingPolicyList{})
}
