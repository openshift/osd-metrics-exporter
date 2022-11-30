/*
Copyright 2022.
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

// Package clusterrole implements a controller for watching ClusterRole resources and tracking them.
// Deprecated: Since we don't need to track ClusterRole objects anymore this is deprecated.
package clusterrole

import (
	"context"
	"fmt"

	"github.com/openshift/osd-metrics-exporter/controllers/utils"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

var log = logf.Log.WithName("controller_cluster_role")

const (
	finalizer        = "osd-metrics-exporter/finalizer"
	clusterAdminName = "cluster-admin"
)

// ReconcileClusterRole reconciles a ClusterRole object
type ClusterRoleReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// Reconcile reads the ClusterRole object and removes the finalizer if present. It does nothing other than that.
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ClusterRoleReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", req.Namespace, "Request.Name", req.Name)
	reqLogger.Info("Reconciling ClusterRole")

	// Fetch the ClusterRole instance
	instance := &rbacv1.ClusterRole{}
	err := r.Client.Get(ctx, req.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return ctrl.Result{}, err
	}

	if instance.Name != clusterAdminName {
		// This should never happen because we filter out other cluster roles in the predicates
		return ctrl.Result{}, fmt.Errorf("received unknown cluster role: %s", instance.Name)
	}

	if utils.ContainsString(instance.ObjectMeta.Finalizers, finalizer) {
		controllerutil.RemoveFinalizer(instance, finalizer)
		if err := r.Client.Update(context.Background(), instance); err != nil {
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ClusterRoleReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&rbacv1.ClusterRole{}).
		WithEventFilter(predicate.Funcs{
			CreateFunc: func(evt event.CreateEvent) bool {
				return evt.Object.GetName() == clusterAdminName
			},
			DeleteFunc: func(evt event.DeleteEvent) bool {
				return evt.Object.GetName() == clusterAdminName
			},
			UpdateFunc: func(evt event.UpdateEvent) bool {
				return evt.ObjectNew.GetName() == clusterAdminName
			},
			GenericFunc: func(evt event.GenericEvent) bool {
				return evt.Object.GetName() == clusterAdminName
			},
		}).
		Complete(r)
}
