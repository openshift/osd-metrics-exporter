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

	"github.com/go-logr/logr"
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
)

var log = logf.Log.WithName("controller_machine")

// MachineReconciler reconciles a Machine object
type MachineReconciler struct {
	client.Client
	Scheme            *runtime.Scheme
	MetricsAggregator *metrics.AdoptionMetricsAggregator
	ClusterId         string
}

// Reconcile reads that state of the cluster for machine objects and makes changes based the contained data
func (r *MachineReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	reqLogger := logf.FromContext(ctx)
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
		ctx = context.WithValue(ctx, "machine", machine)
		ctx = context.WithValue(ctx, "logger", reqLogger)
		return r.evaluateDeletingMachine(ctx)
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

func getMachineFromCtx(ctx context.Context) (*machinev1beta1.Machine, error) {
	machineRaw := ctx.Value("machine")
	if machineRaw == nil {
		return nil, fmt.Errorf("Machine does not exist in context")
	}

	machine, ok := machineRaw.(*machinev1beta1.Machine)
	if !ok {
		return nil, fmt.Errorf("Could not cast machine context value to Machine type")
	}

	return machine, nil
}

func getLoggerFromCtx(ctx context.Context) (logr.Logger, error) {
	loggerRaw := ctx.Value("logger")
	if loggerRaw == nil {
		return logr.Logger{}, fmt.Errorf("Logger does not exist in context")
	}

	logger, ok := loggerRaw.(logr.Logger)
	if !ok {
		return logr.Logger{}, fmt.Errorf("Could not cast logger context value to Logger type")
	}

	return logger, nil
}

func getMostRecentDrainFailedEvent(reqLogger logr.Logger, eventList *corev1.EventList) *corev1.Event {
	// if there are no events at all exit now
	if len(eventList.Items) == 0 {
		reqLogger.Info("No events")
		return nil
	}

	var newestEvent *corev1.Event

	for i := range eventList.Items {
		event := &eventList.Items[i]
		if event.Reason == "DrainRequeued" {
			if strings.Contains(event.Message, "error when evicting pods") {
				if newestEvent == nil {
					newestEvent = event
					continue
				}
				if newestEvent.LastTimestamp.Time.Before(event.LastTimestamp.Time) {
					newestEvent = event
					continue
				}
			}
		}
	}

	return newestEvent
}

func parsePodsAndNamespacesFromEvent(reqLogger logr.Logger, event *corev1.Event) map[string]string {
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
			reqLogger.Error(fmt.Errorf("Could not get the appropriate amount of matches"), "match", podMatch)
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
	return podNamespaces
}

func (r *MachineReconciler) evaluateDeletingMachine(ctx context.Context) (ctrl.Result, error) {
	machine, err := getMachineFromCtx(ctx)
	if err != nil {
		return utils.RequeueWithError(err)
	}

	reqLogger, err := getLoggerFromCtx(ctx)
	if err != nil {
		return utils.RequeueWithError(err)
	}

	// Check Deleting Timestamp. If it's been less than 15m we don't care, requeue for 5m.
	deletedTime := machine.GetDeletionTimestamp().Time
	now := time.Now()
	if !deletedTime.Before(now.Add(-15 * time.Minute)) {
		reqLogger.Info("Machine was not deleted long enough ago. Requeueing after 5m.")
		return utils.RequeueAfter(5 * time.Minute)
	}

	reqLogger.Info("Evaluating Deleting Machine")

	machineEventList := &corev1.EventList{}
	// we only want the events related to the machine that we're reconciling on
	err = r.Client.List(ctx, machineEventList, &client.ListOptions{Namespace: "openshift-machine-api", FieldSelector: fields.SelectorFromSet(fields.Set{"involvedObject.name": machine.Name})})
	if err != nil {
		reqLogger.Error(err, "Unable to query events for machine")
		return utils.RequeueWithError(err)
	}

	event := getMostRecentDrainFailedEvent(reqLogger, machineEventList)
	if event == nil {
		reqLogger.Info("No events returned for this machine")
		// Try again - if there's no drain failures then we just keep requeueing until the machine is deleted and this is a noop
		return utils.RequeueAfter(5 * time.Minute)
	}
	// TODO - do we need to ensure that the event happened within a specific timeframe? Like, within the last 15 minutes or something?

	podNamespaces := parsePodsAndNamespacesFromEvent(reqLogger, event)

	nodeName := machine.Status.NodeRef.Name
	reqLogger.Info("The following pods are failing to drain from the machine", "node", nodeName, "pods/namespaces", podNamespaces)

	// Update the metrics for this machine
	r.MetricsAggregator.SetFailingDrainPodsForMachine(machine.Name, podNamespaces, nodeName)

	// Requeue every two minutes, even though the event might not be updated for ~10m we'd rather
	// retry every few minutes to catch the new event within a few cycles than potentially only
	// catch the event 9 minutes after it's updated.
	return utils.RequeueAfter(2 * time.Minute)
}
