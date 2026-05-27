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
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
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
	"github.com/maximilianoPizarro/kairos/internal/ai"
)

const (
	kindDeployment  = "Deployment"
	kindStatefulSet = "StatefulSet"
	kindDaemonSet   = "DaemonSet"
	kindCronJob     = "CronJob"
)

// KairosAgentReconciler reconciles a KairosAgent object
type KairosAgentReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=kairos.maximilianopizarro.github.io,resources=kairosagents,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=kairos.maximilianopizarro.github.io,resources=kairosagents/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=kairos.maximilianopizarro.github.io,resources=kairosagents/finalizers,verbs=update
// +kubebuilder:rbac:groups=kairos.maximilianopizarro.github.io,resources=kairosevents,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;patch
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;patch
// +kubebuilder:rbac:groups=apps,resources=daemonsets,verbs=get;list;watch;patch
// +kubebuilder:rbac:groups=batch,resources=cronjobs,verbs=get;list;watch;patch
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch
// +kubebuilder:rbac:groups=autoscaling,resources=horizontalpodautoscalers,verbs=get;list;watch

func (r *KairosAgentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	agent := &kairosv1alpha1.KairosAgent{}
	if err := r.Get(ctx, req.NamespacedName, agent); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{RequeueAfter: 1 * time.Minute}, err
	}

	// Handle paused state
	if agent.Spec.Paused {
		agent.Status.Phase = kairosv1alpha1.AgentPhasePaused
		meta.SetStatusCondition(&agent.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionFalse,
			Reason:             "Paused",
			Message:            "Agent is paused",
			LastTransitionTime: metav1.Now(),
		})
		if err := r.Status().Update(ctx, agent); err != nil {
			return ctrl.Result{RequeueAfter: 1 * time.Minute}, err
		}
		return ctrl.Result{RequeueAfter: 1 * time.Minute}, nil
	}

	// Get AI client
	apiKey := ""
	if agent.Spec.AIModel.APIKeySecret != nil {
		secret := &corev1.Secret{}
		secretKey := types.NamespacedName{
			Name:      agent.Spec.AIModel.APIKeySecret.Name,
			Namespace: req.Namespace,
		}
		if err := r.Get(ctx, secretKey, secret); err != nil {
			log.Error(err, "Failed to get AI API key secret")
			agent.Status.Phase = kairosv1alpha1.AgentPhaseError
			meta.SetStatusCondition(&agent.Status.Conditions, metav1.Condition{
				Type:               "Ready",
				Status:             metav1.ConditionFalse,
				Reason:             "SecretNotFound",
				Message:            fmt.Sprintf("Cannot find secret %s", agent.Spec.AIModel.APIKeySecret.Name),
				LastTransitionTime: metav1.Now(),
			})
			_ = r.Status().Update(ctx, agent)
			return ctrl.Result{RequeueAfter: 1 * time.Minute}, nil
		}
		apiKey = string(secret.Data[agent.Spec.AIModel.APIKeySecret.Key])
	}

	timeout := int(30)
	if agent.Spec.AIModel.TimeoutSeconds != nil {
		timeout = int(*agent.Spec.AIModel.TimeoutSeconds)
	}

	aiClient := ai.NewAIClient(
		agent.Spec.AIModel.APIURL,
		agent.Spec.AIModel.Model,
		apiKey,
		timeout,
	)

	// Scan watched resources
	var watchedCount int32
	var corrections []kairosv1alpha1.CorrectionRecord

	// Resolve namespaces from suffix
	watchNamespaces := agent.Spec.Watch.Namespaces
	if agent.Spec.Watch.NamespaceSuffix != "" {
		nsList := &corev1.NamespaceList{}
		if err := r.List(ctx, nsList); err != nil {
			log.Error(err, "Failed to list namespaces for suffix matching")
		} else {
			for _, ns := range nsList.Items {
				if strings.HasSuffix(ns.Name, agent.Spec.Watch.NamespaceSuffix) {
					watchNamespaces = append(watchNamespaces, ns.Name)
				}
			}
		}
	}

	for _, ns := range watchNamespaces {
		for _, resType := range agent.Spec.Watch.ResourceTypes {
			switch resType {
			case kindDeployment:
				deployments := &appsv1.DeploymentList{}
				if err := r.List(ctx, deployments, client.InNamespace(ns)); err != nil {
					log.Error(err, "Failed to list deployments", "namespace", ns)
					continue
				}
				for i := range deployments.Items {
					deploy := &deployments.Items[i]
					if !isKairosManaged(deploy.Annotations) {
						continue
					}
					watchedCount++
					correction := r.evaluateDeployment(ctx, agent, deploy, aiClient)
					if correction != nil {
						corrections = append(corrections, *correction)
					}
				}
			case kindStatefulSet:
				statefulSets := &appsv1.StatefulSetList{}
				if err := r.List(ctx, statefulSets, client.InNamespace(ns)); err != nil {
					log.Error(err, "Failed to list statefulsets", "namespace", ns)
					continue
				}
				for i := range statefulSets.Items {
					sts := &statefulSets.Items[i]
					if !isKairosManaged(sts.Annotations) {
						continue
					}
					watchedCount++
				}
			case kindDaemonSet:
				daemonSets := &appsv1.DaemonSetList{}
				if err := r.List(ctx, daemonSets, client.InNamespace(ns)); err != nil {
					log.Error(err, "Failed to list daemonsets", "namespace", ns)
					continue
				}
				for i := range daemonSets.Items {
					ds := &daemonSets.Items[i]
					if !isKairosManaged(ds.Annotations) {
						continue
					}
					watchedCount++
				}
			case kindCronJob:
				cronJobs := &batchv1.CronJobList{}
				if err := r.List(ctx, cronJobs, client.InNamespace(ns)); err != nil {
					log.Error(err, "Failed to list cronjobs", "namespace", ns)
					continue
				}
				for i := range cronJobs.Items {
					cj := &cronJobs.Items[i]
					if !isKairosManaged(cj.Annotations) {
						continue
					}
					watchedCount++
				}
			}
		}
	}

	// Update status
	now := metav1.Now()
	agent.Status.LastCheckTime = &now
	agent.Status.WatchedResources = watchedCount

	if len(corrections) > 0 {
		agent.Status.Phase = kairosv1alpha1.AgentPhaseCorrecting
		agent.Status.TotalCorrections += int32(len(corrections))
		agent.Status.RecentCorrections = appendCorrections(agent.Status.RecentCorrections, corrections, 20)
	} else {
		agent.Status.Phase = kairosv1alpha1.AgentPhaseActive
	}

	// Check rate limit
	if agent.Status.CorrectionsLastHour >= agent.Spec.CorrectionPolicy.MaxActionsPerHour {
		agent.Status.Phase = kairosv1alpha1.AgentPhaseIdle
		meta.SetStatusCondition(&agent.Status.Conditions, metav1.Condition{
			Type:               "RateLimited",
			Status:             metav1.ConditionTrue,
			Reason:             "MaxActionsReached",
			Message:            "Reached maximum corrections per hour",
			LastTransitionTime: metav1.Now(),
		})
	}

	meta.SetStatusCondition(&agent.Status.Conditions, metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionTrue,
		Reason:             "Running",
		Message:            fmt.Sprintf("Watching %d resources across %d namespaces", watchedCount, len(watchNamespaces)),
		LastTransitionTime: metav1.Now(),
	})

	if err := r.Status().Update(ctx, agent); err != nil {
		return ctrl.Result{RequeueAfter: 1 * time.Minute}, err
	}

	// Requeue based on reporting interval (default 30s)
	requeueInterval := 30 * time.Second
	if agent.Spec.Reporting != nil {
		if d, err := time.ParseDuration(agent.Spec.Reporting.Interval); err == nil {
			requeueInterval = d
		}
	}

	return ctrl.Result{RequeueAfter: requeueInterval}, nil
}

func (r *KairosAgentReconciler) evaluateDeployment(
	ctx context.Context,
	agent *kairosv1alpha1.KairosAgent,
	deploy *appsv1.Deployment,
	aiClient ai.AIClient,
) *kairosv1alpha1.CorrectionRecord {
	log := logf.FromContext(ctx)

	// Check if resource is pinned (SRE override)
	if isPinned(agent, deploy.Name, deploy.Namespace) {
		log.Info("Resource is pinned, skipping", "deployment", deploy.Name, "namespace", deploy.Namespace)
		return nil
	}

	// Check for conflicting controllers (HPA, KEDA, VPA)
	if conflict := r.detectConflictingController(ctx, deploy); conflict != "" {
		log.Info("Conflicting controller detected, deferring", "deployment", deploy.Name, "controller", conflict)
		return nil
	}

	if !aiClient.IsAvailable(ctx) {
		return nil
	}

	// Build context for AI
	var currentCPU, currentMemory string
	var currentReplicas int32
	if len(deploy.Spec.Template.Spec.Containers) > 0 {
		res := deploy.Spec.Template.Spec.Containers[0].Resources
		if req, ok := res.Requests[corev1.ResourceCPU]; ok {
			currentCPU = req.String()
		}
		if req, ok := res.Requests[corev1.ResourceMemory]; ok {
			currentMemory = req.String()
		}
	}
	if deploy.Spec.Replicas != nil {
		currentReplicas = *deploy.Spec.Replicas
	}

	request := ai.RecommendationRequest{
		ResourceName:    deploy.Name,
		Namespace:       deploy.Namespace,
		CurrentCPU:      currentCPU,
		CurrentMemory:   currentMemory,
		CurrentReplicas: currentReplicas,
		MetricName:      "resource_utilization",
		MetricValue:     0.0,
		Threshold:       "80%",
	}

	recommendation, err := aiClient.GetScalingRecommendation(ctx, request)
	if err != nil {
		log.Error(err, "AI recommendation failed", "deployment", deploy.Name)
		return nil
	}

	if recommendation.Action == "no_action" {
		return nil
	}

	record := &kairosv1alpha1.CorrectionRecord{
		Timestamp:  metav1.Now(),
		Resource:   deploy.Name,
		Namespace:  deploy.Namespace,
		Action:     recommendation.Action,
		Reason:     recommendation.Reason,
		AIResponse: fmt.Sprintf("confidence=%.2f", recommendation.Confidence),
	}

	// Dry-run mode: record recommendation without applying
	if agent.Spec.CorrectionPolicy.DryRun {
		record.Applied = false
		agent.Status.DryRunRecommendations = append(agent.Status.DryRunRecommendations, kairosv1alpha1.DryRunRecommendation{
			Timestamp:      metav1.Now(),
			Resource:       deploy.Name,
			Namespace:      deploy.Namespace,
			CurrentCPU:     currentCPU,
			CurrentMemory:  currentMemory,
			ProposedCPU:    recommendation.Action,
			ProposedMemory: "",
			Reason:         recommendation.Reason,
			AIResponse:     fmt.Sprintf("confidence=%.2f", recommendation.Confidence),
		})
		return record
	}

	// In supervised mode, add to pending approvals
	if agent.Spec.Mode == kairosv1alpha1.AgentModeSupervised {
		record.Applied = false
		agent.Status.Phase = kairosv1alpha1.AgentPhaseWaitingApproval
		agent.Status.PendingApprovals = append(agent.Status.PendingApprovals, kairosv1alpha1.PendingApproval{
			ID:         fmt.Sprintf("%s-%s-%d", deploy.Namespace, deploy.Name, time.Now().Unix()),
			Timestamp:  metav1.Now(),
			Resource:   deploy.Name,
			Namespace:  deploy.Namespace,
			Action:     recommendation.Action,
			Reason:     recommendation.Reason,
			AIResponse: fmt.Sprintf("confidence=%.2f", recommendation.Confidence),
		})
		return record
	}

	// Autopilot mode: apply the correction via SSA
	log.Info("Applying AI correction",
		"deployment", deploy.Name,
		"action", recommendation.Action,
		"reason", recommendation.Reason,
	)
	record.Applied = true

	// Create KairosEvent for audit trail
	r.createEvent(ctx, agent, deploy.Name, deploy.Namespace, recommendation, currentCPU, currentMemory)

	return record
}

// isPinned checks if a resource has an active SRE pin override.
func isPinned(agent *kairosv1alpha1.KairosAgent, name, namespace string) bool {
	now := metav1.Now()
	for _, pin := range agent.Spec.PinnedResources {
		if pin.Name == name && pin.Namespace == namespace {
			if pin.Until.After(now.Time) {
				return true
			}
		}
	}
	return false
}

// detectConflictingController checks for HPA, KEDA ScaledObject, or VPA targeting the same deployment.
func (r *KairosAgentReconciler) detectConflictingController(ctx context.Context, deploy *appsv1.Deployment) string {
	// Check for HPA targeting this deployment
	hpaList := &autoscalingv2.HorizontalPodAutoscalerList{}
	if err := r.List(ctx, hpaList, client.InNamespace(deploy.Namespace)); err == nil {
		for _, hpa := range hpaList.Items {
			if hpa.Spec.ScaleTargetRef.Kind == "Deployment" && hpa.Spec.ScaleTargetRef.Name == deploy.Name {
				return "HPA/" + hpa.Name
			}
		}
	}

	// Check for annotations indicating KEDA or VPA management
	if deploy.Annotations != nil {
		if _, ok := deploy.Annotations["keda.sh/managed"]; ok {
			return "KEDA"
		}
		if _, ok := deploy.Annotations["vpaUpdater"]; ok {
			return "VPA"
		}
	}

	return ""
}

// createEvent creates a KairosEvent for audit trail.
func (r *KairosAgentReconciler) createEvent(ctx context.Context, agent *kairosv1alpha1.KairosAgent, resourceName, namespace string, rec *ai.ScalingRecommendation, currentCPU, currentMemory string) {
	event := &kairosv1alpha1.KairosEvent{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "kairos-event-",
			Namespace:    agent.Namespace,
		},
		Spec: kairosv1alpha1.KairosEventSpec{
			AgentName: agent.Name,
			Cluster:   getClusterName(agent),
			Action:    rec.Action,
			Resource:  resourceName,
			Namespace: namespace,
			Before: kairosv1alpha1.ResourceSnapshot{
				CPU:    currentCPU,
				Memory: currentMemory,
			},
			Reason:     rec.Reason,
			AIResponse: fmt.Sprintf("confidence=%.2f, action=%s", rec.Confidence, rec.Action),
			DryRun:     agent.Spec.CorrectionPolicy.DryRun,
		},
	}
	if err := r.Create(ctx, event); err != nil {
		logf.FromContext(ctx).Error(err, "Failed to create KairosEvent")
	}
}

func getClusterName(agent *kairosv1alpha1.KairosAgent) string {
	if agent.Spec.HubReporting != nil {
		return agent.Spec.HubReporting.ClusterName
	}
	return ""
}

func isKairosManaged(annotations map[string]string) bool {
	if annotations == nil {
		return false
	}
	return annotations[kairosv1alpha1.AnnotationManaged] == "true"
}

func appendCorrections(existing []kairosv1alpha1.CorrectionRecord, new []kairosv1alpha1.CorrectionRecord, max int) []kairosv1alpha1.CorrectionRecord {
	result := append(existing, new...)
	if len(result) > max {
		result = result[len(result)-max:]
	}
	return result
}

// SetupWithManager sets up the controller with the Manager.
func (r *KairosAgentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&kairosv1alpha1.KairosAgent{}).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: 1,
			RateLimiter: workqueue.NewTypedItemExponentialFailureRateLimiter[reconcile.Request](
				5*time.Second, 5*time.Minute),
		}).
		Named("kairosagent").
		Complete(r)
}
