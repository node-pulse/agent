package report

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/node-pulse/agent/internal/config"
	"github.com/node-pulse/agent/internal/logger"
)

// Buffer handles buffering failed reports to disk
type Buffer struct {
	config *config.Config
	mu     sync.Mutex
}

// NewBuffer creates a new buffer
func NewBuffer(cfg *config.Config) (*Buffer, error) {
	// Ensure buffer directory exists
	if err := cfg.EnsureBufferDir(); err != nil {
		return nil, err
	}

	return &Buffer{
		config: cfg,
	}, nil
}

// SavePrometheus saves Prometheus text format data to buffer
// Filename format: YYYYMMDD-HHMMSS-<server_id>.prom
func (b *Buffer) SavePrometheus(data []byte, serverID string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Generate unique filename with timestamp
	now := time.Now()
	filename := fmt.Sprintf("%s-%s.prom", now.Format("20060102-150405"), serverID)
	filePath := filepath.Join(b.config.Buffer.Path, filename)

	// Write Prometheus text format to file
	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write buffer file: %w", err)
	}

	logger.Debug("Saved Prometheus data to buffer",
		logger.String("file", filename),
		logger.Int("bytes", len(data)))

	return nil
}

// GetBufferFiles returns all buffer file paths in chronological order (oldest first)
func (b *Buffer) GetBufferFiles() ([]string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.getBufferFiles()
}

// PrometheusEntry represents a buffered Prometheus scrape
type PrometheusEntry struct {
	ServerID string
	Data     []byte
}

// LoadPrometheusFile loads Prometheus text format from a buffer file
func (b *Buffer) LoadPrometheusFile(filePath string) (*PrometheusEntry, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Read file
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	// Extract server_id from filename
	// Format: YYYYMMDD-HHMMSS-<server_id>.prom
	filename := filepath.Base(filePath)
	parts := strings.Split(strings.TrimSuffix(filename, ".prom"), "-")
	if len(parts) < 3 {
		return nil, fmt.Errorf("invalid filename format: %s", filename)
	}

	// Server ID is everything after the second dash
	serverID := strings.Join(parts[2:], "-")

	return &PrometheusEntry{
		ServerID: serverID,
		Data:     data,
	}, nil
}

// DeleteFile deletes a specific buffer file
func (b *Buffer) DeleteFile(filePath string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	return os.Remove(filePath)
}

// getBufferFiles returns all buffer files sorted by name (chronological order)
func (b *Buffer) getBufferFiles() ([]string, error) {
	// Get Prometheus buffer files (.prom)
	pattern := filepath.Join(b.config.Buffer.Path, "*.prom")
	files, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}

	// Sort files by name (chronological due to format YYYYMMDD-HHMMSS)
	sort.Strings(files)

	return files, nil
}

// Cleanup removes buffer files older than retention period
func (b *Buffer) Cleanup() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	files, err := b.getBufferFiles()
	if err != nil {
		return err
	}

	cutoffTime := time.Now().Add(-time.Duration(b.config.Buffer.RetentionHours) * time.Hour)

	for _, filePath := range files {
		// Extract timestamp from filename
		// Format: YYYYMMDD-HHMMSS-<server_id>.prom
		filename := filepath.Base(filePath)

		// Remove .prom extension
		if !strings.HasSuffix(filename, ".prom") {
			continue
		}

		// Extract timestamp part (first two segments before first dash)
		parts := strings.SplitN(strings.TrimSuffix(filename, ".prom"), "-", 3)
		if len(parts) < 2 {
			logger.Debug("Invalid buffer file format, skipping", logger.String("file", filename))
			continue
		}

		timeStr := parts[0] + "-" + parts[1]

		// Parse timestamp from filename
		fileTime, err := time.Parse("20060102-150405", timeStr)
		if err != nil {
			logger.Debug("Failed to parse buffer file timestamp, skipping", logger.String("file", filename), logger.Err(err))
			continue
		}

		// If file is older than cutoff, delete it
		if fileTime.Before(cutoffTime) {
			if err := os.Remove(filePath); err != nil {
				logger.Warn("Failed to remove old buffer file", logger.String("file", filePath), logger.Err(err))
			} else {
				logger.Debug("Removed old buffer file", logger.String("file", filePath))
			}
		}
	}

	return nil
}

// Close closes the buffer (currently no-op)
func (b *Buffer) Close() error {
	return nil
}
