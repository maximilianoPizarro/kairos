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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AgentMode defines how the agent operates.
// +kubebuilder:validation:Enum=autopilot;supervised
type AgentMode string

const (
	AgentModeAutopilot  AgentMode = "autopilot"
	AgentModeSupervised AgentMode = "supervised"
)

// TLSConfig defines TLS settings for connections.
type TLSConfig struct {
	// Skip TLS certificate verification (for disconnected/air-gapped environments)
	// +optional
	InsecureSkipVerify bool `json:"insecureSkipVerify,omitempty"`
	// Custom CA certificate secret reference
	// +optional
	CASecretRef *SecretKeyRef `json:"caSecretRef,omitempty"`
	// Client certificate secret reference (mTLS)
	// +optional
	CertSecretRef *SecretKeyRef `json:"certSecretRef,omitempty"`
}

// HubReportingConfig defines how the agent reports to the hub cluster console.
type HubReportingConfig struct {
	// Enable reporting agent status to hub console
	Enabled bool `json:"enabled"`
	// Hub console API endpoint (e.g. https://kairos-console.apps.hub-cluster.example.com)
	Endpoint string `json:"endpoint"`
	// Cluster name as displayed in the hub console (e.g. "east", "west", "factory-floor")
	// +optional
	ClusterName string `json:"clusterName,omitempty"`
	// Skip TLS verification for hub connection (air-gapped / self-signed certs)
	// +optional
	InsecureSkipVerify bool `json:"insecureSkipVerify,omitempty"`
	// Bearer token secret for authenticating to the hub
	// +optional
	TokenSecretRef *SecretKeyRef `json:"tokenSecretRef,omitempty"`
	// TLS settings for hub communication
	// +optional
	TLS *TLSConfig `json:"tls,omitempty"`
}

// AIModelConfig defines the AI model connection.
type AIModelConfig struct {
	// OpenAI-compatible API endpoint
	APIURL string `json:"apiURL"`
	// Model identifier
	Model string `json:"model"`
	// Secret reference for API key
	// +optional
	APIKeySecret *SecretKeyRef `json:"apiKeySecret,omitempty"`
	// Request timeout
	// +optional
	TimeoutSeconds *int32 `json:"timeoutSeconds,omitempty"`
	// Temperature for model responses (0.0 - 1.0)
	// +optional
	Temperature *string `json:"temperature,omitempty"`
	// TLS settings for AI API connection
	// +optional
	TLS *TLSConfig `json:"tls,omitempty"`
}

// SecretKeyRef references a key in a Secret.
type SecretKeyRef struct {
	Name string `json:"name"`
	Key  string `json:"key"`
}

// WatchConfig defines what the agent monitors.
type WatchConfig struct {
	// Namespaces to watch
	Namespaces []string `json:"namespaces"`
	// Resource types to monitor
	ResourceTypes []string `json:"resourceTypes"`
	// Label selector for filtering resources
	// +optional
	LabelSelector string `json:"labelSelector,omitempty"`
	// NamespaceSuffix filters namespaces by suffix (e.g. "-dev", "-test", "-qa", "-prod").
	// When set, the agent watches ALL namespaces ending with this suffix.
	// Can be combined with explicit Namespaces list.
	// +optional
	NamespaceSuffix string `json:"namespaceSuffix,omitempty"`
}

// CorrectionPolicy defines guardrails for autonomous corrections.
type CorrectionPolicy struct {
	// Maximum corrections per hour
	MaxActionsPerHour int32 `json:"maxActionsPerHour"`
	// Changes above this percentage require human approval
	// +optional
	RequireApprovalAbove string `json:"requireApprovalAbove,omitempty"`
	// Automatically rollback if a correction causes degradation
	// +optional
	RollbackOnFailure bool `json:"rollbackOnFailure,omitempty"`
	// Dry-run mode: log decisions without applying them
	// +optional
	DryRun bool `json:"dryRun,omitempty"`
}

// ReportingConfig defines how the agent reports its activity.
type ReportingConfig struct {
	// How often the agent reports status
	Interval string `json:"interval"`
	// Send events to OTel collector
	// +optional
	OtelExport bool `json:"otelExport,omitempty"`
}

// KairosAgentSpec defines the desired state of KairosAgent.
type KairosAgentSpec struct {
	// Operating mode: autopilot or supervised
	Mode AgentMode `json:"mode"`
	// AI model configuration
	AIModel AIModelConfig `json:"aiModel"`
	// What resources to watch
	Watch WatchConfig `json:"watch"`
	// Correction guardrails
	CorrectionPolicy CorrectionPolicy `json:"correctionPolicy"`
	// Reporting configuration
	// +optional
	Reporting *ReportingConfig `json:"reporting,omitempty"`
	// Hub reporting: push agent status to the hub console
	// +optional
	HubReporting *HubReportingConfig `json:"hubReporting,omitempty"`
	// Pause the agent
	// +optional
	Paused bool `json:"paused,omitempty"`
}

// AgentPhase represents the agent lifecycle phase.
// +kubebuilder:validation:Enum=Active;Idle;Correcting;WaitingApproval;Error;Paused
type AgentPhase string

const (
	AgentPhaseActive          AgentPhase = "Active"
	AgentPhaseIdle            AgentPhase = "Idle"
	AgentPhaseCorrecting      AgentPhase = "Correcting"
	AgentPhaseWaitingApproval AgentPhase = "WaitingApproval"
	AgentPhaseError           AgentPhase = "Error"
	AgentPhasePaused          AgentPhase = "Paused"
)

// CorrectionRecord records a correction the agent made or proposed.
type CorrectionRecord struct {
	Timestamp  metav1.Time `json:"timestamp"`
	Resource   string      `json:"resource"`
	Namespace  string      `json:"namespace"`
	Action     string      `json:"action"`
	Reason     string      `json:"reason"`
	Applied    bool        `json:"applied"`
	AIResponse string      `json:"aiResponse,omitempty"`
}

// PendingApproval represents a correction waiting for human approval.
type PendingApproval struct {
	ID         string      `json:"id"`
	Timestamp  metav1.Time `json:"timestamp"`
	Resource   string      `json:"resource"`
	Namespace  string      `json:"namespace"`
	Action     string      `json:"action"`
	Reason     string      `json:"reason"`
	AIResponse string      `json:"aiResponse,omitempty"`
}

// DryRunRecommendation records a resource change that would be applied if dry-run were disabled.
type DryRunRecommendation struct {
	Timestamp     metav1.Time `json:"timestamp"`
	Resource      string      `json:"resource"`
	Namespace     string      `json:"namespace"`
	CurrentCPU    string      `json:"currentCPU,omitempty"`
	CurrentMemory string      `json:"currentMemory,omitempty"`
	ProposedCPU   string      `json:"proposedCPU,omitempty"`
	ProposedMemory string     `json:"proposedMemory,omitempty"`
	Reason        string      `json:"reason"`
	AIResponse    string      `json:"aiResponse,omitempty"`
}

// KairosAgentStatus defines the observed state of KairosAgent.
type KairosAgentStatus struct {
	// Current phase of the agent
	// +optional
	Phase AgentPhase `json:"phase,omitempty"`
	// Conditions
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	// Total corrections applied
	// +optional
	TotalCorrections int32 `json:"totalCorrections,omitempty"`
	// Corrections in the last hour
	// +optional
	CorrectionsLastHour int32 `json:"correctionsLastHour,omitempty"`
	// Recent corrections (last 20)
	// +optional
	RecentCorrections []CorrectionRecord `json:"recentCorrections,omitempty"`
	// Dry-run recommendations (last 20, only when dryRun: true)
	// +optional
	DryRunRecommendations []DryRunRecommendation `json:"dryRunRecommendations,omitempty"`
	// Pending approvals (supervised mode)
	// +optional
	PendingApprovals []PendingApproval `json:"pendingApprovals,omitempty"`
	// Last time the agent checked resources
	// +optional
	LastCheckTime *metav1.Time `json:"lastCheckTime,omitempty"`
	// Watched resource count
	// +optional
	WatchedResources int32 `json:"watchedResources,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Mode",type=string,JSONPath=`.spec.mode`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="DryRun",type=boolean,JSONPath=`.spec.correctionPolicy.dryRun`
// +kubebuilder:printcolumn:name="Corrections",type=integer,JSONPath=`.status.totalCorrections`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// KairosAgent is the Schema for the kairosagents API.
type KairosAgent struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   KairosAgentSpec   `json:"spec,omitempty"`
	Status KairosAgentStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// KairosAgentList contains a list of KairosAgent.
type KairosAgentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []KairosAgent `json:"items"`
}

func init() {
	SchemeBuilder.Register(&KairosAgent{}, &KairosAgentList{})
}
