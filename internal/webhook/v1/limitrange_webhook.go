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

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// nolint:unused
// log is for logging in this package.
var limitrangelog = logf.Log.WithName("limitrange-resource")

// SetupLimitRangeWebhookWithManager registers the webhook for LimitRange in the manager.
func SetupLimitRangeWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&v1.LimitRange{}).
		WithValidator(&LimitRangeCustomValidator{}).
		Complete()
}

// TODO(user): EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
// NOTE: The 'path' attribute must follow a specific pattern and should not be modified directly here.
// Modifying the path for an invalid path can cause API server errors; failing to locate the webhook.
// +kubebuilder:webhook:path=/validate--v1-limitrange,mutating=false,failurePolicy=fail,sideEffects=None,groups="",resources=limitranges,verbs=create;update;delete,versions=v1,name=vlimitrange-v1.kb.io,admissionReviewVersions=v1

// LimitRangeCustomValidator struct is responsible for validating the LimitRange resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type LimitRangeCustomValidator struct {
	// TODO(user): Add more fields as needed for validation
}

var _ webhook.CustomValidator = &LimitRangeCustomValidator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type LimitRange.
func (v *LimitRangeCustomValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	limitrange, ok := obj.(*v1.LimitRange)
	if !ok {
		limitrangelog.Error(nil, "received invalid object type", "expected", "LimitRange", "got", fmt.Sprintf("%T", obj))
		return nil, fmt.Errorf("expected a LimitRange object but got %T", obj)
	}
	limitrangelog.Info("validating limitrange creation", "name", limitrange.GetName(), "namespace", limitrange.GetNamespace())

	if !isManagedByQuotaProfile(limitrange.GetLabels()) {
		limitrangelog.Info("limitrange is not managed by quota profile, skipping validation", "name", limitrange.GetName(), "namespace", limitrange.GetNamespace())
		return nil, nil
	}

	if !isServiceAccount(ctx) {
		limitrangelog.Info("unauthorized request", "name", limitrange.GetName(), "namespace", limitrange.GetNamespace())
		return nil, fmt.Errorf("only service accounts are allowed to create managed limit ranges")
	}

	limitrangelog.Info("limitrange creation validated successfully", "name", limitrange.GetName(), "namespace", limitrange.GetNamespace())
	return nil, nil
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type LimitRange.
func (v *LimitRangeCustomValidator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	limitrange, ok := newObj.(*v1.LimitRange)
	if !ok {
		limitrangelog.Error(nil, "received invalid object type", "expected", "LimitRange", "got", fmt.Sprintf("%T", newObj))
		return nil, fmt.Errorf("expected a LimitRange object for the newObj but got %T", newObj)
	}
	limitrangelog.Info("validating limitrange update", "name", limitrange.GetName(), "namespace", limitrange.GetNamespace())

	if !isManagedByQuotaProfile(limitrange.GetLabels()) {
		limitrangelog.Info("limitrange is not managed by quota profile, skipping validation", "name", limitrange.GetName(), "namespace", limitrange.GetNamespace())
		return nil, nil
	}

	if !isServiceAccount(ctx) {
		limitrangelog.Info("unauthorized request", "name", limitrange.GetName(), "namespace", limitrange.GetNamespace())
		return nil, fmt.Errorf("only service accounts are allowed to update managed limit ranges")
	}

	limitrangelog.Info("limitrange update validated successfully", "name", limitrange.GetName(), "namespace", limitrange.GetNamespace())
	return nil, nil
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type LimitRange.
func (v *LimitRangeCustomValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {

	limitrange, ok := obj.(*v1.LimitRange)
	if !ok {
		limitrangelog.Error(nil, "received invalid object type", "expected", "LimitRange", "got", fmt.Sprintf("%T", obj))
		return nil, fmt.Errorf("expected a LimitRange object but got %T", obj)
	}
	limitrangelog.Info("validating limitrange deletion", "name", limitrange.GetName(), "namespace", limitrange.GetNamespace())

	if !isManagedByQuotaProfile(limitrange.GetLabels()) {
		limitrangelog.Info("limitrange is not managed by quota profile, skipping validation", "name", limitrange.GetName(), "namespace", limitrange.GetNamespace())
		return nil, nil
	}

	if !isServiceAccount(ctx) {
		limitrangelog.Info("unauthorized request", "name", limitrange.GetName(), "namespace", limitrange.GetNamespace())
		return nil, fmt.Errorf("only service accounts are allowed to delete managed limit ranges")
	}

	limitrangelog.Info("limitrange deletion validated successfully", "name", limitrange.GetName(), "namespace", limitrange.GetNamespace())
	return nil, nil
}
