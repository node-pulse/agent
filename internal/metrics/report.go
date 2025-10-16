package metrics

import (
	"encoding/json"
	"os"
	"time"
)

// Report represents the complete metrics report sent to the server
type Report struct {
	Timestamp  string           `json:"timestamp"`
	ServerID   string           `json:"server_id"`
	Hostname   string           `json:"hostname"`
	SystemInfo *SystemInfo      `json:"system_info,omitempty"`
	CPU        *CPUMetrics      `json:"cpu"`
	Memory     *MemoryMetrics   `json:"memory"`
	Network    *NetworkMetrics  `json:"network"`
	Uptime     *UptimeMetrics   `json:"uptime"`
	Processes  *ProcessMetrics  `json:"processes"`
}

// Collect gathers all metrics and creates a complete report
func Collect(serverID string) (*Report, error) {
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}

	report := &Report{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		ServerID:  serverID,
		Hostname:  hostname,
	}

	// Collect system info (cached after first call)
	if sysInfo, err := CollectSystemInfo(); err == nil {
		report.SystemInfo = sysInfo
	}

	// Collect each metric independently
	// If one fails, set it to nil but continue with others
	allFailed := true

	if cpu, err := CollectCPU(); err == nil {
		report.CPU = cpu
		allFailed = false
	}

	if memory, err := CollectMemory(); err == nil {
		report.Memory = memory
		allFailed = false
	}

	if network, err := CollectNetwork(); err == nil {
		report.Network = network
		allFailed = false
	}

	if uptime, err := CollectUptime(); err == nil {
		report.Uptime = uptime
		allFailed = false
	}

	if processes, err := CollectProcesses(); err == nil {
		report.Processes = processes
		allFailed = false
	}

	// If all metrics failed, return error
	if allFailed {
		return nil, ErrAllMetricsFailed
	}

	return report, nil
}

// ToJSON converts the report to JSON bytes
func (r *Report) ToJSON() ([]byte, error) {
	return json.Marshal(r)
}

// ToJSONL converts the report to a single-line JSON for JSONL format
func (r *Report) ToJSONL() ([]byte, error) {
	data, err := json.Marshal(r)
	if err != nil {
		return nil, err
	}
	// Append newline for JSONL format
	return append(data, '\n'), nil
}
