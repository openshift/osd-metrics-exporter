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

package proxy

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"

	configv1 "github.com/openshift/api/config/v1"
	"github.com/openshift/osd-metrics-exporter/pkg/metrics"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	testName      = "test"
	testNamespace = "test"
)

func makeTestProxy(name, namespace string, proxySpec configv1.ProxySpec, proxyStatus configv1.ProxyStatus) *configv1.Proxy {
	proxy := &configv1.Proxy{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec:       proxySpec,
		Status:     proxyStatus,
	}
	return proxy
}

func TestReconcileProxy_Reconcile(t *testing.T) {
	for _, tc := range []struct {
		name                     string
		proxyStatus              configv1.ProxyStatus
		proxySpec                configv1.ProxySpec
		expectedClusterIDResults string
		expectedProxyResults     string
	}{
		{
			name: "with ca",
			proxySpec: configv1.ProxySpec{
				TrustedCA: configv1.ConfigMapNameReference{
					Name: "test",
				},
			},
			proxyStatus: configv1.ProxyStatus{
				HTTPProxy:  "http://example.com",
				HTTPSProxy: "https://example.com",
			},
			expectedClusterIDResults: `
# HELP cluster_id Indicates the cluster id
# TYPE cluster_id gauge
cluster_id{_id="cluster-id",name="osd_exporter"} 1
	`,
			expectedProxyResults: `
# HELP cluster_proxy Indicates cluster proxy state
# TYPE cluster_proxy gauge
cluster_proxy{http="1",https="1",name="osd_exporter",trusted_ca="1"} 1
`,
		},
		{
			name:      "no ca",
			proxySpec: configv1.ProxySpec{},
			proxyStatus: configv1.ProxyStatus{
				HTTPProxy:  "http://example.com",
				HTTPSProxy: "https://example.com",
			},
			expectedClusterIDResults: `
# HELP cluster_id Indicates the cluster id
# TYPE cluster_id gauge
cluster_id{_id="cluster-id",name="osd_exporter"} 1
	`,
			expectedProxyResults: `
# HELP cluster_proxy Indicates cluster proxy state
# TYPE cluster_proxy gauge
cluster_proxy{http="1",https="1",name="osd_exporter",trusted_ca="0"} 1
`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			metricsAggregator := metrics.NewMetricsAggregator(time.Second, "cluster-id")
			done := metricsAggregator.Run()
			defer close(done)
			err := configv1.Install(scheme.Scheme)
			require.NoError(t, err)

			fakeClient := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(makeTestProxy(testName, testNamespace, tc.proxySpec, tc.proxyStatus)).Build()
			reconciler := ProxyReconciler{
				Client:            fakeClient,
				MetricsAggregator: metricsAggregator,
				ClusterId:         "cluster-id",
			}
			result, err := reconciler.Reconcile(context.TODO(), ctrl.Request{
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
			var testProxy configv1.Proxy
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
