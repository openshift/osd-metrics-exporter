package proxy

import (
	"context"
	"fmt"
	"os"

	openshiftapi "github.com/openshift/api/config/v1"
	"github.com/openshift/osd-metrics-exporter/pkg/metrics"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("controller_proxy")

const EnvClusterID = "CLUSTER_ID"

// Add creates a new Proxy Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileProxy{
		client:            mgr.GetClient(),
		scheme:            mgr.GetScheme(),
		metricsAggregator: metrics.GetMetricsAggregator(),
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("proxy-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource Proxy
	err = c.Watch(&source.Kind{Type: &openshiftapi.Proxy{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	return nil
}

// blank assignment to verify that ReconcileProxy implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileProxy{}

// ReconcileProxy reconciles a Proxy object
type ReconcileProxy struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client            client.Client
	scheme            *runtime.Scheme
	metricsAggregator *metrics.AdoptionMetricsAggregator
}

// Reconcile reads that state of the cluster for a Proxy object and makes changes based on the state read
// and what is in the Proxy.Spec
// a Pod as an example
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileProxy) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling Proxy")

	uuid := os.Getenv(EnvClusterID)
	if uuid == "" {
		return reconcile.Result{}, fmt.Errorf("Cluster ID returned as empty string")
	}
	r.metricsAggregator.SetClusterID(uuid)

	// Fetch the Proxy instance
	instance := &openshiftapi.Proxy{}
	err := r.client.Get(context.TODO(), request.NamespacedName, instance)
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
	r.metricsAggregator.SetClusterProxy(proxyHTTP, proxyHTTPS, proxyTrustedCA, proxyEnabled)
	return reconcile.Result{}, nil
}
