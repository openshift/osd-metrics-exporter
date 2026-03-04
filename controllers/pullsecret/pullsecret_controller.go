/*
Copyright 2025.
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

package pullsecret

import (
	"context"
	"encoding/json"
	"fmt"

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
	pullSecretName      = "pull-secret"
	pullSecretNamespace = "openshift-config" // #nosec G101 -- this is a namespace, not a credential
	dockerConfigJSONKey = ".dockerconfigjson"
)

var expectedRegistries = []string{
	"cloud.openshift.com",
	"quay.io",
	"registry.redhat.io",
	"registry.connect.redhat.com",
}

var log = logf.Log.WithName("controller_pullsecret")

type dockerConfigJSON struct {
	Auths map[string]dockerConfigAuth `json:"auths"`
}

// dockerConfigAuth represents a single registry auth entry
type dockerConfigAuth struct {
	Auth string `json:"auth"`
}

// PullSecretReconciler reconciles the cluster pull secret
type PullSecretReconciler struct {
	client.Client
	Scheme            *runtime.Scheme
	MetricsAggregator *metrics.AdoptionMetricsAggregator
	ClusterId         string
}

// Reconcile reads the pull secret and validates its structure and registry entries
func (r *PullSecretReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", req.Namespace, "Request.Name", req.Name)
	reqLogger.Info("Reconciling pull secret")

	// Fetch the pull secret
	secret := &corev1.Secret{}
	err := r.Get(ctx, types.NamespacedName{Namespace: pullSecretNamespace, Name: pullSecretName}, secret)
	if err != nil {
		if errors.IsNotFound(err) {
			reqLogger.Info("Pull secret not found, marking as invalid")
			r.MetricsAggregator.SetPullSecretValid(r.ClusterId, false)
			return reconcile.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	valid, reason := validatePullSecret(secret)
	if !valid {
		reqLogger.Info(fmt.Sprintf("Pull secret is invalid: %s", reason))
		r.MetricsAggregator.SetPullSecretValid(r.ClusterId, false)
		return ctrl.Result{}, nil
	}

	reqLogger.Info("Pull secret is valid")
	r.MetricsAggregator.SetPullSecretValid(r.ClusterId, true)
	return ctrl.Result{}, nil
}

// validatePullSecret checks integrity and presence of expected registries.
func validatePullSecret(secret *corev1.Secret) (bool, string) {
	data, ok := secret.Data[dockerConfigJSONKey]
	if !ok {
		return false, fmt.Sprintf("secret is missing %s key", dockerConfigJSONKey)
	}

	var config dockerConfigJSON
	if err := json.Unmarshal(data, &config); err != nil {
		return false, fmt.Sprintf("failed to parse %s: %v", dockerConfigJSONKey, err)
	}

	if len(config.Auths) == 0 {
		return false, "auths map is empty"
	}

	// Check that each expected registry is present with a non-empty auth token
	for _, registry := range expectedRegistries {
		auth, ok := config.Auths[registry]
		if !ok {
			return false, fmt.Sprintf("missing expected registry: %s", registry)
		}
		if auth.Auth == "" {
			return false, fmt.Sprintf("empty auth token for registry: %s", registry)
		}
	}

	return true, ""
}

// SetupWithManager sets up the controller with the Manager.
func (r *PullSecretReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Secret{}).
		WithEventFilter(predicate.Funcs{
			CreateFunc: func(evt event.CreateEvent) bool {
				return evt.Object.GetName() == pullSecretName && evt.Object.GetNamespace() == pullSecretNamespace
			},
			DeleteFunc: func(evt event.DeleteEvent) bool {
				return evt.Object.GetName() == pullSecretName && evt.Object.GetNamespace() == pullSecretNamespace
			},
			UpdateFunc: func(evt event.UpdateEvent) bool {
				return evt.ObjectNew.GetName() == pullSecretName && evt.ObjectNew.GetNamespace() == pullSecretNamespace
			},
			GenericFunc: func(evt event.GenericEvent) bool {
				return evt.Object.GetName() == pullSecretName && evt.Object.GetNamespace() == pullSecretNamespace
			},
		}).
		Complete(r)
}
