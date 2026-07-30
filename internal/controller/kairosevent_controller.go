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
	"fmt"
	"strconv"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	kairosv1alpha1 "github.com/maximilianoPizarro/kairos/api/v1alpha1"
	"github.com/maximilianoPizarro/kairos/internal/scaler"
)

// KairosEventReconciler applies human-approved KairosEvents to workloads.
type KairosEventReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=kairos.maximilianopizarro.github.io,resources=kairosevents,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=apps,resources=deployments;statefulsets,verbs=get;list;watch;patch;update

func (r *KairosEventReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	event := &kairosv1alpha1.KairosEvent{}
	if err := r.Get(ctx, req.NamespacedName, event); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if event.Spec.Status != kairosv1alpha1.EventStatusApproved {
		return ctrl.Result{}, nil
	}
	if event.Spec.DryRun {
		event.Spec.Status = kairosv1alpha1.EventStatusDryRun
		if err := r.Update(ctx, event); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	target, action, err := scaleActionFromEvent(event)
	if err != nil {
		log.Error(err, "Cannot build scale action from approved event", "event", event.Name)
		event.Spec.Status = kairosv1alpha1.EventStatusFailed
		event.Spec.Reason = appendEventReason(event.Spec.Reason, err.Error())
		if updateErr := r.Update(ctx, event); updateErr != nil {
			return ctrl.Result{}, updateErr
		}
		return ctrl.Result{}, nil
	}

	coord := scaler.NewCoordinator(r.Client)
	scaler.SetCooldownPeriod(coord, 0)

	if err := coord.ApplyScaling(ctx, target, action); err != nil {
		log.Error(err, "Failed to apply approved KairosEvent", "event", event.Name)
		event.Spec.Status = kairosv1alpha1.EventStatusFailed
		event.Spec.Reason = appendEventReason(event.Spec.Reason, err.Error())
		if updateErr := r.Update(ctx, event); updateErr != nil {
			return ctrl.Result{RequeueAfter: 15 * time.Second}, updateErr
		}
		return ctrl.Result{}, nil
	}

	// Refresh after snapshot from live workload when possible.
	if state, err := coord.GetCurrentState(ctx, target); err == nil {
		event.Spec.After = snapshotFromState(state)
	}
	event.Spec.Status = kairosv1alpha1.EventStatusApplied
	if err := r.Update(ctx, event); err != nil {
		return ctrl.Result{RequeueAfter: 5 * time.Second}, err
	}

	log.Info("Applied approved KairosEvent",
		"event", event.Name,
		"resource", fmt.Sprintf("%s/%s", target.Namespace, target.Name),
		"action", event.Spec.Action,
	)
	return ctrl.Result{}, nil
}

func scaleActionFromEvent(event *kairosv1alpha1.KairosEvent) (scaler.TargetInfo, scaler.ScaleAction, error) {
	ns := event.Spec.Namespace
	if ns == "" {
		ns = event.Namespace
	}
	if event.Spec.Resource == "" || ns == "" {
		return scaler.TargetInfo{}, scaler.ScaleAction{}, fmt.Errorf("event missing resource/namespace")
	}

	target := scaler.TargetInfo{
		APIVersion: "apps/v1",
		Kind:       "Deployment",
		Name:       event.Spec.Resource,
		Namespace:  ns,
	}

	after := event.Spec.After
	actionName := strings.ToLower(event.Spec.Action)

	var replicas *int32
	if after.Replicas != "" {
		n, err := strconv.ParseInt(after.Replicas, 10, 32)
		if err != nil {
			return target, scaler.ScaleAction{}, fmt.Errorf("invalid after.replicas %q: %w", after.Replicas, err)
		}
		r := int32(n)
		replicas = &r
	}

	var resources *corev1.ResourceRequirements
	if after.CPU != "" || after.Memory != "" {
		resources = &corev1.ResourceRequirements{
			Requests: corev1.ResourceList{},
			Limits:   corev1.ResourceList{},
		}
		if after.CPU != "" {
			q, err := resource.ParseQuantity(after.CPU)
			if err != nil {
				return target, scaler.ScaleAction{}, fmt.Errorf("invalid after.cpu %q: %w", after.CPU, err)
			}
			resources.Requests[corev1.ResourceCPU] = q
			resources.Limits[corev1.ResourceCPU] = q
		}
		if after.Memory != "" {
			q, err := resource.ParseQuantity(after.Memory)
			if err != nil {
				return target, scaler.ScaleAction{}, fmt.Errorf("invalid after.memory %q: %w", after.Memory, err)
			}
			resources.Requests[corev1.ResourceMemory] = q
			resources.Limits[corev1.ResourceMemory] = q
		}
	}

	isScale := strings.Contains(actionName, "scale")
	isResource := strings.Contains(actionName, "resource")
	if isScale && replicas == nil {
		return target, scaler.ScaleAction{}, fmt.Errorf("scale action %q requires after.replicas", event.Spec.Action)
	}
	if isResource && resources == nil {
		return target, scaler.ScaleAction{}, fmt.Errorf("resource action %q requires after.cpu/memory", event.Spec.Action)
	}

	// Prefer action semantics over a full after snapshot. Including template
	// resources on a pure scale_* SSA Apply can clear selector/labels.
	switch {
	case isScale && !isResource:
		resources = nil
	case isResource && !isScale:
		replicas = nil
	}

	scaleType := ""
	switch {
	case replicas != nil && resources != nil:
		scaleType = "both"
	case replicas != nil:
		scaleType = "horizontal"
	case resources != nil:
		scaleType = "vertical"
	default:
		return target, scaler.ScaleAction{}, fmt.Errorf("no after.* changes to apply for action %q", event.Spec.Action)
	}

	return target, scaler.ScaleAction{
		Type:      scaleType,
		Resources: resources,
		Replicas:  replicas,
		Reason:    event.Spec.Reason,
	}, nil
}

func snapshotFromState(state *scaler.CurrentState) kairosv1alpha1.ResourceSnapshot {
	snap := kairosv1alpha1.ResourceSnapshot{
		Replicas: strconv.FormatInt(int64(state.Replicas), 10),
	}
	if req, ok := state.Resources.Requests[corev1.ResourceCPU]; ok {
		snap.CPU = req.String()
	}
	if req, ok := state.Resources.Requests[corev1.ResourceMemory]; ok {
		snap.Memory = req.String()
	}
	return snap
}

func appendEventReason(base, extra string) string {
	if base == "" {
		return extra
	}
	if strings.Contains(base, extra) {
		return base
	}
	return base + "; " + extra
}

// SetupWithManager sets up the controller with the Manager.
func (r *KairosEventReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&kairosv1alpha1.KairosEvent{}).
		WithEventFilter(predicate.NewPredicateFuncs(func(obj client.Object) bool {
			ev, ok := obj.(*kairosv1alpha1.KairosEvent)
			return ok && ev.Spec.Status == kairosv1alpha1.EventStatusApproved
		})).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: 2,
			RateLimiter: workqueue.NewTypedItemExponentialFailureRateLimiter[reconcile.Request](
				2*time.Second, 2*time.Minute),
		}).
		Named("kairosevent").
		Complete(r)
}
