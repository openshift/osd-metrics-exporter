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

package machine

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	machinev1beta1 "github.com/openshift/api/machine/v1beta1"
	"github.com/openshift/osd-metrics-exporter/controllers/utils"
	"github.com/openshift/osd-metrics-exporter/pkg/metrics"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	machineNamespace = "openshift-machine-api"
	logName          = "controller_machine"

	// timeBuffer is the delay between when a machine is deleted to when we want to care
	// if a customer's pod is not draining before we start emitting metrics. We don't
	// want to set this too low so that it starts emitting metrics before the pods actually
	// have a chance to drain and reschedule, but want to set this to a time where it
	// might be considered a problem for the node to not be draining properly.
	timeBuffer = 15 * time.Minute

	// defaultDelayInterval is the default time to requeue a machine that's being evaluated
	// so that it can be evaluated again when there's no active metrics being fired.
	defaultDelayInterval = 5 * time.Minute

	// podFailingDrainRecheckInterval is the time we re-evaluate a machine that's actively
	// failing to drain pods. We want this to be less than the default delay interval as
	// we'll want to recheck this more often once this condition is set in order to more
	// appropriately resolve once the condition is fixed.
	podFailingDrainRecheckInterval = 2 * time.Minute
)

var (
	errNoEvents = fmt.Errorf("No Events")
)

// MachineReconciler reconciles a Machine object
type MachineReconciler struct {
	client.Client
	Scheme            *runtime.Scheme
	MetricsAggregator *metrics.AdoptionMetricsAggregator
	ClusterId         string
}

// Reconcile reads that state of the cluster for machine objects and makes changes based the contained data
func (r *MachineReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	reqLogger := logf.FromContext(ctx).WithName(logName)
	logf.IntoContext(ctx, reqLogger)
	reqLogger.Info("Reconciling Machine")
	defer func() {
		reqLogger.Info("Reconcile Complete")
	}()

	// Fetch the machines in openshift-machine-api
	machine := &machinev1beta1.Machine{}
	err := r.Client.Get(ctx, client.ObjectKey{Namespace: machineNamespace, Name: req.Name}, machine)
	if err != nil {
		if errors.IsNotFound(err) {
			reqLogger.Info("Machine not found. Ensuring that no metrics for this machine are leftover")
			// use the req.Name here because the Machine does not exist and will be nil
			r.MetricsAggregator.RemoveMachineMetrics(req.Name)
			return utils.DoNotRequeue()
		}
		reqLogger.Error(err, "An error occurred getting the machine")
		return utils.RequeueWithError(err)
	}

	if machine != nil && machine.Status.Phase != nil && *machine.Status.Phase == "Deleting" {
		reqLogger.Info("Found machine in deleting state. Looking for customer pods failing to delete")
		return r.evaluateDeletingMachine(ctx, machine)
	}
	return utils.DoNotRequeue()
}

// SetupWithManager sets up the controller with the Manager.
func (r *MachineReconciler) SetupWithManager(mgr ctrl.Manager) error {
	indexerFunc := func(rawObj client.Object) []string {
		event := rawObj.(*corev1.Event)
		return []string{event.InvolvedObject.Name}
	}

	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &corev1.Event{}, "involvedObject.name", indexerFunc); err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&machinev1beta1.Machine{}).
		Complete(r)
}

func getMostRecentDrainFailedEvent(eventList *corev1.EventList) (*corev1.Event, error) {
	// if there are no events at all exit now
	if len(eventList.Items) == 0 {
		return nil, errNoEvents
	}

	var newestEvent *corev1.Event

	for i := range eventList.Items {
		event := &eventList.Items[i]
		if !isErrorEvictingPodsEvent(event) {
			continue
		}

		newestEvent = getMostRecentEvent(newestEvent, event)
	}

	return newestEvent, nil
}

// isErrorEvictingPodsEvent determinds whether the event was specifically an error
// evicting the pods
func isErrorEvictingPodsEvent(event *corev1.Event) bool {
	return event.Reason == "DrainRequeued" && strings.Contains(event.Message, "error when evicting pods")
}

// getMostRecentEvent compares the two timestamps and returns the most recently
// occuring event.
func getMostRecentEvent(a, b *corev1.Event) *corev1.Event {
	if a == nil {
		return b
	}
	if a.LastTimestamp.Time.Before(b.LastTimestamp.Time) {
		return b
	}
	return a
}

func parsePodsAndNamespacesFromEvent(event *corev1.Event) (map[string]string, error) {
	re := regexp.MustCompile(`pods\/"([\w-]+)" -n "([\w-]+)"`)
	matches := re.FindAllStringSubmatch(event.Message, -1)

	// store the pod/namespace combinations with the pod as the key because
	// the pod name _should_ generally be unique, where there may be multiple pods
	// in each namespace
	podNamespaces := map[string]string{}

	for _, podMatch := range matches {
		if len(podMatch) != 3 {
			// I don't think this should ever happen, but this prevents trying to access indexes
			// in the match slice that may not exist
			return podNamespaces, fmt.Errorf("Could not get the appropriate amount of matches")
			continue
		}
		// From the regex match we'll always get this podMatch slice with the following format:
		// ['pods/"myPod-aaabbb" -n "namespace"', 'myPod-aaabbb', 'namespace']
		podName := podMatch[1]
		podNamespace := podMatch[2]

		namespaceRe := regexp.MustCompile(`openshift-.*`)
		// We don't generally care about openshift namespaces for customer metrics
		if !namespaceRe.MatchString(podNamespace) {
			podNamespaces[podName] = podNamespace
		}
	}
	return podNamespaces, nil
}

func (r *MachineReconciler) evaluateDeletingMachine(ctx context.Context, machine *machinev1beta1.Machine) (ctrl.Result, error) {
	reqLogger := logf.FromContext(ctx).WithName(logName)

	// Check Deleting Timestamp.
	// If it's been less than the timeBuffer we don't care, requeue for the default delay interval.
	deletedTime := machine.GetDeletionTimestamp().Time
	now := time.Now()
	if !deletedTime.Before(now.Add(-timeBuffer)) {
		reqLogger.Info("Machine was not deleted long enough ago. Requeueing after 5m.")
		return utils.RequeueAfter(defaultDelayInterval)
	}

	reqLogger.Info("Evaluating Deleting Machine")

	machineEventList := &corev1.EventList{}
	// we only want the events related to the machine that we're reconciling on
	err := r.Client.List(ctx, machineEventList, &client.ListOptions{Namespace: "openshift-machine-api", FieldSelector: fields.SelectorFromSet(fields.Set{"involvedObject.name": machine.Name})})
	if err != nil {
		reqLogger.Error(err, "Unable to query events for machine")
		return utils.RequeueWithError(err)
	}

	event, err := getMostRecentDrainFailedEvent(machineEventList)
	if err != nil {
		if err == errNoEvents {
			reqLogger.Info("No events for this machine")
			return utils.RequeueAfter(defaultDelayInterval)
		}
		reqLogger.Info("Unhandled Error getting most recent drain fail event")
		return utils.RequeueAfter(defaultDelayInterval)
	}

	if event == nil {
		reqLogger.Info("No drainRequeued events for this machine")
		// Try again - if there's no drain failures then we just keep requeueing until the machine is deleted and this is a noop
		return utils.RequeueAfter(defaultDelayInterval)
	}

	podNamespaces, err := parsePodsAndNamespacesFromEvent(event)
	if err != nil {
		reqLogger.Error(err, "No namespace pod matches from event", "event", event)
		return utils.RequeueAfter(defaultDelayInterval)
	}

	nodeName := machine.Status.NodeRef.Name
	reqLogger.Info("The following non-OpenShift pods are failing to drain from the machine", "node", nodeName, "pods/namespaces", podNamespaces)

	// Update the metrics for this machine
	r.MetricsAggregator.SetFailingDrainPodsForMachine(machine.Name, podNamespaces, nodeName)

	// Requeue every two minutes, even though the event might not be updated for ~10m we'd rather
	// retry every few minutes to catch the new event within a few cycles than potentially only
	// catch the event 9 minutes after it's updated.
	return utils.RequeueAfter(podFailingDrainRecheckInterval)
}
