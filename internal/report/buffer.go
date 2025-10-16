package report

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/node-pulse/agent/internal/config"
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

// Save saves a report to the buffer
func (b *Buffer) Save(report *metrics.Report) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Get buffer file path for current hour
	now := time.Now()
	filePath := b.config.GetBufferFilePath(now)

	// Open file in append mode
	file, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open buffer file: %w", err)
	}
	defer file.Close()

	// Convert to JSONL
	data, err := report.ToJSONL()
	if err != nil {
		return fmt.Errorf("failed to marshal report: %w", err)
	}

	// Write to file
	if _, err := file.Write(data); err != nil {
		return fmt.Errorf("failed to write to buffer: %w", err)
	}

	return nil
}

// GetBufferFiles returns all buffer file paths in chronological order (oldest first)
func (b *Buffer) GetBufferFiles() ([]string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.getBufferFiles()
}

// LoadFile loads all reports from a specific buffer file without deleting it
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

// readBufferFile reads all reports from a buffer file
func (b *Buffer) readBufferFile(filePath string) ([]*metrics.Report, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var reports []*metrics.Report
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var report metrics.Report
		if err := json.Unmarshal(line, &report); err != nil {
			// Skip invalid lines
			continue
		}

		reports = append(reports, &report)
	}

	return reports, scanner.Err()
}

// getBufferFiles returns all buffer files sorted by name (chronological order)
func (b *Buffer) getBufferFiles() ([]string, error) {
	pattern := filepath.Join(b.config.Buffer.Path, "*.jsonl")
	files, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}

	// Sort files by name (which is chronological due to format YYYY-MM-DD-HH)
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
		filename := filepath.Base(filePath)
		// Format: 2006-01-02-15.jsonl
		if len(filename) < 13 {
			continue
		}

		timeStr := filename[:13] // Extract "2006-01-02-15"
		fileTime, err := time.Parse("2006-01-02-15", timeStr)
		if err != nil {
			continue
		}

		// If file is older than cutoff, delete it
		if fileTime.Before(cutoffTime) {
			os.Remove(filePath)
		}
	}

	return nil
}

// Close closes the buffer (currently no-op)
func (b *Buffer) Close() error {
	return nil
}
