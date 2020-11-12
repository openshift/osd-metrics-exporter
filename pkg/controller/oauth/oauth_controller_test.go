package oauth

import (
	"context"
	"testing"
	"time"

	"github.com/openshift/osd-metrics-exporter/pkg/metrics"

	openshiftapi "github.com/openshift/api/config/v1"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	providerLabel = "provider"
	testName      = "test"
	testNamespace = "test"
)

func makeTestOAuth(name, namespace string, providers ...openshiftapi.IdentityProviderType) *openshiftapi.OAuth {
	oauth := &openshiftapi.OAuth{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec:       openshiftapi.OAuthSpec{},
	}
	for _, p := range providers {
		oauth.Spec.IdentityProviders = append(oauth.Spec.IdentityProviders, openshiftapi.IdentityProvider{
			IdentityProviderConfig: openshiftapi.IdentityProviderConfig{
				Type: p,
			},
		})
	}
	return oauth
}
func TestReconcileOAuth_Reconcile(t *testing.T) {
	for _, tc := range []struct {
		name              string
		providers         []openshiftapi.IdentityProviderType
		existingProviders []openshiftapi.IdentityProviderType
		expectedResult    map[openshiftapi.IdentityProviderType]int
	}{
		{
			name: "basic",
			providers: []openshiftapi.IdentityProviderType{
				openshiftapi.IdentityProviderTypeGoogle,
				openshiftapi.IdentityProviderTypeGitHub,
				openshiftapi.IdentityProviderTypeLDAP,
			},
			expectedResult: map[openshiftapi.IdentityProviderType]int{
				openshiftapi.IdentityProviderTypeGoogle: 1,
				openshiftapi.IdentityProviderTypeGitHub: 1,
				openshiftapi.IdentityProviderTypeLDAP:   1,
			},
		},
		{
			name: "provider removed",
			providers: []openshiftapi.IdentityProviderType{
				openshiftapi.IdentityProviderTypeGoogle,
				openshiftapi.IdentityProviderTypeGitHub,
			},
			existingProviders: []openshiftapi.IdentityProviderType{
				openshiftapi.IdentityProviderTypeBasicAuth,
				openshiftapi.IdentityProviderTypeGoogle,
			},
			expectedResult: map[openshiftapi.IdentityProviderType]int{
				openshiftapi.IdentityProviderTypeGoogle:    1,
				openshiftapi.IdentityProviderTypeGitHub:    1,
				openshiftapi.IdentityProviderTypeBasicAuth: 0,
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			metricsAggregator := metrics.NewMetricsAggregator(time.Second)
			done := metricsAggregator.Run()
			defer close(done)
			err := openshiftapi.Install(scheme.Scheme)
			require.NoError(t, err)
			if tc.existingProviders == nil {
				tc.existingProviders = make([]openshiftapi.IdentityProviderType, 0)
			}
			fakeClient := fake.NewFakeClientWithScheme(scheme.Scheme, makeTestOAuth(testName, testNamespace, tc.existingProviders...))
			reconciler := ReconcileOAuth{
				client:            fakeClient,
				metricsAggregator: metricsAggregator,
			}
			_, err = reconciler.Reconcile(reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: testNamespace,
					Name:      testName,
				},
			})
			require.NoError(t, err)

			err = fakeClient.Update(context.Background(), makeTestOAuth(testName, testNamespace, tc.providers...))
			require.NoError(t, err)

			result, err := reconciler.Reconcile(reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: testNamespace,
					Name:      testName,
				},
			})
			// sleep to allow the aggregator to aggregate metrics in the background
			time.Sleep(time.Second * 3)
			require.NoError(t, err)
			require.NotNil(t, result)
			var testOAuth openshiftapi.OAuth
			err = fakeClient.Get(context.Background(), types.NamespacedName{Name: "test", Namespace: "test"}, &testOAuth)
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
