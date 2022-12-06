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

package limited_support

import (
	"context"
	"fmt"
	"os"

	"github.com/openshift/osd-metrics-exporter/pkg/metrics"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

const (
	limitedSupportConfigMapName      = "limited-support"
	limitedSupportConfigMapNamespace = "openshift-osd-metrics"
)

const EnvClusterID = "CLUSTER_ID"

var log = logf.Log.WithName("controller_limited_support")

// LimitedSupportConfigMapReconciler reconciles a ConfigMap object
type LimitedSupportConfigMapReconciler struct {
	client.Client
	Scheme            *runtime.Scheme
	MetricsAggregator *metrics.AdoptionMetricsAggregator
}

// Reconcile reads that state of the cluster for a ConfigMap object limited-support and makes changes based the contained data
func (r *LimitedSupportConfigMapReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", req.Namespace, "Request.Name", req.Name)
	reqLogger.Info("Reconciling Limited Support ConfigMap")

	// Fetch the ConfigMap openshift-osd-metrics/limited-support
	uuid := os.Getenv(EnvClusterID)
	cfgMap := &corev1.ConfigMap{}
	ns := limitedSupportConfigMapNamespace
	err := r.Client.Get(ctx, types.NamespacedName{Namespace: ns, Name: limitedSupportConfigMapName}, cfgMap)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			reqLogger.Info(fmt.Sprintf("Did not find ConfigMap %v", limitedSupportConfigMapName))
			r.MetricsAggregator.SetLimitedSupport(uuid, false)
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return ctrl.Result{}, err
	}

	reqLogger.Info(fmt.Sprintf("Found ConfigMap %v", limitedSupportConfigMapName))
	r.MetricsAggregator.SetLimitedSupport(uuid, true)
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *LimitedSupportConfigMapReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.ConfigMap{}).
		WithEventFilter(predicate.Funcs{
			CreateFunc: func(evt event.CreateEvent) bool {
				return evt.Object.GetName() == limitedSupportConfigMapName && evt.Object.GetNamespace() == limitedSupportConfigMapNamespace
			},
			DeleteFunc: func(evt event.DeleteEvent) bool {
				return evt.Object.GetName() == limitedSupportConfigMapName && evt.Object.GetNamespace() == limitedSupportConfigMapNamespace
			},
			UpdateFunc: func(evt event.UpdateEvent) bool {
				return evt.ObjectNew.GetName() == limitedSupportConfigMapName && evt.ObjectNew.GetNamespace() == limitedSupportConfigMapNamespace
			},
			GenericFunc: func(evt event.GenericEvent) bool {
				return evt.Object.GetName() == limitedSupportConfigMapName && evt.Object.GetNamespace() == limitedSupportConfigMapNamespace
			},
		}).
		Complete(r)
}
