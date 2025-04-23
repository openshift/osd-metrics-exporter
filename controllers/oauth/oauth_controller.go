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

package oauth

import (
	"context"

	configv1 "github.com/openshift/api/config/v1"
	"github.com/openshift/osd-metrics-exporter/controllers/utils"
	"github.com/openshift/osd-metrics-exporter/pkg/metrics"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var log = logf.Log.WithName("controller_oauth")

const finalizer = "finalizers.osd.metrics.exporter.openshift.io"

// OAuthReconciler reconciles a OAuth object
type OAuthReconciler struct {
	client.Client
	Scheme            *runtime.Scheme
	MetricsAggregator *metrics.AdoptionMetricsAggregator
}

// Reconcile reads that state of the cluster for a OAuth object and makes changes based on the state read
// and what is in the OAuth.Spec
// a Pod as an example
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *OAuthReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", req.Namespace, "Request.Name", req.Name)
	reqLogger.Info("Reconciling OAuth")

	// Fetch the OAuth instance
	instance := &configv1.OAuth{}
	err := r.Get(ctx, req.NamespacedName, instance)
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

	if instance.DeletionTimestamp.IsZero() {
		if !utils.ContainsString(instance.Finalizers, finalizer) {
			controllerutil.AddFinalizer(instance, finalizer)
			if err := r.Update(ctx, instance); err != nil {
				return ctrl.Result{}, err
			}
		}
		r.MetricsAggregator.SetOAuthIDP(instance.Name, instance.Namespace, instance.Spec.IdentityProviders)
	} else {
		if utils.ContainsString(instance.Finalizers, finalizer) {
			controllerutil.RemoveFinalizer(instance, finalizer)
			if err := r.Update(ctx, instance); err != nil {
				return ctrl.Result{}, err
			}
		}
		r.MetricsAggregator.DeleteOAuthIDP(instance.Name, instance.Namespace)
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *OAuthReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&configv1.OAuth{}).
		Complete(r)
}
