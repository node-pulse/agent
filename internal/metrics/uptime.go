package metrics

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// UptimeMetrics represents system uptime information
type UptimeMetrics struct {
	Days float64 `json:"days"`
}

// CollectUptime collects system uptime from /proc/uptime
func CollectUptime() (*UptimeMetrics, error) {
	uptimeSeconds, err := readUptime()
	if err != nil {
		return nil, fmt.Errorf("failed to read uptime: %w", err)
	}

	// Convert seconds to days
	days := uptimeSeconds / 86400.0

	return &UptimeMetrics{Days: days}, nil
}

// readUptime reads system uptime from /proc/uptime
func readUptime() (float64, error) {
	file, err := os.Open("/proc/uptime")
	if err != nil {
		return 0, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	if !scanner.Scan() {
		return 0, fmt.Errorf("empty /proc/uptime file")
	}

	line := scanner.Text()
	fields := strings.Fields(line)
	if len(fields) < 1 {
		return 0, fmt.Errorf("invalid /proc/uptime format")
	}

	uptimeSeconds, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse uptime: %w", err)
	}

	return uptimeSeconds, nil
}
