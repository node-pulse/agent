package report

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"time"

	"github.com/node-pulse/agent/internal/config"
	"github.com/node-pulse/agent/internal/logger"
	"github.com/node-pulse/agent/internal/metrics"
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

// Send saves a metrics report to the buffer
// The report will be sent asynchronously by the drain goroutine
func (s *Sender) Send(report *metrics.Report) error {
	// Always save to buffer first (WAL pattern)
	if err := s.buffer.Save(report); err != nil {
		return fmt.Errorf("failed to save to buffer: %w", err)
	}

	logger.Debug("Report saved to buffer", logger.String("server_id", report.ServerID))
	return nil
}

// sendHTTP sends data to the server via HTTP POST
func (s *Sender) sendHTTP(data []byte) error {
	req, err := http.NewRequest("POST", s.config.Server.Endpoint, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "node-pulse-agent/1.0")

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
// Batches up to 5 reports per request for efficiency
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

		// Determine batch size: up to configured batch_size
		batchSize := len(files)
		if batchSize > s.config.Buffer.BatchSize {
			batchSize = s.config.Buffer.BatchSize
		}

		// Process batch of files (oldest first)
		batch := files[:batchSize]
		if err := s.processBatch(batch); err != nil {
			// Failed to send - keep files and retry after delay
			logger.Debug("Failed to process batch, will retry", logger.Int("batch_size", batchSize), logger.Err(err))
		}

		// Wait random delay before next attempt
		s.randomDelay()
	}
}

// processBatch loads and sends a batch of buffer files as a single array request
// Returns error if send fails (files are kept for retry)
// Handles corrupted files by sending N/A markers
func (s *Sender) processBatch(filePaths []string) error {
	var reports []*metrics.Report
	var validFiles []string

	// Load all reports from the batch
	for _, filePath := range filePaths {
		fileReports, err := s.buffer.LoadFile(filePath)
		if err != nil {
			// File is corrupted - send N/A marker and delete
			logger.Warn("Corrupted buffer file detected in batch, sending N/A metrics",
				logger.String("file", filePath),
				logger.Err(err))

			// Create N/A report for this corrupted file
			naReport := s.createNAReport()
			reports = append(reports, naReport)

			// Delete corrupted file immediately
			if delErr := s.buffer.DeleteFile(filePath); delErr != nil {
				logger.Error("Failed to delete corrupted buffer file",
					logger.String("file", filePath),
					logger.Err(delErr))
			} else {
				logger.Info("Deleted corrupted buffer file", logger.String("file", filePath))
			}
			continue
		}

		// File loaded successfully - add to batch
		if len(fileReports) > 0 {
			reports = append(reports, fileReports...)
			validFiles = append(validFiles, filePath)
		}
	}

	// If no reports to send, we're done
	if len(reports) == 0 {
		return nil
	}

	// Send batch as array
	if err := s.sendBatch(reports); err != nil {
		// Send failed - keep valid files for retry
		return fmt.Errorf("failed to send batch of %d reports: %w", len(reports), err)
	}

	// Send succeeded - delete all valid files
	for _, filePath := range validFiles {
		if err := s.buffer.DeleteFile(filePath); err != nil {
			logger.Error("Failed to delete buffer file after successful send",
				logger.String("file", filePath),
				logger.Err(err))
		}
	}

	logger.Info("Successfully sent batch",
		logger.Int("reports", len(reports)),
		logger.Int("files", len(validFiles)))

	// Periodically clean up old buffer files
	if err := s.buffer.Cleanup(); err != nil {
		logger.Warn("Failed to cleanup old buffer files", logger.Err(err))
	}

	return nil
}

// processBufferFile attempts to send all reports from a buffer file (DEPRECATED - kept for compatibility)
// Returns error if any report fails to send (file is kept for retry)
// If file is corrupted, sends N/A metrics and deletes the corrupted file
func (s *Sender) processBufferFile(filePath string) error {
	// Load reports from this file
	reports, err := s.buffer.LoadFile(filePath)
	if err != nil {
		// File is corrupted - send N/A metrics and delete the file
		logger.Warn("Corrupted buffer file detected, sending N/A metrics",
			logger.String("file", filePath),
			logger.Err(err))

		if sendErr := s.sendCorruptedFileMarker(filePath); sendErr != nil {
			logger.Error("Failed to send N/A marker for corrupted file",
				logger.String("file", filePath),
				logger.Err(sendErr))
			// Continue anyway - we want to delete the corrupted file
		}

		// Delete corrupted file to prevent infinite loop
		if delErr := s.buffer.DeleteFile(filePath); delErr != nil {
			logger.Error("Failed to delete corrupted buffer file",
				logger.String("file", filePath),
				logger.Err(delErr))
		} else {
			logger.Info("Deleted corrupted buffer file", logger.String("file", filePath))
		}

		return nil // Don't return error - we handled it by deleting
	}

	// If no reports in file (empty), just delete it
	if len(reports) == 0 {
		logger.Warn("Empty buffer file, deleting", logger.String("file", filePath))
		if err := s.buffer.DeleteFile(filePath); err != nil {
			logger.Error("Failed to delete empty buffer file", logger.String("file", filePath), logger.Err(err))
		}
		return nil
	}

	// Try to send all reports from this file
	for _, report := range reports {
		data, err := report.ToJSON()
		if err != nil {
			logger.Debug("Failed to marshal buffered report, skipping", logger.Err(err))
			continue
		}

		// Try to send
		if err := s.sendHTTP(data); err != nil {
			// Send failed - return error to keep file
			return fmt.Errorf("failed to send report: %w", err)
		}

		logger.Debug("Successfully sent buffered report", logger.String("server_id", report.ServerID))
	}

	// All reports sent successfully - delete the file
	if err := s.buffer.DeleteFile(filePath); err != nil {
		logger.Error("Failed to delete buffer file after successful send", logger.String("file", filePath), logger.Err(err))
	} else {
		logger.Info("Successfully drained buffer file", logger.String("file", filePath), logger.Int("reports", len(reports)))
	}

	// Periodically clean up old buffer files
	if err := s.buffer.Cleanup(); err != nil {
		logger.Warn("Failed to cleanup old buffer files", logger.Err(err))
	}

	return nil
}

// sendBatch sends an array of reports to the server
func (s *Sender) sendBatch(reports []*metrics.Report) error {
	// Marshal array of reports to JSON
	data, err := json.Marshal(reports)
	if err != nil {
		return fmt.Errorf("failed to marshal batch: %w", err)
	}

	// Send via HTTP
	if err := s.sendHTTP(data); err != nil {
		return fmt.Errorf("failed to send batch: %w", err)
	}

	return nil
}

// createNAReport creates a report with all metrics set to null (N/A)
func (s *Sender) createNAReport() *metrics.Report {
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}

	return &metrics.Report{
		ServerID:   s.config.Agent.ServerID,
		Timestamp:  time.Now().UTC().Format(time.RFC3339),
		Hostname:   hostname,
		SystemInfo: nil,
		CPU:        nil,
		Memory:     nil,
		Disk:       nil,
		Network:    nil,
		Uptime:     nil,
		Processes:  nil,
	}
}

// sendCorruptedFileMarker sends a report with all metrics set to null (N/A)
// This keeps the timeline intact when a corrupted buffer file is encountered
func (s *Sender) sendCorruptedFileMarker(filePath string) error {
	// Get hostname for the report
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}

	// Create a report with all null metrics
	naReport := &metrics.Report{
		ServerID:   s.config.Agent.ServerID,
		Timestamp:  time.Now().UTC().Format(time.RFC3339),
		Hostname:   hostname,
		SystemInfo: nil, // Will serialize as null
		CPU:        nil,
		Memory:     nil,
		Disk:       nil,
		Network:    nil,
		Uptime:     nil,
		Processes:  nil,
	}

	data, err := naReport.ToJSON()
	if err != nil {
		return fmt.Errorf("failed to marshal N/A report: %w", err)
	}

	// Send the N/A marker
	if err := s.sendHTTP(data); err != nil {
		return fmt.Errorf("failed to send N/A marker: %w", err)
	}

	logger.Info("Sent N/A metrics marker for corrupted file", logger.String("file", filePath))
	return nil
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
