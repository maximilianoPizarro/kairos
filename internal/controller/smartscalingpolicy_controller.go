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
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	kairosv1alpha1 "github.com/maximilianoPizarro/kairos/api/v1alpha1"
)

const (
	evaluationInterval   = 30 * time.Second
	errorRequeueInterval = 1 * time.Minute
	maxRecentEvents      = 10
)

// SmartScalingPolicyReconciler reconciles a SmartScalingPolicy object.
type SmartScalingPolicyReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=kairos.maximilianopizarro.github.io,resources=smartscalingpolicies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=kairos.maximilianopizarro.github.io,resources=smartscalingpolicies/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=kairos.maximilianopizarro.github.io,resources=smartscalingpolicies/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;patch;update
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;patch;update
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch

// Reconcile evaluates scaling rules and applies changes to managed workloads.
func (r *SmartScalingPolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx).WithValues("smartscalingpolicy", req.NamespacedName)

	policy := &kairosv1alpha1.SmartScalingPolicy{}
	if err := r.Get(ctx, req.NamespacedName, policy); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		log.Error(err, "failed to fetch SmartScalingPolicy")
		return ctrl.Result{RequeueAfter: errorRequeueInterval}, err
	}

	now := metav1.Now()
	policy.Status.LastEvaluationTime = &now

	if policy.Spec.Paused {
		log.Info("policy is paused, skipping evaluation")
		setCondition(&policy.Status.Conditions, string(kairosv1alpha1.ConditionReady), metav1.ConditionTrue, "Paused", "Scaling actions are paused")
		setCondition(&policy.Status.Conditions, string(kairosv1alpha1.ConditionScaling), metav1.ConditionFalse, "Paused", "Scaling actions are paused")
		if err := r.updateStatus(ctx, policy); err != nil {
			log.Error(err, "failed to update status for paused policy")
			return ctrl.Result{RequeueAfter: errorRequeueInterval}, err
		}
		return ctrl.Result{RequeueAfter: evaluationInterval}, nil
	}

	targetNS := policy.Namespace
	if policy.Spec.Target.Namespace != "" {
		targetNS = policy.Spec.Target.Namespace
	}
	targetName := types.NamespacedName{
		Namespace: targetNS,
		Name:      policy.Spec.Target.Name,
	}

	workload, managed, err := r.loadManagedWorkload(ctx, policy, targetName)
	if err != nil {
		log.Error(err, "failed to load target workload", "target", targetName)
		setCondition(&policy.Status.Conditions, string(kairosv1alpha1.ConditionReady), metav1.ConditionFalse, "TargetError", err.Error())
		setCondition(&policy.Status.Conditions, string(kairosv1alpha1.ConditionDegraded), metav1.ConditionTrue, "TargetError", err.Error())
		if statusErr := r.updateStatus(ctx, policy); statusErr != nil {
			log.Error(statusErr, "failed to update status after target error")
		}
		return ctrl.Result{RequeueAfter: errorRequeueInterval}, err
	}

	if !managed {
		log.Info("target workload is not managed by Kairos, skipping scaling actions",
			"target", targetName,
			"annotation", kairosv1alpha1.AnnotationManaged,
		)
		setCondition(&policy.Status.Conditions, string(kairosv1alpha1.ConditionReady), metav1.ConditionFalse, "NotManaged",
			fmt.Sprintf("Target missing annotation %s=true", kairosv1alpha1.AnnotationManaged))
		setCondition(&policy.Status.Conditions, string(kairosv1alpha1.ConditionDegraded), metav1.ConditionTrue, "NotManaged",
			fmt.Sprintf("Target missing annotation %s=true", kairosv1alpha1.AnnotationManaged))
		setCondition(&policy.Status.Conditions, string(kairosv1alpha1.ConditionScaling), metav1.ConditionFalse, "NotManaged", "Workload is not managed")
		if err := r.updateStatus(ctx, policy); err != nil {
			log.Error(err, "failed to update status for unmanaged target")
			return ctrl.Result{RequeueAfter: errorRequeueInterval}, err
		}
		return ctrl.Result{RequeueAfter: evaluationInterval}, nil
	}

	r.syncObservedState(policy, workload)

	activeRules, actions, evalErrs := r.evaluateRules(ctx, policy, now.Time)
	policy.Status.ActiveRules = activeRules

	if len(evalErrs) > 0 {
		msg := evalErrs[0].Error()
		if len(evalErrs) > 1 {
			msg = fmt.Sprintf("%d evaluation errors; first: %s", len(evalErrs), msg)
		}
		log.Error(evalErrs[0], "rule evaluation failed", "errorCount", len(evalErrs))
		setCondition(&policy.Status.Conditions, string(kairosv1alpha1.ConditionDegraded), metav1.ConditionTrue, "EvaluationError", msg)
	}

	scaling := false
	for _, actionPlan := range actions {
		if err := r.applyScalingAction(ctx, policy, workload, actionPlan); err != nil {
			log.Error(err, "failed to apply scaling action", "rule", actionPlan.ruleName, "action", actionPlan.action.Type)
			event := kairosv1alpha1.ScalingEvent{
				Timestamp: metav1.Now(),
				Rule:      actionPlan.ruleName,
				Action:    actionPlan.action.Type,
				Detail:    err.Error(),
				Success:   false,
			}
			recordScalingEvent(policy, event)
			setCondition(&policy.Status.Conditions, string(kairosv1alpha1.ConditionScaling), metav1.ConditionFalse, "ApplyFailed", err.Error())
			setCondition(&policy.Status.Conditions, string(kairosv1alpha1.ConditionDegraded), metav1.ConditionTrue, "ApplyFailed", err.Error())
			if statusErr := r.updateStatus(ctx, policy); statusErr != nil {
				log.Error(statusErr, "failed to update status after apply error")
			}
			return ctrl.Result{RequeueAfter: errorRequeueInterval}, err
		}

		scaling = true
		detail := describeAction(actionPlan.action)
		event := kairosv1alpha1.ScalingEvent{
			Timestamp: metav1.Now(),
			Rule:      actionPlan.ruleName,
			Action:    actionPlan.action.Type,
			Detail:    detail,
			Success:   true,
		}
		recordScalingEvent(policy, event)
		log.Info("applied scaling action",
			"rule", actionPlan.ruleName,
			"action", actionPlan.action.Type,
			"detail", detail,
			"target", targetName,
		)
	}

	if scaling {
		setCondition(&policy.Status.Conditions, string(kairosv1alpha1.ConditionScaling), metav1.ConditionTrue, "Scaled", "Scaling action applied")
	} else {
		setCondition(&policy.Status.Conditions, string(kairosv1alpha1.ConditionScaling), metav1.ConditionFalse, "Idle", "No scaling actions required")
	}

	if len(evalErrs) == 0 {
		setCondition(&policy.Status.Conditions, string(kairosv1alpha1.ConditionDegraded), metav1.ConditionFalse, "Healthy", "Policy evaluation succeeded")
		setCondition(&policy.Status.Conditions, string(kairosv1alpha1.ConditionReady), metav1.ConditionTrue, "Reconciled", "Policy evaluated successfully")
	} else {
		setCondition(&policy.Status.Conditions, string(kairosv1alpha1.ConditionReady), metav1.ConditionFalse, "EvaluationError", "One or more rules failed evaluation")
	}

	if err := r.updateStatus(ctx, policy); err != nil {
		log.Error(err, "failed to update status")
		return ctrl.Result{RequeueAfter: errorRequeueInterval}, err
	}

	return ctrl.Result{RequeueAfter: evaluationInterval}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *SmartScalingPolicyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&kairosv1alpha1.SmartScalingPolicy{}).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: 2,
			RateLimiter: workqueue.NewTypedMaxOfRateLimiter(
				workqueue.NewTypedItemExponentialFailureRateLimiter[reconcile.Request](5*time.Second, 5*time.Minute),
			),
		}).
		Named("smartscalingpolicy").
		Complete(r)
}

type workloadTarget struct {
	kind       string
	deployment *appsv1.Deployment
	statefulSet *appsv1.StatefulSet
}

type plannedAction struct {
	ruleName string
	action   kairosv1alpha1.ScalingAction
}

func (r *SmartScalingPolicyReconciler) loadManagedWorkload(
	ctx context.Context,
	policy *kairosv1alpha1.SmartScalingPolicy,
	targetName types.NamespacedName,
) (*workloadTarget, bool, error) {
	kind := policy.Spec.Target.Kind
	if kind == "" {
		kind = "Deployment"
	}

	switch kind {
	case "Deployment":
		dep := &appsv1.Deployment{}
		if err := r.Get(ctx, targetName, dep); err != nil {
			return nil, false, fmt.Errorf("getting Deployment %s: %w", targetName, err)
		}
		return &workloadTarget{kind: "Deployment", deployment: dep}, isManagedResource(dep.Annotations), nil
	case "StatefulSet":
		sts := &appsv1.StatefulSet{}
		if err := r.Get(ctx, targetName, sts); err != nil {
			return nil, false, fmt.Errorf("getting StatefulSet %s: %w", targetName, err)
		}
		return &workloadTarget{kind: "StatefulSet", statefulSet: sts}, isManagedResource(sts.Annotations), nil
	default:
		return nil, false, fmt.Errorf("unsupported target kind %q", kind)
	}
}

func isManagedResource(annotations map[string]string) bool {
	if annotations == nil {
		return false
	}
	return annotations[kairosv1alpha1.AnnotationManaged] == "true"
}

func (r *SmartScalingPolicyReconciler) syncObservedState(policy *kairosv1alpha1.SmartScalingPolicy, workload *workloadTarget) {
	replicas := workload.replicaCount()
	policy.Status.CurrentReplicas = &replicas

	containers := workload.containers()
	if len(containers) > 0 {
		res := containers[0].Resources
		policy.Status.CurrentResources = &corev1.ResourceRequirements{
			Requests: res.Requests.DeepCopy(),
			Limits:   res.Limits.DeepCopy(),
		}
	}
}

func (w *workloadTarget) replicaCount() int32 {
	switch w.kind {
	case "Deployment":
		if w.deployment.Spec.Replicas != nil {
			return *w.deployment.Spec.Replicas
		}
		return 1
	case "StatefulSet":
		if w.statefulSet.Spec.Replicas != nil {
			return *w.statefulSet.Spec.Replicas
		}
		return 1
	default:
		return 1
	}
}

func (w *workloadTarget) containers() []corev1.Container {
	switch w.kind {
	case "Deployment":
		return w.deployment.Spec.Template.Spec.Containers
	case "StatefulSet":
		return w.statefulSet.Spec.Template.Spec.Containers
	default:
		return nil
	}
}

func (w *workloadTarget) namespacedName() types.NamespacedName {
	switch w.kind {
	case "Deployment":
		return types.NamespacedName{Namespace: w.deployment.Namespace, Name: w.deployment.Name}
	case "StatefulSet":
		return types.NamespacedName{Namespace: w.statefulSet.Namespace, Name: w.statefulSet.Name}
	default:
		return types.NamespacedName{}
	}
}

func (r *SmartScalingPolicyReconciler) evaluateRules(
	ctx context.Context,
	policy *kairosv1alpha1.SmartScalingPolicy,
	now time.Time,
) ([]string, []plannedAction, []error) {
	var activeRules []string
	var actions []plannedAction
	var evalErrs []error

	for _, rule := range policy.Spec.Rules {
		if !ruleEnabled(rule.Enabled) {
			continue
		}

		triggered, err := r.evaluateMetricRule(ctx, policy, rule)
		if err != nil {
			evalErrs = append(evalErrs, fmt.Errorf("rule %q: %w", rule.Name, err))
			continue
		}
		if !triggered {
			continue
		}

		activeRules = append(activeRules, rule.Name)
		if r.inCooldown(policy, rule.Name, rule.Action) {
			continue
		}
		actions = append(actions, plannedAction{ruleName: rule.Name, action: rule.Action})
	}

	for _, schedule := range policy.Spec.Schedule {
		if !ruleEnabled(schedule.Enabled) {
			continue
		}

		triggered, err := evaluateScheduleRule(schedule.Cron, now)
		if err != nil {
			evalErrs = append(evalErrs, fmt.Errorf("schedule %q: %w", schedule.Name, err))
			continue
		}
		if !triggered {
			continue
		}

		activeRules = append(activeRules, schedule.Name)
		if r.inCooldown(policy, schedule.Name, schedule.Action) {
			continue
		}
		actions = append(actions, plannedAction{ruleName: schedule.Name, action: schedule.Action})
	}

	return activeRules, actions, evalErrs
}

func ruleEnabled(enabled *bool) bool {
	return enabled == nil || *enabled
}

// stubMetricEvaluator returns deterministic dummy metric values for development.
type stubMetricEvaluator struct{}

func (stubMetricEvaluator) Evaluate(_ context.Context, metric string) (float64, error) {
	if metric == "" {
		return 0, fmt.Errorf("metric name is required")
	}
	// Stub value chosen to exercise GreaterThan/LessThan rules during development.
	return 150, nil
}

func (r *SmartScalingPolicyReconciler) evaluateMetricRule(
	ctx context.Context,
	_ *kairosv1alpha1.SmartScalingPolicy,
	rule kairosv1alpha1.ScalingRule,
) (bool, error) {
	evaluator := stubMetricEvaluator{}
	value, err := evaluator.Evaluate(ctx, rule.When.Metric)
	if err != nil {
		return false, err
	}
	return compareMetric(value, rule.When.Operator, rule.When.Threshold)
}

func compareMetric(value float64, operator kairosv1alpha1.ComparisonOperator, threshold string) (bool, error) {
	limit, err := parseThreshold(threshold)
	if err != nil {
		return false, err
	}

	switch operator {
	case kairosv1alpha1.OperatorGreaterThan:
		return value > limit, nil
	case kairosv1alpha1.OperatorLessThan:
		return value < limit, nil
	case kairosv1alpha1.OperatorEqual:
		return math.Abs(value-limit) < 0.0001, nil
	default:
		return false, fmt.Errorf("unsupported operator %q", operator)
	}
}

func parseThreshold(threshold string) (float64, error) {
	trimmed := strings.TrimSpace(threshold)
	if trimmed == "" {
		return 0, fmt.Errorf("threshold is empty")
	}

	lower := strings.ToLower(trimmed)
	switch {
	case strings.HasSuffix(lower, "ms"):
		d, err := time.ParseDuration(lower)
		if err != nil {
			return 0, err
		}
		return float64(d.Milliseconds()), nil
	case strings.HasSuffix(lower, "s"):
		d, err := time.ParseDuration(lower)
		if err != nil {
			return 0, err
		}
		return d.Seconds(), nil
	case strings.HasSuffix(trimmed, "%"):
		return strconv.ParseFloat(strings.TrimSuffix(trimmed, "%"), 64)
	default:
		return strconv.ParseFloat(trimmed, 64)
	}
}

func evaluateScheduleRule(cronExpr string, now time.Time) (bool, error) {
	schedule, err := cron.ParseStandard(cronExpr)
	if err != nil {
		return false, fmt.Errorf("invalid cron expression %q: %w", cronExpr, err)
	}

	windowStart := now.Add(-evaluationInterval)
	next := schedule.Next(windowStart)
	return !next.After(now), nil
}

func (r *SmartScalingPolicyReconciler) inCooldown(
	policy *kairosv1alpha1.SmartScalingPolicy,
	ruleName string,
	action kairosv1alpha1.ScalingAction,
) bool {
	if action.Cooldown == "" || policy.Status.LastScalingEvent == nil {
		return false
	}

	cooldown, err := time.ParseDuration(action.Cooldown)
	if err != nil {
		return false
	}

	last := policy.Status.LastScalingEvent
	if last.Rule != ruleName || last.Action != action.Type {
		return false
	}

	return time.Since(last.Timestamp.Time) < cooldown
}

func (r *SmartScalingPolicyReconciler) applyScalingAction(
	ctx context.Context,
	_ *kairosv1alpha1.SmartScalingPolicy,
	workload *workloadTarget,
	plan plannedAction,
) error {
	switch plan.action.Type {
	case kairosv1alpha1.ActionAddReplicas, kairosv1alpha1.ActionRemoveReplicas,
		kairosv1alpha1.ActionSetMinReplicas:
		replicas, err := computeReplicaAction(workload.replicaCount(), plan.action)
		if err != nil {
			return err
		}
		return r.applyReplicaCount(ctx, workload, replicas)
	case kairosv1alpha1.ActionIncreaseResources, kairosv1alpha1.ActionDecreaseResources,
		kairosv1alpha1.ActionSetResources:
		containers, err := computeResourceAction(workload.containers(), plan.action)
		if err != nil {
			return err
		}
		return r.applyContainerResources(ctx, workload, containers)
	default:
		return fmt.Errorf("unsupported action type %q", plan.action.Type)
	}
}

func computeReplicaAction(current int32, action kairosv1alpha1.ScalingAction) (int32, error) {
	delta := int32(1)
	if action.Replicas != nil {
		delta = *action.Replicas
	}

	switch action.Type {
	case kairosv1alpha1.ActionAddReplicas:
		next := current + delta
		if action.MaxReplicas != nil && next > *action.MaxReplicas {
			next = *action.MaxReplicas
		}
		return next, nil
	case kairosv1alpha1.ActionRemoveReplicas:
		next := current - delta
		if next < 1 {
			next = 1
		}
		if action.MinReplicas != nil && next < *action.MinReplicas {
			next = *action.MinReplicas
		}
		return next, nil
	case kairosv1alpha1.ActionSetMinReplicas:
		if action.MinReplicas == nil {
			return 0, fmt.Errorf("SetMinReplicas requires minReplicas")
		}
		next := *action.MinReplicas
		if action.MaxReplicas != nil && next > *action.MaxReplicas {
			next = *action.MaxReplicas
		}
		if next < 1 {
			next = 1
		}
		return next, nil
	default:
		return 0, fmt.Errorf("not a replica action: %q", action.Type)
	}
}

func computeResourceAction(containers []corev1.Container, action kairosv1alpha1.ScalingAction) ([]corev1.Container, error) {
	if len(containers) == 0 {
		return nil, fmt.Errorf("target workload has no containers")
	}

	updated := make([]corev1.Container, len(containers))
	for i, container := range containers {
		updated[i] = *container.DeepCopy()

		switch action.Type {
		case kairosv1alpha1.ActionSetResources:
			if action.Resources == nil {
				return nil, fmt.Errorf("SetResources requires resources")
			}
			updated[i].Resources = *action.Resources.DeepCopy()
		case kairosv1alpha1.ActionIncreaseResources:
			updated[i].Resources = adjustResources(updated[i].Resources, action, true)
		case kairosv1alpha1.ActionDecreaseResources:
			updated[i].Resources = adjustResources(updated[i].Resources, action, false)
		default:
			return nil, fmt.Errorf("not a resource action: %q", action.Type)
		}
	}

	return updated, nil
}

func adjustResources(current corev1.ResourceRequirements, action kairosv1alpha1.ScalingAction, increase bool) corev1.ResourceRequirements {
	result := *current.DeepCopy()

	memPercent := int32(10)
	if action.IncreaseMemoryPercent != nil {
		memPercent = *action.IncreaseMemoryPercent
	}
	cpuPercent := int32(10)
	if action.IncreaseCPUPercent != nil {
		cpuPercent = *action.IncreaseCPUPercent
	}
	if !increase {
		memPercent = -memPercent
		cpuPercent = -cpuPercent
	}

	if qty, ok := result.Requests[corev1.ResourceMemory]; ok {
		result.Requests[corev1.ResourceMemory] = scaleQuantity(qty, memPercent, action.MinMemory, action.MaxMemory)
	}
	if qty, ok := result.Limits[corev1.ResourceMemory]; ok {
		result.Limits[corev1.ResourceMemory] = scaleQuantity(qty, memPercent, action.MinMemory, action.MaxMemory)
	}
	if qty, ok := result.Requests[corev1.ResourceCPU]; ok {
		result.Requests[corev1.ResourceCPU] = scaleQuantity(qty, cpuPercent, action.MinCPU, action.MaxCPU)
	}
	if qty, ok := result.Limits[corev1.ResourceCPU]; ok {
		result.Limits[corev1.ResourceCPU] = scaleQuantity(qty, cpuPercent, action.MinCPU, action.MaxCPU)
	}

	return result
}

func scaleQuantity(current resource.Quantity, percent int32, minBound, maxBound *resource.Quantity) resource.Quantity {
	milliValue := current.MilliValue()
	scaled := milliValue + (milliValue * int64(percent) / 100)
	result := *resource.NewMilliQuantity(scaled, current.Format)

	if minBound != nil && result.Cmp(*minBound) < 0 {
		result = *minBound
	}
	if maxBound != nil && result.Cmp(*maxBound) > 0 {
		result = *maxBound
	}
	return result
}

func (r *SmartScalingPolicyReconciler) applyReplicaCount(ctx context.Context, workload *workloadTarget, replicas int32) error {
	name := workload.namespacedName().Name
	namespace := workload.namespacedName().Namespace

	switch workload.kind {
	case "Deployment":
		dep := &appsv1.Deployment{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Spec: appsv1.DeploymentSpec{
				Replicas: &replicas,
			},
		}
		return r.Patch(ctx, dep, client.Apply,
			client.FieldOwner(kairosv1alpha1.FieldOwnerKairos),
			client.ForceOwnership,
		)
	case "StatefulSet":
		sts := &appsv1.StatefulSet{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "apps/v1",
				Kind:       "StatefulSet",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Spec: appsv1.StatefulSetSpec{
				Replicas: &replicas,
			},
		}
		return r.Patch(ctx, sts, client.Apply,
			client.FieldOwner(kairosv1alpha1.FieldOwnerKairos),
			client.ForceOwnership,
		)
	default:
		return fmt.Errorf("unsupported workload kind %q", workload.kind)
	}
}

func (r *SmartScalingPolicyReconciler) applyContainerResources(
	ctx context.Context,
	workload *workloadTarget,
	containers []corev1.Container,
) error {
	name := workload.namespacedName().Name
	namespace := workload.namespacedName().Namespace

	containerSpecs := make([]corev1.Container, len(containers))
	for i, container := range containers {
		containerSpecs[i] = corev1.Container{
			Name:      container.Name,
			Resources: container.Resources,
		}
	}

	podTemplate := corev1.PodTemplateSpec{
		Spec: corev1.PodSpec{
			Containers: containerSpecs,
		},
	}

	switch workload.kind {
	case "Deployment":
		dep := &appsv1.Deployment{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Spec: appsv1.DeploymentSpec{
				Template: podTemplate,
			},
		}
		return r.Patch(ctx, dep, client.Apply,
			client.FieldOwner(kairosv1alpha1.FieldOwnerKairos),
			client.ForceOwnership,
		)
	case "StatefulSet":
		sts := &appsv1.StatefulSet{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "apps/v1",
				Kind:       "StatefulSet",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Spec: appsv1.StatefulSetSpec{
				Template: podTemplate,
			},
		}
		return r.Patch(ctx, sts, client.Apply,
			client.FieldOwner(kairosv1alpha1.FieldOwnerKairos),
			client.ForceOwnership,
		)
	default:
		return fmt.Errorf("unsupported workload kind %q", workload.kind)
	}
}

func recordScalingEvent(policy *kairosv1alpha1.SmartScalingPolicy, event kairosv1alpha1.ScalingEvent) {
	policy.Status.LastScalingEvent = &event
	policy.Status.RecentEvents = append([]kairosv1alpha1.ScalingEvent{event}, policy.Status.RecentEvents...)
	if len(policy.Status.RecentEvents) > maxRecentEvents {
		policy.Status.RecentEvents = policy.Status.RecentEvents[:maxRecentEvents]
	}
}

func describeAction(action kairosv1alpha1.ScalingAction) string {
	switch action.Type {
	case kairosv1alpha1.ActionAddReplicas, kairosv1alpha1.ActionRemoveReplicas, kairosv1alpha1.ActionSetMinReplicas:
		if action.Replicas != nil {
			return fmt.Sprintf("replicas delta=%d", *action.Replicas)
		}
		if action.MinReplicas != nil {
			return fmt.Sprintf("minReplicas=%d", *action.MinReplicas)
		}
		return "replica scaling"
	case kairosv1alpha1.ActionIncreaseResources, kairosv1alpha1.ActionDecreaseResources:
		return "resource scaling"
	case kairosv1alpha1.ActionSetResources:
		return "resources set"
	default:
		return string(action.Type)
	}
}

func setCondition(conditions *[]metav1.Condition, conditionType string, status metav1.ConditionStatus, reason, message string) {
	apimeta.SetStatusCondition(conditions, metav1.Condition{
		Type:               conditionType,
		Status:             status,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: metav1.Now(),
	})
}

func (r *SmartScalingPolicyReconciler) updateStatus(ctx context.Context, policy *kairosv1alpha1.SmartScalingPolicy) error {
	return r.Status().Update(ctx, policy)
}
