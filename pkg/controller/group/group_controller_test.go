package group

import (
	"context"
	"testing"
	"time"

	userv1 "github.com/openshift/api/user/v1"
	"github.com/openshift/osd-metrics-exporter/pkg/metrics"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestReconcileGroup_Reconcile(t *testing.T) {
	for _, tc := range []struct {
		name        string
		users       []string
		result      int
		delete      bool
	}{
		{
			name:   "empty cluster-admins group",
			result: 0,
		},
		{
			name:   "single user",
			users:  []string{"abc"},
			result: 1,
		},
		{
			name:   "deletion set",
			users:  []string{"abc"},
			result: 0,
			delete: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := userv1.Install(scheme.Scheme)
			require.NoError(t, err)
			group := &userv1.Group{
				ObjectMeta: metav1.ObjectMeta{Name: clusterAdminGroupName},
				Users:      tc.users,
			}
			if tc.delete {
				now := metav1.Now()
				group.DeletionTimestamp = &now
			}
			fakeClient := fake.NewFakeClientWithScheme(scheme.Scheme, group)
			reconcileGroup := &ReconcileGroup{
				client:            fakeClient,
				scheme:            scheme.Scheme,
				metricsAggregator: metrics.NewMetricsAggregator(time.Second * 10),
			}
			_, err = reconcileGroup.Reconcile(reconcile.Request{
				NamespacedName: types.NamespacedName{Name: clusterAdminGroupName},
			})
			require.NoError(t, err)
			err = fakeClient.Get(context.Background(), client.ObjectKey{Name: clusterAdminGroupName}, group)
			require.NoError(t, err)
			if tc.delete {
				require.NotContains(t, group.Finalizers, finalizer)
			} else {
				require.Contains(t, group.Finalizers, finalizer)
			}
			metric := reconcileGroup.metricsAggregator.GetClusterRoleMetric()
			value := testutil.ToFloat64(metric)
			require.EqualValues(t, tc.result, value)
		})
	}
}
