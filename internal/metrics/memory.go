package metrics

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// MemoryMetrics represents memory usage information
type MemoryMetrics struct {
	UsedMB       uint64  `json:"used_mb"`
	TotalMB      uint64  `json:"total_mb"`
	UsagePercent float64 `json:"usage_percent"`
}

// CollectMemory collects memory usage metrics from /proc/meminfo
func CollectMemory() (*MemoryMetrics, error) {
	memInfo, err := readMemInfo()
	if err != nil {
		return nil, fmt.Errorf("failed to read memory info: %w", err)
	}

	// Calculate used memory
	// Used = Total - Available (more accurate than Total - Free)
	usedKB := memInfo["MemTotal"] - memInfo["MemAvailable"]
	totalKB := memInfo["MemTotal"]

	// Convert KB to MB
	usedMB := usedKB / 1024
	totalMB := totalKB / 1024

	var usagePercent float64
	if totalMB > 0 {
		usagePercent = 100.0 * float64(usedMB) / float64(totalMB)
	}

	return &MemoryMetrics{
		UsedMB:       usedMB,
		TotalMB:      totalMB,
		UsagePercent: usagePercent,
	}, nil
}

// readMemInfo reads memory information from /proc/meminfo
func readMemInfo() (map[string]uint64, error) {
	file, err := os.Open("/proc/meminfo")
	if err != nil {
		return nil, err
	}
	defer file.Close()

	memInfo := make(map[string]uint64)
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}

		key := strings.TrimSuffix(fields[0], ":")
		value, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			continue
		}

		memInfo[key] = value
	}

	// Validate required fields
	if _, ok := memInfo["MemTotal"]; !ok {
		return nil, fmt.Errorf("MemTotal not found in /proc/meminfo")
	}
	if _, ok := memInfo["MemAvailable"]; !ok {
		return nil, fmt.Errorf("MemAvailable not found in /proc/meminfo")
	}

	return memInfo, scanner.Err()
}
