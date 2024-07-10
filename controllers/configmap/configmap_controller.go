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

package configmap

import (
	"context"
	"fmt"
	"strings"

	"github.com/openshift/cluster-network-operator/pkg/names"
	"github.com/openshift/cluster-network-operator/pkg/util/validation"
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
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	userCABundleConfigMapName = "user-ca-bundle"
)

var log = logf.Log.WithName("controller_configmap")

// ConfigMapReconciler reconciles a ConfigMap object
type ConfigMapReconciler struct {
	client.Client
	Scheme            *runtime.Scheme
	MetricsAggregator *metrics.AdoptionMetricsAggregator
	ClusterId         string
}

// Reconcile reads that state of the cluster for a ConfigMap object and makes changes based the contained data
func (r *ConfigMapReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", req.Namespace, "Request.Name", req.Name)
	reqLogger.Info("Reconciling ConfigMap")

	// Fetch the ConfigMap openshift-config/user-ca-bundle
	cfgMap := &corev1.ConfigMap{}
	ns := names.ADDL_TRUST_BUNDLE_CONFIGMAP_NS
	err := r.Client.Get(ctx, types.NamespacedName{Namespace: ns, Name: userCABundleConfigMapName}, cfgMap)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return ctrl.Result{}, err
	}

	certBundle, _, err := validation.TrustBundleConfigMap(cfgMap)
	if err != nil {
		// Reporting of failed validation is already funneled through CVO via ClusterOperatorDegraded. This is to explicitly catch this condition
		// and handle gracefully rather then causing stacktrace.
		if strings.Contains(err.Error(), "failed parsing certificate") {
			reqLogger.Info("failed parsing certificate")
			r.MetricsAggregator.SetClusterProxyCAValid(r.ClusterId, false)
			reqLogger.Info("setting CA valid metric to false")
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	countCertBundle := len(certBundle)
	if countCertBundle == 0 {
		reqLogger.Info("No cert bundles found in user-ca-bundle")
		return ctrl.Result{}, nil
	}
	reqLogger.Info(fmt.Sprintf("Found %d cert bundles", countCertBundle))
	for _, cert := range certBundle {
		reqLogger.Info(fmt.Sprintf("Certificate Expiry %d", cert.NotAfter.Unix()))
		r.MetricsAggregator.SetClusterProxyCAExpiry(r.ClusterId, cert.Subject.String(), cert.NotAfter.UTC().Unix())
		r.MetricsAggregator.SetClusterProxyCAValid(r.ClusterId, true)
	}
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ConfigMapReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.ConfigMap{}).
		WithEventFilter(predicate.Funcs{
			CreateFunc: func(evt event.CreateEvent) bool {
				return evt.Object.GetName() == userCABundleConfigMapName && evt.Object.GetNamespace() == names.ADDL_TRUST_BUNDLE_CONFIGMAP_NS

			},
			DeleteFunc: func(evt event.DeleteEvent) bool {
				return evt.Object.GetName() == userCABundleConfigMapName && evt.Object.GetNamespace() == names.ADDL_TRUST_BUNDLE_CONFIGMAP_NS
			},
			UpdateFunc: func(evt event.UpdateEvent) bool {
				return evt.ObjectNew.GetName() == userCABundleConfigMapName && evt.ObjectNew.GetNamespace() == names.ADDL_TRUST_BUNDLE_CONFIGMAP_NS
			},
			GenericFunc: func(evt event.GenericEvent) bool {
				return evt.Object.GetName() == userCABundleConfigMapName && evt.Object.GetNamespace() == names.ADDL_TRUST_BUNDLE_CONFIGMAP_NS
			},
		}).
		Complete(r)
}
