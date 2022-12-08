package metrics

import (
	"os"
	"sync"
	"time"

	configv1 "github.com/openshift/api/config/v1"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	providerLabel       = "provider"
	osdExporterValue    = "osd_exporter"
	proxyHTTPLabel      = "http"
	proxyHTTPSLabel     = "https"
	proxyCALabel        = "trusted_ca"
	proxyCASubjectLabel = "subject"
	clusterIDLabel      = "_id"
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
	identityProviders    *prometheus.GaugeVec
	clusterAdmin         prometheus.Gauge
	limitedSupport       *prometheus.GaugeVec
	providerMap          map[providerKey][]configv1.IdentityProviderType
	clusterProxy         *prometheus.GaugeVec
	clusterProxyCAExpiry *prometheus.GaugeVec
	clusterProxyCAValid  prometheus.Gauge
	clusterID            *prometheus.GaugeVec
	mutex                sync.Mutex
	aggregationInterval  time.Duration
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
		limitedSupport: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name:        "limited_support_enabled",
			Help:        "Indicates if limited support is enabled",
			ConstLabels: map[string]string{"name": osdExporterValue},
		}, []string{clusterIDLabel}),
		clusterProxy: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name:        "cluster_proxy",
			Help:        "Indicates cluster proxy state",
			ConstLabels: map[string]string{"name": osdExporterValue},
		}, []string{proxyHTTPLabel, proxyHTTPSLabel, proxyCALabel}),
		clusterProxyCAExpiry: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name:        "cluster_proxy_ca_expiry_timestamp",
			Help:        "Indicates cluster proxy CA expiry unix timestamp in UTC",
			ConstLabels: map[string]string{"name": osdExporterValue},
		}, []string{proxyCASubjectLabel}),
		clusterProxyCAValid: prometheus.NewGauge(prometheus.GaugeOpts{
			Name:        "cluster_proxy_ca_valid",
			Help:        "Indicates if cluster proxy CA valid",
			ConstLabels: map[string]string{"name": osdExporterValue},
		}),
		clusterID: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name:        "cluster_id",
			Help:        "Indicates the cluster id",
			ConstLabels: map[string]string{"name": osdExporterValue},
		}, []string{clusterIDLabel}),
		providerMap:         make(map[providerKey][]configv1.IdentityProviderType),
		aggregationInterval: aggregationInterval,
	}
	collector.clusterAdmin.Set(0)
	uuid := os.Getenv("CLUSTER_ID")
	collector.SetLimitedSupport(uuid, false)
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

func (a *AdoptionMetricsAggregator) SetLimitedSupport(uuid string, enabled bool) {
	labels := prometheus.Labels{
		clusterIDLabel: uuid,
	}

	if enabled {
		a.limitedSupport.With(labels).Set(1)

	} else {
		a.limitedSupport.With(labels).Set(0)

	}
}

func (a *AdoptionMetricsAggregator) SetClusterProxy(proxyHTTP string, proxyHTTPS string, proxyTrustedCA string, proxyEnabled int) {
	a.clusterProxy.With(prometheus.Labels{
		proxyHTTPLabel:  proxyHTTP,
		proxyHTTPSLabel: proxyHTTPS,
		proxyCALabel:    proxyTrustedCA,
	}).Set(float64(proxyEnabled))
}

func (a *AdoptionMetricsAggregator) SetClusterProxyCAExpiry(subject string, clusterProxyCAExpiry int64) {
	a.clusterProxyCAExpiry.With(prometheus.Labels{
		proxyCASubjectLabel: subject,
	}).Set(float64(clusterProxyCAExpiry))
}

func (a *AdoptionMetricsAggregator) SetClusterProxyCAValid(valid bool) {
	if valid {
		a.clusterProxyCAValid.Set(1)
	} else {
		a.clusterProxyCAValid.Set(0)
	}
}

func (a *AdoptionMetricsAggregator) SetClusterID(uuid string) {
	a.clusterID.With(prometheus.Labels{
		clusterIDLabel: uuid,
	}).Set(1)
}

func (a *AdoptionMetricsAggregator) GetMetrics() []prometheus.Collector {
	return []prometheus.Collector{a.identityProviders, a.clusterAdmin, a.limitedSupport, a.clusterProxy, a.clusterProxyCAExpiry, a.clusterProxyCAValid, a.clusterID}
}

func (a *AdoptionMetricsAggregator) GetClusterRoleMetric() prometheus.Gauge {
	return a.clusterAdmin
}

func (a *AdoptionMetricsAggregator) GetLimitedsupportStatus() *prometheus.GaugeVec {
	return a.limitedSupport
}

func (a *AdoptionMetricsAggregator) GetIdentityProviderMetric() *prometheus.GaugeVec {
	return a.identityProviders
}

func (a *AdoptionMetricsAggregator) GetClusterProxyMetric() *prometheus.GaugeVec {
	return a.clusterProxy
}

func (a *AdoptionMetricsAggregator) GetClusterIDMetric() *prometheus.GaugeVec {
	return a.clusterID
}

func (a *AdoptionMetricsAggregator) GetClusterProxyCAExpiryMetrics() *prometheus.GaugeVec {
	return a.clusterProxyCAExpiry
}

func (a *AdoptionMetricsAggregator) GetClusterProxyCAValidMetrics() prometheus.Gauge {
	return a.clusterProxyCAValid
}
