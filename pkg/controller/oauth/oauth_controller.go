package oauth

import (
	"context"

	openshiftapi "github.com/openshift/api/config/v1"
	"github.com/openshift/osd-metrics-exporter/pkg/metrics"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("controller_oauth")

const (
	finalizer = "finalizers.osd.metrics.exporter.openshift.io"
)

// Add creates a new OAuth Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileOAuth{
		client:            mgr.GetClient(),
		scheme:            mgr.GetScheme(),
		metricsAggregator: metrics.GetMetricsAggregator(),
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("oauth-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource OAuth
	err = c.Watch(&source.Kind{Type: &openshiftapi.OAuth{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	return nil
}

// blank assignment to verify that ReconcileOAuth implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileOAuth{}

// ReconcileOAuth reconciles a OAuth object
type ReconcileOAuth struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client            client.Client
	scheme            *runtime.Scheme
	metricsAggregator *metrics.AdoptionMetricsAggregator
}

// Reconcile reads that state of the cluster for a OAuth object and makes changes based on the state read
// and what is in the OAuth.Spec
// a Pod as an example
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileOAuth) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling OAuth")

	// Fetch the OAuth instance
	instance := &openshiftapi.OAuth{}
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

	if instance.ObjectMeta.DeletionTimestamp.IsZero() {
		if !containsString(instance.ObjectMeta.Finalizers, finalizer) {
			controllerutil.AddFinalizer(instance, finalizer)
			if err := r.client.Update(context.Background(), instance); err != nil {
				return reconcile.Result{}, err
			}
		}
		r.metricsAggregator.SetOAuthIDP(instance.Name, instance.Namespace, instance.Spec.IdentityProviders)
	} else {
		if containsString(instance.ObjectMeta.Finalizers, finalizer) {
			controllerutil.RemoveFinalizer(instance, finalizer)
			if err := r.client.Update(context.Background(), instance); err != nil {
				return reconcile.Result{}, err
			}
		}
		r.metricsAggregator.DeleteOAuthIDP(instance.Name, instance.Namespace)
	}

	return reconcile.Result{}, nil
}

func containsString(stringArray []string, candidate string) bool {
	for _, s := range stringArray {
		if s == candidate {
			return true
		}
	}
	return false
}
