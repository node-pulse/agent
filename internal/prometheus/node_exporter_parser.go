package prometheus

import (
	"bufio"
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// NodeExporterMetricSnapshot represents a parsed snapshot of node_exporter metrics
// This matches the admiral.metrics table schema (raw values, no percentages)
// Specifically designed for Prometheus node_exporter metrics only
type NodeExporterMetricSnapshot struct {
	Timestamp time.Time `json:"timestamp"`

	// CPU Metrics (seconds, raw values from counters)
	CPUIdleSeconds   float64 `json:"cpu_idle_seconds"`
	CPUIowaitSeconds float64 `json:"cpu_iowait_seconds"`
	CPUSystemSeconds float64 `json:"cpu_system_seconds"`
	CPUUserSeconds   float64 `json:"cpu_user_seconds"`
	CPUStealSeconds  float64 `json:"cpu_steal_seconds"`
	CPUCores         int     `json:"cpu_cores"`

	// Memory Metrics (bytes, raw values)
	MemoryTotalBytes     int64 `json:"memory_total_bytes"`
	MemoryAvailableBytes int64 `json:"memory_available_bytes"`
	MemoryFreeBytes      int64 `json:"memory_free_bytes"`
	MemoryCachedBytes    int64 `json:"memory_cached_bytes"`
	MemoryBuffersBytes   int64 `json:"memory_buffers_bytes"`
	MemoryActiveBytes    int64 `json:"memory_active_bytes"`
	MemoryInactiveBytes  int64 `json:"memory_inactive_bytes"`

	// Swap Metrics (bytes, raw values)
	SwapTotalBytes  int64 `json:"swap_total_bytes"`
	SwapFreeBytes   int64 `json:"swap_free_bytes"`
	SwapCachedBytes int64 `json:"swap_cached_bytes"`

	// Disk Metrics (bytes for root filesystem)
	DiskTotalBytes     int64 `json:"disk_total_bytes"`
	DiskFreeBytes      int64 `json:"disk_free_bytes"`
	DiskAvailableBytes int64 `json:"disk_available_bytes"`

	// Disk I/O (counters and totals)
	DiskReadsCompletedTotal  int64   `json:"disk_reads_completed_total"`
	DiskWritesCompletedTotal int64   `json:"disk_writes_completed_total"`
	DiskReadBytesTotal       int64   `json:"disk_read_bytes_total"`
	DiskWrittenBytesTotal    int64   `json:"disk_written_bytes_total"`
	DiskIOTimeSecondsTotal   float64 `json:"disk_io_time_seconds_total"`

	// Network Metrics (counters and totals)
	NetworkReceiveBytesTotal    int64 `json:"network_receive_bytes_total"`
	NetworkTransmitBytesTotal   int64 `json:"network_transmit_bytes_total"`
	NetworkReceivePacketsTotal  int64 `json:"network_receive_packets_total"`
	NetworkTransmitPacketsTotal int64 `json:"network_transmit_packets_total"`
	NetworkReceiveErrsTotal     int64 `json:"network_receive_errs_total"`
	NetworkTransmitErrsTotal    int64 `json:"network_transmit_errs_total"`
	NetworkReceiveDropTotal     int64 `json:"network_receive_drop_total"`
	NetworkTransmitDropTotal    int64 `json:"network_transmit_drop_total"`

	// System Load Average
	Load1Min  float64 `json:"load_1min"`
	Load5Min  float64 `json:"load_5min"`
	Load15Min float64 `json:"load_15min"`

	// Process Counts
	ProcessesRunning int `json:"processes_running"`
	ProcessesBlocked int `json:"processes_blocked"`
	ProcessesTotal   int `json:"processes_total"`

	// System Uptime
	UptimeSeconds int64 `json:"uptime_seconds"`
}

// ParseNodeExporterMetrics parses Prometheus node_exporter text format and extracts essential metrics
// Returns a NodeExporterMetricSnapshot with raw counter values (no percentages calculated)
// This parser is specifically designed for node_exporter metrics only
func ParseNodeExporterMetrics(data []byte) (*NodeExporterMetricSnapshot, error) {
	snapshot := &NodeExporterMetricSnapshot{
		Timestamp: time.Now().UTC(),
	}

	scanner := bufio.NewScanner(bytes.NewReader(data))

	// Track CPU metrics per core for aggregation
	cpuIdlePerCore := make(map[string]float64)
	cpuUserPerCore := make(map[string]float64)
	cpuSystemPerCore := make(map[string]float64)
	cpuIowaitPerCore := make(map[string]float64)
	cpuStealPerCore := make(map[string]float64)

	// Track network metrics per device for primary interface selection
	networkDevices := make(map[string]*networkMetrics)

	// Track disk metrics per device for primary disk selection
	diskDevices := make(map[string]*diskMetrics)

	for scanner.Scan() {
		line := scanner.Text()

		// Skip comments and empty lines
		if len(line) == 0 || line[0] == '#' {
			continue
		}

		// Parse metric line: metric_name{labels} value [timestamp]
		if err := parseLine(line, snapshot, cpuIdlePerCore, cpuUserPerCore, cpuSystemPerCore,
			cpuIowaitPerCore, cpuStealPerCore, networkDevices, diskDevices); err != nil {
			// Log but don't fail on individual parse errors
			continue
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scanner error: %w", err)
	}

	// Aggregate CPU metrics across all cores
	snapshot.CPUIdleSeconds = sumMap(cpuIdlePerCore)
	snapshot.CPUUserSeconds = sumMap(cpuUserPerCore)
	snapshot.CPUSystemSeconds = sumMap(cpuSystemPerCore)
	snapshot.CPUIowaitSeconds = sumMap(cpuIowaitPerCore)
	snapshot.CPUStealSeconds = sumMap(cpuStealPerCore)
	snapshot.CPUCores = len(cpuIdlePerCore)

	// Select primary network interface (usually eth0, or first non-loopback)
	selectPrimaryNetwork(snapshot, networkDevices)

	// Select primary disk (vda, sda, or first available)
	selectPrimaryDisk(snapshot, diskDevices)

	// Calculate uptime from boot time
	if bootTime := snapshot.UptimeSeconds; bootTime > 0 {
		snapshot.UptimeSeconds = time.Now().Unix() - bootTime
	}

	return snapshot, nil
}

type networkMetrics struct {
	rxBytes   int64
	txBytes   int64
	rxPackets int64
	txPackets int64
	rxErrs    int64
	txErrs    int64
	rxDrop    int64
	txDrop    int64
}

type diskMetrics struct {
	readsCompleted  int64
	writesCompleted int64
	readBytes       int64
	writtenBytes    int64
	ioTimeSeconds   float64
}

func parseLine(line string, snapshot *NodeExporterMetricSnapshot,
	cpuIdle, cpuUser, cpuSystem, cpuIowait, cpuSteal map[string]float64,
	networkDevices map[string]*networkMetrics,
	diskDevices map[string]*diskMetrics) error {

	// Split metric name and rest
	parts := strings.Fields(line)
	if len(parts) < 2 {
		return fmt.Errorf("invalid line format")
	}

	metricPart := parts[0]
	valuePart := parts[1]

	// Extract metric name and labels
	var metricName string
	var labels map[string]string

	if idx := strings.Index(metricPart, "{"); idx != -1 {
		metricName = metricPart[:idx]
		labelsStr := metricPart[idx+1 : len(metricPart)-1] // Remove {}
		labels = parseLabels(labelsStr)
	} else {
		metricName = metricPart
		labels = make(map[string]string)
	}

	value, err := parseValue(valuePart)
	if err != nil {
		return err
	}

	// Parse specific metrics
	switch metricName {
	// CPU metrics
	case "node_cpu_seconds_total":
		cpu := labels["cpu"]
		mode := labels["mode"]
		switch mode {
		case "idle":
			cpuIdle[cpu] = value
		case "user":
			cpuUser[cpu] = value
		case "system":
			cpuSystem[cpu] = value
		case "iowait":
			cpuIowait[cpu] = value
		case "steal":
			cpuSteal[cpu] = value
		}

	// Memory metrics
	case "node_memory_MemTotal_bytes":
		snapshot.MemoryTotalBytes = int64(value)
	case "node_memory_MemAvailable_bytes":
		snapshot.MemoryAvailableBytes = int64(value)
	case "node_memory_MemFree_bytes":
		snapshot.MemoryFreeBytes = int64(value)
	case "node_memory_Cached_bytes":
		snapshot.MemoryCachedBytes = int64(value)
	case "node_memory_Buffers_bytes":
		snapshot.MemoryBuffersBytes = int64(value)
	case "node_memory_Active_bytes":
		snapshot.MemoryActiveBytes = int64(value)
	case "node_memory_Inactive_bytes":
		snapshot.MemoryInactiveBytes = int64(value)

	// Swap metrics
	case "node_memory_SwapTotal_bytes":
		snapshot.SwapTotalBytes = int64(value)
	case "node_memory_SwapFree_bytes":
		snapshot.SwapFreeBytes = int64(value)
	case "node_memory_SwapCached_bytes":
		snapshot.SwapCachedBytes = int64(value)

	// Disk filesystem metrics (root mountpoint only)
	case "node_filesystem_size_bytes":
		if labels["mountpoint"] == "/" && !isVirtualFilesystem(labels["fstype"]) {
			snapshot.DiskTotalBytes = int64(value)
		}
	case "node_filesystem_free_bytes":
		if labels["mountpoint"] == "/" && !isVirtualFilesystem(labels["fstype"]) {
			snapshot.DiskFreeBytes = int64(value)
		}
	case "node_filesystem_avail_bytes":
		if labels["mountpoint"] == "/" && !isVirtualFilesystem(labels["fstype"]) {
			snapshot.DiskAvailableBytes = int64(value)
		}

	// Disk I/O metrics
	case "node_disk_reads_completed_total":
		device := labels["device"]
		if isPhysicalDisk(device) {
			if diskDevices[device] == nil {
				diskDevices[device] = &diskMetrics{}
			}
			diskDevices[device].readsCompleted = int64(value)
		}
	case "node_disk_writes_completed_total":
		device := labels["device"]
		if isPhysicalDisk(device) {
			if diskDevices[device] == nil {
				diskDevices[device] = &diskMetrics{}
			}
			diskDevices[device].writesCompleted = int64(value)
		}
	case "node_disk_read_bytes_total":
		device := labels["device"]
		if isPhysicalDisk(device) {
			if diskDevices[device] == nil {
				diskDevices[device] = &diskMetrics{}
			}
			diskDevices[device].readBytes = int64(value)
		}
	case "node_disk_written_bytes_total":
		device := labels["device"]
		if isPhysicalDisk(device) {
			if diskDevices[device] == nil {
				diskDevices[device] = &diskMetrics{}
			}
			diskDevices[device].writtenBytes = int64(value)
		}
	case "node_disk_io_time_seconds_total":
		device := labels["device"]
		if isPhysicalDisk(device) {
			if diskDevices[device] == nil {
				diskDevices[device] = &diskMetrics{}
			}
			diskDevices[device].ioTimeSeconds = value
		}

	// Network metrics
	case "node_network_receive_bytes_total":
		device := labels["device"]
		if isPhysicalNetwork(device) {
			if networkDevices[device] == nil {
				networkDevices[device] = &networkMetrics{}
			}
			networkDevices[device].rxBytes = int64(value)
		}
	case "node_network_transmit_bytes_total":
		device := labels["device"]
		if isPhysicalNetwork(device) {
			if networkDevices[device] == nil {
				networkDevices[device] = &networkMetrics{}
			}
			networkDevices[device].txBytes = int64(value)
		}
	case "node_network_receive_packets_total":
		device := labels["device"]
		if isPhysicalNetwork(device) {
			if networkDevices[device] == nil {
				networkDevices[device] = &networkMetrics{}
			}
			networkDevices[device].rxPackets = int64(value)
		}
	case "node_network_transmit_packets_total":
		device := labels["device"]
		if isPhysicalNetwork(device) {
			if networkDevices[device] == nil {
				networkDevices[device] = &networkMetrics{}
			}
			networkDevices[device].txPackets = int64(value)
		}
	case "node_network_receive_errs_total":
		device := labels["device"]
		if isPhysicalNetwork(device) {
			if networkDevices[device] == nil {
				networkDevices[device] = &networkMetrics{}
			}
			networkDevices[device].rxErrs = int64(value)
		}
	case "node_network_transmit_errs_total":
		device := labels["device"]
		if isPhysicalNetwork(device) {
			if networkDevices[device] == nil {
				networkDevices[device] = &networkMetrics{}
			}
			networkDevices[device].txErrs = int64(value)
		}
	case "node_network_receive_drop_total":
		device := labels["device"]
		if isPhysicalNetwork(device) {
			if networkDevices[device] == nil {
				networkDevices[device] = &networkMetrics{}
			}
			networkDevices[device].rxDrop = int64(value)
		}
	case "node_network_transmit_drop_total":
		device := labels["device"]
		if isPhysicalNetwork(device) {
			if networkDevices[device] == nil {
				networkDevices[device] = &networkMetrics{}
			}
			networkDevices[device].txDrop = int64(value)
		}

	// Load average
	case "node_load1":
		snapshot.Load1Min = value
	case "node_load5":
		snapshot.Load5Min = value
	case "node_load15":
		snapshot.Load15Min = value

	// Processes
	case "node_procs_running":
		snapshot.ProcessesRunning = int(value)
	case "node_procs_blocked":
		snapshot.ProcessesBlocked = int(value)
	case "node_forks_total":
		snapshot.ProcessesTotal = int(value)

	// Uptime (boot time - will be converted to uptime later)
	case "node_boot_time_seconds":
		snapshot.UptimeSeconds = int64(value)
	}

	return nil
}

func parseLabels(labelsStr string) map[string]string {
	labels := make(map[string]string)
	pairs := strings.Split(labelsStr, ",")
	for _, pair := range pairs {
		kv := strings.Split(pair, "=")
		if len(kv) == 2 {
			key := strings.TrimSpace(kv[0])
			value := strings.Trim(strings.TrimSpace(kv[1]), "\"")
			labels[key] = value
		}
	}
	return labels
}

func parseValue(s string) (float64, error) {
	return strconv.ParseFloat(s, 64)
}

func sumMap(m map[string]float64) float64 {
	sum := 0.0
	for _, v := range m {
		sum += v
	}
	return sum
}

func isVirtualFilesystem(fstype string) bool {
	virtualFS := []string{"tmpfs", "devtmpfs", "overlay", "squashfs", "devfs"}
	for _, vfs := range virtualFS {
		if fstype == vfs {
			return true
		}
	}
	return false
}

func isPhysicalDisk(device string) bool {
	// Match vda, sda, nvme0n1, etc.
	return strings.HasPrefix(device, "vd") ||
		strings.HasPrefix(device, "sd") ||
		strings.HasPrefix(device, "nvme") ||
		strings.HasPrefix(device, "hd")
}

func isPhysicalNetwork(device string) bool {
	// Exclude loopback, docker, and virtual interfaces
	if device == "lo" || strings.HasPrefix(device, "docker") ||
		strings.HasPrefix(device, "veth") || strings.HasPrefix(device, "virbr") {
		return false
	}
	return true
}

func selectPrimaryNetwork(snapshot *NodeExporterMetricSnapshot, devices map[string]*networkMetrics) {
	// Priority: eth0 > en0 > first available
	var primary *networkMetrics
	if devices["eth0"] != nil {
		primary = devices["eth0"]
	} else if devices["en0"] != nil {
		primary = devices["en0"]
	} else {
		// Get first available
		for _, metrics := range devices {
			primary = metrics
			break
		}
	}

	if primary != nil {
		snapshot.NetworkReceiveBytesTotal = primary.rxBytes
		snapshot.NetworkTransmitBytesTotal = primary.txBytes
		snapshot.NetworkReceivePacketsTotal = primary.rxPackets
		snapshot.NetworkTransmitPacketsTotal = primary.txPackets
		snapshot.NetworkReceiveErrsTotal = primary.rxErrs
		snapshot.NetworkTransmitErrsTotal = primary.txErrs
		snapshot.NetworkReceiveDropTotal = primary.rxDrop
		snapshot.NetworkTransmitDropTotal = primary.txDrop
	}
}

func selectPrimaryDisk(snapshot *NodeExporterMetricSnapshot, devices map[string]*diskMetrics) {
	// Priority: vda > sda > nvme0n1 > first available
	var primary *diskMetrics
	if devices["vda"] != nil {
		primary = devices["vda"]
	} else if devices["sda"] != nil {
		primary = devices["sda"]
	} else if devices["nvme0n1"] != nil {
		primary = devices["nvme0n1"]
	} else {
		// Get first available
		for _, metrics := range devices {
			primary = metrics
			break
		}
	}

	if primary != nil {
		snapshot.DiskReadsCompletedTotal = primary.readsCompleted
		snapshot.DiskWritesCompletedTotal = primary.writesCompleted
		snapshot.DiskReadBytesTotal = primary.readBytes
		snapshot.DiskWrittenBytesTotal = primary.writtenBytes
		snapshot.DiskIOTimeSecondsTotal = primary.ioTimeSeconds
	}
}
