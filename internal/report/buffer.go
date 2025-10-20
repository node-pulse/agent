package report

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/node-pulse/agent/internal/config"
	"github.com/node-pulse/agent/internal/logger"
	"github.com/node-pulse/agent/internal/metrics"
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

// Save saves a report to the buffer as an individual JSON file
// Filename format: YYYYMMDD-HHMMSS.json (sortable by timestamp)
func (b *Buffer) Save(report *metrics.Report) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Generate unique filename with timestamp (sortable, compact)
	now := time.Now()
	filename := fmt.Sprintf("%s.json", now.Format("20060102-150405"))
	filePath := filepath.Join(b.config.Buffer.Path, filename)

	// Convert to JSON
	data, err := report.ToJSON()
	if err != nil {
		return fmt.Errorf("failed to marshal report: %w", err)
	}

	// Write to individual file
	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write buffer file: %w", err)
	}

	return nil
}

// GetBufferFiles returns all buffer file paths in chronological order (oldest first)
func (b *Buffer) GetBufferFiles() ([]string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.getBufferFiles()
}

// LoadFile loads a single report from a buffer file
// Returns a slice with one report for consistency with the API
func (b *Buffer) LoadFile(filePath string) ([]*metrics.Report, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.readBufferFile(filePath)
}

// DeleteFile deletes a specific buffer file
func (b *Buffer) DeleteFile(filePath string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	return os.Remove(filePath)
}

// readBufferFile reads a single report from a JSON buffer file
func (b *Buffer) readBufferFile(filePath string) ([]*metrics.Report, error) {
	// Read entire file
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	// Parse JSON
	var report metrics.Report
	if err := json.Unmarshal(data, &report); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}

	// Return as slice for consistency with API
	return []*metrics.Report{&report}, nil
}

// getBufferFiles returns all buffer files sorted by name (chronological order)
func (b *Buffer) getBufferFiles() ([]string, error) {
	pattern := filepath.Join(b.config.Buffer.Path, "*.json")
	files, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}

	// Sort files by name (chronological due to format YYYY-MM-DD-HH-MM-SS-nanos)
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
		// Format: YYYYMMDD-HHMMSS.json (e.g., 20251020-143045.json)
		filename := filepath.Base(filePath)

		// Remove .json extension
		if !strings.HasSuffix(filename, ".json") {
			continue
		}
		timeStr := strings.TrimSuffix(filename, ".json")

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
