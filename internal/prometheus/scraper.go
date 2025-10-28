package prometheus

import (
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/node-pulse/agent/internal/logger"
)

// ScraperConfig holds configuration for Prometheus scraper
type ScraperConfig struct {
	Endpoint string        // e.g., "http://localhost:9100/metrics"
	Timeout  time.Duration // HTTP timeout
}

// Scraper scrapes Prometheus exporters via HTTP
type Scraper struct {
	config *ScraperConfig
	client *http.Client
}

// NewScraper creates a new Prometheus scraper
func NewScraper(cfg *ScraperConfig) *Scraper {
	return &Scraper{
		config: cfg,
		client: &http.Client{
			Timeout: cfg.Timeout,
		},
	}
}

// Scrape fetches Prometheus text format from the exporter
// Returns the raw Prometheus text format data
func (s *Scraper) Scrape() ([]byte, error) {
	logger.Debug("Scraping Prometheus exporter", logger.String("endpoint", s.config.Endpoint))

	resp, err := s.client.Get(s.config.Endpoint)
	if err != nil {
		return nil, fmt.Errorf("failed to scrape %s: %w", s.config.Endpoint, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("scrape returned status %d from %s", resp.StatusCode, s.config.Endpoint)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response from %s: %w", s.config.Endpoint, err)
	}

	logger.Debug("Successfully scraped Prometheus exporter",
		logger.String("endpoint", s.config.Endpoint),
		logger.Int("bytes", len(data)))

	return data, nil
}

// Verify checks if the Prometheus exporter is accessible
// Useful for startup checks
func (s *Scraper) Verify() error {
	_, err := s.Scrape()
	if err != nil {
		return fmt.Errorf("prometheus exporter verification failed: %w", err)
	}
	logger.Info("Prometheus exporter verified", logger.String("endpoint", s.config.Endpoint))
	return nil
}
