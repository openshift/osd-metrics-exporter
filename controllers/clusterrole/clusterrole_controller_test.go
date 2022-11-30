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

package clusterrole

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
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
			fakeClient := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(clusterRole).Build()
			reconciler := &ClusterRoleReconciler{
				Client: fakeClient,
			}
			result, err := reconciler.Reconcile(context.TODO(), ctrl.Request{
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
