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
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
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
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=route.openshift.io,resources=routes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterrolebindings,verbs=get;list;watch;create;update;patch;delete

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

	// Reconcile Route
	if console.Spec.Route.Enabled {
		if err := r.reconcileRoute(ctx, console); err != nil {
			log.Error(err, "Failed to reconcile Route")
			return ctrl.Result{RequeueAfter: 1 * time.Minute}, err
		}
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

	// Add OAuth redirect annotation when using openshift-oauth
	if r.isOAuthEnabled(console) {
		redirectURI := fmt.Sprintf("https://%s/oauth/callback", console.Spec.Route.Host)
		sa.Annotations = map[string]string{
			"serviceaccounts.openshift.io/oauth-redirectreference.kairos": fmt.Sprintf(
				`{"kind":"OAuthRedirectReference","apiVersion":"v1","reference":{"kind":"Route","name":"%s"}}`,
				consoleName(console),
			),
			"serviceaccounts.openshift.io/oauth-redirecturi.kairos": redirectURI,
		}
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

	// Update annotations if changed
	if r.isOAuthEnabled(console) {
		existing.Annotations = sa.Annotations
		return r.Update(ctx, existing)
	}
	return nil
}

func (r *KairosConsoleReconciler) isOAuthEnabled(console *kairosv1alpha1.KairosConsole) bool {
	return console.Spec.Auth != nil && console.Spec.Auth.Type == kairosv1alpha1.AuthTypeOpenshiftOAuth
}

func (r *KairosConsoleReconciler) reconcileDeployment(ctx context.Context, console *kairosv1alpha1.KairosConsole, image string, replicas int32) error {
	labels := map[string]string{
		"app":                         consoleName(console),
		kairosv1alpha1.LabelManagedBy: kairosv1alpha1.LabelManagedByValue,
		kairosv1alpha1.LabelComponent: "console",
	}

	containers := []corev1.Container{
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
	}

	var volumes []corev1.Volume

	// Add oauth-proxy sidecar when auth type is openshift-oauth
	if r.isOAuthEnabled(console) {
		oauthImage := "registry.redhat.io/openshift4/ose-oauth-proxy:latest"
		requiredRole := "cluster-admin"
		httpsPort := int32(8443)
		cookieSecretName := fmt.Sprintf("%s-oauth-cookie", consoleName(console))

		if console.Spec.Auth.OAuth != nil {
			if console.Spec.Auth.OAuth.Image != "" {
				oauthImage = console.Spec.Auth.OAuth.Image
			}
			if console.Spec.Auth.OAuth.RequiredRole != "" {
				requiredRole = console.Spec.Auth.OAuth.RequiredRole
			}
			if console.Spec.Auth.OAuth.HTTPSPort > 0 {
				httpsPort = console.Spec.Auth.OAuth.HTTPSPort
			}
			if console.Spec.Auth.OAuth.CookieSecret != "" {
				cookieSecretName = console.Spec.Auth.OAuth.CookieSecret
			}
		}

		// Ensure cookie secret exists
		if err := r.ensureOAuthCookieSecret(ctx, console, cookieSecretName); err != nil {
			return err
		}

		oauthProxy := corev1.Container{
			Name:  "oauth-proxy",
			Image: oauthImage,
			Args: []string{
				"--https-address=:" + fmt.Sprintf("%d", httpsPort),
				"--http-address=",
				"--provider=openshift",
				"--upstream=http://localhost:8080",
				"--tls-cert=/etc/tls/private/tls.crt",
				"--tls-key=/etc/tls/private/tls.key",
				"--cookie-secret-file=/etc/oauth/cookie-secret",
				"--openshift-service-account=" + consoleName(console),
				"--openshift-sar={\"resource\":\"namespaces\",\"verb\":\"get\"}",
				"--openshift-delegate-urls={\"/\":{\"resource\":\"namespaces\",\"verb\":\"get\",\"group\":\"\",\"subresource\":\"\"}}",
				fmt.Sprintf("--openshift-sar={\"resource\":\"clusterroles\",\"verb\":\"bind\",\"resourceName\":\"%s\"}", requiredRole),
			},
			Ports: []corev1.ContainerPort{
				{
					Name:          "https",
					ContainerPort: httpsPort,
					Protocol:      corev1.ProtocolTCP,
				},
			},
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("50m"),
					corev1.ResourceMemory: resource.MustParse("64Mi"),
				},
				Limits: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("100m"),
					corev1.ResourceMemory: resource.MustParse("128Mi"),
				},
			},
			VolumeMounts: []corev1.VolumeMount{
				{
					Name:      "tls-certs",
					MountPath: "/etc/tls/private",
					ReadOnly:  true,
				},
				{
					Name:      "oauth-cookie",
					MountPath: "/etc/oauth",
					ReadOnly:  true,
				},
			},
		}
		containers = append(containers, oauthProxy)

		tlsSecretName := fmt.Sprintf("%s-tls", consoleName(console))
		volumes = []corev1.Volume{
			{
				Name: "tls-certs",
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName: tlsSecretName,
					},
				},
			},
			{
				Name: "oauth-cookie",
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName: cookieSecretName,
					},
				},
			},
		}
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
					Containers:         containers,
					Volumes:            volumes,
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

	// When OAuth is enabled, add the HTTPS port and annotate for serving-cert
	if r.isOAuthEnabled(console) {
		httpsPort := int32(8443)
		if console.Spec.Auth.OAuth != nil && console.Spec.Auth.OAuth.HTTPSPort > 0 {
			httpsPort = console.Spec.Auth.OAuth.HTTPSPort
		}
		svc.Spec.Ports = append(svc.Spec.Ports, corev1.ServicePort{
			Name:       "https",
			Port:       httpsPort,
			TargetPort: intstr.FromInt32(httpsPort),
			Protocol:   corev1.ProtocolTCP,
		})
		// Annotation for OpenShift to auto-generate TLS certificate
		svc.Annotations = map[string]string{
			"service.beta.openshift.io/serving-cert-secret-name": fmt.Sprintf("%s-tls", consoleName(console)),
		}
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
	if svc.Annotations != nil {
		if existing.Annotations == nil {
			existing.Annotations = make(map[string]string)
		}
		for k, v := range svc.Annotations {
			existing.Annotations[k] = v
		}
	}
	return r.Update(ctx, existing)
}

func (r *KairosConsoleReconciler) reconcileRoute(ctx context.Context, console *kairosv1alpha1.KairosConsole) error {
	routeName := consoleName(console)

	// Determine host: use spec.route.host or leave empty for wildcard
	host := console.Spec.Route.Host

	// Determine TLS and target port
	targetPort := "http"
	tlsTermination := "edge"
	if r.isOAuthEnabled(console) {
		targetPort = "https"
		tlsTermination = "reencrypt"
	}

	route := &unstructured.Unstructured{}
	route.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "route.openshift.io",
		Version: "v1",
		Kind:    "Route",
	})
	route.SetName(routeName)
	route.SetNamespace(console.Namespace)
	route.SetLabels(map[string]string{
		"app":                          routeName,
		kairosv1alpha1.LabelManagedBy:  kairosv1alpha1.LabelManagedByValue,
		kairosv1alpha1.LabelComponent:  "console",
	})

	spec := map[string]interface{}{
		"to": map[string]interface{}{
			"kind":   "Service",
			"name":   routeName,
			"weight": int64(100),
		},
		"port": map[string]interface{}{
			"targetPort": targetPort,
		},
		"tls": map[string]interface{}{
			"termination":                   tlsTermination,
			"insecureEdgeTerminationPolicy": "Redirect",
		},
	}

	if host != "" {
		spec["host"] = host
	}

	route.Object["spec"] = spec

	// Set owner reference
	ownerRef := metav1.OwnerReference{
		APIVersion: console.APIVersion,
		Kind:       console.Kind,
		Name:       console.Name,
		UID:        console.UID,
	}
	route.SetOwnerReferences([]metav1.OwnerReference{ownerRef})

	existing := &unstructured.Unstructured{}
	existing.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "route.openshift.io",
		Version: "v1",
		Kind:    "Route",
	})
	err := r.Get(ctx, types.NamespacedName{Name: routeName, Namespace: console.Namespace}, existing)
	if err != nil {
		if errors.IsNotFound(err) {
			return r.Create(ctx, route)
		}
		return err
	}

	// Update spec if needed
	existing.Object["spec"] = spec
	existing.SetLabels(route.GetLabels())
	return r.Update(ctx, existing)
}

func (r *KairosConsoleReconciler) ensureOAuthCookieSecret(ctx context.Context, console *kairosv1alpha1.KairosConsole, secretName string) error {
	secret := &corev1.Secret{}
	if err := r.Get(ctx, types.NamespacedName{Name: secretName, Namespace: console.Namespace}, secret); err != nil {
		if errors.IsNotFound(err) {
			// Generate a random cookie secret (32 bytes base64 encoded)
			cookieValue := "kairos-oauth-session-secret-key!" // 32 bytes
			secret = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: console.Namespace,
					Labels: map[string]string{
						kairosv1alpha1.LabelManagedBy: kairosv1alpha1.LabelManagedByValue,
						kairosv1alpha1.LabelComponent: "console",
					},
				},
				Type: corev1.SecretTypeOpaque,
				Data: map[string][]byte{
					"cookie-secret": []byte(cookieValue),
				},
			}
			if err := controllerutil.SetControllerReference(console, secret, r.Scheme); err != nil {
				return err
			}
			return r.Create(ctx, secret)
		}
		return err
	}
	return nil
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
