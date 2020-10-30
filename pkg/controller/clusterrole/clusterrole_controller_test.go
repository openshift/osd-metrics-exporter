package clusterrole

import (
	"context"
	"testing"

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
		name             string
		crName           string
		finalizerPresent bool
		expectError      bool
	}{
		{
			name:             "finalizer present",
			crName:           "cluster-admin",
			finalizerPresent: true,
			expectError:      false,
		},
		{
			name:        "incorrect cluster role",
			crName:      "cluster-owner",
			expectError: true,
		},
		{
			name:             "finalizer not present",
			crName:           "cluster-admin",
			finalizerPresent: false,
			expectError:      false,
		},
	} {
		t.Run(tc.name, func(tt *testing.T) {
			clusterRole := &rbacv1.ClusterRole{
				ObjectMeta: metav1.ObjectMeta{
					Name: tc.crName,
				},
			}
			if tc.finalizerPresent {
				clusterRole.Finalizers = append(clusterRole.Finalizers, finalizer)
			}
			fakeClient := fake.NewFakeClientWithScheme(scheme.Scheme, clusterRole)
			reconciler := &ReconcileClusterRole{
				client: fakeClient,
			}
			result, err := reconciler.Reconcile(reconcile.Request{
				NamespacedName: types.NamespacedName{Name: tc.crName},
			})
			if tc.expectError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, result)
			clusterRole = &rbacv1.ClusterRole{}
			err = fakeClient.Get(context.Background(), types.NamespacedName{Name: tc.crName}, clusterRole)
			require.NoError(t, err)
			require.NotContains(t, clusterRole.Finalizers, finalizer)
		})
	}
}
