/*
Copyright 2025.

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
	"encoding/json"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	appv1alpha1 "github.com/example/appbundle-operator/api/v1alpha1"
)

var _ = Describe("AppBundle Controller", func() {
	Context("When reconciling a resource", func() {
		var resourceName string
		var typeNamespacedName types.NamespacedName
		var appbundle *appv1alpha1.AppBundle

		ctx := context.Background()

		BeforeEach(func() {
			// Generate unique resource name for each test
			resourceName = fmt.Sprintf("test-resource-%d", time.Now().UnixNano())
			typeNamespacedName = types.NamespacedName{
				Name:      resourceName,
				Namespace: "default",
			}
			appbundle = &appv1alpha1.AppBundle{}
		})

		BeforeEach(func() {
			By("creating the custom resource for the Kind AppBundle")
			err := k8sClient.Get(ctx, typeNamespacedName, appbundle)
			if err != nil && errors.IsNotFound(err) {
				// Create a valid ConfigMap template for testing
				configMapTemplate := map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "ConfigMap",
					"metadata": map[string]interface{}{
						"name":      "test-config",
						"namespace": "default",
					},
					"data": map[string]interface{}{
						"key": "value",
					},
				}
				configMapBytes, _ := json.Marshal(configMapTemplate)

				// Create a valid Service template for testing
				serviceTemplate := map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "Service",
					"metadata": map[string]interface{}{
						"name":      "test-service",
						"namespace": "default",
					},
					"spec": map[string]interface{}{
						"selector": map[string]interface{}{
							"app": "test",
						},
						"ports": []interface{}{
							map[string]interface{}{
								"port":       80,
								"targetPort": 8080,
							},
						},
					},
				}
				serviceBytes, _ := json.Marshal(serviceTemplate)

				resource := &appv1alpha1.AppBundle{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: appv1alpha1.AppBundleSpec{
						Groups: []appv1alpha1.Group{
							{
								Name:  "infrastructure",
								Order: 0,
								Components: []appv1alpha1.Component{
									{
										Name:     "config",
										Order:    0,
										Template: runtime.RawExtension{Raw: configMapBytes},
									},
								},
							},
							{
								Name:  "application",
								Order: 1,
								Components: []appv1alpha1.Component{
									{
										Name:     "service",
										Order:    0,
										Template: runtime.RawExtension{Raw: serviceBytes},
									},
								},
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			// Cleanup logic after each test, like removing the resource instance.
			resource := &appv1alpha1.AppBundle{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			if err == nil {
				By("Cleanup the specific resource instance AppBundle")
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())

				// Trigger reconciliation to process finalizer during deletion
				controllerReconciler := &AppBundleReconciler{
					Client: k8sClient,
					Scheme: k8sClient.Scheme(),
				}

				// Reconcile to process the finalizer
				_, _ = controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				// Don't fail on error since resource might already be deleted

				// Wait for the resource to be fully deleted (longer timeout for finalizer processing)
				Eventually(func() bool {
					err := k8sClient.Get(ctx, typeNamespacedName, resource)
					return errors.IsNotFound(err)
				}, "10s", "1s").Should(BeTrue())
			}
		})
		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			controllerReconciler := &AppBundleReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking that the AppBundle status is updated")
			Eventually(func() error {
				err := k8sClient.Get(ctx, typeNamespacedName, appbundle)
				return err
			}).Should(Succeed())

			By("Verifying the finalizer is added")
			Expect(appbundle.Finalizers).To(ContainElement("app.example.com/finalizer"))

			By("Verifying the status phase is set")
			// The phase should be at least Pending after reconciliation
			Expect(string(appbundle.Status.Phase)).To(BeElementOf("Pending", "Deploying", "Deployed"))
		})

		It("should handle deletion with finalizer cleanup", func() {
			By("Creating and reconciling the resource first")
			controllerReconciler := &AppBundleReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			// First reconciliation to set up finalizers
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Deleting the resource")
			// Refresh the resource to get latest version
			err = k8sClient.Get(ctx, typeNamespacedName, appbundle)
			Expect(err).NotTo(HaveOccurred())

			err = k8sClient.Delete(ctx, appbundle)
			Expect(err).NotTo(HaveOccurred())

			By("Reconciling during deletion to process finalizer")
			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying the resource is eventually deleted")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, typeNamespacedName, appbundle)
				return errors.IsNotFound(err)
			}, "10s", "1s").Should(BeTrue())
		})
	})
})
