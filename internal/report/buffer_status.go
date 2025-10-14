package report

import (
	"os"
	"path/filepath"
	"time"
)

// BufferStatus represents the current state of the buffer
type BufferStatus struct {
	FileCount    int
	ReportCount  int
	OldestFile   time.Time
	TotalSizeKB  int64
	HasBuffered  bool
}

// GetBufferStatus returns the current buffer status
func (b *Buffer) GetBufferStatus() BufferStatus {
	if b == nil {
		return BufferStatus{}
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	files, err := b.getBufferFiles()
	if err != nil || len(files) == 0 {
		return BufferStatus{}
	}

	status := BufferStatus{
		FileCount:   len(files),
		HasBuffered: true,
	}

	var totalSize int64
	var oldestTime time.Time

	for _, filePath := range files {
		// Count lines in file
		reports, _ := b.readBufferFile(filePath)
		status.ReportCount += len(reports)

		// Get file size
		if info, err := os.Stat(filePath); err == nil {
			totalSize += info.Size()
		}

		// Get file timestamp
		filename := filepath.Base(filePath)
		if len(filename) >= 13 {
			timeStr := filename[:13]
			if fileTime, err := time.Parse("2006-01-02-15", timeStr); err == nil {
				if oldestTime.IsZero() || fileTime.Before(oldestTime) {
					oldestTime = fileTime
				}
			}
		}
	}

	status.OldestFile = oldestTime
	status.TotalSizeKB = totalSize / 1024

	return status
}
