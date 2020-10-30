package group

import (
	"context"

	userv1 "github.com/openshift/api/user/v1"
	"github.com/openshift/osd-metrics-exporter/pkg/controller/utils"
	"github.com/openshift/osd-metrics-exporter/pkg/metrics"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const (
	clusterAdminGroupName = "cluster-admins"
	finalizer             = "osd-metrics-exporter/finalizer"
)

var log = logf.Log.WithName("controller_group")

// Add creates a new Group Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileGroup{
		client:            mgr.GetClient(),
		scheme:            mgr.GetScheme(),
		metricsAggregator: metrics.GetMetricsAggregator(),
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("group-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource Group
	err = c.Watch(&source.Kind{Type: &userv1.Group{}}, &handler.EnqueueRequestForObject{}, predicate.Funcs{
		CreateFunc: func(createEvent event.CreateEvent) bool {
			return createEvent.Meta.GetName() == clusterAdminGroupName
		},
		DeleteFunc: func(deleteEvent event.DeleteEvent) bool {
			return deleteEvent.Meta.GetName() == clusterAdminGroupName
		},
		UpdateFunc: func(updateEvent event.UpdateEvent) bool {
			return updateEvent.MetaNew.GetName() == clusterAdminGroupName
		},
		GenericFunc: func(genericEvent event.GenericEvent) bool {
			return genericEvent.Meta.GetName() == clusterAdminGroupName
		},
	})
	if err != nil {
		return err
	}

	return nil
}

// blank assignment to verify that ReconcileGroup implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileGroup{}

// ReconcileGroup reconciles a Group object
type ReconcileGroup struct {
	client            client.Client
	scheme            *runtime.Scheme
	metricsAggregator *metrics.AdoptionMetricsAggregator
}

// Reconcile reads that state of the cluster for a Group object and makes changes based on the state read
// and what is in the Group.Spec
func (r *ReconcileGroup) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling Group")

	// Fetch the Group group
	group := &userv1.Group{}
	err := r.client.Get(context.TODO(), request.NamespacedName, group)
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
	if group.ObjectMeta.DeletionTimestamp.IsZero() {
		if !utils.ContainsString(group.Finalizers, finalizer) {
			controllerutil.AddFinalizer(group, finalizer)
			if err := r.client.Update(context.Background(), group); err != nil {
				return reconcile.Result{}, err
			}
		}
		r.metricsAggregator.SetClusterAdmin(len(group.Users) > 0)
	} else {
		r.metricsAggregator.SetClusterAdmin(false)
		if utils.ContainsString(group.Finalizers, finalizer) {
			controllerutil.RemoveFinalizer(group, finalizer)
			if err := r.client.Update(context.Background(), group); err != nil {
				return reconcile.Result{}, err
			}
		}
	}
	return reconcile.Result{}, nil
}
