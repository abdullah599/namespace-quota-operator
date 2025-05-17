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
	"context"

	"github.com/abdullah599/namespace-quota-operator/api/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	admissionv1 "k8s.io/api/admission/v1"
	authenticationv1 "k8s.io/api/authentication/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

var _ = Describe("LimitRangee Webhook", func() {
	var (
		ctx          context.Context
		managedObj   *v1.LimitRange
		unmanagedObj *v1.LimitRange
		validator    LimitRangeCustomValidator
	)

	BeforeEach(func() {
		managedObj = &v1.LimitRange{
			TypeMeta: metav1.TypeMeta{
				Kind:       "LimitRange",
				APIVersion: "v1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-limitrange",
				Labels: map[string]string{
					"app.kubernetes.io/name":      "test-limitrange",
					v1alpha1.QuotaProfileLabelKey: "test-profile",
				},
			},
		}

		unmanagedObj = &v1.LimitRange{
			TypeMeta: metav1.TypeMeta{
				Kind:       "LimitRange",
				APIVersion: "v1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-limitrange",
				Labels: map[string]string{
					"app.kubernetes.io/name": "test-limitrange",
				},
			},
		}

		ctx = context.TODO()

		ctx = admission.NewContextWithRequest(ctx, admission.Request{
			AdmissionRequest: admissionv1.AdmissionRequest{
				UserInfo: authenticationv1.UserInfo{
					Username: "test-user",
				},
			},
		})

		validator = LimitRangeCustomValidator{}
		Expect(validator).NotTo(BeNil(), "Expected validator to be initialized")
		Expect(unmanagedObj).NotTo(BeNil(), "Expected unmanagedObj to be initialized")
		Expect(managedObj).NotTo(BeNil(), "Expected managedObj to be initialized")
		Expect(ctx).NotTo(BeNil())
	})

	Context("When creating or updating LimitRangee under Validating Webhook", func() {

		It("Should deny creation by user if a LimitRange is managed by operator", func() {
			Expect(validator.ValidateCreate(ctx, managedObj)).Error().To(HaveOccurred())
		})
		It("Should allow creation by user if a LimitRange is not managed by operator", func() {
			Expect(validator.ValidateCreate(ctx, unmanagedObj)).Error().ToNot(HaveOccurred())
		})
		It("Should deny update by user if a LimitRange is managed by operator", func() {
			Expect(validator.ValidateUpdate(ctx, managedObj, managedObj)).Error().To(HaveOccurred())
		})
		It("Should allow update by user if a LimitRange is not managed by operator", func() {
			Expect(validator.ValidateUpdate(ctx, unmanagedObj, unmanagedObj)).Error().ToNot(HaveOccurred())
		})
		It("Should deny deletion by user if a LimitRange is managed by operator", func() {
			Expect(validator.ValidateDelete(ctx, managedObj)).Error().To(HaveOccurred())
		})
		It("Should allow deletion by user if a LimitRange is not managed by operator", func() {
			Expect(validator.ValidateDelete(ctx, unmanagedObj)).Error().ToNot(HaveOccurred())
		})
	})

})
