package metrics

import "time"

const (
	aggregatorResyncInterval = time.Minute
)

var (
	aggregator *AdoptionMetricsAggregator
)

func GetMetricsAggregator() *AdoptionMetricsAggregator {
	if aggregator == nil {
		aggregator = NewMetricsAggregator(aggregatorResyncInterval)
	}
	return aggregator
}
