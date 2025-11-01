package exporters

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

// ProcessExporter represents a Prometheus process_exporter instance
type ProcessExporter struct {
	name     string
	endpoint string
	timeout  time.Duration
	client   *http.Client
}

var _ Exporter = (*ProcessExporter)(nil)

// NewProcessExporter creates a new ProcessExporter instance
func NewProcessExporter(endpoint string, timeout time.Duration) *ProcessExporter {
	// Use defaults if not specified
	if endpoint == "" {
		endpoint = "http://127.0.0.1:9256/metrics"
	}
	if timeout == 0 {
		timeout = 3 * time.Second
	}

	return &ProcessExporter{
		name:     "process_exporter",
		endpoint: endpoint,
		timeout:  timeout,
		client: &http.Client{
			Timeout: timeout,
		},
	}
}

// Name returns the exporter name
func (e *ProcessExporter) Name() string {
	return e.name
}

// Endpoint returns the metrics endpoint URL
func (e *ProcessExporter) Endpoint() string {
	return e.endpoint
}

// Scrape fetches metrics from process_exporter
func (e *ProcessExporter) Scrape(ctx context.Context) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", e.endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	return data, nil
}

// Verify checks if the exporter is accessible
func (e *ProcessExporter) Verify() error {
	ctx, cancel := context.WithTimeout(context.Background(), e.timeout)
	defer cancel()

	_, err := e.Scrape(ctx)
	return err
}
