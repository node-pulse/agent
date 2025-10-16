package metrics

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
)

// NetworkMetrics represents network I/O information (delta since last collection)
type NetworkMetrics struct {
	UploadBytes   uint64 `json:"upload_bytes"`
	DownloadBytes uint64 `json:"download_bytes"`
}

var (
	lastNetStats networkStats
	netMutex     sync.Mutex
)

type networkStats struct {
	rxBytes uint64
	txBytes uint64
}

// CollectNetwork collects network I/O metrics from /proc/net/dev
func CollectNetwork() (*NetworkMetrics, error) {
	currentStats, err := readNetworkStats()
	if err != nil {
		return nil, fmt.Errorf("failed to read network stats: %w", err)
	}

	netMutex.Lock()
	defer netMutex.Unlock()

	// On first run, store stats and return zeros
	if lastNetStats.rxBytes == 0 && lastNetStats.txBytes == 0 {
		lastNetStats = currentStats
		return &NetworkMetrics{
			UploadBytes:   0,
			DownloadBytes: 0,
		}, nil
	}

	// Calculate deltas since last collection
	downloadBytes := currentStats.rxBytes - lastNetStats.rxBytes
	uploadBytes := currentStats.txBytes - lastNetStats.txBytes

	// Store current stats for next calculation
	lastNetStats = currentStats

	return &NetworkMetrics{
		UploadBytes:   uploadBytes,
		DownloadBytes: downloadBytes,
	}, nil
}

// readNetworkStats reads network statistics from /proc/net/dev
// Sums all interfaces except loopback
func readNetworkStats() (networkStats, error) {
	file, err := os.Open("/proc/net/dev")
	if err != nil {
		return networkStats{}, err
	}
	defer file.Close()

	stats := networkStats{}
	scanner := bufio.NewScanner(file)

	// Skip header lines
	scanner.Scan()
	scanner.Scan()

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Split interface name and stats
		parts := strings.Split(line, ":")
		if len(parts) != 2 {
			continue
		}

		interfaceName := strings.TrimSpace(parts[0])

		// Skip loopback interface
		if interfaceName == "lo" {
			continue
		}

		fields := strings.Fields(parts[1])
		if len(fields) < 9 {
			continue
		}

		// fields[0] = receive bytes, fields[8] = transmit bytes
		rxBytes, err := strconv.ParseUint(fields[0], 10, 64)
		if err != nil {
			continue
		}
		txBytes, err := strconv.ParseUint(fields[8], 10, 64)
		if err != nil {
			continue
		}

		stats.rxBytes += rxBytes
		stats.txBytes += txBytes
	}

	return stats, scanner.Err()
}

// ResetNetworkStats resets the network stats tracker (useful for testing)
func ResetNetworkStats() {
	netMutex.Lock()
	defer netMutex.Unlock()
	lastNetStats = networkStats{}
}
