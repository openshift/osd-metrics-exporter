/*
Copyright 2024.
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

package cpms

import (
	"context"
	"encoding/json"
	"fmt"

	machinev1 "github.com/openshift/api/machine/v1"
	machinev1beta1 "github.com/openshift/api/machine/v1beta1"
	"github.com/openshift/osd-metrics-exporter/controllers/utils"
	"github.com/openshift/osd-metrics-exporter/pkg/metrics"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	cpmsNamespace = "openshift-machine-api"
	cpmsName      = "cluster"
	logName       = "controller_cpms"
)

// CPMSReconciler reconciles the cpms object
type CPMSReconciler struct {
	client.Client
	Scheme            *runtime.Scheme
	MetricsAggregator *metrics.AdoptionMetricsAggregator
	ClusterId         string
}

func (r *CPMSReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	reqLogger := logf.FromContext(ctx).WithName(logName)
	logf.IntoContext(ctx, reqLogger)
	reqLogger.Info("Reconciling ControlPlaneMachineSet")
	defer func() {
		reqLogger.Info("Reconcile Complete")
	}()
	// Fetch cpms
	cpms := &machinev1.ControlPlaneMachineSet{}
	err := r.Get(ctx, client.ObjectKey{Namespace: cpmsNamespace, Name: cpmsName}, cpms)
	if err != nil {
		if errors.IsNotFound(err) {
			reqLogger.Info("ControlPlaneMachineSet not found.")
			return utils.DoNotRequeue()
		}
		reqLogger.Error(err, "An error occurred getting the ControlPlaneMachineSet")
		return utils.RequeueWithError(err)
	}

	reqLogger.Info("Found ControlPlaneMachineSet")

	// Fetch the instance type from cpms spec
	if cpms.Spec.Template.MachineType != "machines_v1beta1_machine_openshift_io" {
		err := fmt.Errorf(
			"unexpected machine type in Spec.Template.MachineType\nWant: 'machines_v1beta1_machine_openshift_io', got: %s",
			cpms.Spec.Template.MachineType,
		)
		reqLogger.Error(err, "failed to fetch instance type from ControlPlaneMachineSet spec")
		return utils.RequeueWithError(err)
	}
	specRaw := cpms.Spec.Template.OpenShiftMachineV1Beta1Machine.Spec.ProviderSpec.Value.Raw

	var instance_type string
	platform := cpms.Spec.Template.OpenShiftMachineV1Beta1Machine.FailureDomains.Platform
	// the machine template is provider specific
	switch platform {
	case "AWS":
		machineProviderConfig := machinev1beta1.AWSMachineProviderConfig{}
		err := json.Unmarshal(specRaw, &machineProviderConfig)
		if err != nil {
			reqLogger.Error(err, "failed to unmarshal machine config")
			return utils.RequeueWithError(err)
		}
		instance_type = machineProviderConfig.InstanceType
	case "GCP":
		machineProviderConfig := machinev1beta1.GCPMachineProviderSpec{}
		err := json.Unmarshal(specRaw, &machineProviderConfig)
		if err != nil {
			reqLogger.Error(err, "failed to unmarshal machine config")
			return utils.RequeueWithError(err)
		}
		instance_type = machineProviderConfig.MachineType
	default:
		err := fmt.Errorf("unsupported MachineProvider: %s. Supported cloud providers are 'AWS' and 'GCP'", platform)
		reqLogger.Error(err, "failed to fetch instance type from ControlPlaneMachineSet spec")
		return utils.RequeueWithError(err)
	}

	if cpms.Spec.State == "Active" {
		r.MetricsAggregator.SetCPMSEnabled(r.ClusterId, instance_type, true)
	} else {
		r.MetricsAggregator.SetCPMSEnabled(r.ClusterId, instance_type, false)
	}
	return utils.DoNotRequeue()
}

func (r *CPMSReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&machinev1.ControlPlaneMachineSet{}).
		Complete(r)
}
