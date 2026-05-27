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

// ResourceSnapshot captures resource values at a point in time.
type ResourceSnapshot struct {
	CPU    string `json:"cpu,omitempty"`
	Memory string `json:"memory,omitempty"`
}

// KairosEventSpec defines the desired state of KairosEvent.
type KairosEventSpec struct {
	// Name of the KairosAgent that generated this event
	AgentName string `json:"agentName"`
	// Cluster where the event originated
	Cluster string `json:"cluster"`
	// Action taken (e.g. scale_up, scale_down, restart, patch)
	Action string `json:"action"`
	// Target resource name
	Resource string `json:"resource"`
	// Target resource namespace
	Namespace string `json:"namespace"`
	// Resource state before the action
	// +optional
	Before ResourceSnapshot `json:"before,omitempty"`
	// Resource state after the action
	// +optional
	After ResourceSnapshot `json:"after,omitempty"`
	// Human-readable reason for the action
	// +optional
	Reason string `json:"reason,omitempty"`
	// Raw AI model response
	// +optional
	AIResponse string `json:"aiResponse,omitempty"`
	// Whether this was a dry-run (no actual changes applied)
	// +optional
	DryRun bool `json:"dryRun,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:printcolumn:name="Agent",type=string,JSONPath=`.spec.agentName`
// +kubebuilder:printcolumn:name="Action",type=string,JSONPath=`.spec.action`
// +kubebuilder:printcolumn:name="Resource",type=string,JSONPath=`.spec.resource`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// KairosEvent is the Schema for the kairosevents API.
type KairosEvent struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec KairosEventSpec `json:"spec,omitempty"`
}

// +kubebuilder:object:root=true

// KairosEventList contains a list of KairosEvent.
type KairosEventList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []KairosEvent `json:"items"`
}

func init() {
	SchemeBuilder.Register(&KairosEvent{}, &KairosEventList{})
}
