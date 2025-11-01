package prometheus

import (
	"testing"
)

func TestParseProcessExporterMetrics(t *testing.T) {
	// Sample process_exporter output with mode labels (user/system)
	input := `# HELP namedprocess_namegroup_num_procs Number of processes in this group
# TYPE namedprocess_namegroup_num_procs gauge
namedprocess_namegroup_num_procs{groupname="nginx"} 4
namedprocess_namegroup_num_procs{groupname="postgres"} 1

# HELP namedprocess_namegroup_cpu_seconds_total CPU time consumed by this process group
# TYPE namedprocess_namegroup_cpu_seconds_total counter
namedprocess_namegroup_cpu_seconds_total{groupname="nginx",mode="system"} 123.45
namedprocess_namegroup_cpu_seconds_total{groupname="nginx",mode="user"} 456.78
namedprocess_namegroup_cpu_seconds_total{groupname="postgres",mode="system"} 10.5
namedprocess_namegroup_cpu_seconds_total{groupname="postgres",mode="user"} 20.3

# HELP namedprocess_namegroup_memory_bytes Memory used by this process group
# TYPE namedprocess_namegroup_memory_bytes gauge
namedprocess_namegroup_memory_bytes{groupname="nginx",memtype="resident"} 104857600
namedprocess_namegroup_memory_bytes{groupname="nginx",memtype="virtual"} 209715200
namedprocess_namegroup_memory_bytes{groupname="postgres",memtype="resident"} 52428800
namedprocess_namegroup_memory_bytes{groupname="postgres",memtype="virtual"} 104857600
`

	snapshots, err := ParseProcessExporterMetrics([]byte(input))
	if err != nil {
		t.Fatalf("ParseProcessExporterMetrics failed: %v", err)
	}

	// Should have 2 process groups
	if len(snapshots) != 2 {
		t.Fatalf("Expected 2 snapshots, got %d", len(snapshots))
	}

	// Find nginx snapshot
	var nginx *ProcessExporterMetricSnapshot
	for i := range snapshots {
		if snapshots[i].Name == "nginx" {
			nginx = &snapshots[i]
			break
		}
	}

	if nginx == nil {
		t.Fatal("nginx snapshot not found")
	}

	// Verify nginx metrics
	if nginx.NumProcs != 4 {
		t.Errorf("Expected nginx.NumProcs=4, got %d", nginx.NumProcs)
	}

	// CPU should be sum of user + system = 123.45 + 456.78 = 580.23
	expectedCPU := 123.45 + 456.78
	if nginx.CPUSecondsTotal != expectedCPU {
		t.Errorf("Expected nginx.CPUSecondsTotal=%.2f, got %.2f", expectedCPU, nginx.CPUSecondsTotal)
	}

	// Memory should be resident (RSS)
	if nginx.MemoryBytes != 104857600 {
		t.Errorf("Expected nginx.MemoryBytes=104857600, got %d", nginx.MemoryBytes)
	}

	// Find postgres snapshot
	var postgres *ProcessExporterMetricSnapshot
	for i := range snapshots {
		if snapshots[i].Name == "postgres" {
			postgres = &snapshots[i]
			break
		}
	}

	if postgres == nil {
		t.Fatal("postgres snapshot not found")
	}

	// Verify postgres metrics
	if postgres.NumProcs != 1 {
		t.Errorf("Expected postgres.NumProcs=1, got %d", postgres.NumProcs)
	}

	// CPU should be sum of user + system = 10.5 + 20.3 = 30.8
	expectedPostgresCPU := 10.5 + 20.3
	if postgres.CPUSecondsTotal != expectedPostgresCPU {
		t.Errorf("Expected postgres.CPUSecondsTotal=%.2f, got %.2f", expectedPostgresCPU, postgres.CPUSecondsTotal)
	}

	if postgres.MemoryBytes != 52428800 {
		t.Errorf("Expected postgres.MemoryBytes=52428800, got %d", postgres.MemoryBytes)
	}
}

func TestParseProcessExporterMetrics_OnlyNumProcs(t *testing.T) {
	// Test with process that only has num_procs > 0
	input := `namedprocess_namegroup_num_procs{groupname="test"} 2
namedprocess_namegroup_cpu_seconds_total{groupname="test",mode="system"} 0
namedprocess_namegroup_cpu_seconds_total{groupname="test",mode="user"} 0
namedprocess_namegroup_memory_bytes{groupname="test",memtype="resident"} 1024
`

	snapshots, err := ParseProcessExporterMetrics([]byte(input))
	if err != nil {
		t.Fatalf("ParseProcessExporterMetrics failed: %v", err)
	}

	if len(snapshots) != 1 {
		t.Fatalf("Expected 1 snapshot, got %d", len(snapshots))
	}

	if snapshots[0].Name != "test" {
		t.Errorf("Expected name='test', got '%s'", snapshots[0].Name)
	}

	if snapshots[0].NumProcs != 2 {
		t.Errorf("Expected NumProcs=2, got %d", snapshots[0].NumProcs)
	}
}

func TestParseProcessExporterMetrics_ZeroProcs(t *testing.T) {
	// Process with 0 procs should be filtered out
	input := `namedprocess_namegroup_num_procs{groupname="dead"} 0
namedprocess_namegroup_cpu_seconds_total{groupname="dead",mode="system"} 100
namedprocess_namegroup_cpu_seconds_total{groupname="dead",mode="user"} 200
`

	snapshots, err := ParseProcessExporterMetrics([]byte(input))
	if err != nil {
		t.Fatalf("ParseProcessExporterMetrics failed: %v", err)
	}

	// Should be filtered out because numProcs == 0
	if len(snapshots) != 0 {
		t.Fatalf("Expected 0 snapshots (filtered), got %d", len(snapshots))
	}
}
