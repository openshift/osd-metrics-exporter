package limited_support

import (
	"context"
	"fmt"

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
	limitedSupportConfigMapName      = "limited-support"
	limitedSupportConfigMapNamespace = "openshift-osd-metrics"
)

var log = logf.Log.WithName("controller_limited_support")

// Add creates a new Limited Support Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileLimitedSupportConfigMap{
		client:            mgr.GetClient(),
		scheme:            mgr.GetScheme(),
		metricsAggregator: metrics.GetMetricsAggregator(),
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("limited-support-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource limited-support ConfigMap
	err = c.Watch(&source.Kind{Type: &corev1.ConfigMap{}}, &handler.EnqueueRequestForObject{}, predicate.Funcs{
		CreateFunc: func(createEvent event.CreateEvent) bool {
			return createEvent.Meta.GetName() == limitedSupportConfigMapName && createEvent.Meta.GetNamespace() == limitedSupportConfigMapNamespace

		},
		DeleteFunc: func(deleteEvent event.DeleteEvent) bool {
			return deleteEvent.Meta.GetName() == limitedSupportConfigMapName && deleteEvent.Meta.GetNamespace() == limitedSupportConfigMapNamespace
		},
		UpdateFunc: func(updateEvent event.UpdateEvent) bool {
			return updateEvent.MetaNew.GetName() == limitedSupportConfigMapName && updateEvent.MetaNew.GetNamespace() == limitedSupportConfigMapNamespace
		},
		GenericFunc: func(genericEvent event.GenericEvent) bool {
			return genericEvent.Meta.GetName() == limitedSupportConfigMapName && genericEvent.Meta.GetNamespace() == limitedSupportConfigMapNamespace
		},
	})
	if err != nil {
		return err
	}

	return nil
}

// blank assignment to verify that ReconcileGroup implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileLimitedSupportConfigMap{}

// ReconcileLimitedSupportConfigMap reconciles a ConfigMap object
type ReconcileLimitedSupportConfigMap struct {
	client            client.Client
	scheme            *runtime.Scheme
	metricsAggregator *metrics.AdoptionMetricsAggregator
}

// Reconcile reads that state of the cluster for a ConfigMap object limited-support and makes changes based the contained data
func (r *ReconcileLimitedSupportConfigMap) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling Limited Support ConfigMap")

	// Fetch the ConfigMap openshift-osd-metrics/limited-support
	cfgMap := &corev1.ConfigMap{}
	ns := limitedSupportConfigMapNamespace
	err := r.client.Get(context.TODO(), types.NamespacedName{Namespace: ns, Name: limitedSupportConfigMapName}, cfgMap)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			reqLogger.Info(fmt.Sprintf("Did not find ConfigMap %v", limitedSupportConfigMapName))
			r.metricsAggregator.SetLimitedSupport(false)
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	reqLogger.Info(fmt.Sprintf("Found ConfigMap %v", limitedSupportConfigMapName))
	r.metricsAggregator.SetLimitedSupport(true)
	return reconcile.Result{}, nil
}
