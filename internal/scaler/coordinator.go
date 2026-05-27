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

package scaler

import (
	"context"
	"errors"
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	kairosv1alpha1 "github.com/maximilianoPizarro/kairos/api/v1alpha1"
)

const (
	scaleTypeVertical   = "vertical"
	scaleTypeHorizontal = "horizontal"
	scaleTypeBoth       = "both"

	kindDeployment  = "Deployment"
	kindStatefulSet = "StatefulSet"

	defaultCooldownPeriod = 5 * time.Minute
)

var (
	ErrCooldownActive        = errors.New("scaling action blocked by cooldown period")
	ErrUnsupportedTargetKind = errors.New("unsupported scaling target kind")
	ErrNoScalingChanges      = errors.New("no scaling changes to apply")
)

// ScaleAction describes a coordinated scaling change.
type ScaleAction struct {
	Type      string
	Resources *corev1.ResourceRequirements
	Replicas  *int32
	Reason    string
}

// TargetInfo identifies the workload to scale.
type TargetInfo struct {
	APIVersion string
	Kind       string
	Name       string
	Namespace  string
}

// CurrentState captures the observed workload configuration.
type CurrentState struct {
	Replicas   int32
	Resources  corev1.ResourceRequirements
	LastAction time.Time
}

// Coordinator applies horizontal and vertical scaling using Server-Side Apply.
type Coordinator interface {
	ApplyScaling(ctx context.Context, target TargetInfo, action ScaleAction) error
	GetCurrentState(ctx context.Context, target TargetInfo) (*CurrentState, error)
}

type coordinator struct {
	client            client.Client
	cooldownPeriod    time.Duration
	verticalExhausted bool
}

// NewCoordinator creates a scaling coordinator backed by the Kubernetes API client.
func NewCoordinator(c client.Client) Coordinator {
	return &coordinator{
		client:         c,
		cooldownPeriod: defaultCooldownPeriod,
	}
}

func (c *coordinator) ApplyScaling(ctx context.Context, target TargetInfo, action ScaleAction) error {
	current, err := c.GetCurrentState(ctx, target)
	if err != nil {
		return err
	}

	if !current.LastAction.IsZero() && time.Since(current.LastAction) < c.cooldownPeriod {
		return fmt.Errorf("%w: last action at %s", ErrCooldownActive, current.LastAction.Format(time.RFC3339))
	}

	combined := BuildCombinedAction(action, c.verticalExhausted)
	switch combined.Type {
	case scaleTypeVertical:
		if combined.Resources == nil {
			return ErrNoScalingChanges
		}
		return c.applyPatch(ctx, target, combined.Resources, nil, combined.Reason)
	case scaleTypeHorizontal:
		if combined.Replicas == nil {
			return ErrNoScalingChanges
		}
		return c.applyPatch(ctx, target, nil, combined.Replicas, combined.Reason)
	case scaleTypeBoth:
		if combined.Resources == nil && combined.Replicas == nil {
			return ErrNoScalingChanges
		}
		return c.applyPatch(ctx, target, combined.Resources, combined.Replicas, combined.Reason)
	default:
		return fmt.Errorf("unsupported scale action type %q", combined.Type)
	}
}

func (c *coordinator) GetCurrentState(ctx context.Context, target TargetInfo) (*CurrentState, error) {
	switch target.Kind {
	case kindDeployment:
		dep := &appsv1.Deployment{}
		if err := c.client.Get(ctx, types.NamespacedName{Name: target.Name, Namespace: target.Namespace}, dep); err != nil {
			return nil, err
		}
		return stateFromObject(dep.Spec.Replicas, dep.Spec.Template.Spec.Containers, dep.Annotations), nil
	case kindStatefulSet:
		sts := &appsv1.StatefulSet{}
		if err := c.client.Get(ctx, types.NamespacedName{Name: target.Name, Namespace: target.Namespace}, sts); err != nil {
			return nil, err
		}
		return stateFromObject(sts.Spec.Replicas, sts.Spec.Template.Spec.Containers, sts.Annotations), nil
	default:
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedTargetKind, target.Kind)
	}
}

// BuildCombinedAction merges vertical and horizontal decisions. When vertical limits are
// exhausted, horizontal scaling is preferred over further vertical changes.
func BuildCombinedAction(action ScaleAction, verticalExhausted bool) ScaleAction {
	if !verticalExhausted {
		return action
	}

	switch action.Type {
	case scaleTypeVertical:
		if action.Replicas != nil {
			return ScaleAction{
				Type:     scaleTypeHorizontal,
				Replicas: action.Replicas,
				Reason:   appendReason(action.Reason, "vertical limits reached; escalating to horizontal scaling"),
			}
		}
		return ScaleAction{
			Type:   scaleTypeHorizontal,
			Reason: appendReason(action.Reason, "vertical limits reached; horizontal scaling required"),
		}
	case scaleTypeBoth:
		if action.Replicas != nil {
			return ScaleAction{
				Type:      scaleTypeBoth,
				Resources: action.Resources,
				Replicas:  action.Replicas,
				Reason:    appendReason(action.Reason, "applying horizontal scaling alongside vertical changes"),
			}
		}
		return ScaleAction{
			Type:   scaleTypeHorizontal,
			Reason: appendReason(action.Reason, "vertical limits reached; applying horizontal scaling"),
		}
	default:
		return action
	}
}

func (c *coordinator) applyPatch(
	ctx context.Context,
	target TargetInfo,
	resources *corev1.ResourceRequirements,
	replicas *int32,
	reason string,
) error {
	containerName, err := c.primaryContainerName(ctx, target)
	if err != nil {
		return err
	}

	switch target.Kind {
	case kindDeployment:
		patch := &appsv1.Deployment{
			TypeMeta: metav1.TypeMeta{
				APIVersion: appsv1.SchemeGroupVersion.String(),
				Kind:       "Deployment",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      target.Name,
				Namespace: target.Namespace,
				Annotations: map[string]string{
					kairosv1alpha1.AnnotationLastAction:     reason,
					kairosv1alpha1.AnnotationLastActionTime: time.Now().UTC().Format(time.RFC3339),
				},
			},
		}
		patch.Spec.Replicas = replicas
		if resources != nil {
			patch.Spec.Template.Spec.Containers = []corev1.Container{
				{
					Name:      containerName,
					Resources: *resources,
				},
			}
		}
		return c.client.Patch(
			ctx,
			patch,
			client.Apply,
			client.FieldOwner(kairosv1alpha1.FieldOwnerKairos),
			client.ForceOwnership,
		)
	case kindStatefulSet:
		patch := &appsv1.StatefulSet{
			TypeMeta: metav1.TypeMeta{
				APIVersion: appsv1.SchemeGroupVersion.String(),
				Kind:       "StatefulSet",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      target.Name,
				Namespace: target.Namespace,
				Annotations: map[string]string{
					kairosv1alpha1.AnnotationLastAction:     reason,
					kairosv1alpha1.AnnotationLastActionTime: time.Now().UTC().Format(time.RFC3339),
				},
			},
		}
		patch.Spec.Replicas = replicas
		if resources != nil {
			patch.Spec.Template.Spec.Containers = []corev1.Container{
				{
					Name:      containerName,
					Resources: *resources,
				},
			}
		}
		return c.client.Patch(
			ctx,
			patch,
			client.Apply,
			client.FieldOwner(kairosv1alpha1.FieldOwnerKairos),
			client.ForceOwnership,
		)
	default:
		return fmt.Errorf("%w: %s", ErrUnsupportedTargetKind, target.Kind)
	}
}

func (c *coordinator) primaryContainerName(ctx context.Context, target TargetInfo) (string, error) {
	switch target.Kind {
	case kindDeployment:
		dep := &appsv1.Deployment{}
		if err := c.client.Get(ctx, types.NamespacedName{Name: target.Name, Namespace: target.Namespace}, dep); err != nil {
			return "", err
		}
		if len(dep.Spec.Template.Spec.Containers) == 0 {
			return "", fmt.Errorf("deployment %s/%s has no containers", target.Namespace, target.Name)
		}
		return dep.Spec.Template.Spec.Containers[0].Name, nil
	case kindStatefulSet:
		sts := &appsv1.StatefulSet{}
		if err := c.client.Get(ctx, types.NamespacedName{Name: target.Name, Namespace: target.Namespace}, sts); err != nil {
			return "", err
		}
		if len(sts.Spec.Template.Spec.Containers) == 0 {
			return "", fmt.Errorf("statefulset %s/%s has no containers", target.Namespace, target.Name)
		}
		return sts.Spec.Template.Spec.Containers[0].Name, nil
	default:
		return "", fmt.Errorf("%w: %s", ErrUnsupportedTargetKind, target.Kind)
	}
}

func stateFromObject(replicas *int32, containers []corev1.Container, annotations map[string]string) *CurrentState {
	state := &CurrentState{}
	if replicas != nil {
		state.Replicas = *replicas
	}

	if len(containers) > 0 {
		state.Resources = containers[0].Resources
	}

	if annotations != nil {
		if raw := annotations[kairosv1alpha1.AnnotationLastActionTime]; raw != "" {
			if parsed, err := time.Parse(time.RFC3339, raw); err == nil {
				state.LastAction = parsed
			}
		}
	}

	return state
}

func appendReason(base, extra string) string {
	if base == "" {
		return extra
	}
	return base + "; " + extra
}

// SetVerticalExhausted configures whether vertical scaling limits have been reached.
// When true, ApplyScaling escalates to horizontal scaling when vertical changes alone
// are insufficient.
func SetVerticalExhausted(c Coordinator, exhausted bool) {
	if impl, ok := c.(*coordinator); ok {
		impl.verticalExhausted = exhausted
	}
}

// SetCooldownPeriod overrides the default cooldown between scaling actions.
func SetCooldownPeriod(c Coordinator, period time.Duration) {
	if impl, ok := c.(*coordinator); ok && period > 0 {
		impl.cooldownPeriod = period
	}
}

// IsNotFound reports whether an error indicates the target resource does not exist.
func IsNotFound(err error) bool {
	return apierrors.IsNotFound(err)
}
