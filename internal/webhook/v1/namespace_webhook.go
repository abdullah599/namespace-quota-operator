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
	"sort"
	"strings"
	"time"

	"github.com/abdullah599/namespace-quota-operator/api/v1alpha1"
	"github.com/samber/lo"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

// nolint:unused
// log is for logging in this package.
var namespacelog = logf.Log.WithName("namespace-resource")

var C client.Client

// SetupNamespaceWebhookWithManager registers the webhook for Namespace in the manager.
func SetupNamespaceWebhookWithManager(mgr ctrl.Manager) error {
	C = mgr.GetClient()
	namespacelog.Info("setting up namespace webhook")
	return ctrl.NewWebhookManagedBy(mgr).For(&v1.Namespace{}).
		WithDefaulter(&NamespaceCustomDefaulter{}).
		Complete()
}

// TODO(user): EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

// +kubebuilder:webhook:path=/mutate--v1-namespace,mutating=true,failurePolicy=fail,sideEffects=None,groups="",resources=namespaces,verbs=update,versions=v1,name=mnamespace-v1.kb.io,admissionReviewVersions=v1

// NamespaceCustomDefaulter struct is responsible for setting default values on the custom resource of the
// Kind Namespace when those are created or updated.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as it is used only for temporary operations and does not need to be deeply copied.
type NamespaceCustomDefaulter struct {
	// TODO(user): Add more fields as needed for defaulting
}

var _ webhook.CustomDefaulter = &NamespaceCustomDefaulter{}

// Default implements webhook.CustomDefaulter so a webhook will be registered for the Kind Namespace.
func (d *NamespaceCustomDefaulter) Default(ctx context.Context, obj runtime.Object) error {
	namespace, ok := obj.(*v1.Namespace)

	if !ok {
		return fmt.Errorf("expected an Namespace object but got %T", obj)
	}
	namespacelog.Info("defaulting for namespace", "name", namespace.GetName())

	quotaProfiles := &v1alpha1.QuotaProfileList{}
	if err := C.List(ctx, quotaProfiles); err != nil {
		namespacelog.Error(err, "failed to list quota profiles")
		return err
	}

	sort.Slice(quotaProfiles.Items, func(i, j int) bool {
		return quotaProfiles.Items[i].CreationTimestamp.Before(&quotaProfiles.Items[j].CreationTimestamp)
	})

	matched := false

	for _, quotaProfile := range quotaProfiles.Items {
		if quotaProfile.DeletionTimestamp != nil {
			namespacelog.Info("quota profile is being deleted, skipping", "quotaProfile", quotaProfile.Name)
			continue
		}
		if quotaProfile.Spec.NamespaceSelector.MatchName != nil {
			if *quotaProfile.Spec.NamespaceSelector.MatchName == namespace.GetName() {
				namespacelog.Info("matched namespace by name", "namespace", namespace.GetName(), "quotaProfile", quotaProfile.Name)
				addLabel(namespace, &quotaProfile)
				return nil
			}
		} else if quotaProfile.Spec.NamespaceSelector.MatchLabels != nil {
			keys := lo.Keys(quotaProfile.Spec.NamespaceSelector.MatchLabels)
			key := keys[0]
			value := quotaProfile.Spec.NamespaceSelector.MatchLabels[key]

			if _, ok := namespace.GetLabels()[key]; !ok {
				namespacelog.Info("namespace does not have required label", "namespace", namespace.GetName(), "label", key)
				continue
			}
			// selector match
			if namespace.GetLabels()[key] == value {
				namespacelog.Info("matched namespace by label", "namespace", namespace.GetName(), "quotaProfile", quotaProfile.Name, "label", fmt.Sprintf("%s=%s", key, value))
				if _, ok := namespace.Labels[v1alpha1.QuotaProfileLabelKey]; !ok {
					addLabel(namespace, &quotaProfile)
					matched = true
				} else {
					existingProfileID := namespace.Labels[v1alpha1.QuotaProfileLabelKey]
					existingProfileNamespace, existingProfileName := splitProfileID(existingProfileID)
					if existingProfileNamespace == quotaProfile.Namespace && existingProfileName == quotaProfile.Name {
						namespacelog.Info("namespace already has matching quota profile", "namespace", namespace.GetName(), "quotaProfile", quotaProfile.Name)
						matched = true
						continue
					} else {
						namespacelog.Info("resolving conflict between quota profiles", "namespace", namespace.GetName(), "existing", existingProfileID, "new", quotaProfile.Name)
						if err := resolveConflict(ctx, &quotaProfile, namespace); err != nil {
							namespacelog.Error(err, "failed to resolve conflict")
							return err
						}
						matched = true
					}
				}
			}
		}
	}

	if !matched {
		namespacelog.Info("no matching quota profile found, removing labels", "namespace", namespace.GetName())
		removeLabel(namespace)
	}

	return nil
}

func removeLabel(ns *v1.Namespace) {
	namespacelog.Info("removing quota profile labels", "namespace", ns.GetName())
	delete(ns.Labels, v1alpha1.QuotaProfileLabelKey)
	delete(ns.Labels, v1alpha1.QuotaProfileLastUpdateTimestamp)
}

func resolveConflict(ctx context.Context, quotaProfile *v1alpha1.QuotaProfile, ns *v1.Namespace) error {
	existingProfileID := ns.Labels[v1alpha1.QuotaProfileLabelKey]
	existingProfileNamespace, existingProfileName := splitProfileID(existingProfileID)

	existingProfile := &v1alpha1.QuotaProfile{}
	if err := C.Get(ctx, types.NamespacedName{Name: existingProfileName, Namespace: existingProfileNamespace}, existingProfile); err != nil {
		namespacelog.Error(err, "failed to get existing quota profile", "profile", existingProfileID)
		return err
	}

	if (existingProfile == &v1alpha1.QuotaProfile{}) {
		namespacelog.Info("existing profile not found, using new profile", "namespace", ns.GetName(), "profile", quotaProfile.Name)
		addLabel(ns, quotaProfile)
		return nil
	}

	if existingProfile.Spec.Precedence > quotaProfile.Spec.Precedence {
		namespacelog.Info("keeping existing profile due to higher precedence", "namespace", ns.GetName(), "existing", existingProfileID, "existingPrecedence", existingProfile.Spec.Precedence, "newPrecedence", quotaProfile.Spec.Precedence)
		return nil
	}

	namespacelog.Info("using new profile due to higher or equal precedence", "namespace", ns.GetName(), "new", quotaProfile.Name, "existingPrecedence", existingProfile.Spec.Precedence, "newPrecedence", quotaProfile.Spec.Precedence)
	addLabel(ns, quotaProfile)

	return nil
}

func splitProfileID(profileID string) (string, string) {
	parts := strings.Split(profileID, ".")
	if len(parts) != 2 {
		namespacelog.Info("invalid profile ID format", "profileID", profileID)
		return "", ""
	}
	return parts[0], parts[1]
}

func addLabel(ns *v1.Namespace, quotaProfile *v1alpha1.QuotaProfile) {
	namespacelog.Info("adding quota profile labels", "namespace", ns.GetName(), "quotaProfile", quotaProfile.Name)
	ns.Labels[v1alpha1.QuotaProfileLabelKey] = quotaProfile.Namespace + "." + quotaProfile.Name
	ns.Labels[v1alpha1.QuotaProfileLastUpdateTimestamp] = strings.ReplaceAll(time.Now().Format(time.RFC3339), ":", "-")
}
