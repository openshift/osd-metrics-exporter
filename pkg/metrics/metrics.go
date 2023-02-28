package metrics

import "time"

const (
	aggregatorResyncInterval = time.Minute
)

var (
	aggregator *AdoptionMetricsAggregator
)

func GetMetricsAggregator(clusterId string) *AdoptionMetricsAggregator {
	if aggregator == nil {
		aggregator = NewMetricsAggregator(aggregatorResyncInterval, clusterId)
	}
	return aggregator
}
