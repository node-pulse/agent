package prometheus

import (
	"encoding/json"
	"os"
	"testing"
)

func TestParsePrometheusMetrics(t *testing.T) {
	// Read the metrics.txt file from the current directory
	metricFile := "metrics.txt"
	data, err := os.ReadFile(metricFile)
	if err != nil {
		t.Fatalf("Failed to read metrics.txt: %v", err)
	}

	snapshot, err := ParsePrometheusMetrics(data)
	if err != nil {
		t.Fatalf("ParsePrometheusMetrics failed: %v", err)
	}

	// Verify timestamp is set
	if snapshot.Timestamp.IsZero() {
		t.Error("Timestamp should be set")
	}

	// Verify CPU metrics
	t.Run("CPU Metrics", func(t *testing.T) {
		if snapshot.CPUCores == 0 {
			t.Error("CPUCores should be > 0")
		}
		t.Logf("CPU Cores: %d", snapshot.CPUCores)

		if snapshot.CPUIdleSeconds == 0 {
			t.Error("CPUIdleSeconds should be > 0")
		}
		t.Logf("CPU Idle: %.2f seconds", snapshot.CPUIdleSeconds)

		if snapshot.CPUUserSeconds == 0 {
			t.Error("CPUUserSeconds should be > 0")
		}
		t.Logf("CPU User: %.2f seconds", snapshot.CPUUserSeconds)

		if snapshot.CPUSystemSeconds == 0 {
			t.Error("CPUSystemSeconds should be > 0")
		}
		t.Logf("CPU System: %.2f seconds", snapshot.CPUSystemSeconds)

		// Calculate CPU usage percentage to verify values make sense
		total := snapshot.CPUIdleSeconds + snapshot.CPUUserSeconds +
			snapshot.CPUSystemSeconds + snapshot.CPUIowaitSeconds + snapshot.CPUStealSeconds
		if total == 0 {
			t.Error("Total CPU time should be > 0")
		}
		cpuUsage := 100 - (snapshot.CPUIdleSeconds/total)*100
		t.Logf("Calculated CPU Usage: %.2f%%", cpuUsage)
		if cpuUsage < 0 || cpuUsage > 100 {
			t.Errorf("CPU usage should be between 0-100%%, got %.2f%%", cpuUsage)
		}
	})

	// Verify Memory metrics
	t.Run("Memory Metrics", func(t *testing.T) {
		if snapshot.MemoryTotalBytes == 0 {
			t.Error("MemoryTotalBytes should be > 0")
		}
		t.Logf("Memory Total: %d bytes (%.2f GB)", snapshot.MemoryTotalBytes,
			float64(snapshot.MemoryTotalBytes)/1024/1024/1024)

		if snapshot.MemoryAvailableBytes == 0 {
			t.Error("MemoryAvailableBytes should be > 0")
		}
		t.Logf("Memory Available: %d bytes (%.2f GB)", snapshot.MemoryAvailableBytes,
			float64(snapshot.MemoryAvailableBytes)/1024/1024/1024)

		if snapshot.MemoryAvailableBytes > snapshot.MemoryTotalBytes {
			t.Error("Available memory should not exceed total memory")
		}

		// Calculate memory usage
		used := snapshot.MemoryTotalBytes - snapshot.MemoryAvailableBytes
		usagePercent := (float64(used) / float64(snapshot.MemoryTotalBytes)) * 100
		t.Logf("Memory Usage: %.2f%%", usagePercent)
	})

	// Verify Swap metrics
	t.Run("Swap Metrics", func(t *testing.T) {
		if snapshot.SwapTotalBytes == 0 {
			t.Log("Warning: SwapTotalBytes is 0 (might be intentional)")
		} else {
			t.Logf("Swap Total: %d bytes (%.2f GB)", snapshot.SwapTotalBytes,
				float64(snapshot.SwapTotalBytes)/1024/1024/1024)
			t.Logf("Swap Free: %d bytes (%.2f GB)", snapshot.SwapFreeBytes,
				float64(snapshot.SwapFreeBytes)/1024/1024/1024)
		}
	})

	// Verify Disk metrics
	t.Run("Disk Metrics", func(t *testing.T) {
		if snapshot.DiskTotalBytes == 0 {
			t.Error("DiskTotalBytes should be > 0")
		}
		t.Logf("Disk Total: %d bytes (%.2f GB)", snapshot.DiskTotalBytes,
			float64(snapshot.DiskTotalBytes)/1024/1024/1024)

		if snapshot.DiskAvailableBytes == 0 {
			t.Error("DiskAvailableBytes should be > 0")
		}
		t.Logf("Disk Available: %d bytes (%.2f GB)", snapshot.DiskAvailableBytes,
			float64(snapshot.DiskAvailableBytes)/1024/1024/1024)

		// Calculate disk usage
		used := snapshot.DiskTotalBytes - snapshot.DiskAvailableBytes
		usagePercent := (float64(used) / float64(snapshot.DiskTotalBytes)) * 100
		t.Logf("Disk Usage: %.2f%%", usagePercent)
	})

	// Verify Disk I/O metrics
	t.Run("Disk I/O Metrics", func(t *testing.T) {
		if snapshot.DiskReadBytesTotal == 0 {
			t.Error("DiskReadBytesTotal should be > 0")
		}
		t.Logf("Disk Read Total: %d bytes (%.2f MB)", snapshot.DiskReadBytesTotal,
			float64(snapshot.DiskReadBytesTotal)/1024/1024)

		if snapshot.DiskWrittenBytesTotal == 0 {
			t.Error("DiskWrittenBytesTotal should be > 0")
		}
		t.Logf("Disk Written Total: %d bytes (%.2f MB)", snapshot.DiskWrittenBytesTotal,
			float64(snapshot.DiskWrittenBytesTotal)/1024/1024)

		if snapshot.DiskReadsCompletedTotal == 0 {
			t.Error("DiskReadsCompletedTotal should be > 0")
		}
		t.Logf("Disk Reads Completed: %d", snapshot.DiskReadsCompletedTotal)

		if snapshot.DiskWritesCompletedTotal == 0 {
			t.Error("DiskWritesCompletedTotal should be > 0")
		}
		t.Logf("Disk Writes Completed: %d", snapshot.DiskWritesCompletedTotal)
	})

	// Verify Network metrics
	t.Run("Network Metrics", func(t *testing.T) {
		if snapshot.NetworkReceiveBytesTotal == 0 {
			t.Error("NetworkReceiveBytesTotal should be > 0")
		}
		t.Logf("Network RX Total: %d bytes (%.2f GB)", snapshot.NetworkReceiveBytesTotal,
			float64(snapshot.NetworkReceiveBytesTotal)/1024/1024/1024)

		if snapshot.NetworkTransmitBytesTotal == 0 {
			t.Error("NetworkTransmitBytesTotal should be > 0")
		}
		t.Logf("Network TX Total: %d bytes (%.2f GB)", snapshot.NetworkTransmitBytesTotal,
			float64(snapshot.NetworkTransmitBytesTotal)/1024/1024/1024)

		t.Logf("Network RX Packets: %d", snapshot.NetworkReceivePacketsTotal)
		t.Logf("Network TX Packets: %d", snapshot.NetworkTransmitPacketsTotal)
	})

	// Verify System metrics
	t.Run("System Metrics", func(t *testing.T) {
		t.Logf("Load 1min: %.2f", snapshot.Load1Min)
		t.Logf("Load 5min: %.2f", snapshot.Load5Min)
		t.Logf("Load 15min: %.2f", snapshot.Load15Min)

		if snapshot.ProcessesRunning == 0 {
			t.Error("ProcessesRunning should be > 0")
		}
		t.Logf("Processes Running: %d", snapshot.ProcessesRunning)

		t.Logf("Processes Blocked: %d", snapshot.ProcessesBlocked)

		if snapshot.UptimeSeconds == 0 {
			t.Error("UptimeSeconds should be > 0")
		}
		t.Logf("Uptime: %d seconds (%.2f days)", snapshot.UptimeSeconds,
			float64(snapshot.UptimeSeconds)/86400)
	})

	// Test JSON serialization
	t.Run("JSON Serialization", func(t *testing.T) {
		jsonData, err := json.MarshalIndent(snapshot, "", "  ")
		if err != nil {
			t.Fatalf("Failed to marshal to JSON: %v", err)
		}

		t.Log("Generated JSON:")
		t.Log(string(jsonData))

		// Verify JSON can be unmarshaled back
		var decoded MetricSnapshot
		if err := json.Unmarshal(jsonData, &decoded); err != nil {
			t.Fatalf("Failed to unmarshal JSON: %v", err)
		}

		// Verify key fields match
		if decoded.CPUCores != snapshot.CPUCores {
			t.Error("CPUCores mismatch after JSON round-trip")
		}
		if decoded.MemoryTotalBytes != snapshot.MemoryTotalBytes {
			t.Error("MemoryTotalBytes mismatch after JSON round-trip")
		}
	})

	// Verify data size
	t.Run("Data Size", func(t *testing.T) {
		jsonData, _ := json.Marshal(snapshot)
		t.Logf("JSON payload size: %d bytes", len(jsonData))

		// Should be much smaller than original Prometheus format
		originalSize := len(data)
		compressionRatio := (1 - float64(len(jsonData))/float64(originalSize)) * 100
		t.Logf("Original Prometheus size: %d bytes", originalSize)
		t.Logf("Compression ratio: %.2f%%", compressionRatio)

		if compressionRatio < 95 {
			t.Errorf("Expected at least 95%% reduction, got %.2f%%", compressionRatio)
		}
	})
}

func TestParsePrometheusMetrics_EmptyInput(t *testing.T) {
	snapshot, err := ParsePrometheusMetrics([]byte{})
	if err != nil {
		t.Fatalf("Should handle empty input: %v", err)
	}

	if snapshot == nil {
		t.Fatal("Snapshot should not be nil")
	}

	// Should return snapshot with defaults
	if snapshot.CPUCores != 0 {
		t.Error("CPUCores should be 0 for empty input")
	}
}

func TestParsePrometheusMetrics_InvalidInput(t *testing.T) {
	invalidData := []byte("invalid prometheus format\ngarbage data")
	snapshot, err := ParsePrometheusMetrics(invalidData)

	if err != nil {
		t.Fatalf("Should handle invalid input gracefully: %v", err)
	}

	// Should return snapshot with defaults (parser is lenient)
	if snapshot == nil {
		t.Fatal("Snapshot should not be nil even for invalid input")
	}
}
