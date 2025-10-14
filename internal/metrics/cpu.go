package metrics

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
)

// CPUMetrics represents CPU usage information
type CPUMetrics struct {
	UsagePercent float64 `json:"usage_percent"`
}

var (
	lastCPUStats cpuStats
	cpuMutex     sync.Mutex
)

type cpuStats struct {
	user   uint64
	nice   uint64
	system uint64
	idle   uint64
	iowait uint64
	irq    uint64
	softirq uint64
	steal  uint64
	total  uint64
}

// CollectCPU collects CPU usage metrics from /proc/stat
func CollectCPU() (*CPUMetrics, error) {
	currentStats, err := readCPUStats()
	if err != nil {
		return nil, fmt.Errorf("failed to read CPU stats: %w", err)
	}

	cpuMutex.Lock()
	defer cpuMutex.Unlock()

	// On first run, we need two data points to calculate percentage
	// Return 0% for now and store the stats for next time
	if lastCPUStats.total == 0 {
		lastCPUStats = currentStats
		return &CPUMetrics{UsagePercent: 0.0}, nil
	}

	// Calculate deltas
	totalDelta := currentStats.total - lastCPUStats.total
	idleDelta := currentStats.idle - lastCPUStats.idle

	var usagePercent float64
	if totalDelta > 0 {
		usagePercent = 100.0 * float64(totalDelta-idleDelta) / float64(totalDelta)
	}

	// Store current stats for next calculation
	lastCPUStats = currentStats

	return &CPUMetrics{UsagePercent: usagePercent}, nil
}

// readCPUStats reads CPU statistics from /proc/stat
func readCPUStats() (cpuStats, error) {
	file, err := os.Open("/proc/stat")
	if err != nil {
		return cpuStats{}, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "cpu ") {
			fields := strings.Fields(line)
			if len(fields) < 8 {
				return cpuStats{}, fmt.Errorf("invalid cpu line format")
			}

			stats := cpuStats{}
			stats.user, _ = strconv.ParseUint(fields[1], 10, 64)
			stats.nice, _ = strconv.ParseUint(fields[2], 10, 64)
			stats.system, _ = strconv.ParseUint(fields[3], 10, 64)
			stats.idle, _ = strconv.ParseUint(fields[4], 10, 64)
			stats.iowait, _ = strconv.ParseUint(fields[5], 10, 64)
			stats.irq, _ = strconv.ParseUint(fields[6], 10, 64)
			stats.softirq, _ = strconv.ParseUint(fields[7], 10, 64)
			if len(fields) > 8 {
				stats.steal, _ = strconv.ParseUint(fields[8], 10, 64)
			}

			stats.total = stats.user + stats.nice + stats.system + stats.idle +
				stats.iowait + stats.irq + stats.softirq + stats.steal

			return stats, nil
		}
	}

	return cpuStats{}, fmt.Errorf("cpu stats not found in /proc/stat")
}

// ResetCPUStats resets the CPU stats tracker (useful for testing)
func ResetCPUStats() {
	cpuMutex.Lock()
	defer cpuMutex.Unlock()
	lastCPUStats = cpuStats{}
}
