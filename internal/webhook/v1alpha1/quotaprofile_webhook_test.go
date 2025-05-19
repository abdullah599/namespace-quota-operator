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

package v1alpha1

import (
	"context"

	quotav1alpha1 "github.com/abdullah599/namespace-quota-operator/api/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("QuotaProfile Webhook", func() {
	var (
		ctx       context.Context
		obj       *quotav1alpha1.QuotaProfile
		oldObj    *quotav1alpha1.QuotaProfile
		validator QuotaProfileCustomValidator
	)

	BeforeEach(func() {
		ctx = context.TODO()
		obj = &quotav1alpha1.QuotaProfile{
			TypeMeta: metav1.TypeMeta{
				Kind:       "QuotaProfile",
				APIVersion: "quota.dev.operator/v1alpha1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-profile",
				Namespace: "default",
			},
			Spec: quotav1alpha1.QuotaProfileSpec{
				Precedence: 10,
				NamespaceSelector: quotav1alpha1.NamespaceSelector{
					MatchLabels: map[string]string{
						"environment": "dev",
					},
				},
				ResourceQuotaSpecs: []v1.ResourceQuotaSpec{
					{
						Hard: v1.ResourceList{
							v1.ResourceCPU:    resource.MustParse("1"),
							v1.ResourceMemory: resource.MustParse("1Gi"),
						},
					},
				},
				LimitRangeSpecs: []v1.LimitRangeSpec{
					{
						Limits: []v1.LimitRangeItem{
							{
								Type: v1.LimitTypeContainer,
								Default: v1.ResourceList{
									v1.ResourceCPU:    resource.MustParse("500m"),
									v1.ResourceMemory: resource.MustParse("512Mi"),
								},
								DefaultRequest: v1.ResourceList{
									v1.ResourceCPU:    resource.MustParse("250m"),
									v1.ResourceMemory: resource.MustParse("256Mi"),
								},
								Max: v1.ResourceList{
									v1.ResourceCPU:    resource.MustParse("2"),
									v1.ResourceMemory: resource.MustParse("2Gi"),
								},
							},
						},
					},
				},
			},
		}
		oldObj = obj.DeepCopy()
		validator = QuotaProfileCustomValidator{}
		Expect(validator).NotTo(BeNil(), "Expected validator to be initialized")
		Expect(oldObj).NotTo(BeNil(), "Expected oldObj to be initialized")
		Expect(obj).NotTo(BeNil(), "Expected obj to be initialized")
	})

	Context("When creating QuotaProfile", func() {
		It("Should deny creation if no namespace selector is specified", func() {
			obj.Spec.NamespaceSelector = quotav1alpha1.NamespaceSelector{}
			Expect(validator.ValidateCreate(ctx, obj)).Error().To(HaveOccurred())
		})

		It("Should deny creation if both matchLabels and matchName are specified", func() {
			obj.Spec.NamespaceSelector.MatchName = ptr("test-ns")
			Expect(validator.ValidateCreate(ctx, obj)).Error().To(HaveOccurred())
		})

		It("Should deny creation if matchLabels has more than one label", func() {
			obj.Spec.NamespaceSelector.MatchLabels["extra"] = "label"
			Expect(validator.ValidateCreate(ctx, obj)).Error().To(HaveOccurred())
		})

		It("Should allow creation with valid matchLabels", func() {
			Expect(validator.ValidateCreate(ctx, obj)).Error().ToNot(HaveOccurred())
		})

		It("Should allow creation with valid matchName", func() {
			obj.Spec.NamespaceSelector = quotav1alpha1.NamespaceSelector{
				MatchName: ptr("test-ns"),
			}
			Expect(validator.ValidateCreate(ctx, obj)).Error().ToNot(HaveOccurred())
		})
	})

	Context("When updating QuotaProfile", func() {
		It("Should deny update if removing namespace selector", func() {
			obj.Spec.NamespaceSelector = quotav1alpha1.NamespaceSelector{}
			Expect(validator.ValidateUpdate(ctx, oldObj, obj)).Error().To(HaveOccurred())
		})

		It("Should allow update with valid changes", func() {
			obj.Spec.Precedence = 20
			obj.Spec.ResourceQuotaSpecs[0].Hard[v1.ResourceCPU] = resource.MustParse("2")
			Expect(validator.ValidateUpdate(ctx, oldObj, obj)).Error().ToNot(HaveOccurred())
		})
	})

})

func ptr(s string) *string {
	return &s
}
