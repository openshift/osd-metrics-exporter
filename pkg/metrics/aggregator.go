package metrics

import (
	"sync"
	"time"

	configv1 "github.com/openshift/api/config/v1"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	providerLabel    = "provider"
	osdExporterValue = "osd_exporter"
	proxyHTTPLabel   = "http"
	proxyHTTPSLabel  = "https"
	proxyCALabel     = "trusted_ca"
)

var knownIdentityProviderTypes = []configv1.IdentityProviderType{
	configv1.IdentityProviderTypeBasicAuth,
	configv1.IdentityProviderTypeGitHub,
	configv1.IdentityProviderTypeGitLab,
	configv1.IdentityProviderTypeGoogle,
	configv1.IdentityProviderTypeHTPasswd,
	configv1.IdentityProviderTypeKeystone,
	configv1.IdentityProviderTypeLDAP,
	configv1.IdentityProviderTypeOpenID,
	configv1.IdentityProviderTypeRequestHeader,
}

type providerKey struct {
	name      string
	namespace string
}

type AdoptionMetricsAggregator struct {
	identityProviders   *prometheus.GaugeVec
	clusterAdmin        prometheus.Gauge
	providerMap         map[providerKey][]configv1.IdentityProviderType
	clusterProxy        *prometheus.GaugeVec
	mutex               sync.Mutex
	aggregationInterval time.Duration
}

// NewMetricsAggregator creates a metric aggregator. Should not be used directory but through GetMetricsAggregator
func NewMetricsAggregator(aggregationInterval time.Duration) *AdoptionMetricsAggregator {
	collector := &AdoptionMetricsAggregator{
		identityProviders: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name:        "identity_provider",
			Help:        "Indicates if an identity provider is enabled",
			ConstLabels: map[string]string{"name": osdExporterValue},
		}, []string{providerLabel}),
		clusterAdmin: prometheus.NewGauge(prometheus.GaugeOpts{
			Name:        "cluster_admin_enabled",
			Help:        "Indicates if the cluster-admin role is enabled",
			ConstLabels: map[string]string{"name": osdExporterValue},
		}),
		clusterProxy: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name:        "cluster_proxy",
			Help:        "Indicates cluster proxy state",
			ConstLabels: map[string]string{"name": osdExporterValue},
		}, []string{proxyHTTPLabel, proxyHTTPSLabel, proxyCALabel}),
		providerMap:         make(map[providerKey][]configv1.IdentityProviderType),
		aggregationInterval: aggregationInterval,
	}
	collector.clusterAdmin.Set(0)
	return collector
}

func (a *AdoptionMetricsAggregator) Run() chan interface{} {
	ticker := time.NewTicker(a.aggregationInterval)
	done := make(chan interface{})
	go func() {
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				a.aggregate()
			}
		}
	}()
	return done
}

func (a *AdoptionMetricsAggregator) SetOAuthIDP(name, namespace string, provider []configv1.IdentityProvider) {
	providerTypes := make([]configv1.IdentityProviderType, len(provider))
	for i, p := range provider {
		providerTypes[i] = p.Type
	}
	a.mutex.Lock()
	defer a.mutex.Unlock()
	a.providerMap[providerKey{name: name, namespace: namespace}] = providerTypes
}

func (a *AdoptionMetricsAggregator) DeleteOAuthIDP(name, namespace string) {
	a.mutex.Lock()
	defer a.mutex.Unlock()
	delete(a.providerMap, providerKey{name: name, namespace: namespace})
}

func (a *AdoptionMetricsAggregator) aggregate() {
	a.mutex.Lock()
	defer a.mutex.Unlock()
	providers := make(map[configv1.IdentityProviderType]int)
	for _, v := range a.providerMap {
		for _, p := range v {
			if _, ok := providers[p]; !ok {
				providers[p] = 0
			}
			providers[p] += 1
		}
	}

	for _, t := range knownIdentityProviderTypes {
		if count, ok := providers[t]; ok {
			a.identityProviders.With(prometheus.Labels{providerLabel: string(t)}).Set(float64(count))
		} else {
			a.identityProviders.With(prometheus.Labels{providerLabel: string(t)}).Set(0)
		}
	}
}

func (a *AdoptionMetricsAggregator) SetClusterAdmin(enabled bool) {
	if enabled {
		a.clusterAdmin.Set(1)
	} else {
		a.clusterAdmin.Set(0)
	}
}

func (a *AdoptionMetricsAggregator) SetClusterProxy(proxyHTTP string, proxyHTTPS string, proxyTrustedCA string, proxyEnabled int) {
	a.clusterProxy.With(prometheus.Labels{
		proxyHTTPLabel:  proxyHTTP,
		proxyHTTPSLabel: proxyHTTPS,
		proxyCALabel:    proxyTrustedCA,
	}).Set(float64(proxyEnabled))
}

func (a *AdoptionMetricsAggregator) GetMetrics() []prometheus.Collector {
	return []prometheus.Collector{a.identityProviders, a.clusterAdmin, a.clusterProxy}
}

func (a *AdoptionMetricsAggregator) GetClusterRoleMetric() prometheus.Gauge {
	return a.clusterAdmin
}

func (a *AdoptionMetricsAggregator) GetIdentityProviderMetric() *prometheus.GaugeVec {
	return a.identityProviders
}

func (a *AdoptionMetricsAggregator) GetClusterProxyMetric() *prometheus.GaugeVec {
	return a.clusterProxy
}
