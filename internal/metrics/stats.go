package metrics

import (
	"sync"
	"time"
)

// HourlyStats tracks aggregated statistics for the current hour
type HourlyStats struct {
	mu              sync.RWMutex
	currentHour     int
	collectionCount int
	successCount    int
	failedCount     int
	cpuSum          float64
	memorySum       float64
	uploadSum       uint64
	downloadSum     uint64
	startTime       time.Time
}

var globalStats = &HourlyStats{
	startTime: time.Now(),
}

// GetGlobalStats returns the global hourly stats tracker
func GetGlobalStats() *HourlyStats {
	return globalStats
}

// RecordCollection records a successful metrics collection
func (s *HourlyStats) RecordCollection(report *Report) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if hour changed, reset if needed
	currentHour := time.Now().Hour()
	if s.currentHour != currentHour {
		s.reset(currentHour)
	}

	s.collectionCount++

	if report.CPU != nil {
		s.cpuSum += report.CPU.UsagePercent
	}
	if report.Memory != nil {
		s.memorySum += report.Memory.UsagePercent
	}
	if report.Network != nil {
		s.uploadSum += report.Network.UploadBytes
		s.downloadSum += report.Network.DownloadBytes
	}
}

// RecordSuccess records a successful send
func (s *HourlyStats) RecordSuccess() {
	s.mu.Lock()
	defer s.mu.Unlock()

	currentHour := time.Now().Hour()
	if s.currentHour != currentHour {
		s.reset(currentHour)
	}

	s.successCount++
}

// RecordFailure records a failed send
func (s *HourlyStats) RecordFailure() {
	s.mu.Lock()
	defer s.mu.Unlock()

	currentHour := time.Now().Hour()
	if s.currentHour != currentHour {
		s.reset(currentHour)
	}

	s.failedCount++
}

// GetStats returns a snapshot of current stats
func (s *HourlyStats) GetStats() HourlyStatsSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var avgCPU, avgMemory float64
	if s.collectionCount > 0 {
		avgCPU = s.cpuSum / float64(s.collectionCount)
		avgMemory = s.memorySum / float64(s.collectionCount)
	}

	return HourlyStatsSnapshot{
		CurrentHour:     s.currentHour,
		CollectionCount: s.collectionCount,
		SuccessCount:    s.successCount,
		FailedCount:     s.failedCount,
		AvgCPU:          avgCPU,
		AvgMemory:       avgMemory,
		TotalUpload:     s.uploadSum,
		TotalDownload:   s.downloadSum,
		StartTime:       s.startTime,
	}
}

// HourlyStatsSnapshot is a read-only snapshot of hourly stats
type HourlyStatsSnapshot struct {
	CurrentHour     int
	CollectionCount int
	SuccessCount    int
	FailedCount     int
	AvgCPU          float64
	AvgMemory       float64
	TotalUpload     uint64
	TotalDownload   uint64
	StartTime       time.Time
}

func (s *HourlyStats) reset(hour int) {
	s.currentHour = hour
	s.collectionCount = 0
	s.successCount = 0
	s.failedCount = 0
	s.cpuSum = 0
	s.memorySum = 0
	s.uploadSum = 0
	s.downloadSum = 0
}
