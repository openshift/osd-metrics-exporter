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

package cpms

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"

	machinev1 "github.com/openshift/api/machine/v1"
	"github.com/openshift/osd-metrics-exporter/pkg/metrics"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const ()

func makeTestCPMS(name, namespace string, cpmsSpec machinev1.ControlPlaneMachineSetSpec) *machinev1.ControlPlaneMachineSet {
	cpms := &machinev1.ControlPlaneMachineSet{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec:       cpmsSpec,
	}
	return cpms
}

func TestReconcileCPMS_Reconcile(t *testing.T) {
	for _, tc := range []struct {
		name                     string
		cpmsSpec                 machinev1.ControlPlaneMachineSetSpec
		expectedClusterIDResults string
		expectedCPMSResults      string
	}{
		{
			name: "with active ControlPlaneMachineSet",
			cpmsSpec: machinev1.ControlPlaneMachineSetSpec{
				State: "Active",
			},
			expectedCPMSResults: `
# HELP cpms_enabled Indicates if the controlplanemachineset is enabled
# TYPE cpms_enabled gauge
cpms_enabled{_id="cluster-id",name="osd_exporter"} 1
`,
		},
		{
			name: "with inactive ControlPlaneMachineSet",
			cpmsSpec: machinev1.ControlPlaneMachineSetSpec{
				State: "Inactive",
			},
			expectedCPMSResults: `
# HELP cpms_enabled Indicates if the controlplanemachineset is enabled
# TYPE cpms_enabled gauge
cpms_enabled{_id="cluster-id",name="osd_exporter"} 0
`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			metricsAggregator := metrics.NewMetricsAggregator(time.Second, "cluster-id")
			done := metricsAggregator.Run()
			defer close(done)
			err := machinev1.Install(scheme.Scheme)
			require.NoError(t, err)

			testName := "cluster"
			testNamespace := "openshift-machine-api"

			fakeClient := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(makeTestCPMS(testName, testNamespace, tc.cpmsSpec)).Build()
			reconciler := CPMSReconciler{
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
			var testCPMS machinev1.ControlPlaneMachineSet
			err = fakeClient.Get(context.Background(), types.NamespacedName{Name: testName, Namespace: testNamespace}, &testCPMS)
			require.NoError(t, err)

			metric := metricsAggregator.GetCPMSMetric()
			err = testutil.CollectAndCompare(metric, strings.NewReader(tc.expectedCPMSResults))
			require.NoError(t, err)
		})
	}
}
