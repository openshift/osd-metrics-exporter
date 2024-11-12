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

package configmap

import (
	"context"
	"fmt"
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
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	userCABundle    = "user-ca-bundle"
	openshiftConfig = "openshift-config"
	caBundleCRT     = "ca-bundle.crt"
)

var expiredCA = `
-----BEGIN CERTIFICATE-----
MIIFYTCCA0mgAwIBAgIUH1izUJplJEtQrGGUh5RyH/X8GFowDQYJKoZIhvcNAQEL
BQAwQDELMAkGA1UEBhMCWFgxEzARBgNVBAcMCk1lZ2FjaXR5IDExHDAaBgNVBAoM
E0RlZmF1bHQgQ29tcGFueSBMdGQwHhcNMjQxMTEwMTIxNTU4WhcNMjQxMTExMTIx
NTU4WjBAMQswCQYDVQQGEwJYWDETMBEGA1UEBwwKTWVnYWNpdHkgMTEcMBoGA1UE
CgwTRGVmYXVsdCBDb21wYW55IEx0ZDCCAiIwDQYJKoZIhvcNAQEBBQADggIPADCC
AgoCggIBAIq+1aLpklDeRX88TbHpRIqTCDVZ5PyRVTEJmGDhQbLfLbkOAjht+YBT
VwpvJelbQAkV4maugmYkOa63av7HidJqsdED3olwKgmY6vGuMpZ8K6gt1TgQ+HbW
do+1Ur1ixxChXodfOIHNx6opoY0r2a5vKfNbJ7QOcuP0OUwiuPZFrWobpD/w6Hey
Zv1P/IXaDJeD8jXrtbsnxbB3dA6zg7EXowri9o6nINOIdLf0I/+/mE22z8Dp9+89
pEyIUIt1aY6kJZ5Yd78UNRUEDXJ1849czQaoaClWPFB/2KCgp8ZrZtm3U6Cj6bBy
Aei3pj6SDkubRqdGzeMrGPBhturIiRnQwLbV8LU6uH6mQXL1I3AABkdhsgoJqWdN
KiHOHfOmNpV+q06t+VSmpMvs7Nhhk89XfMy7KgUoMYm+1fckZu3Qj0cp99FDbNcR
gvPO9zPghXx/kh27PU2FXwZs27OSJOozAXtJmag77ZMINuyHOWSkUm/t6JJGO/Ca
Q8J84anC4ksZq7zIyPaKO6DQx6YGHS2MRT4wvu+1fxv0zw2FDTrlwpAF/healdwk
8Viw23+zjYS3qdG0TGVNqt9PAkt0YHYA0NqTWBIIGVvAPFHTmoiCoywCtBIZIOIO
YTeJmnDoJYQS3ox5ZKxJTLDwhs3zPNcWrRgcsvoFf9CpHqId1TUXAgMBAAGjUzBR
MB0GA1UdDgQWBBS6XfsAbTru+mJOComvyWOUUlpsgzAfBgNVHSMEGDAWgBS6XfsA
bTru+mJOComvyWOUUlpsgzAPBgNVHRMBAf8EBTADAQH/MA0GCSqGSIb3DQEBCwUA
A4ICAQAB0b8DXNFcNCTskKEOwHQpLKYTmNQgcMKPMIbSS59+BYe4jIdlPyraF1FB
qr+VcFVGmBu6OhcW8S/6X4CvmO5aOn+ZV1NaTE1syNmLZHKXbKJai6D/FbJI1cTD
AWiIZ0Wn9i6O1PIzw49om48MuGwpCjTtX1pgoPVlzxuG/XxnxL+VcfB/YbNRdX8/
zamCucAhhXexv2qM7hYFBMoiJ9NgoLzQ0R3keYehH6EL1FBgcvFfSDh/cUTyx2cH
jLXb0cig/XqKQ/i/VmkEGor/mZqXEP0luZSJGkVZgQUdEr7ENRLhjmpR5+KVwmPI
vAuapuahdxVM7CGoEPB/MHjWk4rxe45aIuRSpXIWbjILC9fTueyO+Cg3sgCXeCcY
78ZuQlm8Ov3OR5vZ1C5GNxtTYVqYtrUJgi4EHWa7VkI5izBEsLNqJn2aHRNKZIi5
6FegC5yo3biAmygl3jCEKY8eX2ETqRFBjFre1SVuv6bFIP0ttMGlU7Q6cFtY9vIV
U/NylDQhl5qYGXHwpQowoWxAg4a6vbceyC5N+pmNvIKRSODGhWEkjPIfjLKustxa
+WUlM1VPRd9vyaj2N6HY5lh1rreNInhFw+3WkcCDMMXzEoK3QXRKBSNyGWaxXwtu
0q3qZJJFPJIxFY87bYXkO09r5oNjXoYdfO+toIhBcpk7N3rDoQ==
-----END CERTIFICATE-----
`

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
		name                  string
		cfgMapData            map[string]string
		expectedExpireResults string
		expectedValidResults  string
		clusterId             string
	}{
		{
			clusterId:  "i-am-a-cluster-id",
			name:       "user-ca-bundle exists",
			cfgMapData: makeTestCAData(caBundleCRT, testCA),
			expectedExpireResults: `
# HELP cluster_proxy_ca_expiry_timestamp Indicates cluster proxy CA expiry unix timestamp in UTC
# TYPE cluster_proxy_ca_expiry_timestamp gauge
cluster_proxy_ca_expiry_timestamp{_id="i-am-a-cluster-id", name="osd_exporter",subject="O=Default Company Ltd,L=Default City,C=XX"} 1.734086723e+09
`,
			expectedValidResults: `
# HELP cluster_proxy_ca_valid Indicates if cluster proxy CA valid
# TYPE cluster_proxy_ca_valid gauge
cluster_proxy_ca_valid{_id="i-am-a-cluster-id", name="osd_exporter"} 1
`,
		},
		{
			name:       "user-ca-bundle with expired cert",
			cfgMapData: makeTestCAData(caBundleCRT, expiredCA),
			expectedExpireResults: `
# HELP cluster_proxy_ca_expiry_timestamp Indicates cluster proxy CA expiry unix timestamp in UTC
# TYPE cluster_proxy_ca_expiry_timestamp gauge
cluster_proxy_ca_expiry_timestamp{_id="i-am-a-cluster-id", name="osd_exporter",subject="O=Default Company Ltd,L=Megacity 1,C=XX"} 1.731327358e+09
`,
			expectedValidResults: `
# HELP cluster_proxy_ca_valid Indicates if cluster proxy CA valid
# TYPE cluster_proxy_ca_valid gauge
cluster_proxy_ca_valid{_id="i-am-a-cluster-id", name="osd_exporter"} 1
`,
			clusterId: "i-am-a-cluster-id",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			metricsAggregator := metrics.NewMetricsAggregator(time.Second, tc.clusterId)
			done := metricsAggregator.Run()
			defer close(done)
			err := corev1.AddToScheme(scheme.Scheme)
			require.NoError(t, err)

			testConfigMap := makeTestConfigMap(userCABundle, openshiftConfig, tc.cfgMapData)
			fakeClient := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(testConfigMap).Build()
			reconciler := ConfigMapReconciler{
				Client:            fakeClient,
				MetricsAggregator: metricsAggregator,
				ClusterId:         tc.clusterId,
			}
			result, err := reconciler.Reconcile(context.TODO(), ctrl.Request{
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
			expire_metric := metricsAggregator.GetClusterProxyCAExpiryMetrics()
			valid_metric := metricsAggregator.GetClusterProxyCAValidMetrics()
			err = testutil.CollectAndCompare(expire_metric, strings.NewReader(tc.expectedExpireResults))
			require.NoError(t, err)
			err = testutil.CollectAndCompare(valid_metric, strings.NewReader(tc.expectedValidResults))
			require.NoError(t, err)
		})
	}

	for _, tc := range []struct {
		name            string
		cfgMapData      map[string]string
		expectedResults string
		clusterId       string
	}{
		{
			name:       "user-ca-bundle invalid PEM",
			clusterId:  "i-am-a-cluster-id",
			cfgMapData: makeTestCAData(caBundleCRT, "derp"),
			expectedResults: `
# HELP cluster_proxy_ca_valid Indicates if cluster proxy CA valid
# TYPE cluster_proxy_ca_valid gauge
cluster_proxy_ca_valid{_id="i-am-a-cluster-id", name="osd_exporter"} 0
`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			metricsAggregator := metrics.NewMetricsAggregator(time.Second, tc.clusterId)
			done := metricsAggregator.Run()
			defer close(done)
			err := corev1.AddToScheme(scheme.Scheme)
			require.NoError(t, err)

			testConfigMap := makeTestConfigMap(userCABundle, openshiftConfig, tc.cfgMapData)
			fakeClient := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(testConfigMap).Build()
			reconciler := ConfigMapReconciler{
				Client:            fakeClient,
				MetricsAggregator: metricsAggregator,
				ClusterId:         tc.clusterId,
			}
			result, err := reconciler.Reconcile(context.TODO(), ctrl.Request{
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

// This test will simulate a customer fixing an invalid CA in between Reconcile runs:
// 1. Run with an invalid CA present
// 2. Remove the CA from the ConfigMap
// 3. Rerun the Reconcile
// 4. Ensure the Metric is gone
func TestReconcileConfigMapModifiedCA_Reconcile(t *testing.T) {
	for _, tc := range []struct {
		name                                 string
		expectedExpireResultsFirstReconcile  string
		expectedExpireResultsSecondReconcile string
		clusterId                            string
	}{
		{
			name: "user-ca-bundle with expired cert",
			expectedExpireResultsFirstReconcile: `
# HELP cluster_proxy_ca_expiry_timestamp Indicates cluster proxy CA expiry unix timestamp in UTC
# TYPE cluster_proxy_ca_expiry_timestamp gauge
cluster_proxy_ca_expiry_timestamp{_id="i-am-a-cluster-id", name="osd_exporter",subject="O=Default Company Ltd,L=Default City,C=XX"} 1.734086723e+09
cluster_proxy_ca_expiry_timestamp{_id="i-am-a-cluster-id", name="osd_exporter",subject="O=Default Company Ltd,L=Megacity 1,C=XX"} 1.731327358e+09
`,
			expectedExpireResultsSecondReconcile: `
# HELP cluster_proxy_ca_expiry_timestamp Indicates cluster proxy CA expiry unix timestamp in UTC
# TYPE cluster_proxy_ca_expiry_timestamp gauge
cluster_proxy_ca_expiry_timestamp{_id="i-am-a-cluster-id", name="osd_exporter",subject="O=Default Company Ltd,L=Default City,C=XX"} 1.734086723e+09
`,
			clusterId: "i-am-a-cluster-id",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			metricsAggregator := metrics.NewMetricsAggregator(time.Second, tc.clusterId)
			done := metricsAggregator.Run()
			defer close(done)
			err := corev1.AddToScheme(scheme.Scheme)
			require.NoError(t, err)

			var testCfgMap corev1.ConfigMap
			configMapRef := types.NamespacedName{
				Namespace: openshiftConfig,
				Name:      userCABundle,
			}

			testConfigMap := makeTestConfigMap(userCABundle, openshiftConfig, makeTestCAData(caBundleCRT, fmt.Sprintf("%s\n%s", testCA, expiredCA)))
			fakeClient := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(testConfigMap).Build()
			reconciler := ConfigMapReconciler{
				Client:            fakeClient,
				MetricsAggregator: metricsAggregator,
				ClusterId:         tc.clusterId,
			}
			result, err := reconciler.Reconcile(context.TODO(), ctrl.Request{
				NamespacedName: configMapRef,
			})

			// sleep to allow the aggregator to aggregate metrics in the background
			time.Sleep(time.Second * 3)
			require.NoError(t, err)
			require.NotNil(t, result)
			err = fakeClient.Get(context.Background(), configMapRef, &testCfgMap)
			require.NoError(t, err)
			expire_metric := metricsAggregator.GetClusterProxyCAExpiryMetrics()
			err = testutil.CollectAndCompare(expire_metric, strings.NewReader(tc.expectedExpireResultsFirstReconcile))
			require.NoError(t, err)

			testConfigMap = makeTestConfigMap(userCABundle, openshiftConfig, makeTestCAData(caBundleCRT, testCA))
			err = fakeClient.Update(context.TODO(), testConfigMap)
			require.NoError(t, err)
			result, err = reconciler.Reconcile(context.TODO(), ctrl.Request{
				NamespacedName: configMapRef,
			})

			// sleep to allow the aggregator to aggregate metrics in the background
			time.Sleep(time.Second * 3)
			require.NoError(t, err)
			require.NotNil(t, result)
			err = fakeClient.Get(context.Background(), configMapRef, &testCfgMap)
			require.NoError(t, err)
			expire_metric = metricsAggregator.GetClusterProxyCAExpiryMetrics()
			err = testutil.CollectAndCompare(expire_metric, strings.NewReader(tc.expectedExpireResultsSecondReconcile))
			require.NoError(t, err)
		})
	}
}
