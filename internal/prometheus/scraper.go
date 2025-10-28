package prometheus

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"
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

// AddTimestamps adds explicit timestamps to Prometheus text format metrics
// This ensures all metrics are reported with aligned collection times
// Example: node_cpu_seconds_total{cpu="0",mode="idle"} 123.45 → node_cpu_seconds_total{cpu="0",mode="idle"} 123.45 1730102400000
func AddTimestamps(data []byte, collectionTime time.Time) []byte {
	timestampMs := collectionTime.UnixMilli()

	var result bytes.Buffer
	scanner := bufio.NewScanner(bytes.NewReader(data))

	for scanner.Scan() {
		line := scanner.Text()

		// Skip empty lines, comments, and metadata lines
		if len(line) == 0 || line[0] == '#' {
			result.WriteString(line)
			result.WriteString("\n")
			continue
		}

		// Parse metric line: metric_name{labels} value [timestamp]
		// If timestamp already exists, skip adding
		if strings.Contains(line, " ") {
			parts := strings.Fields(line)
			// If line has 3 parts (name, value, timestamp), timestamp already exists
			if len(parts) >= 3 {
				result.WriteString(line)
				result.WriteString("\n")
				continue
			}

			// Add timestamp (line has name and value, but no timestamp)
			result.WriteString(line)
			result.WriteString(fmt.Sprintf(" %d\n", timestampMs))
		} else {
			// Invalid line format, keep as-is
			result.WriteString(line)
			result.WriteString("\n")
		}
	}

	return result.Bytes()
}
