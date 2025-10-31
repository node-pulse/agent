package prometheus

import (
	"bufio"
	"bytes"
	"fmt"
	"strings"
	"time"
)

// ProcessExporterMetricSnapshot represents parsed process metrics from process_exporter
// This captures per-process group metrics (grouped by command name)
type ProcessExporterMetricSnapshot struct {
	Timestamp time.Time       `json:"timestamp"`
	Processes []ProcessMetric `json:"processes"`
}

// ProcessMetric represents metrics for a single process group (e.g., all "nginx" processes)
type ProcessMetric struct {
	Name            string  `json:"name"`              // Process name (groupname from process_exporter)
	NumProcs        int     `json:"num_procs"`         // Number of processes in this group
	CPUSecondsTotal float64 `json:"cpu_seconds_total"` // Total CPU time consumed (counter)
	MemoryBytes     int64   `json:"memory_bytes"`      // Resident memory (RSS) in bytes
}

// ParseProcessExporterMetrics parses Prometheus process_exporter text format
// Extracts per-process group metrics (CPU, memory, count)
//
// Expected metrics from process_exporter:
// - namedprocess_namegroup_num_procs{groupname="nginx"} 4
// - namedprocess_namegroup_cpu_seconds_total{groupname="nginx"} 1234.56
// - namedprocess_namegroup_memory_bytes{groupname="nginx",memtype="resident"} 104857600
func ParseProcessExporterMetrics(data []byte) (*ProcessExporterMetricSnapshot, error) {
	snapshot := &ProcessExporterMetricSnapshot{
		Timestamp: time.Now().UTC(),
		Processes: []ProcessMetric{},
	}

	scanner := bufio.NewScanner(bytes.NewReader(data))

	// Track metrics per process group (groupname)
	processMetrics := make(map[string]*ProcessMetric)

	for scanner.Scan() {
		line := scanner.Text()

		// Skip comments and empty lines
		if len(line) == 0 || line[0] == '#' {
			continue
		}

		// Parse metric line
		if err := parseProcessLine(line, processMetrics); err != nil {
			// Log but don't fail on individual parse errors
			continue
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scanner error: %w", err)
	}

	// Convert map to slice
	for _, pm := range processMetrics {
		// Only include processes that have at least 1 running instance
		if pm.NumProcs > 0 {
			snapshot.Processes = append(snapshot.Processes, *pm)
		}
	}

	return snapshot, nil
}

func parseProcessLine(line string, processMetrics map[string]*ProcessMetric) error {
	// Split metric name and value
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

	// Extract groupname (process name)
	groupname, ok := labels["groupname"]
	if !ok {
		return fmt.Errorf("missing groupname label")
	}

	// Ensure process metric entry exists
	if processMetrics[groupname] == nil {
		processMetrics[groupname] = &ProcessMetric{
			Name: groupname,
		}
	}

	pm := processMetrics[groupname]

	// Parse specific metrics
	switch metricName {
	case "namedprocess_namegroup_num_procs":
		pm.NumProcs = int(value)

	case "namedprocess_namegroup_cpu_seconds_total":
		pm.CPUSecondsTotal = value

	case "namedprocess_namegroup_memory_bytes":
		// Only use resident memory (RSS)
		memtype, ok := labels["memtype"]
		if ok && memtype == "resident" {
			pm.MemoryBytes = int64(value)
		}
	}

	return nil
}

// Note: parseLabels() and parseValue() are already defined in node_exporter_parser.go
// They are package-level functions shared across all parsers in the prometheus package
