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

// AuthType defines authentication method for the console.
// +kubebuilder:validation:Enum=openshift-oauth;none;token
type AuthType string

const (
	AuthTypeOpenshiftOAuth AuthType = "openshift-oauth"
	AuthTypeNone           AuthType = "none"
	AuthTypeToken          AuthType = "token"
)

// RouteConfig defines the OpenShift Route for the console.
type RouteConfig struct {
	Enabled bool `json:"enabled"`
	// +optional
	Host string `json:"host,omitempty"`
	// +optional
	TLSEnabled bool `json:"tlsEnabled,omitempty"`
}

// ConsoleAuthConfig defines authentication for the console.
type ConsoleAuthConfig struct {
	Type AuthType `json:"type"`
	// +optional
	TokenSecret *SecretKeyRef `json:"tokenSecret,omitempty"`
}

// KairosConsoleSpec defines the desired state of KairosConsole.
type KairosConsoleSpec struct {
	// Number of console replicas
	// +kubebuilder:default=1
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`
	// Console container image override
	// +optional
	Image string `json:"image,omitempty"`
	// Route configuration
	Route RouteConfig `json:"route"`
	// Authentication configuration
	// +optional
	Auth *ConsoleAuthConfig `json:"auth,omitempty"`
	// Clusters to display in the console
	// +optional
	Clusters []ClusterRef `json:"clusters,omitempty"`
}

// ClusterRef references a managed cluster for the console dashboard.
type ClusterRef struct {
	Name   string `json:"name"`
	Region string `json:"region,omitempty"`
	APIURL string `json:"apiURL,omitempty"`
}

// KairosConsoleStatus defines the observed state of KairosConsole.
type KairosConsoleStatus struct {
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	// Console URL once deployed
	// +optional
	URL string `json:"url,omitempty"`
	// Ready replicas
	// +optional
	ReadyReplicas int32 `json:"readyReplicas,omitempty"`
	// Console version deployed
	// +optional
	Version string `json:"version,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="URL",type=string,JSONPath=`.status.url`
// +kubebuilder:printcolumn:name="Ready",type=integer,JSONPath=`.status.readyReplicas`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// KairosConsole is the Schema for the kairosconsoles API.
type KairosConsole struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   KairosConsoleSpec   `json:"spec,omitempty"`
	Status KairosConsoleStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// KairosConsoleList contains a list of KairosConsole.
type KairosConsoleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []KairosConsole `json:"items"`
}

func init() {
	SchemeBuilder.Register(&KairosConsole{}, &KairosConsoleList{})
}
