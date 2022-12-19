package metrics

import "time"

const (
	aggregatorResyncInterval = time.Minute
	clusterId                = clusterIDLabel
)

var (
	aggregator *AdoptionMetricsAggregator
)

func GetMetricsAggregator() *AdoptionMetricsAggregator {
	if aggregator == nil {
		aggregator = NewMetricsAggregator(aggregatorResyncInterval, clusterId)
	}
	return aggregator
}
