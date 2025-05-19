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
	"fmt"
	"strings"
	"time"

	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	quotav1alpha1 "github.com/abdullah599/namespace-quota-operator/api/v1alpha1"
	"github.com/samber/lo"
)

// QuotaProfileReconciler reconciles a QuotaProfile object
type QuotaProfileReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=quota.dev.operator,resources=quotaprofiles,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=quota.dev.operator,resources=quotaprofiles/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=quota.dev.operator,resources=quotaprofiles/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch;update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// the QuotaProfile object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.20.0/pkg/reconcile
func (r *QuotaProfileReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	l := log.FromContext(ctx)
	l.Info("starting reconciliation", "quotaProfile", req.NamespacedName)

	// Get the QuotaProfile instance
	quotaProfile := &quotav1alpha1.QuotaProfile{}
	if err := r.Get(ctx, req.NamespacedName, quotaProfile); err != nil {
		if apierrors.IsNotFound(err) {
			l.Info("quota profile not found", "quotaProfile", req.NamespacedName)
			// Object not found, return. Created objects are automatically garbage collected.
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		l.Error(err, "failed to get quota profile", "quotaProfile", req.NamespacedName)
		return ctrl.Result{}, err
	}

	// Check if the QuotaProfile instance is marked for deletion
	if !quotaProfile.DeletionTimestamp.IsZero() {
		l.Info("quota profile is being deleted", "quotaProfile", req.NamespacedName)
		return r.handleDeletion(ctx, quotaProfile)
	}

	// Add finalizer if it doesn't exist
	if !controllerutil.ContainsFinalizer(quotaProfile, quotav1alpha1.QuotaProfileFinalizer) {
		l.Info("adding finalizer", "quotaProfile", req.NamespacedName)
		controllerutil.AddFinalizer(quotaProfile, quotav1alpha1.QuotaProfileFinalizer)
		if err := r.Update(ctx, quotaProfile); err != nil {
			l.Error(err, "failed to add finalizer", "quotaProfile", req.NamespacedName)
			return ctrl.Result{}, err
		}
	}

	l.Info("reconciling namespaces", "quotaProfile", req.NamespacedName)
	if err := r.reconcileNamespace(ctx, req); err != nil {
		l.Error(err, "failed to reconcile namespaces", "quotaProfile", req.NamespacedName)
		return ctrl.Result{}, err
	}

	l.Info("successfully reconciled quota profile", "quotaProfile", req.NamespacedName)
	return ctrl.Result{}, nil
}

func (r *QuotaProfileReconciler) reconcileNamespace(ctx context.Context, req ctrl.Request) error {
	l := log.FromContext(ctx)

	nsList := &v1.NamespaceList{}
	if err := r.List(ctx, nsList); err != nil {
		l.Error(err, "failed to list namespaces")
		return err
	}

	quotaProfile := &quotav1alpha1.QuotaProfile{}
	if err := r.Get(ctx, req.NamespacedName, quotaProfile); err != nil {
		l.Error(err, "failed to get quota profile", "quotaProfile", req.NamespacedName)
		return err
	}

	if quotaProfile.Spec.NamespaceSelector.MatchName != nil {
		for _, ns := range nsList.Items {
			if ns.Name == *quotaProfile.Spec.NamespaceSelector.MatchName {
				l.Info("found matching namespace with name selector", "namespace", ns.Name)
				setQuotaProfileLabels(&ns, quotaProfile)
				if err := r.Update(ctx, &ns); err != nil {
					l.Error(err, "failed to set quota profile labels", "namespace", ns.Name)
					return err
				}
			}
		}
	} else if quotaProfile.Spec.NamespaceSelector.MatchLabels != nil {
		for _, ns := range nsList.Items {
			if ns.Labels == nil {
				continue
			}

			keys := lo.Keys(quotaProfile.Spec.NamespaceSelector.MatchLabels)

			// validating webhook ensures that only one key is present
			key := keys[0]
			value := quotaProfile.Spec.NamespaceSelector.MatchLabels[key]

			if ns.Labels[key] == value {
				l.Info("found matching namespace with label selector", "namespace", ns.Name)
				if err := r.addLabelToNamespace(ctx, quotaProfile, &ns); err != nil {
					l.Error(err, "failed to add label to namespace", "namespace", ns.Name)
					return err
				}
			}
		}
	}
	return nil
}

// addLabelToNamespace adds the quota profile label to the namespace if it doesn't exist.
// If a label already exists, it resolves any conflicts based on precedence.
// When precedences are equal, the new quota profile takes precedence.
// The quota profile label key is defined in the API package.
// Returns an error if updating the namespace fails.
func (r *QuotaProfileReconciler) addLabelToNamespace(ctx context.Context, quotaProfile *quotav1alpha1.QuotaProfile, ns *v1.Namespace) error {
	l := log.FromContext(ctx)

	if ns.Labels == nil {
		ns.Labels = make(map[string]string)
	}

	if ns.Labels[quotav1alpha1.QuotaProfileLabelKey] == "" {
		l.Info("adding quota profile label to namespace", "namespace", ns.Name, "quotaProfile", quotaProfile.Name)
		setQuotaProfileLabels(ns, quotaProfile)
		return r.Update(ctx, ns)
	}

	return r.resolveConflict(ctx, quotaProfile, ns)
}

// resolveConflict resolves conflicts between quota profiles when a namespace already has a quota profile label.
// It compares the precedence of the existing profile with the new one.
// If the existing profile has higher precedence, no change is made.
// Otherwise, the namespace is updated with the new quota profile label.
// Returns an error if getting the existing profile or updating the namespace fails.
func (r *QuotaProfileReconciler) resolveConflict(ctx context.Context, quotaProfile *quotav1alpha1.QuotaProfile, ns *v1.Namespace) error {
	l := log.FromContext(ctx)

	existingProfileID := ns.Labels[quotav1alpha1.QuotaProfileLabelKey]
	existingProfileNamespace, existingProfileName := splitProfileID(existingProfileID)

	if existingProfileNamespace == quotaProfile.Namespace && existingProfileName == quotaProfile.Name {
		l.Info("namespace already has this quota profile", "namespace", ns.Name, "quotaProfile", quotaProfile.Name)
		setQuotaProfileLabels(ns, quotaProfile)
		return r.Update(ctx, ns)
	}

	existingProfile := &quotav1alpha1.QuotaProfile{}
	if err := r.Get(ctx, types.NamespacedName{Name: existingProfileName, Namespace: existingProfileNamespace}, existingProfile); client.IgnoreNotFound(err) != nil {
		l.Error(err, "failed to get existing quota profile", "namespace", existingProfileNamespace, "name", existingProfileName)
		return err
	}

	if (existingProfile == &quotav1alpha1.QuotaProfile{}) {
		l.Info("existing profile not found, adding new profile", "namespace", ns.Name, "quotaProfile", quotaProfile.Name)
		setQuotaProfileLabels(ns, quotaProfile)
		return r.Update(ctx, ns)
	}

	if existingProfile.Spec.NamespaceSelector.MatchName != nil {
		if ns.Name == *existingProfile.Spec.NamespaceSelector.MatchName {
			l.Info("namespace matches name selector", "namespace", ns.Name, "existingProfile", existingProfile.Name)
			setQuotaProfileLabels(ns, existingProfile)
			return r.Update(ctx, ns)
		}
	}

	if existingProfile.Spec.Precedence > quotaProfile.Spec.Precedence || existingProfile.CreationTimestamp.After(quotaProfile.CreationTimestamp.Time) {
		l.Info("keeping existing profile due to higher precedence", "namespace", ns.Name, "existingProfile", existingProfile.Name)
		setQuotaProfileLabels(ns, existingProfile)
		return r.Update(ctx, ns)
	}

	l.Info("updating quota profile label", "namespace", ns.Name, "oldProfile", existingProfile.Name, "newProfile", quotaProfile.Name)
	setQuotaProfileLabels(ns, quotaProfile)

	return r.Update(ctx, ns)
}

func splitProfileID(profileID string) (string, string) {
	parts := strings.Split(profileID, ".")
	if len(parts) != 2 {
		return "", ""
	}
	return parts[0], parts[1]
}

func setQuotaProfileLabels(ns *v1.Namespace, quotaProfile *quotav1alpha1.QuotaProfile) {
	ns.Labels[quotav1alpha1.QuotaProfileLabelKey] = quotaProfile.Namespace + "." + quotaProfile.Name
	ns.Labels[quotav1alpha1.QuotaProfileLastUpdateTimestamp] = fmt.Sprintf("%d", time.Now().UnixMicro())
}

// handleDeletion handles the cleanup when a QuotaProfile is being deleted
func (r *QuotaProfileReconciler) handleDeletion(ctx context.Context, quotaProfile *quotav1alpha1.QuotaProfile) (ctrl.Result, error) {
	l := log.FromContext(ctx)

	// Check if finalizer exists
	if !controllerutil.ContainsFinalizer(quotaProfile, quotav1alpha1.QuotaProfileFinalizer) {
		l.Info("finalizer not found, skipping cleanup", "quotaProfile", quotaProfile.Name)
		return ctrl.Result{}, nil
	}

	// Cleanup logic: Remove quota profile label from all namespaces that were using this profile
	nsList := &v1.NamespaceList{}
	if err := r.List(ctx, nsList); err != nil {
		l.Error(err, "failed to list namespaces during cleanup", "quotaProfile", quotaProfile.Name)
		return ctrl.Result{}, err
	}

	for _, ns := range nsList.Items {
		if ns.Labels == nil {
			continue
		}

		// Check if namespace has this quota profile
		if profileID, ok := ns.Labels[quotav1alpha1.QuotaProfileLabelKey]; ok {
			// Extract namespace and name from profile ID
			profileNs, profileName := splitProfileID(profileID)
			if profileNs == quotaProfile.Namespace && profileName == quotaProfile.Name {
				l.Info("removing quota profile label from namespace", "namespace", ns.Name, "quotaProfile", quotaProfile.Name)
				// Remove the quota profile label
				delete(ns.Labels, quotav1alpha1.QuotaProfileLabelKey)
				delete(ns.Labels, quotav1alpha1.QuotaProfileLastUpdateTimestamp)
				if err := r.Update(ctx, &ns); err != nil {
					l.Error(err, "failed to remove quota profile label from namespace", "namespace", ns.Name)
					return ctrl.Result{}, err
				}
			}
		}
	}

	// Remove finalizer
	l.Info("removing finalizer", "quotaProfile", quotaProfile.Name)
	controllerutil.RemoveFinalizer(quotaProfile, quotav1alpha1.QuotaProfileFinalizer)
	if err := r.Update(ctx, quotaProfile); err != nil {
		l.Error(err, "failed to remove finalizer", "quotaProfile", quotaProfile.Name)
		return ctrl.Result{}, err
	}

	l.Info("successfully cleaned up quota profile", "quotaProfile", quotaProfile.Name)
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *QuotaProfileReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&quotav1alpha1.QuotaProfile{}).
		Named("quotaprofile").
		Complete(r)
}
