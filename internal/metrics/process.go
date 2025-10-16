package metrics

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// ProcessMetrics represents top processes by CPU and memory
type ProcessMetrics struct {
	TopCPU    []ProcessInfo `json:"top_cpu"`
	TopMemory []ProcessInfo `json:"top_memory"`
}

// ProcessInfo represents information about a single process
type ProcessInfo struct {
	PID        int     `json:"pid"`
	Name       string  `json:"name"`
	CPUTime    float64 `json:"cpu_time"`    // Total CPU time in seconds
	MemoryMB   float64 `json:"memory_mb"`   // Memory usage in MB
	MemoryPerc float64 `json:"memory_perc"` // Memory usage as percentage of total
}

type processData struct {
	pid     int
	name    string
	cpuTime uint64 // Total CPU time in jiffies (utime + stime)
	memRSS  uint64 // Memory in KB
}

// CollectProcesses collects top processes by CPU and memory usage
func CollectProcesses() (*ProcessMetrics, error) {
	processes, err := readProcessData()
	if err != nil {
		return nil, fmt.Errorf("failed to read process data: %w", err)
	}

	if len(processes) == 0 {
		return nil, fmt.Errorf("no processes found")
	}

	// Get total system memory for percentage calculation
	totalMemKB := getTotalMemoryKB()

	// Get top 10 by CPU
	topCPU := getTopProcessesByCPU(processes, 10, totalMemKB)

	// Get top 10 by memory
	topMem := getTopProcessesByMemory(processes, 10, totalMemKB)

	return &ProcessMetrics{
		TopCPU:    topCPU,
		TopMemory: topMem,
	}, nil
}

// readProcessData reads all process information from /proc
func readProcessData() ([]processData, error) {
	processes := []processData{}

	// Read all /proc/[pid] directories
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		// Skip if not a directory or not a numeric name (PID)
		if !entry.IsDir() {
			continue
		}
		pidStr := entry.Name()
		pid, err := strconv.Atoi(pidStr)
		if err != nil {
			continue
		}

		// Read process name from /proc/[pid]/comm
		commPath := filepath.Join("/proc", pidStr, "comm")
		commData, err := os.ReadFile(commPath)
		if err != nil {
			continue
		}
		name := strings.TrimSpace(string(commData))

		// Read CPU time from /proc/[pid]/stat
		statPath := filepath.Join("/proc", pidStr, "stat")
		statData, err := os.ReadFile(statPath)
		if err != nil {
			continue
		}

		// Parse stat file: fields are space-separated
		// utime is field 14 (index 13), stime is field 15 (index 14)
		statFields := strings.Fields(string(statData))
		if len(statFields) < 15 {
			continue
		}

		utime, _ := strconv.ParseUint(statFields[13], 10, 64)
		stime, _ := strconv.ParseUint(statFields[14], 10, 64)
		cpuTime := utime + stime

		// Read memory from /proc/[pid]/status
		statusPath := filepath.Join("/proc", pidStr, "status")
		statusData, err := os.ReadFile(statusPath)
		if err != nil {
			continue
		}

		// Find VmRSS line (resident memory in KB)
		var memRSS uint64
		for _, line := range strings.Split(string(statusData), "\n") {
			if strings.HasPrefix(line, "VmRSS:") {
				fields := strings.Fields(line)
				if len(fields) >= 2 {
					memRSS, _ = strconv.ParseUint(fields[1], 10, 64)
				}
				break
			}
		}

		processes = append(processes, processData{
			pid:     pid,
			name:    name,
			cpuTime: cpuTime,
			memRSS:  memRSS,
		})
	}

	return processes, nil
}

// getTopProcessesByCPU returns top N processes sorted by CPU time
func getTopProcessesByCPU(processes []processData, n int, totalMemKB uint64) []ProcessInfo {
	// Sort by CPU time (descending)
	sort.Slice(processes, func(i, j int) bool {
		return processes[i].cpuTime > processes[j].cpuTime
	})

	// Get top N
	result := []ProcessInfo{}
	for i := 0; i < len(processes) && i < n; i++ {
		p := processes[i]
		memPerc := 0.0
		if totalMemKB > 0 {
			memPerc = float64(p.memRSS) / float64(totalMemKB) * 100.0
		}
		result = append(result, ProcessInfo{
			PID:        p.pid,
			Name:       p.name,
			CPUTime:    float64(p.cpuTime) / 100.0, // Convert jiffies to seconds (100 jiffies = 1 second on most systems)
			MemoryMB:   float64(p.memRSS) / 1024.0,
			MemoryPerc: memPerc,
		})
	}

	return result
}

// getTopProcessesByMemory returns top N processes sorted by memory usage
func getTopProcessesByMemory(processes []processData, n int, totalMemKB uint64) []ProcessInfo {
	// Sort by memory (descending)
	sort.Slice(processes, func(i, j int) bool {
		return processes[i].memRSS > processes[j].memRSS
	})

	// Get top N
	result := []ProcessInfo{}
	for i := 0; i < len(processes) && i < n; i++ {
		p := processes[i]
		memPerc := 0.0
		if totalMemKB > 0 {
			memPerc = float64(p.memRSS) / float64(totalMemKB) * 100.0
		}
		result = append(result, ProcessInfo{
			PID:        p.pid,
			Name:       p.name,
			CPUTime:    float64(p.cpuTime) / 100.0, // Convert jiffies to seconds
			MemoryMB:   float64(p.memRSS) / 1024.0,
			MemoryPerc: memPerc,
		})
	}

	return result
}

// getTotalMemoryKB returns total system memory in KB from /proc/meminfo
func getTotalMemoryKB() uint64 {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "MemTotal:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				total, _ := strconv.ParseUint(fields[1], 10, 64)
				return total
			}
		}
	}

	return 0
}
