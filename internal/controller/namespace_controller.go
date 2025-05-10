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
	"strconv"
	"strings"

	quotav1alpha1 "github.com/abdullah599/namespace-quota-operator/api/v1alpha1"
	"github.com/go-logr/logr"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// NamespaceReconciler reconciles a Namespace object
type NamespaceReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	log logr.Logger
}

// +kubebuilder:rbac:groups=dev.operator,resources=namespaces,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=dev.operator,resources=namespaces/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=dev.operator,resources=namespaces/finalizers,verbs=update

// +kubebuilder:rbac:groups="",resources=resourcequotas,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=limitranges,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Namespace object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.20.0/pkg/reconcile
func (r *NamespaceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {

	r.log = log.FromContext(ctx)
	ns := &v1.Namespace{}
	if err := r.Get(ctx, req.NamespacedName, ns); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// todo: remove this as we are using the predicate to filter the namespaces that have the quota profile label
	if _, exists := ns.Labels[quotav1alpha1.QuotaProfileLabelKey]; !exists {
		return ctrl.Result{}, nil
	}

	profileID := ns.Labels[quotav1alpha1.QuotaProfileLabelKey]
	profileNamespace, profileName := splitProfileID(profileID)

	profile := &quotav1alpha1.QuotaProfile{}
	if err := r.Get(ctx, types.NamespacedName{Namespace: profileNamespace, Name: profileName}, profile); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if err := r.reconcileQuotaProfile(ctx, *profile, ns.Name); err != nil {
		r.log.Error(err, "failed to reconcile quota profile")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *NamespaceReconciler) reconcileQuotaProfile(ctx context.Context, q quotav1alpha1.QuotaProfile, namespace string) error {

	if err := r.reconcileResourceQuotas(ctx, q, namespace); err != nil {
		r.log.Error(err, "failed to reconcile resource quotas")
		return err
	}

	if err := r.reconcileLimitRanges(ctx, q, namespace); err != nil {
		r.log.Error(err, "failed to reconcile limit ranges")
		return err
	}

	return nil
}

func (r *NamespaceReconciler) reconcileResourceQuotas(ctx context.Context, q quotav1alpha1.QuotaProfile, namespace string) error {

	rqs := &v1.ResourceQuotaList{}

	r.List(ctx, rqs, client.InNamespace(namespace))

	if len(rqs.Items) != 0 {
		for _, rg := range rqs.Items {
			if _, exists := rg.Labels[quotav1alpha1.QuotaProfileLabelKey]; !exists {
				continue
			}

			if rg.Labels[quotav1alpha1.QuotaProfileLabelKey] != getProfileID(q.Namespace, q.Name) {
				r.Delete(ctx, &rg)
				continue
			}

			if rg.Labels[quotav1alpha1.QuotaProfileLabelKey] == getProfileID(q.Namespace, q.Name) {
				index, err := getResourceQuotaIndex(rg.Name)
				if err != nil {
					r.log.Error(err, "failed to split resource quota ID")
					continue
				}

				if index >= len(q.Spec.ResourceQuotaSpecs) {
					r.Delete(ctx, &rg)
					continue
				}

				rg.Spec = *q.Spec.ResourceQuotaSpecs[index].DeepCopy()
				if err := r.Update(ctx, &rg); err != nil {
					r.log.Error(err, "failed to update resource quota")
				}
			}
		}
	} else {
		for i, rqspec := range q.Spec.ResourceQuotaSpecs {
			rq := &v1.ResourceQuota{}
			rq.Name = getResourceQuotaID(q.Namespace, q.Name, strconv.Itoa(i))
			rq.Namespace = namespace
			rq.Labels = map[string]string{quotav1alpha1.QuotaProfileLabelKey: getProfileID(q.Namespace, q.Name)}
			rq.Spec = *rqspec.DeepCopy()
			if err := r.Create(ctx, rq); err != nil {
				r.log.Error(err, "failed to create resource quota")
			}
		}
	}

	return nil
}

func (r *NamespaceReconciler) reconcileLimitRanges(ctx context.Context, q quotav1alpha1.QuotaProfile, namespace string) error {
	lrs := &v1.LimitRangeList{}

	r.List(ctx, lrs, client.InNamespace(namespace))

	if len(lrs.Items) != 0 {
		for _, lr := range lrs.Items {
			if _, exists := lr.Labels[quotav1alpha1.QuotaProfileLabelKey]; !exists {
				continue
			}

			if lr.Labels[quotav1alpha1.QuotaProfileLabelKey] != getProfileID(q.Namespace, q.Name) {
				r.Delete(ctx, &lr)
				continue
			}

			if lr.Labels[quotav1alpha1.QuotaProfileLabelKey] == getProfileID(q.Namespace, q.Name) {
				index, err := getLimitRangeIndex(lr.Name)
				if err != nil {
					r.log.Error(err, "failed to split limit range ID")
					continue
				}

				if index >= len(q.Spec.ResourceQuotaSpecs) {
					r.Delete(ctx, &lr)
					continue
				}

				lr.Spec = *q.Spec.LimitRangeSpecs[index].DeepCopy()
				if err := r.Update(ctx, &lr); err != nil {
					r.log.Error(err, "failed to update limit range")
				}
			}
		}
	} else {
		for i, lrSpec := range q.Spec.LimitRangeSpecs {
			lr := &v1.LimitRange{}
			lr.Name = getLimitRangeID(q.Namespace, q.Name, strconv.Itoa(i))
			lr.Namespace = namespace
			lr.Labels = map[string]string{quotav1alpha1.QuotaProfileLabelKey: getProfileID(q.Namespace, q.Name)}
			lr.Spec = *lrSpec.DeepCopy()
			if err := r.Create(ctx, lr); err != nil {
				r.log.Error(err, "failed to create limit range")
			}
		}
	}
	return nil
}

// getResourceQuotaIndex splits the resource quota ID into namespace,  profile name, and index
func getResourceQuotaIndex(id string) (int, error) {
	parts := strings.Split(id, "-")

	index, err := strconv.Atoi(parts[len(parts)-1])
	if err != nil {
		return -1, fmt.Errorf("invalid index: %s", parts[len(parts)-1])
	}

	return index, nil
}

// getLimitRangeIndex splits the limit range ID into namespace,  profile name, and index
func getLimitRangeIndex(id string) (int, error) {
	parts := strings.Split(id, "-")
	if len(parts) != 4 {
		return -1, fmt.Errorf("invalid limit range ID: %s. Expected format: <namespace>-<profile>-<index>-lr", id)
	}

	index, err := strconv.Atoi(parts[len(parts)-1])
	if err != nil {
		return -1, fmt.Errorf("invalid index: %s", parts[len(parts)-1])
	}

	return index, nil
}

func getProfileID(namespace, profile string) string {
	return fmt.Sprintf("%s.%s", namespace, profile)
}

func getResourceQuotaID(namespace, profile, index string) string {
	return fmt.Sprintf("%s-%s-%s-rq", namespace, profile, index)
}

func getLimitRangeID(namespace, profile, index string) string {
	return fmt.Sprintf("%s-%s-%s-lr", namespace, profile, index)
}

// SetupWithManager sets up the controller with the Manager.
func (r *NamespaceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1.Namespace{}, builder.WithPredicates(predicate.NewPredicateFuncs(func(obj client.Object) bool {
			_, exists := obj.GetLabels()[quotav1alpha1.QuotaProfileLabelKey]
			return exists
		}))).
		Named("namespace").
		Complete(r)
}
