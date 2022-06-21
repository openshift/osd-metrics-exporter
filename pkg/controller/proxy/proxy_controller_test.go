package proxy

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"

	openshiftapi "github.com/openshift/api/config/v1"
	"github.com/openshift/osd-metrics-exporter/pkg/metrics"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	testName      = "test"
	testNamespace = "test"
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
		name                     string
		proxyStatus              openshiftapi.ProxyStatus
		proxySpec                openshiftapi.ProxySpec
		testEnvClusterID         string
		expectedClusterIDResults string
		expectedProxyResults     string
	}{
		{
			name: "with ca and cluster id environment non empty string",
			proxySpec: openshiftapi.ProxySpec{
				TrustedCA: openshiftapi.ConfigMapNameReference{
					Name: "test",
				},
			},
			proxyStatus: openshiftapi.ProxyStatus{
				HTTPProxy:  "http://example.com",
				HTTPSProxy: "https://example.com",
			},
			testEnvClusterID: "i-am-a-cluster-id",
			expectedClusterIDResults: `
# HELP cluster_id Indicates the cluster id
# TYPE cluster_id gauge
cluster_id{_id="i-am-a-cluster-id",name="osd_exporter"} 1
	`,
			expectedProxyResults: `
# HELP cluster_proxy Indicates cluster proxy state
# TYPE cluster_proxy gauge
cluster_proxy{http="1",https="1",name="osd_exporter",trusted_ca="1"} 1
`,
		},
		{
			name:      "no ca and cluster id environment non empty string",
			proxySpec: openshiftapi.ProxySpec{},
			proxyStatus: openshiftapi.ProxyStatus{
				HTTPProxy:  "http://example.com",
				HTTPSProxy: "https://example.com",
			},
			testEnvClusterID: "i-am-a-cluster-id",
			expectedClusterIDResults: `
# HELP cluster_id Indicates the cluster id
# TYPE cluster_id gauge
cluster_id{_id="i-am-a-cluster-id",name="osd_exporter"} 1
	`,
			expectedProxyResults: `
# HELP cluster_proxy Indicates cluster proxy state
# TYPE cluster_proxy gauge
cluster_proxy{http="1",https="1",name="osd_exporter",trusted_ca="0"} 1
`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			os.Setenv(EnvClusterID, tc.testEnvClusterID)
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

			metric := metricsAggregator.GetClusterIDMetric()
			err = testutil.CollectAndCompare(metric, strings.NewReader(tc.expectedClusterIDResults))
			require.NoError(t, err)

			metric = metricsAggregator.GetClusterProxyMetric()
			err = testutil.CollectAndCompare(metric, strings.NewReader(tc.expectedProxyResults))
			require.NoError(t, err)
		})
	}
}

func TestReconcileProxy_ReconcileError(t *testing.T) {
	for _, tc := range []struct {
		name                     string
		proxyStatus              openshiftapi.ProxyStatus
		proxySpec                openshiftapi.ProxySpec
		testEnvClusterID         string
		expectedClusterIDResults string
		expectedProxyResults     string
	}{
		{
			name:      "no ca and cluster id env variable is empty string",
			proxySpec: openshiftapi.ProxySpec{},
			proxyStatus: openshiftapi.ProxyStatus{
				HTTPProxy:  "http://example.com",
				HTTPSProxy: "https://example.com",
			},
			testEnvClusterID: "",
			expectedClusterIDResults: `
# HELP cluster_id Indicates the cluster id
# TYPE cluster_id gauge
cluster_id{_id="",name="osd_exporter"} 1
	`,
			expectedProxyResults: `
# HELP cluster_proxy Indicates cluster proxy state
# TYPE cluster_proxy gauge
cluster_proxy{http="1",https="1",name="osd_exporter",trusted_ca="0"} 1
`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			os.Setenv(EnvClusterID, tc.testEnvClusterID)
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
			_, err = reconciler.Reconcile(reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: testNamespace,
					Name:      testName,
				},
			})
			require.Error(t, err)
		})
	}
}
