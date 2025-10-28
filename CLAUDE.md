# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

NodePulse Agent is a lightweight Prometheus forwarder written in Go that scrapes metrics from `node_exporter` and forwards them to a central dashboard via HTTP. The agent is designed to be minimal (<15 MB binary, <40 MB RAM) and efficient, with smart buffering for offline resilience using a Write-Ahead Log (WAL) pattern.

## Build & Development Commands

### Building
```bash
# Build for current platform
make build

# Build for specific platforms
make build-linux-amd64
make build-linux-arm64

# Build for all platforms
make build-all

# Build release with GoReleaser (snapshot mode for testing)
make release
```

### Testing & Development
```bash
# Run tests
make test

# Format code
make fmt

# Lint code
make lint

# Run agent in development mode (builds and runs immediately)
make dev

# Run directly with go
go run . start
```

### Dependencies
```bash
# Download and tidy dependencies
make deps
```

## Core Architecture

### Command Structure (cmd/)
The agent uses [Cobra](https://github.com/spf13/cobra) for CLI command handling:

- **cmd/root.go**: Base command that all subcommands attach to
- **cmd/start.go**: Start command with three modes:
  - Foreground mode (`nodepulse start`): Runs in terminal, blocks execution
  - Daemon mode (`nodepulse start -d`): Spawns background process using `exec.Command` with `Setsid: true`
  - Both modes create PID file EXCEPT when running under systemd (detected via `INVOCATION_ID` env var)
  - **Current**: Scrapes Prometheus instead of collecting custom metrics
- **cmd/stop.go**: Stops daemon mode only (reads PID file, sends SIGTERM → SIGKILL if needed)
  - Will NOT stop systemd-managed processes (they don't create PID files)
  - Provides helpful message if systemd service is running
- **cmd/service.go**: systemd service management (install/start/stop/restart/status/uninstall)
- **cmd/setup.go**: Setup wizard for first-time configuration (command: `nodepulse setup`)
  - **Current**: Interactive TUI mode removed, only quick mode (`--yes`) available
  - Prompts for: endpoint URL and server_id
- **cmd/status.go**: Shows comprehensive agent status including server ID, config, service status, buffer state, and logs

### Prometheus Scraping (internal/prometheus/)
The agent scrapes Prometheus exporters instead of collecting custom metrics:

- **scraper.go**: HTTP scraper for Prometheus text format endpoints
  - Scrapes `http://localhost:9100/metrics` (node_exporter)
  - Returns raw Prometheus text format (100+ metrics)
  - Includes startup verification (`Verify()` checks if exporter is accessible)
- **scraper_test.go**: Unit tests for scraper

**Important**: The agent no longer collects custom metrics. All metrics come from `node_exporter`.

### Reporting System (internal/report/)
Write-Ahead Log (WAL) pattern with HTTP forwarding + file-based buffering:

- **sender.go**: Handles HTTP POST to server endpoint
  - **Always saves to buffer first** (WAL pattern)
  - Separate background goroutine (`StartDraining()`) continuously drains buffer
  - Random jitter (0 to interval) prevents thundering herd
  - Sends Prometheus text format with `server_id` query parameter
  - Content-Type: `text/plain; version=0.0.4`
- **buffer.go**: Prometheus text format buffering system
  - Reports saved as `.prom` files: `YYYYMMDD-HHMMSS-<server_id>.prom`
  - Background drain loop processes files oldest-first
  - Batch processing: up to `batch_size` files per request (default: 5)
  - Files deleted AFTER successful send (not before)
  - Files older than retention period (default 48h) are auto-cleaned
  - Thread-safe with mutex locks
- **buffer_status.go**: Buffer status tracking (file count, oldest file, total size)

### Configuration (internal/config/)
Uses [Viper](https://github.com/spf13/viper) for config loading from YAML:

- **config.go**: Main config loading, validation, and defaults
  - **Current**: Added `PrometheusConfig` section
  - Default interval changed to 15s (Prometheus standard)
  - Allowed intervals: 15s, 30s, 1m (removed 5s, 10s)
  - Default endpoint: `/metrics/prometheus`
- **serverid.go**: Server ID generation and persistence
  - Auto-generates UUID if not set in config
  - Persists to `/var/lib/nodepulse/server_id`
  - Validates format: alphanumeric + dashes, must start/end with alphanumeric

Config search paths (in order):
1. Explicit `--config` flag
2. `/etc/nodepulse/nodepulse.yml`
3. `$HOME/.nodepulse/nodepulse.yml`
4. `./nodepulse.yml`

### Logger (internal/logger/)
Structured logging with [Zap](https://github.com/uber-go/zap):

- Supports stdout and file output
- File rotation with [Lumberjack](https://github.com/natefinch/lumberjack)
- Configurable log level, size limits, and retention

### Update Management
**Agent updates are managed centrally via Ansible**, not by the agent itself:

- Updates deployed through dashboard's Ansible deployment system
- Version controlled with rollback capability
- Staged rollouts across server fleet
- No self-update mechanism in the agent (removed in v0.1.x)

### PID File Management (internal/pidfile/)
Handles process tracking and prevents duplicate agent runs:

- **Location**: `/var/run/nodepulse.pid` (root) or `~/.nodepulse/nodepulse.pid` (user)
- **Stale PID detection**: `CheckRunning()` automatically detects and cleans stale PID files
- **Systemd detection**: Checks for `INVOCATION_ID` environment variable (no PID file created under systemd)

## Agent Main Loop (cmd/start.go)

The agent runs as a **long-running daemon**:

1. Load configuration
2. Initialize logger
3. Create Prometheus scraper
4. **Verify node_exporter is accessible** (startup check)
5. Create HTTP sender with buffer (always enabled)
6. Setup graceful shutdown on SIGINT/SIGTERM
7. **Start background drain goroutine** (continuously attempts to send buffered reports)
8. Scrape and buffer metrics immediately on start
9. Enter infinite ticker loop at configured interval (15s, 30s, or 1m)
10. On each tick:
   - Call `scraper.Scrape()` to get Prometheus text format
   - **Synchronously save to buffer** (Write-Ahead Log pattern)
   - Return immediately (no HTTP blocking)
11. **Background goroutine** (runs concurrently):
   - Continuously checks for buffered reports
   - Batches up to `batch_size` reports (default: 5)
   - Attempts HTTP POST to server with `server_id` query parameter
   - On success: deletes sent files, triggers cleanup
   - On failure: keeps files, waits random delay (0 to interval), retries
   - Random jitter prevents thundering herd

**Key Design Point**: The agent scrapes Prometheus exporters and forwards the text format. It does NOT parse or interpret the metrics - it's a simple forwarder.

## Important Platform Constraints

- **Linux-only**: Requires `node_exporter` running on `localhost:9100`
- **Architectures**: Built for amd64 and arm64 only
- **Dependencies**: `node_exporter` must be installed and running
- **Security**: Port 9100 must be blocked from external access (UFW/iptables)
- **Permissions**:
  - Regular user can run `nodepulse start`, `nodepulse start -d`, `nodepulse stop`
  - Root required for `nodepulse service` and `nodepulse setup` commands
- **Deployment**: Managed via Ansible from central dashboard (not manual installation)

## Testing Notes

The agent requires `node_exporter` to be running on `localhost:9100`.

To test the full flow locally:
1. Install and start node_exporter:
   ```bash
   # Download node_exporter
   wget https://github.com/prometheus/node_exporter/releases/download/v1.8.2/node_exporter-1.8.2.linux-amd64.tar.gz
   tar -xzf node_exporter-1.8.2.linux-amd64.tar.gz
   sudo mv node_exporter-1.8.2.linux-amd64/node_exporter /usr/local/bin/
   /usr/local/bin/node_exporter --web.listen-address=127.0.0.1:9100 &
   ```

2. Verify node_exporter is accessible:
   ```bash
   curl http://localhost:9100/metrics
   ```

3. Run agent:
   ```bash
   make dev
   # or
   go run . start
   ```

4. Verify buffering by pointing endpoint to invalid URL and checking buffer directory:
   ```bash
   ls -la /var/lib/nodepulse/buffer/
   ```

## Release Process

This project uses [GoReleaser](https://goreleaser.com/) for multi-architecture builds:

- Configured in `.goreleaser.yaml`
- Creates tar.gz archives with binary + LICENSE + README + nodepulse.yml
- GitHub releases to `node-pulse/agent` repository
- Use `make release` for local snapshot testing before actual release

## Code Style Notes

- Prometheus scraper returns raw text format (no parsing)
- Buffer operations are thread-safe (use mutex)
- Config validation happens after loading, before agent starts
- Server ID is validated to ensure alphanumeric start/end, can contain dashes in middle
- All logging uses structured fields (zap)
