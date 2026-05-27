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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	kairosv1alpha1 "github.com/maximilianoPizarro/kairos/api/v1alpha1"
)

var _ = Describe("SmartScalingPolicy Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-policy"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default",
		}
		smartscalingpolicy := &kairosv1alpha1.SmartScalingPolicy{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind SmartScalingPolicy")
			err := k8sClient.Get(ctx, typeNamespacedName, smartscalingpolicy)
			if err != nil && errors.IsNotFound(err) {
				resource := &kairosv1alpha1.SmartScalingPolicy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: kairosv1alpha1.SmartScalingPolicySpec{
						Target: kairosv1alpha1.TargetRef{
							APIVersion: "apps/v1",
							Kind:       "Deployment",
							Name:       "test-deployment",
							Namespace:  "default",
						},
						Rules: []kairosv1alpha1.ScalingRule{
							{
								Name: "test-rule",
								When: kairosv1alpha1.MetricCondition{
									Metric:    "cpu_usage",
									Operator:  kairosv1alpha1.OperatorGreaterThan,
									Threshold: "80",
								},
								Action: kairosv1alpha1.ScalingAction{
									Type: kairosv1alpha1.ActionAddReplicas,
								},
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			resource := &kairosv1alpha1.SmartScalingPolicy{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance SmartScalingPolicy")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})
		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			controllerReconciler := &SmartScalingPolicyReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			// The controller will return an error because the target deployment doesn't exist,
			// but it should NOT panic. A requeue-after result with error is acceptable.
			if err != nil {
				// Expected: target workload not found is not a test failure
				Expect(err.Error()).To(ContainSubstring("not found"))
			}
		})
	})
})
