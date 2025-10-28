# NodePulse Agent v2.0 - Project Overview

## Project Information

- **Repository**: `github.com/node-pulse/agent`
- **CLI Command**: `pulse`
- **Language**: Go
- **Framework**: Cobra (CLI)
- **Target**: Linux servers (arm64 + amd64)

## Purpose

Forward Prometheus metrics from `node_exporter` to a central dashboard for monitoring.

## Architecture

### Project Structure

```
agent/
├── cmd/
│   ├── root.go           # Cobra root command
│   ├── start.go          # Start command (Prometheus forwarder)
│   ├── stop.go           # Stop command (stops daemon mode only)
│   ├── setup.go          # Setup wizard (quick mode only)
│   ├── status.go         # Status command (shows server ID, config, service, buffer)
│   ├── service.go        # Service management (systemd)
│   └── update.go         # Self-updater
├── internal/
│   ├── prometheus/       # Prometheus scraper
│   │   ├── scraper.go    # HTTP scraper for node_exporter
│   │   └── scraper_test.go
│   ├── report/           # Forwarding/buffering logic
│   │   ├── sender.go     # HTTP forwarder with WAL pattern
│   │   ├── buffer.go     # Prometheus text format buffer
│   │   └── buffer_status.go
│   ├── config/           # Configuration management
│   │   ├── config.go     # Viper-based config loader
│   │   └── serverid.go   # Server ID persistence
│   ├── logger/           # Structured logging (zap)
│   ├── pidfile/          # PID file management
│   ├── installer/        # Setup installer logic
│   └── updater/          # Self-update system
├── docs/                 # Documentation
├── .goreleaser.yaml      # Cross-compilation config
├── nodepulse.yml         # Example config file
└── main.go
```

## Metrics Source

**node_exporter** (Prometheus exporter for hardware and OS metrics):

- URL: `http://localhost:9100/metrics`
- Format: Prometheus text format
- Metrics: 100+ system metrics including:
  - CPU usage per core, system/user/idle times
  - Memory: total, used, free, available, swap
  - Disk I/O: reads/writes, space usage per mount
  - Network I/O: bytes sent/received, packet counts
  - System: uptime, load average, process counts

**Security**: Port 9100 must be blocked from external access (UFW/iptables)

## Forwarding Configuration

### Intervals

- Default: **15 seconds** (Prometheus standard)
- Allowed: 15s, 30s, 1m
- Configurable via `agent.interval`

### HTTP Forwarding

- Protocol: HTTP POST
- Format: Prometheus text format
- Content-Type: `text/plain; version=0.0.4`
- Endpoint: `{{ dashboard }}/metrics/prometheus?server_id={{ server_id }}`
- Timeout: **5 seconds** (default)
- If timeout or failure → save to buffer (WAL pattern)
- Every request includes `server_id` query parameter

### Server ID (UUID)

Each agent instance requires a unique `server_id` to identify which server is reporting.

**Source:**
- Assigned by dashboard when adding server
- Passed to agent via Ansible deployment or `pulse setup --yes`

**Persistence locations** (tried in order):

- `/var/lib/node-pulse/server_id` ✅ (survives re-setup, not OS reinstall)
- `/etc/node-pulse/server_id`
- `~/.node-pulse/server_id`
- `./server_id` (fallback)

**Stability:**

- UUID persists across agent updates
- UUID is regenerated only after OS reinstall (when persistent file is wiped)
- Check current UUID: `pulse status`

### Buffer Strategy (Write-Ahead Log Pattern)

**Always save to buffer first, then drain in background:**

```
/var/lib/node-pulse/buffer/
├── 20251027-140000-550e8400-e29b-41d4-a716-446655440000.prom
├── 20251027-140015-550e8400-e29b-41d4-a716-446655440000.prom
└── 20251027-140030-550e8400-e29b-41d4-a716-446655440000.prom
```

- Each scrape saved as `.prom` file: `YYYYMMDD-HHMMSS-<server_id>.prom`
- Retention: **48 hours max**
- Background drain goroutine continuously processes buffer:
  - Batch processing: up to 5 files per request (configurable)
  - Oldest-first ordering
  - Random jitter (0 to interval) prevents thundering herd
  - Files deleted AFTER successful send (not before)
- Cleanup: delete files older than 48 hours

**No retry logic within a cycle** - if send fails, buffer it and move on.

## Configuration File

**Location**: `/etc/node-pulse/nodepulse.yml`

```yaml
server:
  endpoint: "https://dashboard.nodepulse.io/metrics/prometheus"
  timeout: 5s

agent:
  server_id: "550e8400-e29b-41d4-a716-446655440000"  # From dashboard
  interval: 15s  # Default (Prometheus standard)

prometheus:
  enabled: true
  endpoint: "http://localhost:9100/metrics"
  timeout: 3s

buffer:
  path: "/var/lib/node-pulse/buffer"
  retention_hours: 48
  batch_size: 5

logging:
  level: "info"
  output: "stdout"
  file:
    path: "/var/log/node-pulse/agent.log"
    max_size_mb: 10
    max_backups: 3
    max_age_days: 7
    compress: true
```

**Configurable Fields (Ansible Deployment):**

Only **TWO** fields are configurable:
1. `server.endpoint`: Dashboard URL
2. `agent.server_id`: UUID from dashboard

All other settings use hardcoded defaults.

## CLI Commands

### Core Commands

```bash
pulse setup --yes            # Quick setup (prompts for endpoint and server_id)
pulse start                  # Run agent in foreground (for testing/development)
pulse start -d               # Run agent in background daemon mode (development only)
pulse stop                   # Stop daemon mode agent (does not affect systemd service)
pulse status                 # Display comprehensive status
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

### Service Management

```bash
pulse service install        # Install systemd service
pulse service start          # Start the service
pulse service stop           # Stop the service
pulse service restart        # Restart the service
pulse service status         # Check detailed systemd service status
pulse service uninstall      # Remove the service
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

## Prometheus Text Format

The agent forwards raw Prometheus text format:

```
# HELP node_cpu_seconds_total Seconds the CPUs spent in each mode.
# TYPE node_cpu_seconds_total counter
node_cpu_seconds_total{cpu="0",mode="idle"} 123456.78
node_cpu_seconds_total{cpu="0",mode="system"} 1234.56

# HELP node_memory_MemTotal_bytes Memory information field MemTotal_bytes.
# TYPE node_memory_MemTotal_bytes gauge
node_memory_MemTotal_bytes 8.589934592e+09
```

**No parsing or interpretation** - agent is a simple forwarder.

## v2.0 Changes

### What's New
- ✅ Prometheus-based architecture
- ✅ Write-Ahead Log (WAL) buffer pattern
- ✅ 100+ metrics from node_exporter
- ✅ Random jitter for load distribution
- ✅ Batch processing for efficiency

### What's Removed
- ❌ TUI dashboard (`pulse watch`)
- ❌ Custom metrics collection
- ❌ JSON format reports
- ❌ Bubble Tea and Lipgloss dependencies

### Migration Required
1. Install node_exporter
2. Block port 9100 externally
3. Update config to include `prometheus` section
4. Change endpoint from `/metrics` to `/metrics/prometheus`
5. Clear old buffer (optional)

## Development Status

- ✅ Prometheus scraping implementation
- ✅ WAL buffer pattern
- ✅ Background drain loop with jitter
- ✅ Batch processing
- ✅ Service management
- ✅ Self-update system
- ✅ Documentation complete
