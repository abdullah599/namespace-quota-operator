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
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	quotav1alpha1 "github.com/abdullah599/namespace-quota-operator/api/v1alpha1"
	"github.com/samber/lo"
)

// nolint:unused
// log is for logging in this package.
var quotaprofilelog = logf.Log.WithName("quotaprofile-resource")

var defaultPrecedence = uint16(0)

var C client.Client

// SetupQuotaProfileWebhookWithManager registers the webhook for QuotaProfile in the manager.
func SetupQuotaProfileWebhookWithManager(mgr ctrl.Manager) error {
	C = mgr.GetClient()
	return ctrl.NewWebhookManagedBy(mgr).For(&quotav1alpha1.QuotaProfile{}).
		WithValidator(&QuotaProfileCustomValidator{}).
		Complete()
}

// TODO(user): EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
// NOTE: The 'path' attribute must follow a specific pattern and should not be modified directly here.
// Modifying the path for an invalid path can cause API server errors; failing to locate the webhook.
// +kubebuilder:webhook:path=/validate-quota-dev-operator-v1alpha1-quotaprofile,mutating=false,failurePolicy=fail,sideEffects=None,groups=quota.dev.operator,resources=quotaprofiles,verbs=create;update,versions=v1alpha1,name=vquotaprofile-v1alpha1.kb.io,admissionReviewVersions=v1

// QuotaProfileCustomValidator struct is responsible for validating the QuotaProfile resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type QuotaProfileCustomValidator struct {
}

var _ webhook.CustomValidator = &QuotaProfileCustomValidator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type QuotaProfile.
func (v *QuotaProfileCustomValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	return v.validate(ctx, obj)
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type QuotaProfile.
func (v *QuotaProfileCustomValidator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	return v.validate(ctx, newObj)
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type QuotaProfile.
func (v *QuotaProfileCustomValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	quotaprofile, ok := obj.(*quotav1alpha1.QuotaProfile)
	if !ok {
		return nil, fmt.Errorf("expected a QuotaProfile object but got %T", obj)
	}
	quotaprofilelog.Info("Validation for QuotaProfile upon deletion", "name", quotaprofile.GetName())

	return nil, nil
}

func (v *QuotaProfileCustomValidator) validate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	quotaprofile, ok := obj.(*quotav1alpha1.QuotaProfile)
	if !ok {
		return nil, fmt.Errorf("expected a QuotaProfile object but got %T", obj)
	}
	quotaprofilelog.Info("Validation for QuotaProfile upon creation", "name", quotaprofile.GetName())

	if quotaprofile.Spec.NamespaceSelector.MatchLabels == nil && quotaprofile.Spec.NamespaceSelector.MatchName == nil {
		return nil, fmt.Errorf("one of namespaceSelector.matchLabels or namespaceSelector.matchName must be set")
	}

	if quotaprofile.Spec.NamespaceSelector.MatchLabels != nil && quotaprofile.Spec.NamespaceSelector.MatchName != nil {
		return nil, fmt.Errorf("only one of namespaceSelector.matchLabels or namespaceSelector.matchName can be set")
	}

	// this is a limitation of this operator, and can be removed in the future
	if quotaprofile.Spec.NamespaceSelector.MatchLabels != nil {
		if len(quotaprofile.Spec.NamespaceSelector.MatchLabels) > 1 {
			return nil, fmt.Errorf("only one label can be used in namespaceSelector.matchLabels")
		}
	}

	//list all quota profiles
	quotaProfiles := &quotav1alpha1.QuotaProfileList{}
	if err := C.List(ctx, quotaProfiles); err != nil {
		return nil, fmt.Errorf("failed to list quota profiles: %w", err)
	}

	// if this quota has name in selector, check if other profiles have same name
	if quotaprofile.Spec.NamespaceSelector.MatchName != nil {
		for _, profile := range quotaProfiles.Items {

			// skip if the profile is the same
			if profile.Namespace == quotaprofile.Namespace && profile.Name == quotaprofile.Name {
				continue
			}
			if profile.Spec.NamespaceSelector.MatchName != nil && *profile.Spec.NamespaceSelector.MatchName == *quotaprofile.Spec.NamespaceSelector.MatchName {
				return nil, fmt.Errorf("quota profile with matchName %s already exists: %s/%s", *quotaprofile.Spec.NamespaceSelector.MatchName, profile.Namespace, profile.Name)
			}
		}
	}

	// if this quota has labels in selector, check if other profiles have same labels
	if quotaprofile.Spec.NamespaceSelector.MatchLabels != nil {
		keys := lo.Keys(quotaprofile.Spec.NamespaceSelector.MatchLabels)
		for _, profile := range quotaProfiles.Items {
			// skip if the profile is the same
			if profile.Namespace == quotaprofile.Namespace && profile.Name == quotaprofile.Name {
				continue
			}
			if profile.Spec.NamespaceSelector.MatchLabels != nil {
				profileKeys := lo.Keys(profile.Spec.NamespaceSelector.MatchLabels)
				if profileKeys[0] == keys[0] {
					return nil, fmt.Errorf("quota profile with matchLabels %v already exists: %s/%s", quotaprofile.Spec.NamespaceSelector.MatchLabels, profile.Namespace, profile.Name)
				}
			}
		}
	}
	return nil, nil
}
