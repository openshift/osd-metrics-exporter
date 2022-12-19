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

package oauth

import (
	"context"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/openshift/osd-metrics-exporter/pkg/metrics"

	configv1 "github.com/openshift/api/config/v1"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	providerLabel = "provider"
	testName      = "test"
	testNamespace = "test"
)

func makeTestOAuth(name, namespace string, providers ...configv1.IdentityProviderType) *configv1.OAuth {
	oauth := &configv1.OAuth{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec:       configv1.OAuthSpec{},
	}
	for _, p := range providers {
		oauth.Spec.IdentityProviders = append(oauth.Spec.IdentityProviders, configv1.IdentityProvider{
			IdentityProviderConfig: configv1.IdentityProviderConfig{
				Type: p,
			},
		})
	}
	return oauth
}
func TestReconcileOAuth_Reconcile(t *testing.T) {
	for _, tc := range []struct {
		name              string
		providers         []configv1.IdentityProviderType
		existingProviders []configv1.IdentityProviderType
		expectedResult    map[configv1.IdentityProviderType]int
		clusterId         string
	}{
		{
			name: "basic",
			providers: []configv1.IdentityProviderType{
				configv1.IdentityProviderTypeGoogle,
				configv1.IdentityProviderTypeGitHub,
				configv1.IdentityProviderTypeLDAP,
			},
			expectedResult: map[configv1.IdentityProviderType]int{
				configv1.IdentityProviderTypeGoogle: 1,
				configv1.IdentityProviderTypeGitHub: 1,
				configv1.IdentityProviderTypeLDAP:   1,
			},
		},
		{
			name: "provider removed",
			providers: []configv1.IdentityProviderType{
				configv1.IdentityProviderTypeGoogle,
				configv1.IdentityProviderTypeGitHub,
			},
			existingProviders: []configv1.IdentityProviderType{
				configv1.IdentityProviderTypeBasicAuth,
				configv1.IdentityProviderTypeGoogle,
			},
			expectedResult: map[configv1.IdentityProviderType]int{
				configv1.IdentityProviderTypeGoogle:    1,
				configv1.IdentityProviderTypeGitHub:    1,
				configv1.IdentityProviderTypeBasicAuth: 0,
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			metricsAggregator := metrics.NewMetricsAggregator(time.Second, tc.clusterId)
			done := metricsAggregator.Run()
			defer close(done)
			err := configv1.Install(scheme.Scheme)
			require.NoError(t, err)
			if tc.existingProviders == nil {
				tc.existingProviders = make([]configv1.IdentityProviderType, 0)
			}

			// Create OAuth Provider
			// Initially create OAuth provider with or without Identity providers depending on test case
			testOAuthCR := makeTestOAuth(testName, testNamespace, tc.existingProviders...)
			fakeClient := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(testOAuthCR).Build()
			reconciler := OAuthReconciler{
				Client:            fakeClient,
				MetricsAggregator: metricsAggregator,
				ClusterId:         tc.clusterId,
			}

			_, err = reconciler.Reconcile(context.TODO(), ctrl.Request{
				NamespacedName: types.NamespacedName{
					Namespace: testNamespace,
					Name:      testName,
				},
			})

			// Reconcile shouldn't trigger any changes
			require.NoError(t, err)

			var testOAuthCRAPI = &configv1.OAuth{}

			// After reconcile the CR metadata will change so Get it from the API, ensure the spec is still as we expect
			err = fakeClient.Get(context.TODO(), types.NamespacedName{Name: testName, Namespace: testNamespace}, testOAuthCRAPI)
			require.NoError(t, err)

			if diff := cmp.Diff(testOAuthCR.Spec, testOAuthCRAPI.Spec); diff != "" {
				t.Errorf("Expected OAuth .Spec to not change got %v want %v", testOAuthCRAPI.Spec, testOAuthCR.Spec)
			}

			// Modify IdentityProviderType list in OAuth
			testOAuthCRAPI.Spec = makeTestOAuth(testName, testNamespace, tc.providers...).Spec

			// Update the API
			err = fakeClient.Update(context.Background(), testOAuthCRAPI)
			require.NoError(t, err)

			result, err := reconciler.Reconcile(context.TODO(), ctrl.Request{
				NamespacedName: types.NamespacedName{
					Namespace: testNamespace,
					Name:      testName,
				},
			})

			// Validate our metrics reflect the changes to the OAuth IdentityProviderType list
			// sleep to allow the aggregator to aggregate metrics in the background
			time.Sleep(time.Second * 3)
			require.NoError(t, err)
			require.NotNil(t, result)
			var testOAuth configv1.OAuth
			err = fakeClient.Get(context.Background(), types.NamespacedName{Name: testName, Namespace: testNamespace}, &testOAuth)
			require.NoError(t, err)
			require.Contains(t, testOAuth.ObjectMeta.Finalizers, finalizer)
			metric := metricsAggregator.GetIdentityProviderMetric()
			for p, v := range tc.expectedResult {
				val := testutil.ToFloat64(metric.With(prometheus.Labels{providerLabel: string(p)}))
				require.EqualValues(t, v, val, "provider label: %s", string(p))
			}
		})
	}
}
