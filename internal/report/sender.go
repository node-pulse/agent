package report

import (
	"bytes"
	"fmt"
	"io"
	"net/http"

	"github.com/node-pulse/agent/internal/config"
	"github.com/node-pulse/agent/internal/logger"
	"github.com/node-pulse/agent/internal/metrics"
)

// Sender handles sending metrics reports to the server
type Sender struct {
	config *config.Config
	client *http.Client
	buffer *Buffer
}

// NewSender creates a new report sender
func NewSender(cfg *config.Config) (*Sender, error) {
	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: cfg.Server.Timeout,
	}

	// Create buffer if enabled
	var buffer *Buffer
	if cfg.Buffer.Enabled {
		var err error
		buffer, err = NewBuffer(cfg)
		if err != nil {
			return nil, fmt.Errorf("failed to create buffer: %w", err)
		}
	}

	return &Sender{
		config: cfg,
		client: client,
		buffer: buffer,
	}, nil
}

// Send sends a metrics report to the server
// If sending fails, the report is buffered (if enabled)
func (s *Sender) Send(report *metrics.Report) error {
	data, err := report.ToJSON()
	if err != nil {
		return fmt.Errorf("failed to marshal report: %w", err)
	}

	// Try to send via HTTP
	if err := s.sendHTTP(data); err != nil {
		// If buffer is enabled, save to buffer
		if s.buffer != nil {
			if bufErr := s.buffer.Save(report); bufErr != nil {
				return fmt.Errorf("send failed: %w, buffer failed: %w", err, bufErr)
			}
		}
		return fmt.Errorf("send failed and saved to buffer: %w", err)
	}

	// If send succeeded and buffer is enabled, try to flush old buffered data
	if s.buffer != nil {
		go s.FlushBuffer()
	}

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

// FlushBuffer attempts to send all buffered reports
// Processes files one at a time, only deleting after successful send
func (s *Sender) FlushBuffer() {
	if s.buffer == nil {
		return
	}

	// Get all buffer files (oldest first)
	files, err := s.buffer.GetBufferFiles()
	if err != nil {
		logger.Warn("Failed to get buffer files for flushing", logger.Err(err))
		return
	}

	// Process each file
	for _, filePath := range files {
		// Load reports from this file
		reports, err := s.buffer.LoadFile(filePath)
		if err != nil {
			// Skip this file, try next one
			logger.Warn("Failed to load buffer file, skipping", logger.String("file", filePath), logger.Err(err))
			continue
		}

		// Try to send all reports from this file
		allSentSuccessfully := true
		for _, report := range reports {
			data, err := report.ToJSON()
			if err != nil {
				logger.Debug("Failed to marshal buffered report, skipping", logger.Err(err))
				continue
			}

			// Try to send
			if err := s.sendHTTP(data); err != nil {
				// Send failed - connection is down again
				// Stop processing and keep this file for next time
				allSentSuccessfully = false
				break
			}
		}

		// Only delete the file if ALL reports were sent successfully
		if allSentSuccessfully {
			if err := s.buffer.DeleteFile(filePath); err != nil {
				logger.Error("Failed to delete buffer file after successful send", logger.String("file", filePath), logger.Err(err))
			} else {
				logger.Debug("Successfully flushed and deleted buffer file", logger.String("file", filePath))
			}
		} else {
			// Connection failed - stop trying, we'll retry next time
			break
		}
	}

	// Clean up old buffer files
	if err := s.buffer.Cleanup(); err != nil {
		logger.Warn("Failed to cleanup old buffer files", logger.Err(err))
	}
}

// Close closes the sender
func (s *Sender) Close() error {
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
