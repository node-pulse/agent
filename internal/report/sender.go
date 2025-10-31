package report

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/node-pulse/agent/internal/config"
	"github.com/node-pulse/agent/internal/logger"
	"github.com/node-pulse/agent/internal/prometheus"
)

// Sender handles sending metrics reports to the server
// New architecture: Write-Ahead Log (WAL) pattern
// - All metrics are written to buffer first
// - Separate goroutine drains buffer continuously with random jitter
type Sender struct {
	config     *config.Config
	client     *http.Client
	buffer     *Buffer
	drainCtx   context.Context
	drainStop  context.CancelFunc
	rng        *rand.Rand
}

// NewSender creates a new report sender
func NewSender(cfg *config.Config) (*Sender, error) {
	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: cfg.Server.Timeout,
	}

	// Create buffer (always enabled in new architecture)
	buffer, err := NewBuffer(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create buffer: %w", err)
	}

	// Create context for drain goroutine
	ctx, cancel := context.WithCancel(context.Background())

	// Create random number generator with time-based seed for jitter
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))

	return &Sender{
		config:    cfg,
		client:    client,
		buffer:    buffer,
		drainCtx:  ctx,
		drainStop: cancel,
		rng:       rng,
	}, nil
}

// BufferPrometheus saves Prometheus text format data to buffer
// The data will be sent asynchronously by the drain goroutine (after parsing to JSON)
func (s *Sender) BufferPrometheus(data []byte, serverID string, exporterName string) error {
	// Always save to buffer first (WAL pattern)
	if err := s.buffer.SavePrometheus(data, serverID, exporterName); err != nil {
		return fmt.Errorf("failed to save prometheus data to buffer: %w", err)
	}

	logger.Debug("Prometheus data saved to buffer",
		logger.String("exporter", exporterName),
		logger.String("server_id", serverID),
		logger.Int("bytes", len(data)))
	return nil
}

// sendJSONHTTP sends JSON metrics to server
func (s *Sender) sendJSONHTTP(data []byte, serverID string) error {
	// Build URL with server_id query parameter
	endpoint := s.config.Server.Endpoint
	u, err := url.Parse(endpoint)
	if err != nil {
		return fmt.Errorf("invalid endpoint URL: %w", err)
	}

	// Add server_id query parameter
	q := u.Query()
	q.Set("server_id", serverID)
	u.RawQuery = q.Encode()

	// Create request
	req, err := http.NewRequest("POST", u.String(), bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "nodepulse-agent/2.0")

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	// Read response body (and discard it)
	io.Copy(io.Discard, resp.Body)

	// Check status code
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("server returned status %d", resp.StatusCode)
	}

	return nil
}

// StartDraining starts the background goroutine that continuously drains the buffer
// It should be called once after creating the sender
func (s *Sender) StartDraining() {
	go s.drainLoop()
	logger.Info("Started buffer drain goroutine with random jitter")
}

// drainLoop continuously drains the buffer with random delays
// Uses smart batching to group files by time windows (5s buckets)
func (s *Sender) drainLoop() {
	for {
		// Check if context is cancelled
		select {
		case <-s.drainCtx.Done():
			logger.Info("Drain goroutine stopped")
			return
		default:
		}

		// Get all buffer files (oldest first)
		files, err := s.buffer.GetBufferFiles()
		if err != nil {
			logger.Warn("Failed to get buffer files for draining", logger.Err(err))
			s.randomDelay()
			continue
		}

		// If no files to process, wait and check again
		if len(files) == 0 {
			s.randomDelay()
			continue
		}

		// Group files by time window (5s buckets) for efficient batching
		timeWindows := s.groupFilesByTimeWindow(files, 5*time.Second)

		// Process first time window (oldest files first)
		if len(timeWindows) > 0 {
			firstWindow := timeWindows[0]

			// Limit batch size to configured batch_size
			batchSize := len(firstWindow)
			if batchSize > s.config.Buffer.BatchSize {
				batchSize = s.config.Buffer.BatchSize
			}

			batch := firstWindow[:batchSize]
			if err := s.processBatch(batch); err != nil {
				// Failed to send - keep files and retry after delay
				logger.Debug("Failed to process batch, will retry",
					logger.Int("batch_size", batchSize),
					logger.Err(err))
			}
		}

		// Wait random delay before next attempt
		s.randomDelay()
	}
}

// processBatch loads and sends buffered files grouped by exporter
// Returns error if send fails (files are kept for retry)
// Payload format: { "node_exporter": [...], "postgres_exporter": [...] }
func (s *Sender) processBatch(filePaths []string) error {
	if len(filePaths) == 0 {
		return nil
	}

	// Group entries by exporter name
	exporterMetrics := make(map[string][]prometheus.NodeExporterMetricSnapshot)
	processedFiles := []string{}
	var serverID string

	for _, filePath := range filePaths {
		// Only process .prom files
		if !strings.HasSuffix(filePath, ".prom") {
			logger.Warn("Unexpected buffer file type, skipping", logger.String("file", filePath))
			continue
		}

		// Load file
		entry, err := s.buffer.LoadPrometheusFile(filePath)
		if err != nil {
			// File is corrupted - delete it
			logger.Warn("Corrupted buffer file detected, deleting",
				logger.String("file", filePath),
				logger.Err(err))
			if delErr := s.buffer.DeleteFile(filePath); delErr != nil {
				logger.Error("Failed to delete corrupted buffer file",
					logger.String("file", filePath),
					logger.Err(delErr))
			}
			continue
		}

		// Store server ID (all files should have same server_id)
		if serverID == "" {
			serverID = entry.ServerID
		}

		// Parse Prometheus text to structured metrics
		// Note: Currently only node_exporter is parsed, other exporters need their own parsers
		snapshot, err := prometheus.ParseNodeExporterMetrics(entry.Data)
		if err != nil {
			logger.Warn("Failed to parse node_exporter metrics, using zero values",
				logger.String("exporter", entry.ExporterName),
				logger.String("file", filePath),
				logger.Err(err))
			// Use zero-value snapshot
			snapshot = &prometheus.NodeExporterMetricSnapshot{
				Timestamp: time.Now().UTC(),
			}
		}

		// Add to exporter's array
		exporterMetrics[entry.ExporterName] = append(
			exporterMetrics[entry.ExporterName],
			*snapshot,
		)

		processedFiles = append(processedFiles, filePath)
	}

	// Nothing to send
	if len(exporterMetrics) == 0 {
		return nil
	}

	// Build payload: { "node_exporter": [...], "mysql_exporter": [...] }
	payload := make(map[string]interface{})
	for exporterName, snapshots := range exporterMetrics {
		payload[exporterName] = snapshots
	}

	// Convert to JSON
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal batch: %w", err)
	}

	// Send batch via HTTP
	if err := s.sendJSONHTTP(jsonData, serverID); err != nil {
		// Send failed - keep all files for retry
		logger.Debug("Failed to send batch, will retry",
			logger.Int("batch_size", len(processedFiles)),
			logger.Err(err))
		return err
	}

	// Success - delete all files in batch
	successCount := 0
	for _, filePath := range processedFiles {
		if err := s.buffer.DeleteFile(filePath); err != nil {
			logger.Error("Failed to delete buffer file after successful send",
				logger.String("file", filePath),
				logger.Err(err))
		} else {
			successCount++
		}
	}

	if successCount > 0 {
		logger.Info("Successfully sent buffered data",
			logger.Int("files", successCount),
			logger.Int("exporters", len(exporterMetrics)))

		// Periodically clean up old buffer files
		if err := s.buffer.Cleanup(); err != nil {
			logger.Warn("Failed to cleanup old buffer files", logger.Err(err))
		}
	}

	return nil
}

// groupFilesByTimeWindow groups files into time buckets (e.g., 5s windows)
// This allows batching multiple exporters that scraped at similar times
// Returns a list of time windows (oldest first), each containing file paths
func (s *Sender) groupFilesByTimeWindow(filePaths []string, windowSize time.Duration) [][]string {
	// Map: timestamp bucket -> file paths
	windows := make(map[int64][]string)

	for _, filePath := range filePaths {
		// Parse timestamp from filename: YYYYMMDD-HHMMSS-...
		timestamp, err := parseTimestampFromFilename(filePath)
		if err != nil {
			logger.Warn("Failed to parse timestamp from filename, skipping",
				logger.String("file", filePath),
				logger.Err(err))
			continue
		}

		// Bucket by time window (e.g., 5s buckets)
		bucket := timestamp.Unix() / int64(windowSize.Seconds())
		windows[bucket] = append(windows[bucket], filePath)
	}

	// Convert to sorted list of windows (oldest first)
	buckets := make([]int64, 0, len(windows))
	for bucket := range windows {
		buckets = append(buckets, bucket)
	}
	sort.Slice(buckets, func(i, j int) bool {
		return buckets[i] < buckets[j]
	})

	result := make([][]string, 0, len(buckets))
	for _, bucket := range buckets {
		result = append(result, windows[bucket])
	}

	return result
}

// parseTimestampFromFilename extracts timestamp from buffer filename
// Format: buffer/<exporter>/YYYYMMDD-HHMMSS-<server_id>.prom
func parseTimestampFromFilename(filePath string) (time.Time, error) {
	filename := filepath.Base(filePath)

	// Remove .prom extension
	if !strings.HasSuffix(filename, ".prom") {
		return time.Time{}, fmt.Errorf("invalid file extension")
	}

	// Extract timestamp part (first two segments: YYYYMMDD-HHMMSS)
	parts := strings.SplitN(strings.TrimSuffix(filename, ".prom"), "-", 3)
	if len(parts) < 2 {
		return time.Time{}, fmt.Errorf("invalid filename format")
	}

	timeStr := parts[0] + "-" + parts[1]

	// Parse timestamp
	timestamp, err := time.Parse("20060102-150405", timeStr)
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to parse timestamp: %w", err)
	}

	return timestamp, nil
}

// randomDelay waits for a random duration between 0 and the configured interval
// This distributes load across the interval window
func (s *Sender) randomDelay() {
	// Generate random delay: 0 to full interval
	maxDelay := s.config.Agent.Interval
	delay := time.Duration(s.rng.Int63n(int64(maxDelay)))

	logger.Debug("Waiting random delay before next drain attempt", logger.Duration("delay", delay))

	// Use select to make delay cancellable
	select {
	case <-s.drainCtx.Done():
		return
	case <-time.After(delay):
		return
	}
}

// Close stops the drain goroutine and closes the sender
func (s *Sender) Close() error {
	// Stop drain goroutine
	if s.drainStop != nil {
		s.drainStop()
	}

	// Close buffer
	if s.buffer != nil {
		return s.buffer.Close()
	}
	return nil
}

// GetBufferStatus returns the current buffer status
func (s *Sender) GetBufferStatus() BufferStatus {
	if s == nil || s.buffer == nil {
		return BufferStatus{}
	}
	return s.buffer.GetBufferStatus()
}
