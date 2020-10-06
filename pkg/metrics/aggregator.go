package metrics

import (
	"sync"
	"time"

	v1 "github.com/openshift/api/config/v1"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	providerLabel = "provider"
)

type providerKey struct {
	name      string
	namespace string
}

type AdoptionMetricsAggregator struct {
	identityProviders   *prometheus.GaugeVec
	clusterAdmin        prometheus.Gauge
	providerMap         map[providerKey][]v1.IdentityProviderType
	mutex               sync.Mutex
	aggregationInterval time.Duration
}

func NewMetricsAggregator(aggregationInterval time.Duration) *AdoptionMetricsAggregator {
	collector := &AdoptionMetricsAggregator{
		identityProviders: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name:        "identity_provider",
			Help:        "Indicates if a identity provider is enabled",
			ConstLabels: map[string]string{"name": "osd_exporter"},
		}, []string{providerLabel}),
		clusterAdmin: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "cluster_admin_enabled",
			Help: "Indicates if the cluster-admin role is enabled",
		}),
		providerMap:         make(map[providerKey][]v1.IdentityProviderType),
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

func (a *AdoptionMetricsAggregator) SetOAuthIDP(name, namespace string, provider []v1.IdentityProvider) {
	providerTypes := make([]v1.IdentityProviderType, len(provider))
	for i, p := range provider {
		providerTypes[i] = p.Type
	}
	a.mutex.Lock()
	defer a.mutex.Unlock()
	a.providerMap[providerKey{name: name, namespace: namespace}] = providerTypes
}

func (a *AdoptionMetricsAggregator) DeleteAuthIDP(name, namespace string) {
	a.mutex.Lock()
	defer a.mutex.Unlock()
	delete(a.providerMap, providerKey{name: name, namespace: namespace})
}

func (a *AdoptionMetricsAggregator) aggregate() {
	a.mutex.Lock()
	defer a.mutex.Unlock()
	providers := make(map[v1.IdentityProviderType]int)
	for _, v := range a.providerMap {
		for _, p := range v {
			if _, ok := providers[p]; !ok {
				providers[p] = 0
			}
			providers[p] += 1
		}
	}

	for k, v := range providers {
		a.identityProviders.With(prometheus.Labels{providerLabel: string(k)}).Set(float64(v))
	}
}

func (a *AdoptionMetricsAggregator) SetClusterAdmin(enabled bool) {
	if enabled {
		a.clusterAdmin.Set(1)
	} else {
		a.clusterAdmin.Set(0)
	}
}

func (a *AdoptionMetricsAggregator) GetMetrics() []prometheus.Collector {
	return []prometheus.Collector{a.identityProviders, a.clusterAdmin}
}

func (a *AdoptionMetricsAggregator) GetClusterRoleMetric() prometheus.Gauge {
	return a.clusterAdmin
}

func (a *AdoptionMetricsAggregator) GetIdentityProviderMetric() *prometheus.GaugeVec {
	return a.identityProviders
}
