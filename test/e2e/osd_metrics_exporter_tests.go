// DO NOT REMOVE TAGS BELOW. IF ANY NEW TEST FILES ARE CREATED UNDER /osde2e, PLEASE ADD THESE TAGS TO THEM IN ORDER TO BE EXCLUDED FROM UNIT TESTS.
//go:build osde2e
// +build osde2e

package osde2etests

import (
	"context"
	"os"
	"strings"
	"time"

	mathrand "math/rand"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	clustersmgmtv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	configv1 "github.com/openshift/api/config/v1"
	"github.com/openshift/osde2e-common/pkg/clients/ocm"
	"github.com/openshift/osde2e-common/pkg/clients/openshift"
	"github.com/openshift/osde2e-common/pkg/clients/prometheus"
	. "github.com/openshift/osde2e-common/pkg/gomega/assertions"
	. "github.com/openshift/osde2e-common/pkg/gomega/matchers"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/util/rand"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

var _ = ginkgo.Describe("osd-metrics-exporter", ginkgo.Ordered, func() {
	var (
		clusterID         string
		k8s               *openshift.Client
		prom              *prometheus.Client
		ocmClient         *ocm.Client
		namespace         = "openshift-osd-metrics"
		serviceName       = "osd-metrics-exporter"
		deploymentName    = "osd-metrics-exporter"
		rolePrefix        = "osd-metrics-exporter"
		clusterRolePrefix = "osd-metrics-exporter"
	)

	ginkgo.BeforeAll(func(ctx context.Context) {
		log.SetLogger(ginkgo.GinkgoLogr)

		clusterID = os.Getenv("OCM_CLUSTER_ID")
		Expect(clusterID).ShouldNot(BeEmpty(), "failed to find OCM_CLUSTER_ID environment variable")

		var err error
		k8s, err = openshift.New(ginkgo.GinkgoLogr)
		Expect(err).ShouldNot(HaveOccurred(), "unable to setup k8s client")

		prom, err = prometheus.New(ctx, k8s)
		Expect(err).ShouldNot(HaveOccurred(), "unable to setup prometheus client")

		var ocmUrl ocm.Environment

		switch os.Getenv("OCM_ENV") {
		case "stage":
			ocmUrl = ocm.Stage
		case "int":
			ocmUrl = ocm.Integration
		default:
			ginkgo.Fail("Unexpected OCM_ENV - use 'stage' or 'int'")
		}

		token := os.Getenv("OCM_TOKEN")
		clientID := os.Getenv("OCM_CLIENT_ID")
		clientSecret := os.Getenv("OCM_CLIENT_SECRET")
		if token != "" {
			ocmClient, err = ocm.New(ctx, token, "", "", ocmUrl)
		} else if clientID != "" && clientSecret != "" {
			ocmClient, err = ocm.New(ctx, "", clientID, clientSecret, ocmUrl)
		} else {
			ginkgo.Fail("set OCM_TOKEN (e.g. from 'ocm token') or both OCM_CLIENT_ID and OCM_CLIENT_SECRET")
		}
		Expect(err).ShouldNot(HaveOccurred(), "unable to setup ocm client")
		ginkgo.DeferCleanup(ocmClient.Connection.Close)
	})

	ginkgo.It("is installed", func(ctx context.Context) {
		ginkgo.By("checking the namespace exists")
		err := k8s.Get(ctx, namespace, "", &corev1.Namespace{})
		Expect(err).ShouldNot(HaveOccurred(), "namespace %s not found", namespace)

		ginkgo.By("checking the role exists")
		var roles rbacv1.RoleList
		err = k8s.WithNamespace(namespace).List(ctx, &roles)
		Expect(err).ShouldNot(HaveOccurred(), "failed to list roles")
		Expect(&roles).Should(ContainItemWithPrefix(rolePrefix), "unable to find roles with prefix %s", rolePrefix)

		ginkgo.By("checking the rolebinding exists")
		var rolebindings rbacv1.RoleBindingList
		err = k8s.List(ctx, &rolebindings)
		Expect(err).ShouldNot(HaveOccurred(), "failed to list rolebindings")
		Expect(&rolebindings).Should(ContainItemWithPrefix(rolePrefix), "unable to find rolebindings with prefix %s", rolePrefix)

		ginkgo.By("checking the clusterrole exists")
		var clusterRoles rbacv1.ClusterRoleList
		err = k8s.List(ctx, &clusterRoles)
		Expect(err).ShouldNot(HaveOccurred(), "failed to list clusterroles")
		Expect(&clusterRoles).Should(ContainItemWithPrefix(clusterRolePrefix), "unable to find cluster role with prefix %s", clusterRolePrefix)

		ginkgo.By("checking the clusterrolebinding exists")
		var clusterRoleBindings rbacv1.ClusterRoleBindingList
		err = k8s.List(ctx, &clusterRoleBindings)
		Expect(err).ShouldNot(HaveOccurred(), "unable to list clusterrolebindings")
		Expect(&clusterRoleBindings).Should(ContainItemWithPrefix(clusterRolePrefix), "unable to find clusterrolebinding with prefix %s", clusterRolePrefix)

		ginkgo.By("checking the service exists")
		err = k8s.Get(ctx, serviceName, namespace, &corev1.Service{})
		Expect(err).ShouldNot(HaveOccurred(), "service %s/%s not found", namespace, serviceName)

		ginkgo.By("checking the deployment exists and is available")
		EventuallyDeployment(ctx, k8s, deploymentName, namespace).Should(BeAvailable())
	})

	ginkgo.It("is exporting metrics", func(ctx context.Context) {
		ginkgo.By("reading cluster ID from ClusterVersion (exporter uses this for metric _id label)")
		var cv configv1.ClusterVersion
		err := k8s.Get(ctx, "version", "", &cv)
		Expect(err).ShouldNot(HaveOccurred(), "failed to get ClusterVersion")
		metricClusterID := string(cv.Spec.ClusterID)
		Expect(metricClusterID).ShouldNot(BeEmpty(), "ClusterVersion spec.clusterID is empty")

		ginkgo.By("checking Prometheus is scraping the exporter")
		results, err := prom.InstantQuery(ctx, `up{job="osd-metrics-exporter"}`)
		Expect(err).ShouldNot(HaveOccurred(), "failed to query prometheus")

		result := results[0].Value
		Expect(int(result)).Should(BeNumerically("==", 1), "prometheus exporter is not healthy")

		user := clustersmgmtv1.NewHTPasswdIdentityProvider().Username(rand.String(14)).Password(generateRandomString(14))

		idp, err := clustersmgmtv1.NewIdentityProvider().Htpasswd(user).Type(clustersmgmtv1.IdentityProviderTypeHtpasswd).Name("osde2e").Build()
		Expect(err).ShouldNot(HaveOccurred(), "unable to build htpasswd IDP object")

		idpClient := ocmClient.Connection.ClustersMgmt().V1().Clusters().Cluster(clusterID).IdentityProviders()
		idpAddResponse, err := idpClient.Add().Body(idp).SendContext(ctx)
		Expect(err).ShouldNot(HaveOccurred(), "failed to create htpasswd IDP for cluster")
		ginkgo.DeferCleanup(idpClient.IdentityProvider(idpAddResponse.Body().ID()).Delete().SendContext)

		Eventually(ctx, func(ctx context.Context) (int, error) {
			query := `identity_provider{provider="HTPasswd", name="osd_exporter"}`
			results, err = prom.InstantQuery(ctx, query)
			if err != nil {
				return 0, err
			}
			return int(results[0].Value), nil
		}).
			WithPolling(5*time.Second).
			WithTimeout(5*time.Minute).
			Should(BeNumerically("==", 1), "identity_provider metric has not updated")

		ginkgo.By("validating cluster_id metric labels and value")
		results, err = prom.InstantQuery(ctx, `cluster_id{_id="`+metricClusterID+`", name="osd_exporter"}`)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(results).ShouldNot(BeEmpty(), "cluster_id metric not found")
		Expect(int(results[0].Value)).Should(BeNumerically("==", 1))

		ginkgo.By("validating limited_support_enabled metric")
		results, err = prom.InstantQuery(ctx, `limited_support_enabled{_id="`+metricClusterID+`", name="osd_exporter"}`)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(results).ShouldNot(BeEmpty())
		Expect(int(results[0].Value)).Should(BeElementOf(0, 1))

		ginkgo.By("validating cluster_admin_enabled metric")
		results, err = prom.InstantQuery(ctx, `cluster_admin_enabled{_id="`+metricClusterID+`", name="osd_exporter"}`)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(results).ShouldNot(BeEmpty())
		Expect(int(results[0].Value)).Should(BeElementOf(0, 1))

		ginkgo.By("validating cluster_proxy metric")
		results, err = prom.InstantQuery(ctx, `cluster_proxy{_id="`+metricClusterID+`", name="osd_exporter"}`)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(results).ShouldNot(BeEmpty())
		Expect(results[0].Value).Should(BeNumerically(">=", 0))
		Expect(results[0].Value).Should(BeNumerically("<=", 1))

		ginkgo.By("validating cluster_proxy_ca_valid metric")
		results, err = prom.InstantQuery(ctx, `cluster_proxy_ca_valid{_id="`+metricClusterID+`", name="osd_exporter"}`)
		Expect(err).ShouldNot(HaveOccurred())
		if len(results) > 0 {
			Expect(int(results[0].Value)).Should(BeElementOf(0, 1))
		}

		ginkgo.By("validating cluster_proxy_ca_expiry_timestamp when present")
		results, err = prom.InstantQuery(ctx, `cluster_proxy_ca_expiry_timestamp{_id="`+metricClusterID+`", name="osd_exporter"}`)
		Expect(err).ShouldNot(HaveOccurred())
		for i := range results {
			ts := int64(results[i].Value)
			Expect(ts).Should(BeNumerically(">=", 1577836800))
			Expect(ts).Should(BeNumerically("<=", 2208988800))
		}

		ginkgo.By("validating identity_provider has valid counts for all series")
		results, err = prom.InstantQuery(ctx, `identity_provider{name="osd_exporter"}`)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(results).ShouldNot(BeEmpty())
		for i := range results {
			Expect(results[i].Value).Should(BeNumerically(">=", 0))
		}

		ginkgo.By("validating cpms_enabled metric")
		results, err = prom.InstantQuery(ctx, `cpms_enabled{_id="`+metricClusterID+`", name="osd_exporter"}`)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(results).ShouldNot(BeEmpty())
		Expect(int(results[0].Value)).Should(BeElementOf(0, 1))

		ginkgo.By("validating pods_preventing_node_drain when series exist")
		results, err = prom.InstantQuery(ctx, `pods_preventing_node_drain`)
		Expect(err).ShouldNot(HaveOccurred())
		for i := range results {
			Expect(results[i].Value).Should(BeNumerically("==", 1))
		}

		ginkgo.By("ensuring all expected metric names are queryable")
		expectedMetricNames := []string{
			"identity_provider", "cluster_admin_enabled", "limited_support_enabled",
			"cluster_proxy", "cluster_proxy_ca_expiry_timestamp", "cluster_proxy_ca_valid",
			"cluster_id", "pods_preventing_node_drain", "cpms_enabled",
		}
		optionalMetrics := map[string]bool{
			"cluster_proxy_ca_expiry_timestamp": true,
			"cluster_proxy_ca_valid":            true,
			"pods_preventing_node_drain":        true,
		}
		for _, metricName := range expectedMetricNames {
			query := metricName + `{name="osd_exporter"}`
			if metricName == "pods_preventing_node_drain" {
				query = metricName
			}
			results, err = prom.InstantQuery(ctx, query)
			Expect(err).ShouldNot(HaveOccurred(), "failed to query metric %s", metricName)
			if !optionalMetrics[metricName] {
				Expect(results).ShouldNot(BeEmpty(), "metric %s has no series", metricName)
			}
		}
	})

	ginkgo.It("all exported metrics are valid after operator upgrade", func(ctx context.Context) {
		// Run after operator upgrade in CI to ensure metric export still works.
		// Wait for exporter to be up (rolling update may cause brief downtime).
		Eventually(ctx, func(ctx context.Context) (int, error) {
			results, err := prom.InstantQuery(ctx, `up{job="osd-metrics-exporter"}`)
			if err != nil || len(results) == 0 {
				return 0, err
			}
			return int(results[0].Value), nil
		}).
			WithPolling(5*time.Second).
			WithTimeout(5*time.Minute).
			Should(BeNumerically("==", 1), "exporter target not up after upgrade")

		expectedMetricNames := []string{
			"identity_provider", "cluster_admin_enabled", "limited_support_enabled",
			"cluster_proxy", "cluster_proxy_ca_expiry_timestamp", "cluster_proxy_ca_valid",
			"cluster_id", "pods_preventing_node_drain", "cpms_enabled",
		}
		optionalMetrics := map[string]bool{
			"cluster_proxy_ca_expiry_timestamp": true,
			"cluster_proxy_ca_valid":            true,
			"pods_preventing_node_drain":        true,
		}
		for _, metricName := range expectedMetricNames {
			query := metricName + `{name="osd_exporter"}`
			if metricName == "pods_preventing_node_drain" {
				query = metricName
			}
			results, err := prom.InstantQuery(ctx, query)
			Expect(err).ShouldNot(HaveOccurred(), "metric %s query failed after upgrade", metricName)
			if !optionalMetrics[metricName] {
				Expect(results).ShouldNot(BeEmpty(), "metric %s has no series after upgrade", metricName)
			}
		}
	})
})

// generates password to set up ocm htpasswd auth
// Password must include uppercase letters, lowercase letters, and numbers or symbols (ASCII-standard characters only)
func generateRandomString(length int) string {
	const (
		lowers  = "abcdefghijklmnopqrstuvwxyz"
		uppers  = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
		numbers = "0123456789"
	)
	var seededRand *mathrand.Rand = mathrand.New(mathrand.NewSource(time.Now().UnixNano()))
	var sb strings.Builder

	for i := 0; i < length; i++ {
		switch i % 3 {
		case 0:
			sb.WriteByte(lowers[seededRand.Intn(len(lowers))])
		case 1:
			sb.WriteByte(uppers[seededRand.Intn(len(uppers))])
		case 2:
			sb.WriteByte(numbers[seededRand.Intn(len(numbers))])
		}
	}
	return sb.String()
}
