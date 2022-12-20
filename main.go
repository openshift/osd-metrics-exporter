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

package main

import (
	"context"
	"errors"
	"flag"
	"os"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	customMetrics "github.com/openshift/operator-custom-metrics/pkg/metrics"
	operatorConfig "github.com/openshift/osd-metrics-exporter/config"
	"github.com/openshift/osd-metrics-exporter/controllers/clusterrole"
	"github.com/openshift/osd-metrics-exporter/controllers/configmap"
	"github.com/openshift/osd-metrics-exporter/controllers/group"
	"github.com/openshift/osd-metrics-exporter/controllers/limited_support"
	"github.com/openshift/osd-metrics-exporter/controllers/oauth"
	"github.com/openshift/osd-metrics-exporter/controllers/proxy"
	"github.com/openshift/osd-metrics-exporter/pkg/metrics"

	configv1 "github.com/openshift/api/config/v1"
	routev1 "github.com/openshift/api/route/v1"
	userv1 "github.com/openshift/api/user/v1"
	promOperatorv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	_ "k8s.io/client-go/plugin/pkg/client/auth"
)

// Change below variables to serve metrics on different host or port.
var (
	scheme      = runtime.NewScheme()
	setupLog    = ctrl.Log.WithName("setup")
	metricsPort = "8383"
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(corev1.AddToScheme(scheme))
	utilruntime.Must(configv1.Install(scheme))
	utilruntime.Must(promOperatorv1.AddToScheme(scheme))
	utilruntime.Must(rbacv1.AddToScheme(scheme))
	utilruntime.Must(routev1.Install(scheme))
	utilruntime.Must(userv1.Install(scheme))
	// +kubebuilder:scaffold:scheme
}

func main() {
	var enableLeaderElection bool
	var probeAddr string

	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "enable-leader-election", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")

	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		// Disable metrics serving
		MetricsBindAddress: "0",
		Port:               9443,

		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "osd-metrics-exporter-lock",
		Namespace:              "openshift-osd-metrics",
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	setupLog.Info("retrieving cluster id")
	clusterId, err := getClusterID(mgr.GetAPIReader())
	if err != nil {
		setupLog.Error(err, "Failed to retrieve")
		os.Exit(1)
	}

	if err = (&clusterrole.ClusterRoleReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ClusterRole")
		os.Exit(1)
	}

	if err = (&configmap.ConfigMapReconciler{
		Client:            mgr.GetClient(),
		Scheme:            mgr.GetScheme(),
		MetricsAggregator: metrics.GetMetricsAggregator(clusterId),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Configmap")
		os.Exit(1)
	}

	if err = (&group.GroupReconciler{
		Client:            mgr.GetClient(),
		Scheme:            mgr.GetScheme(),
		MetricsAggregator: metrics.GetMetricsAggregator(clusterId),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Group")
		os.Exit(1)
	}

	if err = (&limited_support.LimitedSupportConfigMapReconciler{
		Client:            mgr.GetClient(),
		Scheme:            mgr.GetScheme(),
		MetricsAggregator: metrics.GetMetricsAggregator(clusterId),
		ClusterId:         clusterId,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Limited Support")
		os.Exit(1)
	}

	if err = (&oauth.OAuthReconciler{
		Client:            mgr.GetClient(),
		Scheme:            mgr.GetScheme(),
		MetricsAggregator: metrics.GetMetricsAggregator(clusterId),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "OAuth")
		os.Exit(1)
	}

	if err = (&proxy.ProxyReconciler{
		Client:            mgr.GetClient(),
		Scheme:            mgr.GetScheme(),
		MetricsAggregator: metrics.GetMetricsAggregator(clusterId),
		ClusterId:         clusterId,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Proxy")
		os.Exit(1)
	}

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	// Setup metrics collector
	collector := metrics.GetMetricsAggregator(clusterId)
	done := collector.Run()
	defer close(done)
	metricsConfig := customMetrics.NewBuilder(operatorConfig.OperatorNamespace, operatorConfig.OperatorName).
		WithPath("/metrics").
		WithPort(metricsPort).
		WithServiceMonitor().
		WithCollectors(metrics.GetMetricsAggregator(clusterId).GetMetrics()).
		GetConfig()
	if err = customMetrics.ConfigureMetrics(context.TODO(), *metricsConfig); err != nil {
		setupLog.Error(err, "Failed to run metrics server")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

func getClusterID(client client.Reader) (string, error) {
	cv := &configv1.ClusterVersion{}
	if err := client.Get(context.TODO(), types.NamespacedName{Name: "version"}, cv); err != nil {
		return "", err
	}

	if string(cv.Spec.ClusterID) == "" {
		return "", errors.New("got empty string for cluster id from the ClusterVersion custom resource")
	}

	return string(cv.Spec.ClusterID), nil
}
