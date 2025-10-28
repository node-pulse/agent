# Buffer Mechanism

The Node Pulse Agent uses a **Write-Ahead Log (WAL)** pattern for reliable metrics delivery. All scraped metrics are buffered to disk before being sent to the dashboard.

## Architecture

```
node_exporter → Agent scrapes → Save to buffer → Background drain loop → Send to dashboard
                                      ↓
                              Always persisted first
```

## How It Works

### 1. Write-Ahead Log Pattern

**Every scrape is saved to buffer first:**

```go
// From internal/report/sender.go
func (s *Sender) SendPrometheus(data []byte, serverID string) error {
	// Always save to buffer first (WAL pattern)
	if err := s.buffer.SavePrometheus(data, serverID); err != nil {
		return fmt.Errorf("failed to save prometheus data to buffer: %w", err)
	}

	logger.Debug("Prometheus data saved to buffer")
	return nil
}
```

**Key benefit:** Data is persisted to disk before attempting network send. If the agent crashes, metrics are not lost.

### 2. Background Drain Loop

A separate goroutine continuously drains the buffer:

```go
// From internal/report/sender.go
func (s *Sender) StartDraining() {
	go s.drainLoop()
	logger.Info("Started buffer drain goroutine with random jitter")
}

func (s *Sender) drainLoop() {
	for {
		// Get buffered files (oldest first)
		files, err := s.buffer.GetBufferFiles()

		// Batch processing: up to batch_size (default: 5)
		batchSize := len(files)
		if batchSize > s.config.Buffer.BatchSize {
			batchSize = s.config.Buffer.BatchSize
		}

		// Process batch
		s.processBatch(files[:batchSize])

		// Random delay before next attempt
		s.randomDelay()
	}
}
```

**Process:**
1. Check buffer directory for `.prom` files
2. Select oldest files (up to batch size)
3. Attempt to send each file
4. Delete file on successful send
5. Keep file on failed send (retry later)
6. Wait random delay before next iteration

### 3. Random Jitter

Prevents thundering herd problem with multiple agents:

```go
// From internal/report/sender.go
func (s *Sender) randomDelay() {
	// Generate random delay: 0 to full interval
	maxDelay := s.config.Agent.Interval  // Default: 15s
	delay := time.Duration(s.rng.Int63n(int64(maxDelay)))

	logger.Debug("Waiting random delay before next drain attempt",
		logger.Duration("delay", delay))

	time.Sleep(delay)
}
```

**Why random jitter?**
- With 1000 agents, if all retry at the same time → traffic spike
- Random delay (0-15s) spreads retries across the interval window
- Smooths load on dashboard server

### 4. Batch Processing

Process up to 5 files per batch (configurable):

```go
// From internal/report/sender.go
func (s *Sender) processBatch(filePaths []string) error {
	successCount := 0

	for _, filePath := range filePaths {
		// Load Prometheus data from file
		entry, err := s.buffer.LoadPrometheusFile(filePath)
		if err != nil {
			// Corrupted file - delete it
			logger.Warn("Corrupted buffer file detected, deleting")
			s.buffer.DeleteFile(filePath)
			continue
		}

		// Send Prometheus data
		if err := s.sendPrometheusHTTP(entry.Data, entry.ServerID); err != nil {
			// Send failed - keep file for retry
			logger.Debug("Failed to send, will retry later")
			break  // Stop on first failure
		}

		// Send succeeded - delete file
		s.buffer.DeleteFile(filePath)
		successCount++
	}

	return nil
}
```

**Behavior:**
- Process files oldest-first (chronological order)
- Stop batch on first failure (preserve ordering)
- Delete file only after successful HTTP 2xx response

## Buffer File Format

**File naming:**
```
YYYYMMDD-HHMMSS-<server_id>.prom
```

**Examples:**
```
/var/lib/node-pulse/buffer/20251027-140000-550e8400-e29b-41d4-a716-446655440000.prom
/var/lib/node-pulse/buffer/20251027-140015-550e8400-e29b-41d4-a716-446655440000.prom
/var/lib/node-pulse/buffer/20251027-140030-550e8400-e29b-41d4-a716-446655440000.prom
```

**Content:**
Each file contains Prometheus text format from a single scrape:

```
# HELP node_cpu_seconds_total Seconds the CPUs spent in each mode.
# TYPE node_cpu_seconds_total counter
node_cpu_seconds_total{cpu="0",mode="idle"} 123456.78
node_cpu_seconds_total{cpu="0",mode="system"} 1234.56

# HELP node_memory_MemTotal_bytes Memory information field MemTotal_bytes.
# TYPE node_memory_MemTotal_bytes gauge
node_memory_MemTotal_bytes 8.589934592e+09
```

## Configuration

```yaml
buffer:
  path: "/var/lib/node-pulse/buffer"
  retention_hours: 48
  batch_size: 5
```

**Settings:**
- `path`: Buffer directory location (default: `/var/lib/node-pulse/buffer`)
- `retention_hours`: Auto-delete files older than this (default: 48 hours)
- `batch_size`: Maximum files to process per batch (default: 5)

## Example Scenarios

### Scenario 1: Normal Operation

1. Agent scrapes node_exporter every 15s
2. Saves to buffer: `20251027-140000-uuid.prom`
3. Drain loop immediately picks it up (no delay if buffer empty)
4. Sends to dashboard successfully
5. Deletes file
6. Buffer stays empty

**Result:** Near real-time delivery, buffer acts as safety net

### Scenario 2: Dashboard Temporarily Down

1. Agent scrapes every 15s, saves to buffer
2. Drain loop tries to send, fails (connection refused)
3. Keeps file, waits random delay (0-15s)
4. Tries again, fails again
5. Buffer accumulates files: 240 files after 1 hour (3600s / 15s = 240)
6. Dashboard comes back online
7. Drain loop sends oldest 5 files (batch)
8. Deletes those 5 files
9. Random delay (0-15s)
10. Sends next 5 files
11. Continues until buffer empty

**Result:** All metrics eventually delivered, in order, without data loss

### Scenario 3: Network Instability

1. Dashboard intermittently reachable
2. Drain loop succeeds sometimes, fails sometimes
3. Buffer grows slowly during failures, shrinks during successes
4. Oldest-first processing maintains chronological order

**Result:** Metrics delivered in order as network allows

### Scenario 4: Agent Crash

1. Agent scrapes and saves to buffer: `20251027-140000-uuid.prom`
2. Agent crashes before drain loop sends
3. File remains on disk
4. Agent restarts
5. Drain loop finds existing file
6. Sends it successfully

**Result:** No data loss, persisted metrics survive crashes

## Cleanup

**Automatic cleanup runs periodically:**

```go
// From internal/report/buffer.go
func (b *Buffer) Cleanup() error {
	files, err := b.getBufferFiles()
	if err != nil {
		return err
	}

	cutoffTime := time.Now().Add(-time.Duration(b.config.Buffer.RetentionHours) * time.Hour)

	for _, filePath := range files {
		// Parse timestamp from filename
		// Format: YYYYMMDD-HHMMSS-<server_id>.prom
		filename := filepath.Base(filePath)
		parts := strings.SplitN(strings.TrimSuffix(filename, ".prom"), "-", 3)
		timeStr := parts[0] + "-" + parts[1]

		fileTime, err := time.Parse("20060102-150405", timeStr)
		if err != nil {
			continue
		}

		// If file is older than cutoff, delete it
		if fileTime.Before(cutoffTime) {
			os.Remove(filePath)
			logger.Debug("Removed old buffer file", logger.String("file", filePath))
		}
	}

	return nil
}
```

**When it runs:**
- After successful batch send
- Prevents indefinite buffer growth if dashboard is permanently unreachable

**Retention:** 48 hours (configurable)

## Monitoring Buffer Status

Check buffer status with `pulse status`:

```bash
$ pulse status

Node Pulse Agent Status
=====================

Server ID:     550e8400-e29b-41d4-a716-446655440000
Config File:   /etc/node-pulse/nodepulse.yml
Endpoint:      https://dashboard.nodepulse.io/metrics/prometheus
Interval:      15s

Agent:         running (via systemd)

Buffer:        12 report(s) pending in /var/lib/node-pulse/buffer
               Oldest: 2025-10-27 14:00:00
               Total size: 156 KB

Log File:      /var/log/node-pulse/agent.log
```

**Buffer metrics:**
- File count: Number of `.prom` files in buffer
- Report count: Same as file count (1 file = 1 scrape)
- Oldest file: Timestamp of oldest buffered scrape
- Total size: Disk space used by buffer

## Key Benefits

1. **No data loss:** Metrics persisted before sending
2. **Crash recovery:** Files survive agent restarts
3. **Ordered delivery:** Oldest-first processing
4. **Load distribution:** Random jitter prevents spikes
5. **Efficient batching:** Process multiple files per iteration
6. **Automatic cleanup:** Old files deleted after retention period
7. **Resilient:** Continues working during network issues

## Technical Details

**Thread safety:**
- Buffer operations use `sync.Mutex` for concurrent access
- Single drain goroutine (no race conditions)

**Error handling:**
- Corrupted files deleted (logged as warnings)
- Network errors keep files for retry
- HTTP 4xx/5xx errors keep files for retry

**Performance:**
- Minimal disk I/O (small files, sequential writes)
- Batch processing reduces HTTP overhead
- Random jitter smooths load over time

## Configuration Recommendations

**Default settings (recommended for most deployments):**
```yaml
buffer:
  path: "/var/lib/node-pulse/buffer"
  retention_hours: 48
  batch_size: 5
```

**High-traffic servers (>1000 agents):**
```yaml
buffer:
  batch_size: 10  # Drain faster when backlog builds up
```

**Unstable networks:**
```yaml
buffer:
  retention_hours: 168  # 7 days (keep metrics longer)
```

**Storage-constrained systems:**
```yaml
buffer:
  retention_hours: 24  # 1 day (cleanup more aggressively)
```
