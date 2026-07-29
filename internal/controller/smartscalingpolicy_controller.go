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
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
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
	saTokenPath          = "/var/run/secrets/kubernetes.io/serviceaccount/token"
	saCAPath             = "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt"
)

// ruleConditionSince tracks when each metric rule first became true (for `when.for`).
var ruleConditionSince sync.Map

// SmartScalingPolicyReconciler reconciles a SmartScalingPolicy object.
type SmartScalingPolicyReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=kairos.maximilianopizarro.github.io,resources=smartscalingpolicies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=kairos.maximilianopizarro.github.io,resources=smartscalingpolicies/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=kairos.maximilianopizarro.github.io,resources=smartscalingpolicies/finalizers,verbs=update
// +kubebuilder:rbac:groups=kairos.maximilianopizarro.github.io,resources=kairosevents,verbs=get;list;watch;create;update;patch;delete
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
		beforeSnapshot := captureSnapshot(workload)

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

		// Re-fetch workload to capture after state
		afterWorkload, _, _ := r.loadManagedWorkload(ctx, policy, targetName)
		var afterSnapshot kairosv1alpha1.ResourceSnapshot
		if afterWorkload != nil {
			afterSnapshot = captureSnapshot(afterWorkload)
		} else {
			afterSnapshot = beforeSnapshot
		}

		r.createKairosEvent(ctx, policy, beforeSnapshot, afterSnapshot, actionPlan)

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
	kind        string
	deployment  *appsv1.Deployment
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
		kind = kindDeployment
	}

	switch kind {
	case kindDeployment:
		dep := &appsv1.Deployment{}
		if err := r.Get(ctx, targetName, dep); err != nil {
			return nil, false, fmt.Errorf("getting Deployment %s: %w", targetName, err)
		}
		return &workloadTarget{kind: kindDeployment, deployment: dep}, isManagedResource(dep.Annotations), nil
	case kindStatefulSet:
		sts := &appsv1.StatefulSet{}
		if err := r.Get(ctx, targetName, sts); err != nil {
			return nil, false, fmt.Errorf("getting StatefulSet %s: %w", targetName, err)
		}
		return &workloadTarget{kind: kindStatefulSet, statefulSet: sts}, isManagedResource(sts.Annotations), nil
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
	case kindDeployment:
		if w.deployment.Spec.Replicas != nil {
			return *w.deployment.Spec.Replicas
		}
		return 1
	case kindStatefulSet:
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
	case kindDeployment:
		return w.deployment.Spec.Template.Spec.Containers
	case kindStatefulSet:
		return w.statefulSet.Spec.Template.Spec.Containers
	default:
		return nil
	}
}

func (r *SmartScalingPolicyReconciler) evaluateRules(
	ctx context.Context,
	policy *kairosv1alpha1.SmartScalingPolicy,
	now time.Time,
) ([]string, []plannedAction, []error) {
	activeRules := make([]string, 0, len(policy.Spec.Rules))
	actions := make([]plannedAction, 0, len(policy.Spec.Rules))
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
		if r.inCooldown(policy, rule.Action) {
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
		if r.inCooldown(policy, schedule.Action) {
			continue
		}
		actions = append(actions, plannedAction{ruleName: schedule.Name, action: schedule.Action})
	}

	return activeRules, actions, evalErrs
}

func ruleEnabled(enabled *bool) bool {
	return enabled == nil || *enabled
}

func normalizePrometheusEndpoint(endpoint string) string {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return ""
	}
	if strings.HasPrefix(endpoint, "http://") || strings.HasPrefix(endpoint, "https://") {
		return strings.TrimRight(endpoint, "/")
	}
	// OpenShift monitoring services require HTTPS + SA bearer token.
	if strings.Contains(endpoint, "openshift-monitoring") || strings.Contains(endpoint, "thanos-querier") {
		return "https://" + endpoint
	}
	return "http://" + endpoint
}

func prometheusHTTPClient(baseURL string) *http.Client {
	client := &http.Client{Timeout: 10 * time.Second}
	if !strings.HasPrefix(baseURL, "https://") {
		return client
	}

	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12}
	if caPEM, err := os.ReadFile(saCAPath); err == nil {
		pool := x509.NewCertPool()
		if pool.AppendCertsFromPEM(caPEM) {
			tlsConfig.RootCAs = pool
		} else {
			tlsConfig.InsecureSkipVerify = true
		}
	} else {
		// Outside a cluster (unit tests / local run) fall back to system CAs,
		// and allow insecure verify only when SA CA is unavailable in-cluster paths.
		tlsConfig.InsecureSkipVerify = true
	}
	client.Transport = &http.Transport{TLSClientConfig: tlsConfig}
	return client
}

// queryPrometheus performs a PromQL instant query against the configured endpoint.
// For HTTPS / OpenShift monitoring endpoints it attaches the in-cluster SA bearer token.
func queryPrometheus(ctx context.Context, endpoint, promQL string) (float64, error) {
	if endpoint == "" {
		return 0, fmt.Errorf("no prometheus endpoint configured")
	}

	base := normalizePrometheusEndpoint(endpoint)
	queryURL := base + "/api/v1/query?" + url.Values{"query": {promQL}}.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, queryURL, nil)
	if err != nil {
		return 0, fmt.Errorf("create request: %w", err)
	}

	if strings.HasPrefix(base, "https://") {
		if token, err := os.ReadFile(saTokenPath); err == nil {
			req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(string(token)))
		}
	}

	resp, err := prometheusHTTPClient(base).Do(req)
	if err != nil {
		return 0, fmt.Errorf("query prometheus: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("prometheus returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var result struct {
		Status string `json:"status"`
		Data   struct {
			ResultType string `json:"resultType"`
			Result     []struct {
				Value []json.RawMessage `json:"value"`
			} `json:"result"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return 0, fmt.Errorf("parse response: %w", err)
	}
	if result.Status != "success" || len(result.Data.Result) == 0 {
		return 0, fmt.Errorf("no data returned for query: %s", promQL)
	}
	if len(result.Data.Result[0].Value) < 2 {
		return 0, fmt.Errorf("unexpected value format in prometheus response")
	}
	var valueStr string
	if err := json.Unmarshal(result.Data.Result[0].Value[1], &valueStr); err != nil {
		return 0, fmt.Errorf("parse metric value: %w", err)
	}
	return strconv.ParseFloat(valueStr, 64)
}

func conditionKey(policy *kairosv1alpha1.SmartScalingPolicy, ruleName string) string {
	return policy.Namespace + "/" + policy.Name + "/" + ruleName
}

// conditionHeldFor reports whether a metric condition has stayed true for the configured duration.
func conditionHeldFor(key, forDuration string, now time.Time, matches bool) bool {
	if !matches {
		ruleConditionSince.Delete(key)
		return false
	}
	if forDuration == "" {
		return true
	}
	forDur, err := time.ParseDuration(forDuration)
	if err != nil || forDur <= 0 {
		return true
	}
	actual, loaded := ruleConditionSince.LoadOrStore(key, now)
	if !loaded {
		return false
	}
	since, ok := actual.(time.Time)
	if !ok {
		ruleConditionSince.Store(key, now)
		return false
	}
	return !now.Before(since.Add(forDur))
}

func (r *SmartScalingPolicyReconciler) evaluateMetricRule(
	ctx context.Context,
	policy *kairosv1alpha1.SmartScalingPolicy,
	rule kairosv1alpha1.ScalingRule,
) (bool, error) {
	log := logf.FromContext(ctx)
	key := conditionKey(policy, rule.Name)

	promEndpoint := policy.Spec.PrometheusEndpoint
	if promEndpoint == "" {
		log.V(1).Info("No prometheus endpoint configured, skipping metric rule", "rule", rule.Name)
		ruleConditionSince.Delete(key)
		return false, nil
	}

	value, err := queryPrometheus(ctx, promEndpoint, rule.When.Metric)
	if err != nil {
		log.Info("Prometheus query failed, skipping rule (safe default)", "rule", rule.Name, "error", err.Error())
		ruleConditionSince.Delete(key)
		return false, nil
	}

	matches, err := compareMetric(value, rule.When.Operator, rule.When.Threshold)
	if err != nil {
		return false, err
	}
	return conditionHeldFor(key, rule.When.For, time.Now(), matches), nil
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
	action kairosv1alpha1.ScalingAction,
) bool {
	if policy.Status.LastScalingEvent == nil {
		return false
	}

	cooldown := 60 * time.Second
	if action.Cooldown != "" {
		if parsed, err := time.ParseDuration(action.Cooldown); err == nil {
			cooldown = parsed
		}
	}

	last := policy.Status.LastScalingEvent
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
	switch workload.kind {
	case kindDeployment:
		dep := workload.deployment.DeepCopy()
		patch := client.MergeFrom(dep.DeepCopy())
		dep.Spec.Replicas = &replicas
		return r.Patch(ctx, dep, patch)
	case kindStatefulSet:
		sts := workload.statefulSet.DeepCopy()
		patch := client.MergeFrom(sts.DeepCopy())
		sts.Spec.Replicas = &replicas
		return r.Patch(ctx, sts, patch)
	default:
		return fmt.Errorf("unsupported workload kind %q", workload.kind)
	}
}

func (r *SmartScalingPolicyReconciler) applyContainerResources(
	ctx context.Context,
	workload *workloadTarget,
	containers []corev1.Container,
) error {
	switch workload.kind {
	case kindDeployment:
		dep := workload.deployment.DeepCopy()
		patch := client.MergeFrom(dep.DeepCopy())
		for i, updated := range containers {
			if i < len(dep.Spec.Template.Spec.Containers) {
				dep.Spec.Template.Spec.Containers[i].Resources = updated.Resources
			}
		}
		return r.Patch(ctx, dep, patch)
	case kindStatefulSet:
		sts := workload.statefulSet.DeepCopy()
		patch := client.MergeFrom(sts.DeepCopy())
		for i, updated := range containers {
			if i < len(sts.Spec.Template.Spec.Containers) {
				sts.Spec.Template.Spec.Containers[i].Resources = updated.Resources
			}
		}
		return r.Patch(ctx, sts, patch)
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

func captureSnapshot(workload *workloadTarget) kairosv1alpha1.ResourceSnapshot {
	snapshot := kairosv1alpha1.ResourceSnapshot{
		Replicas: fmt.Sprintf("%d", workload.replicaCount()),
	}
	containers := workload.containers()
	if len(containers) > 0 {
		res := containers[0].Resources
		if req, ok := res.Requests[corev1.ResourceCPU]; ok {
			snapshot.CPU = req.String()
		}
		if req, ok := res.Requests[corev1.ResourceMemory]; ok {
			snapshot.Memory = req.String()
		}
	}
	return snapshot
}

func (r *SmartScalingPolicyReconciler) createKairosEvent(ctx context.Context, policy *kairosv1alpha1.SmartScalingPolicy, before, after kairosv1alpha1.ResourceSnapshot, action plannedAction) {
	eventNS := policy.Spec.Target.Namespace
	if eventNS == "" {
		eventNS = policy.Namespace
	}
	event := &kairosv1alpha1.KairosEvent{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "kairos-policy-event-",
			Namespace:    policy.Namespace,
		},
		Spec: kairosv1alpha1.KairosEventSpec{
			PolicyName: policy.Name,
			AgentName:  "",
			Cluster:    "",
			Action:     string(action.action.Type),
			Resource:   policy.Spec.Target.Name,
			Namespace:  eventNS,
			Before:     before,
			After:      after,
			Reason:     fmt.Sprintf("Rule %q triggered", action.ruleName),
			Status:     "applied",
		},
	}
	if err := r.Create(ctx, event); err != nil {
		logf.FromContext(ctx).Error(err, "Failed to create KairosEvent for policy action")
	}
}
