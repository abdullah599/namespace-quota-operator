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
	"fmt"
	"strings"

	"github.com/abdullah599/namespace-quota-operator/api/v1alpha1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// nolint:unused
// log is for logging in this package.
var resourcequotalog = logf.Log.WithName("resourcequota-resource")

// SetupResourceQuotaWebhookWithManager registers the webhook for ResourceQuota in the manager.
func SetupResourceQuotaWebhookWithManager(mgr ctrl.Manager) error {
	resourcequotalog.Info("setting up resourcequota webhook with manager")
	return ctrl.NewWebhookManagedBy(mgr).For(&v1.ResourceQuota{}).
		WithValidator(&ResourceQuotaCustomValidator{}).
		Complete()
}

// NOTE: The 'path' attribute must follow a specific pattern and should not be modified directly here.
// Modifying the path for an invalid path can cause API server errors; failing to locate the webhook.
// +kubebuilder:webhook:path=/validate--v1-resourcequota,mutating=false,failurePolicy=fail,sideEffects=None,groups="",resources=resourcequotas,verbs=create;update;delete,versions=v1,name=vresourcequota-v1.kb.io,admissionReviewVersions=v1

// ResourceQuotaCustomValidator struct is responsible for validating the ResourceQuota resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type ResourceQuotaCustomValidator struct {
	decoder *admission.Decoder
}

var _ webhook.CustomValidator = &ResourceQuotaCustomValidator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type ResourceQuota.
func (v *ResourceQuotaCustomValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	req, err := admission.RequestFromContext(ctx)
	if err != nil {
		resourcequotalog.Error(err, "failed to get request from context")
		return nil, fmt.Errorf("could not get request from context: %v", err)
	}

	resourcequota, ok := obj.(*v1.ResourceQuota)
	if !ok {
		resourcequotalog.Error(nil, "received invalid object type", "expected", "ResourceQuota", "got", fmt.Sprintf("%T", obj))
		return nil, fmt.Errorf("expected a ResourceQuota object but got %T", obj)
	}
	resourcequotalog.Info("validating resourcequota creation", "name", resourcequota.GetName(), "namespace", resourcequota.GetNamespace())

	if !isManagedByQuotaProfile(resourcequota) {
		resourcequotalog.Info("resourcequota is not managed by quota profile, skipping validation", "name", resourcequota.GetName(), "namespace", resourcequota.GetNamespace())
		return nil, nil
	}

	if !isServiceAccount(req) {
		resourcequotalog.Info("unauthorized request", "user", req.UserInfo.Username, "name", resourcequota.GetName(), "namespace", resourcequota.GetNamespace())
		return nil, fmt.Errorf("only service accounts are allowed to create managed resource quotas")
	}

	resourcequotalog.Info("resourcequota creation validated successfully", "name", resourcequota.GetName(), "namespace", resourcequota.GetNamespace())
	return nil, nil
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type ResourceQuota.
func (v *ResourceQuotaCustomValidator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	req, err := admission.RequestFromContext(ctx)
	if err != nil {
		resourcequotalog.Error(err, "failed to get request from context")
		return nil, fmt.Errorf("could not get request from context: %v", err)
	}

	resourcequota, ok := newObj.(*v1.ResourceQuota)
	if !ok {
		resourcequotalog.Error(nil, "received invalid object type", "expected", "ResourceQuota", "got", fmt.Sprintf("%T", newObj))
		return nil, fmt.Errorf("expected a ResourceQuota object for the newObj but got %T", newObj)
	}
	resourcequotalog.Info("validating resourcequota update", "name", resourcequota.GetName(), "namespace", resourcequota.GetNamespace())

	if !isManagedByQuotaProfile(resourcequota) {
		resourcequotalog.Info("resourcequota is not managed by quota profile, skipping validation", "name", resourcequota.GetName(), "namespace", resourcequota.GetNamespace())
		return nil, nil
	}

	if !isServiceAccount(req) {
		resourcequotalog.Info("unauthorized request", "user", req.UserInfo.Username, "name", resourcequota.GetName(), "namespace", resourcequota.GetNamespace())
		return nil, fmt.Errorf("only service accounts are allowed to update managed resource quotas")
	}

	resourcequotalog.Info("resourcequota update validated successfully", "name", resourcequota.GetName(), "namespace", resourcequota.GetNamespace())
	return nil, nil
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type ResourceQuota.
func (v *ResourceQuotaCustomValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	req, err := admission.RequestFromContext(ctx)
	if err != nil {
		resourcequotalog.Error(err, "failed to get request from context")
		return nil, fmt.Errorf("could not get request from context: %v", err)
	}

	resourcequota, ok := obj.(*v1.ResourceQuota)
	if !ok {
		resourcequotalog.Error(nil, "received invalid object type", "expected", "ResourceQuota", "got", fmt.Sprintf("%T", obj))
		return nil, fmt.Errorf("expected a ResourceQuota object but got %T", obj)
	}
	resourcequotalog.Info("validating resourcequota deletion", "name", resourcequota.GetName(), "namespace", resourcequota.GetNamespace())

	if !isManagedByQuotaProfile(resourcequota) {
		resourcequotalog.Info("resourcequota is not managed by quota profile, skipping validation", "name", resourcequota.GetName(), "namespace", resourcequota.GetNamespace())
		return nil, nil
	}

	if !isServiceAccount(req) {
		resourcequotalog.Info("unauthorized request", "user", req.UserInfo.Username, "name", resourcequota.GetName(), "namespace", resourcequota.GetNamespace())
		return nil, fmt.Errorf("only service accounts are allowed to delete managed resource quotas")
	}

	resourcequotalog.Info("resourcequota deletion validated successfully", "name", resourcequota.GetName(), "namespace", resourcequota.GetNamespace())
	return nil, nil
}

func isManagedByQuotaProfile(rq *v1.ResourceQuota) bool {
	labels := rq.GetLabels()
	if labels == nil {
		return false
	}
	return labels[v1alpha1.QuotaProfileLabelKey] != ""
}

func isServiceAccount(req admission.Request) bool {
	return strings.HasPrefix(req.UserInfo.Username, "system:serviceaccount:")
}
