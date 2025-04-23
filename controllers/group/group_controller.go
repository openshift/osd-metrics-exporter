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

package group

import (
	"context"

	userv1 "github.com/openshift/api/user/v1"
	"github.com/openshift/osd-metrics-exporter/controllers/utils"
	"github.com/openshift/osd-metrics-exporter/pkg/metrics"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

const (
	clusterAdminGroupName = "cluster-admins"
	finalizer             = "osd-metrics-exporter/finalizer"
)

var log = logf.Log.WithName("controller_group")

// GroupReconciler reconciles a Group object
type GroupReconciler struct {
	client.Client
	Scheme            *runtime.Scheme
	MetricsAggregator *metrics.AdoptionMetricsAggregator
	ClusterId         string
}

// Reconcile reads that state of the cluster for a Group object and makes changes based on the state read
// and what is in the Group.Spec
func (r *GroupReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", req.Namespace, "Request.Name", req.Name)
	reqLogger.Info("Reconciling Group")

	// Fetch the Group group
	group := &userv1.Group{}
	err := r.Get(ctx, req.NamespacedName, group)
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
	if group.DeletionTimestamp.IsZero() {
		if !utils.ContainsString(group.Finalizers, finalizer) {
			controllerutil.AddFinalizer(group, finalizer)
			if err := r.Update(ctx, group); err != nil {
				return ctrl.Result{}, err
			}
		}
		r.MetricsAggregator.SetClusterAdmin(r.ClusterId, len(group.Users) > 0)
	} else {
		r.MetricsAggregator.SetClusterAdmin(r.ClusterId, false)
		if utils.ContainsString(group.Finalizers, finalizer) {
			controllerutil.RemoveFinalizer(group, finalizer)
			if err := r.Update(ctx, group); err != nil {
				return ctrl.Result{}, err
			}
		}
	}
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *GroupReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&userv1.Group{}).
		WithEventFilter(predicate.Funcs{
			CreateFunc: func(evt event.CreateEvent) bool {
				return evt.Object.GetName() == clusterAdminGroupName
			},
			DeleteFunc: func(evt event.DeleteEvent) bool {
				return evt.Object.GetName() == clusterAdminGroupName
			},
			UpdateFunc: func(evt event.UpdateEvent) bool {
				return evt.ObjectNew.GetName() == clusterAdminGroupName
			},
			GenericFunc: func(evt event.GenericEvent) bool {
				return evt.Object.GetName() == clusterAdminGroupName
			},
		}).
		Complete(r)
}
