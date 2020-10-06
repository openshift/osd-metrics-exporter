package clusterrole

import (
	"context"
	"testing"
	"time"

	"github.com/openshift/osd-metrics-exporter/pkg/metrics"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestReconcileClusterRole_Reconcile(t *testing.T) {
	for _, tc := range []struct {
		name    string
		crName  string
		enabled bool
	}{
		{
			name:    "enabled",
			crName:  "cluster-admin",
			enabled: true,
		},
		{
			name:    "disabled",
			crName:  "cluster-owner",
			enabled: false,
		},
	} {
		t.Run(tc.name, func(tt *testing.T) {
			metrics.Aggregator = metrics.NewMetricsAggregator(time.Second * 2)
			fakeClient := fake.NewFakeClientWithScheme(scheme.Scheme, &rbacv1.ClusterRole{
				ObjectMeta: metav1.ObjectMeta{
					Name: tc.crName,
				},
			})
			reconciler := &ReconcileClusterRole{
				client: fakeClient,
			}
			result, err := reconciler.Reconcile(reconcile.Request{})
			require.NoError(t, err)
			require.NotNil(t, result)
			crMetric := metrics.Aggregator.GetClusterRoleMetric()
			value := testutil.ToFloat64(crMetric)
			clusterRole := &rbacv1.ClusterRole{}
			err = fakeClient.Get(context.Background(), types.NamespacedName{Name: tc.crName}, clusterRole)
			require.NoError(t, err)
			if tc.enabled {
				require.Equal(t, 1.0, value)
				require.Contains(t, clusterRole.ObjectMeta.Finalizers, finalizer)
			} else {
				require.Equal(t, 0.0, value)
				require.NotContains(t, clusterRole.ObjectMeta.Finalizers, finalizer)
			}
		})
	}
}
