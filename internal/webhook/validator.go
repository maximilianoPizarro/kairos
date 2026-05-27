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

package webhook

import (
	"context"
	"fmt"
	"net/http"

	admissionv1 "k8s.io/api/admission/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	kairosv1alpha1 "github.com/maximilianoPizarro/kairos/api/v1alpha1"
)

var (
	scheme = runtime.NewScheme()
	codecs = serializer.NewCodecFactory(scheme)
)

func init() {
	_ = kairosv1alpha1.AddToScheme(scheme)
}

// KairosValidator validates Kairos CRDs.
type KairosValidator struct {
	decoder admission.Decoder
}

// NewKairosValidator creates a new validator.
func NewKairosValidator(decoder admission.Decoder) *KairosValidator {
	return &KairosValidator{decoder: decoder}
}

// Handle validates admission requests for Kairos resources.
func (v *KairosValidator) Handle(ctx context.Context, req admission.Request) admission.Response {
	switch req.Kind.Kind {
	case "SmartScalingPolicy":
		return v.validateSmartScalingPolicy(req)
	case "KairosAgent":
		return v.validateKairosAgent(req)
	default:
		return admission.Allowed("resource type not validated")
	}
}

func (v *KairosValidator) validateSmartScalingPolicy(req admission.Request) admission.Response {
	if req.Operation == admissionv1.Delete {
		return admission.Allowed("delete always allowed")
	}

	policy := &kairosv1alpha1.SmartScalingPolicy{}
	if err := v.decoder.Decode(req, policy); err != nil {
		return admission.Errored(http.StatusBadRequest, fmt.Errorf("decode failed: %w", err))
	}

	// Validate target reference
	if policy.Spec.Target.Kind == "" {
		return admission.Denied("spec.target.kind is required")
	}
	validKinds := map[string]bool{"Deployment": true, "StatefulSet": true, "DaemonSet": true, "CronJob": true}
	if !validKinds[policy.Spec.Target.Kind] {
		return admission.Denied(fmt.Sprintf("spec.target.kind must be one of: Deployment, StatefulSet, DaemonSet, CronJob (got %q)", policy.Spec.Target.Kind))
	}
	if policy.Spec.Target.Name == "" {
		return admission.Denied("spec.target.name is required")
	}

	// Validate rules
	for i, rule := range policy.Spec.Rules {
		if rule.Name == "" {
			return admission.Denied(fmt.Sprintf("spec.rules[%d].name is required", i))
		}
		if rule.When.Metric == "" {
			return admission.Denied(fmt.Sprintf("spec.rules[%d].when.metric is required", i))
		}
		if rule.When.Threshold == "" {
			return admission.Denied(fmt.Sprintf("spec.rules[%d].when.threshold is required", i))
		}
	}

	// Validate scope
	if policy.Spec.Scope != "" {
		validScopes := map[string]bool{"global": true, "cluster": true, "namespace": true}
		if !validScopes[policy.Spec.Scope] {
			return admission.Denied(fmt.Sprintf("spec.scope must be global, cluster, or namespace (got %q)", policy.Spec.Scope))
		}
	}

	return admission.Allowed("valid SmartScalingPolicy")
}

func (v *KairosValidator) validateKairosAgent(req admission.Request) admission.Response {
	if req.Operation == admissionv1.Delete {
		return admission.Allowed("delete always allowed")
	}

	agent := &kairosv1alpha1.KairosAgent{}
	if err := v.decoder.Decode(req, agent); err != nil {
		return admission.Errored(http.StatusBadRequest, fmt.Errorf("decode failed: %w", err))
	}

	// Validate mode
	validModes := map[kairosv1alpha1.AgentMode]bool{
		kairosv1alpha1.AgentModeAutopilot:  true,
		kairosv1alpha1.AgentModeSupervised: true,
	}
	if !validModes[agent.Spec.Mode] {
		return admission.Denied(fmt.Sprintf("spec.mode must be one of: autopilot, supervised (got %q)", agent.Spec.Mode))
	}

	// Validate correction policy
	if agent.Spec.CorrectionPolicy.MaxActionsPerHour <= 0 {
		return admission.Denied("spec.correctionPolicy.maxActionsPerHour must be greater than 0")
	}

	// Validate watch config
	if len(agent.Spec.Watch.ResourceTypes) == 0 {
		return admission.Denied("spec.watch.resourceTypes must contain at least one entry")
	}
	validTypes := map[string]bool{"Deployment": true, "StatefulSet": true, "DaemonSet": true, "CronJob": true}
	for _, rt := range agent.Spec.Watch.ResourceTypes {
		if !validTypes[rt] {
			return admission.Denied(fmt.Sprintf("spec.watch.resourceTypes contains invalid type %q", rt))
		}
	}

	// Validate pinned resources
	for i, pin := range agent.Spec.PinnedResources {
		if pin.Name == "" {
			return admission.Denied(fmt.Sprintf("spec.pinnedResources[%d].name is required", i))
		}
		if pin.Namespace == "" {
			return admission.Denied(fmt.Sprintf("spec.pinnedResources[%d].namespace is required", i))
		}
	}

	return admission.Allowed("valid KairosAgent")
}
