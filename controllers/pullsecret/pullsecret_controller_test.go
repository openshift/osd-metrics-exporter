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
	"strings"
	"testing"
	"time"

	"github.com/openshift/osd-metrics-exporter/pkg/metrics"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const testClusterId = "test-cluster-id"

// makeTestPullSecret creates a pull secret with the given dockerconfigjson data
func makeTestPullSecret(data []byte) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pullSecretName,
			Namespace: pullSecretNamespace,
		},
		Data: map[string][]byte{
			dockerConfigJSONKey: data,
		},
		Type: corev1.SecretTypeDockerConfigJson,
	}
}

// makeValidDockerConfigJSON creates a valid dockerconfigjson with all expected registries
func makeValidDockerConfigJSON() []byte {
	return []byte(`{
		"auths": {
			"cloud.openshift.com": {"auth": "dGVzdHVzZXI6dGVzdHBhc3M="},
			"quay.io": {"auth": "dGVzdHVzZXI6dGVzdHBhc3M="},
			"registry.redhat.io": {"auth": "dGVzdHVzZXI6dGVzdHBhc3M="},
			"registry.connect.redhat.com": {"auth": "dGVzdHVzZXI6dGVzdHBhc3M="}
		}
	}`)
}

func TestReconcilePullSecret_Reconcile(t *testing.T) {
	for _, tc := range []struct {
		name            string
		secret          *corev1.Secret
		expectedResults string
	}{
		{
			name:   "valid pull secret with all registries",
			secret: makeTestPullSecret(makeValidDockerConfigJSON()),
			expectedResults: `
# HELP pull_secret_valid Indicates if the cluster pull secret is valid (1=valid, 0=invalid)
# TYPE pull_secret_valid gauge
pull_secret_valid{_id="test-cluster-id",name="osd_exporter"} 1
`,
		},
		{
			name: "valid pull secret with extra registries",
			secret: makeTestPullSecret([]byte(`{
				"auths": {
					"cloud.openshift.com": {"auth": "dGVzdHVzZXI6dGVzdHBhc3M="},
					"quay.io": {"auth": "dGVzdHVzZXI6dGVzdHBhc3M="},
					"registry.redhat.io": {"auth": "dGVzdHVzZXI6dGVzdHBhc3M="},
					"registry.connect.redhat.com": {"auth": "dGVzdHVzZXI6dGVzdHBhc3M="},
					"my-custom-registry.example.com": {"auth": "dGVzdHVzZXI6dGVzdHBhc3M="}
				}
			}`)),
			expectedResults: `
# HELP pull_secret_valid Indicates if the cluster pull secret is valid (1=valid, 0=invalid)
# TYPE pull_secret_valid gauge
pull_secret_valid{_id="test-cluster-id",name="osd_exporter"} 1
`,
		},
		{
			name: "missing registry - quay.io",
			secret: makeTestPullSecret([]byte(`{
				"auths": {
					"cloud.openshift.com": {"auth": "dGVzdHVzZXI6dGVzdHBhc3M="},
					"registry.redhat.io": {"auth": "dGVzdHVzZXI6dGVzdHBhc3M="},
					"registry.connect.redhat.com": {"auth": "dGVzdHVzZXI6dGVzdHBhc3M="}
				}
			}`)),
			expectedResults: `
# HELP pull_secret_valid Indicates if the cluster pull secret is valid (1=valid, 0=invalid)
# TYPE pull_secret_valid gauge
pull_secret_valid{_id="test-cluster-id",name="osd_exporter"} 0
`,
		},
		{
			name: "empty auth token for registry",
			secret: makeTestPullSecret([]byte(`{
				"auths": {
					"cloud.openshift.com": {"auth": ""},
					"quay.io": {"auth": "dGVzdHVzZXI6dGVzdHBhc3M="},
					"registry.redhat.io": {"auth": "dGVzdHVzZXI6dGVzdHBhc3M="},
					"registry.connect.redhat.com": {"auth": "dGVzdHVzZXI6dGVzdHBhc3M="}
				}
			}`)),
			expectedResults: `
# HELP pull_secret_valid Indicates if the cluster pull secret is valid (1=valid, 0=invalid)
# TYPE pull_secret_valid gauge
pull_secret_valid{_id="test-cluster-id",name="osd_exporter"} 0
`,
		},
		{
			name:   "empty auths map",
			secret: makeTestPullSecret([]byte(`{"auths": {}}`)),
			expectedResults: `
# HELP pull_secret_valid Indicates if the cluster pull secret is valid (1=valid, 0=invalid)
# TYPE pull_secret_valid gauge
pull_secret_valid{_id="test-cluster-id",name="osd_exporter"} 0
`,
		},
		{
			name:   "malformed JSON",
			secret: makeTestPullSecret([]byte(`{not valid json`)),
			expectedResults: `
# HELP pull_secret_valid Indicates if the cluster pull secret is valid (1=valid, 0=invalid)
# TYPE pull_secret_valid gauge
pull_secret_valid{_id="test-cluster-id",name="osd_exporter"} 0
`,
		},
		{
			name: "missing dockerconfigjson key",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      pullSecretName,
					Namespace: pullSecretNamespace,
				},
				Data: map[string][]byte{
					"wrong-key": []byte(`{"auths": {}}`),
				},
			},
			expectedResults: `
# HELP pull_secret_valid Indicates if the cluster pull secret is valid (1=valid, 0=invalid)
# TYPE pull_secret_valid gauge
pull_secret_valid{_id="test-cluster-id",name="osd_exporter"} 0
`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			metricsAggregator := metrics.NewMetricsAggregator(time.Second, testClusterId)
			done := metricsAggregator.Run()
			defer close(done)
			err := corev1.AddToScheme(scheme.Scheme)
			require.NoError(t, err)

			fakeClient := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(tc.secret).Build()
			reconciler := PullSecretReconciler{
				Client:            fakeClient,
				MetricsAggregator: metricsAggregator,
				ClusterId:         testClusterId,
			}
			result, err := reconciler.Reconcile(context.TODO(), ctrl.Request{
				NamespacedName: types.NamespacedName{
					Namespace: pullSecretNamespace,
					Name:      pullSecretName,
				},
			})

			// sleep to allow the aggregator to aggregate metrics in the background
			time.Sleep(time.Second * 3)
			require.NoError(t, err)
			require.NotNil(t, result)
			var testSecret corev1.Secret
			err = fakeClient.Get(context.Background(), types.NamespacedName{Name: pullSecretName, Namespace: pullSecretNamespace}, &testSecret)
			require.NoError(t, err)

			metric := metricsAggregator.GetPullSecretValidMetric()
			err = testutil.CollectAndCompare(metric, strings.NewReader(tc.expectedResults))
			require.NoError(t, err)
		})
	}
}

func TestReconcilePullSecretNotFound_Reconcile(t *testing.T) {
	metricsAggregator := metrics.NewMetricsAggregator(time.Second, testClusterId)
	done := metricsAggregator.Run()
	defer close(done)
	err := corev1.AddToScheme(scheme.Scheme)
	require.NoError(t, err)

	// No pull secret object created - simulate missing secret
	fakeClient := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
	reconciler := PullSecretReconciler{
		Client:            fakeClient,
		MetricsAggregator: metricsAggregator,
		ClusterId:         testClusterId,
	}
	result, err := reconciler.Reconcile(context.TODO(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: pullSecretNamespace,
			Name:      pullSecretName,
		},
	})

	// sleep to allow the aggregator to aggregate metrics in the background
	time.Sleep(time.Second * 3)
	require.NoError(t, err)
	require.NotNil(t, result)
	var testSecret corev1.Secret
	err = fakeClient.Get(context.Background(), types.NamespacedName{Name: pullSecretName, Namespace: pullSecretNamespace}, &testSecret)
	require.True(t, errors.IsNotFound(err))

	expectedResults := `
# HELP pull_secret_valid Indicates if the cluster pull secret is valid (1=valid, 0=invalid)
# TYPE pull_secret_valid gauge
pull_secret_valid{_id="test-cluster-id",name="osd_exporter"} 0
`
	metric := metricsAggregator.GetPullSecretValidMetric()
	err = testutil.CollectAndCompare(metric, strings.NewReader(expectedResults))
	require.NoError(t, err)
}

func TestValidatePullSecret(t *testing.T) {
	for _, tc := range []struct {
		name        string
		secret      *corev1.Secret
		expectValid bool
	}{
		{
			name:        "valid pull secret",
			secret:      makeTestPullSecret(makeValidDockerConfigJSON()),
			expectValid: true,
		},
		{
			name: "missing dockerconfigjson key",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: pullSecretName, Namespace: pullSecretNamespace},
				Data:       map[string][]byte{"other-key": []byte(`{}`)},
			},
			expectValid: false,
		},
		{
			name:        "invalid JSON",
			secret:      makeTestPullSecret([]byte(`not json`)),
			expectValid: false,
		},
		{
			name:        "empty auths",
			secret:      makeTestPullSecret([]byte(`{"auths":{}}`)),
			expectValid: false,
		},
		{
			name: "missing cloud.openshift.com",
			secret: makeTestPullSecret([]byte(`{
				"auths": {
					"quay.io": {"auth": "dGVzdA=="},
					"registry.redhat.io": {"auth": "dGVzdA=="},
					"registry.connect.redhat.com": {"auth": "dGVzdA=="}
				}
			}`)),
			expectValid: false,
		},
		{
			name: "empty auth for quay.io",
			secret: makeTestPullSecret([]byte(`{
				"auths": {
					"cloud.openshift.com": {"auth": "dGVzdA=="},
					"quay.io": {"auth": ""},
					"registry.redhat.io": {"auth": "dGVzdA=="},
					"registry.connect.redhat.com": {"auth": "dGVzdA=="}
				}
			}`)),
			expectValid: false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			valid, reason := validatePullSecret(tc.secret)
			if tc.expectValid {
				require.True(t, valid, "expected valid but got invalid: %s", reason)
			} else {
				require.False(t, valid, "expected invalid but got valid")
				require.NotEmpty(t, reason, "expected a reason for invalidity")
			}
		})
	}
}
