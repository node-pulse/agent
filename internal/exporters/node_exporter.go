package exporters

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/node-pulse/agent/internal/logger"
)

// NodeExporter implements the Exporter interface for Prometheus node_exporter
type NodeExporter struct {
	endpoint string
	client   *http.Client
}

// NewNodeExporter creates a new node_exporter scraper
func NewNodeExporter(endpoint string, timeout time.Duration) *NodeExporter {
	if endpoint == "" {
		endpoint = "http://localhost:9100/metrics"
	}
	if timeout == 0 {
		timeout = 3 * time.Second
	}

	return &NodeExporter{
		endpoint: endpoint,
		client: &http.Client{
			Timeout: timeout,
		},
	}
}

// Ensure NodeExporter implements Exporter interface
var _ Exporter = (*NodeExporter)(nil)

func (n *NodeExporter) Name() string {
	return "node_exporter"
}

func (n *NodeExporter) Scrape(ctx context.Context) ([]byte, error) {
	logger.Debug("Scraping node_exporter", logger.String("endpoint", n.endpoint))

	req, err := http.NewRequestWithContext(ctx, "GET", n.endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := n.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server returned status %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	logger.Debug("Successfully scraped node_exporter",
		logger.String("endpoint", n.endpoint),
		logger.Int("bytes", len(data)))

	return data, nil
}

func (n *NodeExporter) Verify() error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	_, err := n.Scrape(ctx)
	if err != nil {
		return fmt.Errorf("node_exporter verification failed: %w", err)
	}

	logger.Info("node_exporter verified", logger.String("endpoint", n.endpoint))
	return nil
}

func (n *NodeExporter) DefaultEndpoint() string {
	return "http://localhost:9100/metrics"
}

func (n *NodeExporter) DefaultInterval() time.Duration {
	return 15 * time.Second
}
