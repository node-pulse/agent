package metrics

import (
	"fmt"
	"syscall"
)

// DiskMetrics represents disk space information
type DiskMetrics struct {
	UsedGB       uint64  `json:"used_gb"`
	TotalGB      uint64  `json:"total_gb"`
	UsagePercent float64 `json:"usage_percent"`
	MountPoint   string  `json:"mount_point"`
}

// CollectDisk collects disk space metrics for the root filesystem
func CollectDisk() (*DiskMetrics, error) {
	return CollectDiskForPath("/")
}

// CollectDiskForPath collects disk space metrics for a specific path
func CollectDiskForPath(path string) (*DiskMetrics, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return nil, fmt.Errorf("failed to get disk stats for %s: %w", path, err)
	}

	// Calculate total and used space
	// Blocks * BlockSize = Total bytes
	totalBytes := stat.Blocks * uint64(stat.Bsize)
	availBytes := stat.Bavail * uint64(stat.Bsize)
	usedBytes := totalBytes - availBytes

	// Convert bytes to GB
	totalGB := totalBytes / (1024 * 1024 * 1024)
	usedGB := usedBytes / (1024 * 1024 * 1024)

	var usagePercent float64
	if totalGB > 0 {
		usagePercent = 100.0 * float64(usedGB) / float64(totalGB)
	}

	return &DiskMetrics{
		UsedGB:       usedGB,
		TotalGB:      totalGB,
		UsagePercent: usagePercent,
		MountPoint:   path,
	}, nil
}
