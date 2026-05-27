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

const (
	// AnnotationManaged marks a resource as managed by Kairos.
	// Only resources with this annotation set to "true" will be touched.
	AnnotationManaged = "kairos.io/managed"

	// AnnotationPolicy links a resource to a SmartScalingPolicy by name.
	AnnotationPolicy = "kairos.io/policy"

	// AnnotationLastAction records the last action taken by the operator.
	AnnotationLastAction = "kairos.io/last-action"

	// AnnotationLastActionTime records when the last action was taken.
	AnnotationLastActionTime = "kairos.io/last-action-time"

	// FieldOwnerKairos is the Server-Side Apply field manager name.
	// This prevents conflicts with ArgoCD and other controllers.
	FieldOwnerKairos = "kairos-operator"

	// LabelManagedBy standard label for resources created by the operator.
	LabelManagedBy = "app.kubernetes.io/managed-by"

	// LabelManagedByValue value for the managed-by label.
	LabelManagedByValue = "kairos-operator"

	// LabelComponent identifies the component within Kairos.
	LabelComponent = "app.kubernetes.io/component"

	// DefaultOTelEndpoint default OpenTelemetry collector endpoint.
	DefaultOTelEndpoint = "otel-collector.observability:4317"

	// DefaultConsoleImage is the default console container image.
	DefaultConsoleImage = "quay.io/maximilianopizarro/kairos-console:latest"
)
