package configmap

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/openshift/osd-metrics-exporter/pkg/metrics"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	userCABundle    = "user-ca-bundle"
	openshiftConfig = "openshift-config"
	caBundleCRT     = "ca-bundle.crt"
)

var testCA = `
-----BEGIN CERTIFICATE-----
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
e7mqHDqz9C6H
-----END CERTIFICATE-----
`

func makeTestConfigMap(name, namespace string, caData map[string]string) *corev1.ConfigMap {
	cfgmap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Data:       caData,
	}

	return cfgmap
}

func makeTestCAData(k string, v string) map[string]string {
	m := make(map[string]string)
	m[k] = v
	return m
}

func TestReconcileConfigMap_Reconcile(t *testing.T) {
	for _, tc := range []struct {
		name            string
		cfgMapData      map[string]string
		expectedResults string
	}{
		{
			name:       "user-ca-bundle exists",
			cfgMapData: makeTestCAData(caBundleCRT, testCA),
			expectedResults: `
# HELP cluster_proxy_ca_expiry_timestamp Indicates cluster proxy CA expiry unix timestamp in UTC
# TYPE cluster_proxy_ca_expiry_timestamp gauge
cluster_proxy_ca_expiry_timestamp{name="osd_exporter",subject="O=Default Company Ltd,L=Default City,C=XX"} 1.734086723e+09
`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			metricsAggregator := metrics.NewMetricsAggregator(time.Second)
			done := metricsAggregator.Run()
			defer close(done)
			err := corev1.AddToScheme(scheme.Scheme)
			require.NoError(t, err)

			testConfigMap := makeTestConfigMap(userCABundle, openshiftConfig, tc.cfgMapData)
			fakeClient := fake.NewFakeClientWithScheme(scheme.Scheme, testConfigMap)
			reconciler := ReconcileConfigMap{
				client:            fakeClient,
				metricsAggregator: metricsAggregator,
			}
			result, err := reconciler.Reconcile(reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: openshiftConfig,
					Name:      userCABundle,
				},
			})

			// sleep to allow the aggregator to aggregate metrics in the background
			time.Sleep(time.Second * 3)
			require.NoError(t, err)
			require.NotNil(t, result)
			var testCfgMap corev1.ConfigMap
			err = fakeClient.Get(context.Background(), types.NamespacedName{Name: userCABundle, Namespace: openshiftConfig}, &testCfgMap)
			require.NoError(t, err)
			metric := metricsAggregator.GetClusterProxyCAExpiryMetrics()
			err = testutil.CollectAndCompare(metric, strings.NewReader(tc.expectedResults))
			require.NoError(t, err)
		})
	}

	for _, tc := range []struct {
		name            string
		cfgMapData      map[string]string
		expectedResults string
	}{
		{
			name:       "user-ca-bundle invalid PEM",
			cfgMapData: makeTestCAData(caBundleCRT, "derp"),
			expectedResults: `
# HELP cluster_proxy_ca_valid Indicates if cluster proxy CA valid
# TYPE cluster_proxy_ca_valid gauge
cluster_proxy_ca_valid{name="osd_exporter"} 0
`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			metricsAggregator := metrics.NewMetricsAggregator(time.Second)
			done := metricsAggregator.Run()
			defer close(done)
			err := corev1.AddToScheme(scheme.Scheme)
			require.NoError(t, err)

			testConfigMap := makeTestConfigMap(userCABundle, openshiftConfig, tc.cfgMapData)
			fakeClient := fake.NewFakeClientWithScheme(scheme.Scheme, testConfigMap)
			reconciler := ReconcileConfigMap{
				client:            fakeClient,
				metricsAggregator: metricsAggregator,
			}
			result, err := reconciler.Reconcile(reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: openshiftConfig,
					Name:      userCABundle,
				},
			})

			// sleep to allow the aggregator to aggregate metrics in the background
			time.Sleep(time.Second * 3)
			require.NoError(t, err)
			require.NotNil(t, result)
			var testCfgMap corev1.ConfigMap
			err = fakeClient.Get(context.Background(), types.NamespacedName{Name: userCABundle, Namespace: openshiftConfig}, &testCfgMap)
			require.NoError(t, err)
			metric := metricsAggregator.GetClusterProxyCAValidMetrics()
			err = testutil.CollectAndCompare(metric, strings.NewReader(tc.expectedResults))
			require.NoError(t, err)
		})
	}
}
