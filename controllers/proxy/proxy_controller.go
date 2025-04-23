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

package proxy

import (
	"context"

	configv1 "github.com/openshift/api/config/v1"
	"github.com/openshift/osd-metrics-exporter/pkg/metrics"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var log = logf.Log.WithName("controller_proxy")

// ProxyReconciler reconciles a Proxy object
type ProxyReconciler struct {
	client.Client
	Scheme            *runtime.Scheme
	MetricsAggregator *metrics.AdoptionMetricsAggregator
	ClusterId         string
}

// Reconcile reads that state of the cluster for a Proxy object and makes changes based on the state read
// and what is in the Proxy.Spec
// a Pod as an example
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ProxyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", req.Namespace, "Request.Name", req.Name)
	reqLogger.Info("Reconciling Proxy")

	r.MetricsAggregator.SetClusterID(r.ClusterId)

	// Fetch the Proxy instance
	instance := &configv1.Proxy{}
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
	// init metics
	var proxyHTTP = "0"
	var proxyHTTPS = "0"
	var proxyTrustedCA = "0"
	var proxyEnabled = 0
	// when http proxy in status
	if instance.Status.HTTPProxy != "" {
		proxyHTTP = "1"
		proxyEnabled = 1
	}
	//when https proxy in status
	if instance.Status.HTTPSProxy != "" {
		proxyHTTPS = "1"
		proxyEnabled = 1
	}
	//when trusted ca in spec
	if instance.Spec.TrustedCA.Name != "" {
		proxyTrustedCA = "1"
	}
	// aggregate metrics
	r.MetricsAggregator.SetClusterProxy(r.ClusterId, proxyHTTP, proxyHTTPS, proxyTrustedCA, proxyEnabled)
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ProxyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&configv1.Proxy{}).
		Complete(r)
}
