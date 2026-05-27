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
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	kairosv1alpha1 "github.com/maximilianoPizarro/kairos/api/v1alpha1"
)

// KairosConsoleReconciler reconciles a KairosConsole object
type KairosConsoleReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=kairos.maximilianopizarro.github.io,resources=kairosconsoles,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=kairos.maximilianopizarro.github.io,resources=kairosconsoles/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=kairos.maximilianopizarro.github.io,resources=kairosconsoles/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=serviceaccounts,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=route.openshift.io,resources=routes,verbs=get;list;watch;create;update;patch;delete

func (r *KairosConsoleReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	console := &kairosv1alpha1.KairosConsole{}
	if err := r.Get(ctx, req.NamespacedName, console); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{RequeueAfter: 1 * time.Minute}, err
	}

	log.Info("Reconciling KairosConsole", "name", console.Name)

	// Determine image
	image := kairosv1alpha1.DefaultConsoleImage
	if console.Spec.Image != "" {
		image = console.Spec.Image
	}

	// Determine replicas
	var replicas int32 = 1
	if console.Spec.Replicas != nil {
		replicas = *console.Spec.Replicas
	}

	// Reconcile ServiceAccount
	if err := r.reconcileServiceAccount(ctx, console); err != nil {
		log.Error(err, "Failed to reconcile ServiceAccount")
		return ctrl.Result{RequeueAfter: 1 * time.Minute}, err
	}

	// Reconcile Deployment
	if err := r.reconcileDeployment(ctx, console, image, replicas); err != nil {
		log.Error(err, "Failed to reconcile Deployment")
		return ctrl.Result{RequeueAfter: 1 * time.Minute}, err
	}

	// Reconcile Service
	if err := r.reconcileService(ctx, console); err != nil {
		log.Error(err, "Failed to reconcile Service")
		return ctrl.Result{RequeueAfter: 1 * time.Minute}, err
	}

	// Check Deployment status
	deploy := &appsv1.Deployment{}
	deployKey := types.NamespacedName{Name: consoleName(console), Namespace: console.Namespace}
	if err := r.Get(ctx, deployKey, deploy); err == nil {
		console.Status.ReadyReplicas = deploy.Status.ReadyReplicas
	}

	// Set URL in status
	if console.Spec.Route.Enabled && console.Spec.Route.Host != "" {
		scheme := "http"
		if console.Spec.Route.TLSEnabled {
			scheme = "https"
		}
		console.Status.URL = fmt.Sprintf("%s://%s", scheme, console.Spec.Route.Host)
	}

	// Update conditions
	meta.SetStatusCondition(&console.Status.Conditions, metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionTrue,
		Reason:             "Deployed",
		Message:            fmt.Sprintf("Console deployed with %d replicas", replicas),
		LastTransitionTime: metav1.Now(),
	})

	if err := r.Status().Update(ctx, console); err != nil {
		return ctrl.Result{RequeueAfter: 1 * time.Minute}, err
	}

	return ctrl.Result{RequeueAfter: 1 * time.Minute}, nil
}

func consoleName(console *kairosv1alpha1.KairosConsole) string {
	return fmt.Sprintf("%s-console", console.Name)
}

func (r *KairosConsoleReconciler) reconcileServiceAccount(ctx context.Context, console *kairosv1alpha1.KairosConsole) error {
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      consoleName(console),
			Namespace: console.Namespace,
			Labels: map[string]string{
				kairosv1alpha1.LabelManagedBy: kairosv1alpha1.LabelManagedByValue,
				kairosv1alpha1.LabelComponent: "console",
			},
		},
	}

	if err := controllerutil.SetControllerReference(console, sa, r.Scheme); err != nil {
		return err
	}

	existing := &corev1.ServiceAccount{}
	if err := r.Get(ctx, types.NamespacedName{Name: sa.Name, Namespace: sa.Namespace}, existing); err != nil {
		if errors.IsNotFound(err) {
			return r.Create(ctx, sa)
		}
		return err
	}
	return nil
}

func (r *KairosConsoleReconciler) reconcileDeployment(ctx context.Context, console *kairosv1alpha1.KairosConsole, image string, replicas int32) error {
	labels := map[string]string{
		"app":                          consoleName(console),
		kairosv1alpha1.LabelManagedBy: kairosv1alpha1.LabelManagedByValue,
		kairosv1alpha1.LabelComponent: "console",
	}

	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      consoleName(console),
			Namespace: console.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": consoleName(console)},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: consoleName(console),
					Containers: []corev1.Container{
						{
							Name:  "console",
							Image: image,
							Ports: []corev1.ContainerPort{
								{
									Name:          "http",
									ContainerPort: 8080,
									Protocol:      corev1.ProtocolTCP,
								},
							},
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("100m"),
									corev1.ResourceMemory: resource.MustParse("128Mi"),
								},
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("500m"),
									corev1.ResourceMemory: resource.MustParse("512Mi"),
								},
							},
							LivenessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/healthz",
										Port: intstr.FromInt(8080),
									},
								},
								InitialDelaySeconds: 10,
								PeriodSeconds:       30,
							},
							ReadinessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/readyz",
										Port: intstr.FromInt(8080),
									},
								},
								InitialDelaySeconds: 5,
								PeriodSeconds:       10,
							},
						},
					},
				},
			},
		},
	}

	if err := controllerutil.SetControllerReference(console, deploy, r.Scheme); err != nil {
		return err
	}

	existing := &appsv1.Deployment{}
	if err := r.Get(ctx, types.NamespacedName{Name: deploy.Name, Namespace: deploy.Namespace}, existing); err != nil {
		if errors.IsNotFound(err) {
			return r.Create(ctx, deploy)
		}
		return err
	}

	existing.Spec = deploy.Spec
	return r.Update(ctx, existing)
}

func (r *KairosConsoleReconciler) reconcileService(ctx context.Context, console *kairosv1alpha1.KairosConsole) error {
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      consoleName(console),
			Namespace: console.Namespace,
			Labels: map[string]string{
				kairosv1alpha1.LabelManagedBy: kairosv1alpha1.LabelManagedByValue,
				kairosv1alpha1.LabelComponent: "console",
			},
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{"app": consoleName(console)},
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Port:       8080,
					TargetPort: intstr.FromInt(8080),
					Protocol:   corev1.ProtocolTCP,
				},
			},
		},
	}

	if err := controllerutil.SetControllerReference(console, svc, r.Scheme); err != nil {
		return err
	}

	existing := &corev1.Service{}
	if err := r.Get(ctx, types.NamespacedName{Name: svc.Name, Namespace: svc.Namespace}, existing); err != nil {
		if errors.IsNotFound(err) {
			return r.Create(ctx, svc)
		}
		return err
	}

	existing.Spec.Selector = svc.Spec.Selector
	existing.Spec.Ports = svc.Spec.Ports
	return r.Update(ctx, existing)
}

// SetupWithManager sets up the controller with the Manager.
func (r *KairosConsoleReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&kairosv1alpha1.KairosConsole{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: 1,
			RateLimiter: workqueue.NewTypedItemExponentialFailureRateLimiter[reconcile.Request](
				5*time.Second, 5*time.Minute),
		}).
		Named("kairosconsole").
		Complete(r)
}
