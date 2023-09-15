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
	reqLogger := log.WithValues("Request.Namespace", req.Namespace, "Request.Name", req.Name)
	reqLogger.Info("Reconciling Machines")

	// Fetch the machines in openshift-machine-api
	machine := &machinev1beta1.Machine{}
	err := r.Client.Get(ctx, client.ObjectKey{Namespace: machineNamespace, Name: req.Name}, machine)
	if err != nil {
		if errors.IsNotFound(err) {
			reqLogger.Info("Machine not found. Ignoring")
			return utils.DoNotRequeue()
		}
		reqLogger.Error(err, "An error occurred getting the machine")
		return utils.RequeueWithError(err)
	}
	reqLogger.Info(fmt.Sprintf("Found Machine: %s", machine.Name))

	if machine != nil && machine.Status.Phase != nil && *machine.Status.Phase == "Deleting" {
		return r.evaluateMachine(machine)
	}

	// r.MetricsAggregator.DoSomething(r.ClusterId, true)
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

func (r *MachineReconciler) evaluateMachine(machine *machinev1beta1.Machine) (ctrl.Result, error) {
	// Check Deleting Timestamp. If it's been less than 15m we don't care, requeue for 5m.
	deletedTime := machine.GetDeletionTimestamp().Time
	now := time.Now()
	if !deletedTime.Before(now.Add(-15 * time.Minute)) {
		fmt.Println("It hasn't been 15m yet")
		return utils.RequeueAfter(5 * time.Minute)
	}

	fmt.Println("Evaluating Deleting Machine")

	machineEventList := &corev1.EventList{}
	err := r.Client.List(context.TODO(), machineEventList, &client.ListOptions{Namespace: "openshift-machine-api", FieldSelector: fields.SelectorFromSet(fields.Set{"involvedObject.name": machine.Name})})
	if err != nil {
		return utils.RequeueWithError(err)
	}

	for _, event := range machineEventList.Items {
		if event.Reason == "DrainRequeued" {
			if strings.Contains(event.Message, "error when evicting pods") {
				re := regexp.MustCompile(`pods\/"([\w-]+)" -n "([\w-]+)"`)
				matches := re.FindAllStringSubmatch(event.Message, -1)
				fmt.Println("matches found")

				if len(matches) > 1 && now.Sub(event.LastTimestamp.Time) <= (5*time.Minute) {
					fmt.Println("This Event is the one to use")
					fmt.Println(matches)
					fmt.Println("---")
				}

				// TODO - Do something with these events - probably filter out all of the non-managed namespaces - is that a configmap somewhere on cluster we can get that list from?

				// TODO - Update a metric, something like "CustomerPodsFailingDrain" with the machine name, node and instance labels. (node and instance will be the same value but we provide both labels here so that we can match on whatever the upstream OCP metric has it labeled as)
				// TODO - What should that metric's value be? 1 for failing to drain?
				// TODO - Open Question - do we have to manually clear that metric after the machine finally deletes or does it just stop firing?
			}
		}
	}

	return utils.DoNotRequeue()
}
