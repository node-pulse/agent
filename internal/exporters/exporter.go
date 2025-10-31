package exporters

import (
	"context"
	"time"
)

// Exporter defines the interface that all metrics exporters must implement
type Exporter interface {
	// Name returns the unique identifier for this exporter
	// Examples: "node_exporter", "postgres_exporter", "redis_exporter"
	Name() string

	// Scrape retrieves metrics from the exporter endpoint
	// Returns raw Prometheus text format
	Scrape(ctx context.Context) ([]byte, error)

	// Verify checks if the exporter is accessible (used at startup)
	Verify() error

	// DefaultEndpoint returns the standard endpoint for this exporter
	// Example: "http://localhost:9100/metrics" for node_exporter
	DefaultEndpoint() string

	// DefaultInterval returns the recommended scrape interval
	// Example: 15s for node_exporter, 30s for postgres_exporter
	DefaultInterval() time.Duration
}
