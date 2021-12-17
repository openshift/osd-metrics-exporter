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
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const (
	userCABundleConfigMapName = "user-ca-bundle"
)

var log = logf.Log.WithName("controller_configmap")

// Add creates a new Group Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileConfigMap{
		client:            mgr.GetClient(),
		scheme:            mgr.GetScheme(),
		metricsAggregator: metrics.GetMetricsAggregator(),
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("configmap-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource Group
	err = c.Watch(&source.Kind{Type: &corev1.ConfigMap{}}, &handler.EnqueueRequestForObject{}, predicate.Funcs{
		CreateFunc: func(createEvent event.CreateEvent) bool {
			return createEvent.Meta.GetName() == userCABundleConfigMapName && createEvent.Meta.GetNamespace() == names.ADDL_TRUST_BUNDLE_CONFIGMAP_NS

		},
		DeleteFunc: func(deleteEvent event.DeleteEvent) bool {
			return deleteEvent.Meta.GetName() == userCABundleConfigMapName && deleteEvent.Meta.GetNamespace() == names.ADDL_TRUST_BUNDLE_CONFIGMAP_NS
		},
		UpdateFunc: func(updateEvent event.UpdateEvent) bool {
			return updateEvent.MetaNew.GetName() == userCABundleConfigMapName && updateEvent.MetaNew.GetNamespace() == names.ADDL_TRUST_BUNDLE_CONFIGMAP_NS
		},
		GenericFunc: func(genericEvent event.GenericEvent) bool {
			return genericEvent.Meta.GetName() == userCABundleConfigMapName && genericEvent.Meta.GetNamespace() == names.ADDL_TRUST_BUNDLE_CONFIGMAP_NS
		},
	})
	if err != nil {
		return err
	}

	return nil
}

// blank assignment to verify that ReconcileGroup implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileConfigMap{}

// ReconcileConfigMap reconciles a ConfigMap object
type ReconcileConfigMap struct {
	client            client.Client
	scheme            *runtime.Scheme
	metricsAggregator *metrics.AdoptionMetricsAggregator
}

// Reconcile reads that state of the cluster for a ConfigMap object and makes changes based the contained data
func (r *ReconcileConfigMap) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling ConfigMap")

	// Fetch the ConfigMap openshift-config/user-ca-bundle
	cfgMap := &corev1.ConfigMap{}
	ns := names.ADDL_TRUST_BUNDLE_CONFIGMAP_NS
	err := r.client.Get(context.TODO(), types.NamespacedName{Namespace: ns, Name: userCABundleConfigMapName}, cfgMap)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	certBundle, _, err := validation.TrustBundleConfigMap(cfgMap)
	if err != nil {
		// Reporting of failed validation is already funneled through CVO via ClusterOperatorDegraded. This is to explicity catch this condition
		// and handle gracefully rather then causing stacktrace.
		if strings.Contains(err.Error(), "failed parsing certificate") {
			reqLogger.Info("failed parsing certificate")
			r.metricsAggregator.SetClusterProxyCAValid(false)
			reqLogger.Info("setting CA valid metric to false")
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}

	countCertBundle := len(certBundle)
	if countCertBundle == 0 {
		reqLogger.Info("No cert bundles found in user-ca-bundle")
		return reconcile.Result{}, nil
	}
	reqLogger.Info(fmt.Sprintf("Found %d cert bundles", countCertBundle))
	for _, cert := range certBundle {
		reqLogger.Info(fmt.Sprintf("Certificate Expiry %d", cert.NotAfter.Unix()))
		r.metricsAggregator.SetClusterProxyCAExpiry(cert.Subject.String(), cert.NotAfter.UTC().Unix())
		r.metricsAggregator.SetClusterProxyCAValid(true)
	}
	return reconcile.Result{}, nil
}
