# NodePulse Agent - Project Overview

## Project Information

- **Repository**: `github.com/node-pulse/agent`
- **CLI Command**: `nodepulse`
- **Language**: Go
- **Framework**: Cobra (CLI)
- **Target**: Linux servers (arm64 + amd64)
- **Deployment**: Ansible-based (centrally managed from dashboard)

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
│   └── service.go        # Service management (systemd)
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
│   └── installer/        # Setup installer logic
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
- Passed to agent via Ansible deployment or `nodepulse setup --yes`

**Persistence locations** (tried in order):

- `/var/lib/nodepulse/server_id` ✅ (survives re-setup, not OS reinstall)
- `/etc/nodepulse/server_id`
- `~/.nodepulse/server_id`
- `./server_id` (fallback)

**Stability:**

- UUID persists across agent updates (Ansible manages updates)
- UUID is regenerated only after OS reinstall (when persistent file is wiped)
- Check current UUID: `nodepulse status`

### Buffer Strategy (Write-Ahead Log Pattern)

**Always save to buffer first, then drain in background:**

```
/var/lib/nodepulse/buffer/
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

**Location**: `/etc/nodepulse/nodepulse.yml`

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
  path: "/var/lib/nodepulse/buffer"
  retention_hours: 48
  batch_size: 5

logging:
  level: "info"
  output: "stdout"
  file:
    path: "/var/log/nodepulse/agent.log"
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
nodepulse setup --yes            # Quick setup (prompts for endpoint and server_id)
nodepulse start                  # Run agent in foreground (for testing/development)
nodepulse start -d               # Run agent in background daemon mode (development only)
nodepulse stop                   # Stop daemon mode agent (does not affect systemd service)
nodepulse status                 # Display comprehensive status
```

### Running Modes

**1. Foreground Mode** (`nodepulse start`)
- Blocks terminal, runs in foreground
- Creates PID file to prevent duplicate runs
- Stop with: Ctrl+C (graceful shutdown with cleanup)
- Best for: Development and testing

**2. Daemon Mode** (`nodepulse start -d`)
- Runs in background, detached from terminal
- Creates PID file for process management
- Stop with: `nodepulse stop` command
- Sends SIGTERM (5s grace period) then SIGKILL if needed
- Best for: Quick testing, not production

**3. Systemd Service** (`nodepulse service start`)
- Managed by systemd (no PID file)
- Auto-restart on failure, boot on startup
- Stop with: `nodepulse service stop`
- Best for: Production deployments (managed by Ansible)

**Important**: `nodepulse stop` only works for daemon mode. If systemd service is running, it provides helpful guidance to use `nodepulse service stop` instead.

### Service Management

```bash
nodepulse service install        # Install systemd service
nodepulse service start          # Start the service
nodepulse service stop           # Stop the service
nodepulse service restart        # Restart the service
nodepulse service status         # Check detailed systemd service status
nodepulse service uninstall      # Remove the service
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

## Changes from v0.0.x

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
- ✅ Ansible-based deployment and updates
- ✅ Documentation complete

### Update Management

Agent updates are **centrally managed via Ansible** from the dashboard:
- No self-update mechanism in agent
- Version control with rollback capability
- Staged rollouts across server fleet
- Deployment tracking per server
