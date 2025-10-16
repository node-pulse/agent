# NodePulse Agent - Project Overview

## Project Information

- **Repository**: `github.com/node-pulse/agent`
- **CLI Command**: `pulse`
- **Language**: Go
- **Frameworks**: Cobra (CLI) + Bubbletea (TUI)
- **Target**: Linux servers (arm64 + amd64)

## Purpose

Monitor Linux server health metrics and report them to a central server.

## Architecture

### Project Structure

```
agent/
├── cmd/
│   ├── root.go           # Cobra root command
│   ├── start.go          # Start command (foreground and daemon modes)
│   ├── stop.go           # Stop command (stops daemon mode only)
│   ├── setup.go          # Interactive setup wizard
│   ├── watch.go          # TUI watch command (Bubbletea interface)
│   ├── status.go         # Status command (shows server ID, config, service, buffer, logs)
│   └── service.go        # Service management (install/start/stop/restart/status/uninstall)
├── internal/
│   ├── metrics/          # Metrics collection + data models
│   │   ├── cpu.go        # CPU usage collection
│   │   ├── memory.go     # RAM usage collection
│   │   ├── network.go    # Network I/O (upload/download)
│   │   ├── uptime.go     # System uptime in days
│   │   └── report.go     # Main Report struct combining all metrics
│   ├── report/           # Reporting/sending logic
│   │   ├── sender.go     # HTTP sender with timeout
│   │   └── buffer.go     # JSONL buffer management (hourly files)
│   └── config/           # Configuration management
├── .goreleaser.yaml      # Cross-compilation config
├── nodepulse.yml         # Default config file
└── main.go
```

## Metrics Collected

1. **CPU**: Usage percentage
2. **Memory**: RAM usage (used/total)
3. **Network**: Upload/download bytes (delta per interval)
4. **Uptime**: How many days the server has been running

**Collection source**: Linux `/proc` filesystem

- `/proc/stat` - CPU stats
- `/proc/meminfo` - Memory stats
- `/proc/net/dev` - Network I/O stats
- `/proc/uptime` - System uptime

## Reporting Configuration

### Intervals

- Default: **5 seconds**
- Allowed: 5s, 10s, 30s, 1min
- (1s is too aggressive, not recommended)

### HTTP Reporting

- Protocol: HTTP POST
- Format: JSON
- Timeout: **3 seconds** (default)
- If timeout or failure → save to buffer
- Every report includes a `server_id` (UUID) to identify the server

### Server ID (UUID)

Each agent instance requires a unique `server_id` to identify which server is reporting.

**Auto-generation behavior:**

1. If config has valid UUID (not placeholder) → Use it
2. If persisted file exists → Load it
3. Otherwise → Generate new UUID and save to persistent file

**Persistence locations** (tried in order):

- `/var/lib/node-pulse/server_id` ✅ (survives re-setup, not OS reinstall)
- `/etc/node-pulse/server_id`
- `~/.node-pulse/server_id`
- `./server_id` (fallback)

**Stability:**

- UUID persists across project re-setup
- UUID is regenerated only after OS reinstall (when persistent file is wiped)
- Check current UUID: `pulse status`

### Buffer Strategy (Hourly JSONL Files)

When HTTP send fails or times out:

```
/var/lib/node-pulse/buffer/
├── 2025-10-13-14.jsonl  # Hour 14:00-14:59
├── 2025-10-13-15.jsonl  # Hour 15:00-15:59
└── 2025-10-13-16.jsonl  # Current hour
```

- Each failed report appends to current hour's JSONL file
- Retention: **48 hours max**
- On successful send: attempt to send buffered files (oldest first)
  - Files are only deleted AFTER all their reports are successfully sent
  - If flush fails mid-process, remaining files are kept for next retry
- Cleanup: delete files older than 48 hours

**No retry logic within a cycle** - if send fails, buffer it and move on.

## Error Handling

### Collection Failures

If a **single metric** fails to collect (e.g., can't read `/proc/stat`):

- Log the error
- Set that metric to `null` in JSON
- Still send the report with other valid metrics

If **ALL metrics** fail (e.g., `/proc` not accessible):

- Log critical error
- Skip this cycle entirely
- Keep trying next cycle

## Configuration File

**Location**: `/etc/node-pulse/nodepulse.yml` or `./nodepulse.yml`

```yaml
server:
  endpoint: "https://your-server.com/api/metrics"
  timeout: 3s # Default 3 seconds

agent:
  interval: 5s # 5s, 10s, 30s, 1m

buffer:
  enabled: true
  path: "/var/lib/node-pulse/buffer"
  retention_hours: 48 # Keep 48 hours max
```

## CLI Commands

### Core Commands

```bash
pulse setup                  # Interactive setup wizard (creates config, generates server ID)
pulse start                  # Run agent in foreground (for testing/development)
pulse start -d               # Run agent in background daemon mode (development only)
pulse stop                   # Stop daemon mode agent (does not affect systemd service)
pulse watch                  # Launch TUI to see live metrics (Bubbletea)
pulse status                 # Display comprehensive status (server ID, config, service, buffer, logs)
```

### Running Modes

**1. Foreground Mode** (`pulse start`)
- Blocks terminal, runs in foreground
- Creates PID file to prevent duplicate runs
- Stop with: Ctrl+C (graceful shutdown with cleanup)
- Best for: Development and testing

**2. Daemon Mode** (`pulse start -d`)
- Runs in background, detached from terminal
- Creates PID file for process management
- Stop with: `pulse stop` command
- Sends SIGTERM (5s grace period) then SIGKILL if needed
- Best for: Quick testing, not production

**3. Systemd Service** (`pulse service start`)
- Managed by systemd (no PID file)
- Auto-restart on failure, boot on startup
- Stop with: `pulse service stop`
- Best for: Production deployments

**Important**: `pulse stop` only works for daemon mode. If systemd service is running, it provides helpful guidance to use `pulse service stop` instead.

### Service Management (No systemd knowledge required)

```bash
pulse service install        # Install systemd service
pulse service start          # Start the service
pulse service stop           # Stop the service
pulse service restart        # Restart the service
pulse service status         # Check detailed systemd service status
pulse service uninstall      # Remove the service
```

### Systemd Service Details

Service file: `/etc/systemd/system/node-pulse.service`

```ini
[Unit]
Description=NodePulse Server Monitor Agent
After=network.target

[Service]
Type=simple
ExecStart=/usr/local/bin/pulse start
Restart=always
RestartSec=10s

[Install]
WantedBy=multi-user.target
```

## Build & Release

### Build Targets

- Binaries output to `build/` directory (manual builds)
- Binaries output to `dist/` directory (goreleaser)

### Using Makefile

```bash
# Build for current platform
make build

# Build for all platforms
make build-all

# Build release with goreleaser
make release
```

### Using GoReleaser directly

```bash
goreleaser release --snapshot --clean
```

## JSON Report Format

```json
{
  "timestamp": "2025-10-13T14:30:00Z",
  "hostname": "server-01",
  "cpu": {
    "usage_percent": 45.2
  },
  "memory": {
    "used_mb": 2048,
    "total_mb": 8192,
    "usage_percent": 25.0
  },
  "network": {
    "upload_bytes": 1024000,
    "download_bytes": 2048000
  },
  "uptime": {
    "days": 15.5
  }
}
```

If a metric fails to collect, it will be `null`:

```json
{
  "timestamp": "2025-10-13T14:30:00Z",
  "hostname": "server-01",
  "cpu": null,  // Failed to collect
  "memory": { ... },
  "network": { ... },
  "uptime": { ... }
}
```

## Development Phases

1. ✅ Project planning and architecture
2. ✅ Core metrics collection implementation
3. ✅ HTTP sender + buffer logic
4. ✅ Cobra CLI commands
5. ✅ Systemd service management
6. ✅ Bubbletea TUI for live view
7. ✅ GoReleaser configuration
8. ✅ Testing & documentation
