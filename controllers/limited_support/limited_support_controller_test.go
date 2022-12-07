package limited_support

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/openshift/osd-metrics-exporter/pkg/metrics"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func makeTestConfigMap(name string, namespace string) *corev1.ConfigMap {
	cfgmap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
	}
	return cfgmap
}

func TestReconcileLimitedSupportConfigMap_Reconcile(t *testing.T) {
	for _, tc := range []struct {
		name             string
		expectedResults  string
		testEnvClusterID string
	}{
		{
			name:             "limited-support correct ConfigMap",
			testEnvClusterID: "i-am-a-cluster-id",
			expectedResults: `
# HELP limited_support_enabled Indicates if limited support is enabled
# TYPE limited_support_enabled gauge
limited_support_enabled{_id="i-am-a-cluster-id",name="osd_exporter"} 1
`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			os.Setenv(EnvClusterID, tc.testEnvClusterID)
			metricsAggregator := metrics.NewMetricsAggregator(time.Second)
			done := metricsAggregator.Run()
			defer close(done)
			err := corev1.AddToScheme(scheme.Scheme)
			require.NoError(t, err)

			testConfigMap := makeTestConfigMap(limitedSupportConfigMapName, limitedSupportConfigMapNamespace)
			fakeClient := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(testConfigMap).Build()
			reconciler := LimitedSupportConfigMapReconciler{
				Client:            fakeClient,
				MetricsAggregator: metricsAggregator,
			}
			result, err := reconciler.Reconcile(context.TODO(), ctrl.Request{
				NamespacedName: types.NamespacedName{
					Namespace: limitedSupportConfigMapNamespace,
					Name:      limitedSupportConfigMapName,
				},
			})

			// sleep to allow the aggregator to aggregate metrics in the background
			time.Sleep(time.Second * 3)
			require.NoError(t, err)
			require.NotNil(t, result)
			var testCfgMap corev1.ConfigMap
			err = fakeClient.Get(context.Background(), types.NamespacedName{Name: limitedSupportConfigMapName, Namespace: limitedSupportConfigMapNamespace}, &testCfgMap)
			require.NoError(t, err)
			metric := metricsAggregator.GetLimitedsupportStatus()
			err = testutil.CollectAndCompare(metric, strings.NewReader(tc.expectedResults))
			require.NoError(t, err)
		})
	}

	for _, tc := range []struct {
		name             string
		expectedResults  string
		testEnvClusterID string
	}{
		{
			name:             "limited-support invalid configMap",
			testEnvClusterID: "i-am-a-cluster-id",
			expectedResults: `
# HELP limited_support_enabled Indicates if limited support is enabled
# TYPE limited_support_enabled gauge
limited_support_enabled{_id="i-am-a-cluster-id",name="osd_exporter"} 0
`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(EnvClusterID, tc.testEnvClusterID)
			metricsAggregator := metrics.NewMetricsAggregator(time.Second)
			done := metricsAggregator.Run()
			defer close(done)
			err := corev1.AddToScheme(scheme.Scheme)
			require.NoError(t, err)

			testConfigMap := makeTestConfigMap(limitedSupportConfigMapName, "default")
			fakeClient := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(testConfigMap).Build()
			reconciler := LimitedSupportConfigMapReconciler{
				Client:            fakeClient,
				MetricsAggregator: metricsAggregator,
			}
			result, err := reconciler.Reconcile(context.TODO(), ctrl.Request{
				NamespacedName: types.NamespacedName{
					Namespace: "default",
					Name:      limitedSupportConfigMapName,
				},
			})

			// sleep to allow the aggregator to aggregate metrics in the background
			time.Sleep(time.Second * 3)
			require.NoError(t, err)
			require.NotNil(t, result)
			var testCfgMap corev1.ConfigMap
			err = fakeClient.Get(context.Background(), types.NamespacedName{Name: limitedSupportConfigMapName, Namespace: "default"}, &testCfgMap)
			require.NoError(t, err)
			metric := metricsAggregator.GetLimitedsupportStatus()
			err = testutil.CollectAndCompare(metric, strings.NewReader(tc.expectedResults))
			require.NoError(t, err)
		})
	}
}
