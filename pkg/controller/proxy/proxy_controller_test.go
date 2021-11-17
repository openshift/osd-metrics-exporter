package proxy

import (
	"context"
	"testing"
	"time"

	"github.com/openshift/osd-metrics-exporter/pkg/metrics"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"

	openshiftapi "github.com/openshift/api/config/v1"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	testName        = "test"
	testNamespace   = "test"
	proxyHTTPLabel  = "http"
	proxyHTTPSLabel = "https"
	proxyCALabel    = "trusted_ca"
	proxyEnabled    = float64(1)
)

func makeTestProxy(name, namespace string, proxySpec openshiftapi.ProxySpec, proxyStatus openshiftapi.ProxyStatus) *openshiftapi.Proxy {
	proxy := &openshiftapi.Proxy{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec:       proxySpec,
		Status:     proxyStatus,
	}
	return proxy
}
func TestReconcileProxy_Reconcile(t *testing.T) {
	for _, tc := range []struct {
		name        string
		proxyStatus openshiftapi.ProxyStatus
		proxySpec   openshiftapi.ProxySpec
	}{
		{
			name: "with ca",
			proxySpec: openshiftapi.ProxySpec{
				TrustedCA: openshiftapi.ConfigMapNameReference{
					Name: "test",
				},
			},
			proxyStatus: openshiftapi.ProxyStatus{
				HTTPProxy:  "http://example.com",
				HTTPSProxy: "https://example.com",
			},
		},
		{
			name:      "no ca",
			proxySpec: openshiftapi.ProxySpec{},
			proxyStatus: openshiftapi.ProxyStatus{
				HTTPProxy:  "http://example.com",
				HTTPSProxy: "https://example.com",
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			metricsAggregator := metrics.NewMetricsAggregator(time.Second)
			done := metricsAggregator.Run()
			defer close(done)
			err := openshiftapi.Install(scheme.Scheme)
			require.NoError(t, err)

			fakeClient := fake.NewFakeClientWithScheme(scheme.Scheme, makeTestProxy(testName, testNamespace, tc.proxySpec, tc.proxyStatus))
			reconciler := ReconcileProxy{
				client:            fakeClient,
				metricsAggregator: metricsAggregator,
			}
			result, err := reconciler.Reconcile(reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: testNamespace,
					Name:      testName,
				},
			})
			require.NoError(t, err)

			// sleep to allow the aggregator to aggregate metrics in the background
			time.Sleep(time.Second * 3)
			require.NoError(t, err)
			require.NotNil(t, result)
			var testProxy openshiftapi.Proxy
			err = fakeClient.Get(context.Background(), types.NamespacedName{Name: testName, Namespace: testNamespace}, &testProxy)
			require.NoError(t, err)
			// setup metric
			metric := metricsAggregator.GetClusterProxyMetric()
			metricWith := metric.With(prometheus.Labels{proxyHTTPLabel: "1", proxyHTTPSLabel: "1", proxyCALabel: "1"})
			// enable metric
			metricWith.Set(proxyEnabled)
			metricEnabled := testutil.ToFloat64(metricWith)
			require.EqualValues(t, 1, metricEnabled, "metric enabled")
			// disable metric
			metricWith.Set(0)
			metricDisabled := testutil.ToFloat64(metricWith)
			require.EqualValues(t, 0, metricDisabled, "metric enabled")

		})
	}
}
