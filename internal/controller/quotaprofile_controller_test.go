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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	quotav1alpha1 "github.com/abdullah599/namespace-quota-operator/api/v1alpha1"
)

var _ = Describe("QuotaProfile Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		var (
			ctx          context.Context
			fakeClient   client.Client
			s            *runtime.Scheme
			quotaProfile *quotav1alpha1.QuotaProfile
			reconciler   *QuotaProfileReconciler
		)

		BeforeEach(func() {
			log.SetLogger(zap.New(zap.UseDevMode(true), zap.WriteTo(GinkgoWriter)))

			s = runtime.NewScheme()
			_ = clientgoscheme.AddToScheme(s)
			_ = quotav1alpha1.AddToScheme(s)

			ctx = context.Background()

			quotaProfile = &quotav1alpha1.QuotaProfile{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: "default",
				},
				Spec: quotav1alpha1.QuotaProfileSpec{
					Precedence: 10,
					NamespaceSelector: quotav1alpha1.NamespaceSelector{
						MatchLabels: map[string]string{
							"environment": "test",
						},
					},
				},
			}

			testNs1 := &v1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-namespace-with-label",
					Labels: map[string]string{
						"environment": "test",
					},
				},
			}

			testNs2 := &v1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-namespace-without-label",
				},
			}

			fakeClient = fake.NewClientBuilder().
				WithScheme(s).
				WithObjects(quotaProfile, testNs1, testNs2).
				Build()

			reconciler = &QuotaProfileReconciler{
				Client: fakeClient,
				Scheme: s,
			}
		})

		It("should successfully reconcile the resource", func() {
			By("Reconciling the QuotaProfile resource")
			typeNamespacedName := types.NamespacedName{
				Name:      resourceName,
				Namespace: "default",
			}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			// Check that the profile label was added to the namespace
			updatedNs := &v1.Namespace{}
			err = fakeClient.Get(ctx, types.NamespacedName{Name: "test-namespace-with-label"}, updatedNs)
			Expect(err).NotTo(HaveOccurred())

			// Expect the namespace to have the quota profile label
			profileLabelKey := quotav1alpha1.QuotaProfileLabelKey
			Expect(updatedNs.Labels).To(HaveKey(profileLabelKey))
			Expect(updatedNs.Labels[profileLabelKey]).To(Equal("default.test-resource"))

			// Expect the namespace to have the quota profile label
			profileLabelKey = quotav1alpha1.QuotaProfileLabelKey
			Expect(updatedNs.Labels).To(HaveKey(profileLabelKey))
			Expect(updatedNs.Labels[profileLabelKey]).To(Equal("default.test-resource"))

			// Expect the namespace to not have the quota profile label
			err = fakeClient.Get(ctx, types.NamespacedName{Name: "test-namespace-without-label"}, updatedNs)
			Expect(err).NotTo(HaveOccurred())

			Expect(updatedNs.Labels).ToNot(HaveKey(profileLabelKey))

		})
	})
})
