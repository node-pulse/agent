package metrics

import "errors"

var (
	// ErrAllMetricsFailed is returned when all metric collections fail
	ErrAllMetricsFailed = errors.New("all metrics collection failed")
)
