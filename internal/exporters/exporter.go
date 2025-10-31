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
}
