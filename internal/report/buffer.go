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
// Directory structure: buffer/<exporter>/YYYYMMDD-HHMMSS-<server_id>.prom
func (b *Buffer) SavePrometheus(data []byte, serverID string, exporterName string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Sanitize exporter name (remove special chars)
	safeExporterName := sanitizeExporterName(exporterName)

	// Create exporter subdirectory if it doesn't exist
	exporterDir := filepath.Join(b.config.Buffer.Path, safeExporterName)
	if err := os.MkdirAll(exporterDir, 0755); err != nil {
		return fmt.Errorf("failed to create exporter directory: %w", err)
	}

	// Generate filename without exporter name (it's in the directory)
	now := time.Now()
	filename := fmt.Sprintf("%s-%s.prom",
		now.Format("20060102-150405"),
		serverID)
	filePath := filepath.Join(exporterDir, filename)

	// Write Prometheus text format to file
	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write buffer file: %w", err)
	}

	logger.Debug("Saved Prometheus data to buffer",
		logger.String("exporter", exporterName),
		logger.String("file", filepath.Join(safeExporterName, filename)),
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
	ServerID     string
	ExporterName string // Extracted from directory name
	Data         []byte
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

	// Extract metadata from path and filename
	// Path format: buffer/<exporter>/YYYYMMDD-HHMMSS-<server_id>.prom
	dir := filepath.Dir(filePath)
	exporterName := filepath.Base(dir)

	filename := filepath.Base(filePath)
	parts := strings.SplitN(strings.TrimSuffix(filename, ".prom"), "-", 3)

	if len(parts) < 3 {
		return nil, fmt.Errorf("invalid filename format: %s (expected: YYYYMMDD-HHMMSS-serverid.prom)", filename)
	}

	serverID := parts[2]

	return &PrometheusEntry{
		ServerID:     serverID,
		ExporterName: exporterName,
		Data:         data,
	}, nil
}

// DeleteFile deletes a specific buffer file
func (b *Buffer) DeleteFile(filePath string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	return os.Remove(filePath)
}

// getBufferFiles returns all buffer files sorted by name (chronological order)
// Scans all exporter subdirectories
func (b *Buffer) getBufferFiles() ([]string, error) {
	var allFiles []string

	// Read all subdirectories (each is an exporter)
	exporterDirs, err := os.ReadDir(b.config.Buffer.Path)
	if err != nil {
		// If buffer directory doesn't exist yet, return empty list
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}

	// Scan each exporter subdirectory for .prom files
	for _, entry := range exporterDirs {
		if !entry.IsDir() {
			continue // Skip non-directory files
		}

		exporterDir := filepath.Join(b.config.Buffer.Path, entry.Name())
		pattern := filepath.Join(exporterDir, "*.prom")
		files, err := filepath.Glob(pattern)
		if err != nil {
			logger.Warn("Failed to list files in exporter directory",
				logger.String("dir", exporterDir),
				logger.Err(err))
			continue
		}

		allFiles = append(allFiles, files...)
	}

	// Sort files by full path (chronological due to format YYYYMMDD-HHMMSS)
	sort.Strings(allFiles)

	return allFiles, nil
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

		// Extract timestamp part (first two segments)
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

// sanitizeExporterName removes special characters from exporter names
func sanitizeExporterName(name string) string {
	replacer := strings.NewReplacer(
		"/", "_",
		"\\", "_",
		":", "_",
		" ", "_",
		".", "_",
	)
	return replacer.Replace(name)
}

// Close closes the buffer (currently no-op)
func (b *Buffer) Close() error {
	return nil
}
