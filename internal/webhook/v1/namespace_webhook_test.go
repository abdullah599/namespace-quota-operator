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

package v1

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	quotav1alpha1 "github.com/abdullah599/namespace-quota-operator/api/v1alpha1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"

	"sigs.k8s.io/controller-runtime/pkg/log"
)

func setupFakeClientWithScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(s)
	_ = quotav1alpha1.AddToScheme(s)
	return s
}

func getProfileID(namespace, profile string) string {
	return fmt.Sprintf("%s.%s", namespace, profile)
}

var _ = Describe("Namespace Webhook", func() {
	var (
		ns             *v1.Namespace
		qp             *quotav1alpha1.QuotaProfile
		qpIrrelevant   *quotav1alpha1.QuotaProfile
		qpNameSelector *quotav1alpha1.QuotaProfile
		defaulter      NamespaceCustomDefaulter
		fakeClient     client.Client
		s              *runtime.Scheme
	)

	BeforeEach(func() {
		ns = &v1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-namespace-with-label",
				Labels: map[string]string{
					"environment": "test",
				},
			},
		}
		qp = &quotav1alpha1.QuotaProfile{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-quota-profile",
				Namespace: "default-0",
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

		qpIrrelevant = &quotav1alpha1.QuotaProfile{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "irrelevant-quota-profile",
				Namespace: "default-1",
			},
			Spec: quotav1alpha1.QuotaProfileSpec{
				Precedence: 10,
				NamespaceSelector: quotav1alpha1.NamespaceSelector{
					MatchLabels: map[string]string{"environment": "dev"},
				},
			},
		}

		qpNameSelector = &quotav1alpha1.QuotaProfile{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "nameselector-quota-profile",
				Namespace: "default-2",
			},
			Spec: quotav1alpha1.QuotaProfileSpec{
				NamespaceSelector: quotav1alpha1.NamespaceSelector{
					MatchName: &ns.Name,
				},
			},
		}

		Expect(defaulter).NotTo(BeNil(), "Expected defaulter to be initialized")
		Expect(qp).NotTo(BeNil(), "Expected quotaProfile to be initialized")
		Expect(ns).NotTo(BeNil(), "Expected ns to be initialized")

		log.SetLogger(zap.New(zap.UseDevMode(true), zap.WriteTo(GinkgoWriter)))

		s = setupFakeClientWithScheme()

		fakeClient = fake.NewClientBuilder().
			WithScheme(s).
			WithObjects(qp, qpIrrelevant).
			Build()

		defaulter = NamespaceCustomDefaulter{
			c: fakeClient,
		}
	})

	Context("When creating Namespace under Defaulting Webhook", func() {
		It("should set the quota profile label for correct quota profile", func() {
			err := defaulter.Default(ctx, ns)
			Expect(err).NotTo(HaveOccurred(), "Expected no error when setting quota profile label")
			Expect(ns.Labels).To(HaveKeyWithValue(quotav1alpha1.QuotaProfileLabelKey, getProfileID(qp.Namespace, qp.Name)))
		})

		It("should set the quota profile label for nameselector quota profile", func() {
			err := fakeClient.Create(ctx, qpNameSelector)
			Expect(err).NotTo(HaveOccurred(), "Expected no error when creating nameselector quota profile")
			err = defaulter.Default(ctx, ns)
			Expect(err).NotTo(HaveOccurred(), "Expected no error when setting quota profile label")
			Expect(ns.Labels).To(HaveKeyWithValue(quotav1alpha1.QuotaProfileLabelKey, getProfileID(qpNameSelector.Namespace, qpNameSelector.Name)))
		})
	})

})
