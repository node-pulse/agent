# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

NodePulse Agent is a lightweight system monitoring agent written in Go that collects metrics (CPU, memory, network I/O, uptime) from Linux servers and reports them to a central control server via HTTP. The agent is designed to be minimal (<15 MB binary, <40 MB RAM) and efficient, with smart buffering for offline resilience.

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
  - Foreground mode (`pulse start`): Runs in terminal, blocks execution
  - Daemon mode (`pulse start -d`): Spawns background process using `exec.Command` with `Setsid: true`
  - Both modes create PID file EXCEPT when running under systemd (detected via `INVOCATION_ID` env var)
- **cmd/stop.go**: Stops daemon mode only (reads PID file, sends SIGTERM → SIGKILL if needed)
  - Will NOT stop systemd-managed processes (they don't create PID files)
  - Provides helpful message if systemd service is running
- **cmd/watch.go**: TUI dashboard using [Bubble Tea](https://github.com/charmbracelet/bubbletea) for real-time metric visualization
- **cmd/service.go**: systemd service management (install/start/stop/restart/status/uninstall)
  - Also manages updater timer installation/uninstallation
- **cmd/setup.go**: Interactive setup wizard for first-time configuration (command: `pulse setup`)
- **cmd/status.go**: Shows comprehensive agent status including server ID, config, service status, buffer state, and logs
- **cmd/update.go**: Self-update command that checks for new versions and performs updates
  - Called automatically by systemd timer every 10 minutes (UTC-aligned)
  - Can also be run manually: `pulse update`

### Metrics Collection (internal/metrics/)
Each metric type has its own collector that reads from `/proc` filesystem:

- **cpu.go**: Parses `/proc/stat` to calculate CPU usage percentage
- **memory.go**: Parses `/proc/meminfo` for memory usage
- **network.go**: Parses `/proc/net/dev` for upload/download bytes (delta-based)
- **uptime.go**: Reads `/proc/uptime` for system uptime
- **system.go**: Collects static system info from `/proc/version` and `/etc/os-release` (cached after first call)
- **report.go**: Aggregates all metrics into a single Report struct

**Important**: Each metric is collected independently. If one fails, it's set to `null` in the JSON, but collection continues for others. Only if all metrics fail does collection return an error.

### Reporting System (internal/report/)
Two-tier reporting with HTTP + file-based buffering:

- **sender.go**: Handles HTTP POST to server endpoint with configurable timeout
  - On success: attempts to flush any buffered reports in background
  - On failure: saves report to buffer (if enabled)
- **buffer.go**: JSONL-based buffering system
  - Reports are appended to hourly files: `YYYY-MM-DD-HH.jsonl`
  - On next successful send, buffered reports are sent oldest-first
  - Files are only deleted AFTER all their reports are successfully sent (not before)
  - If flush fails mid-process, remaining files are kept for next retry
  - Files older than retention period (default 48h) are auto-cleaned
  - Thread-safe with mutex locks preventing concurrent access issues

### Configuration (internal/config/)
Uses [Viper](https://github.com/spf13/viper) for config loading from YAML:

- **config.go**: Main config loading, validation, and defaults
- **serverid.go**: Server ID generation and persistence
  - Auto-generates UUID if not set in config
  - Persists to `/var/lib/node-pulse/server_id`
  - Validates format: alphanumeric + dashes, must start/end with alphanumeric

Config search paths (in order):
1. Explicit `--config` flag
2. `/etc/node-pulse/nodepulse.yml`
3. `$HOME/.node-pulse/nodepulse.yml`
4. `./nodepulse.yml`

### Logger (internal/logger/)
Structured logging with [Zap](https://github.com/uber-go/zap):

- Supports stdout and file output
- File rotation with [Lumberjack](https://github.com/natefinch/lumberjack)
- Configurable log level, size limits, and retention

### Statistics (internal/metrics/stats.go)
Tracks hourly statistics for the dashboard:

- Collection count, success/failure counts
- Average CPU and memory usage
- Total upload/download bytes
- Stats reset each hour automatically

### Updater (internal/updater/)
Self-update system for the agent:

- **updater.go**: Core updater logic
  - Checks version API endpoint for new releases
  - Downloads new binary with SHA256 checksum verification
  - Atomically replaces `/usr/local/bin/pulse`
  - Restarts agent service via systemd
  - Includes automatic rollback on failure
- **Version endpoint**: `GET /agent/version?version={current}&os={os}&arch={arch}`
  - Returns 204 No Content if no update available
  - Returns 200 OK with JSON if update available:
    ```json
    {
      "version": "1.1.0",
      "url": "https://releases.nodepulse.io/agent/1.1.0/pulse-linux-amd64",
      "checksum": "abc123..."
    }
    ```

### Updater Timer (internal/installer/timer.go)
Systemd timer configuration for automatic updates:

- **Timer**: `/etc/systemd/system/node-pulse-updater.timer`
  - Fires at UTC-aligned 10-minute intervals: `:00`, `:10`, `:20`, `:30`, `:40`, `:50`
  - Uses `OnCalendar=*:00,10,20,30,40,50:00` for precise timing
  - Includes random delay (0-60s) to avoid thundering herd
  - Persistent mode: fires on boot if missed while powered off
- **Service**: `/etc/systemd/system/node-pulse-updater.service`
  - Type: oneshot (exits after each update check)
  - Executes: `/usr/local/bin/pulse update`
  - 5-minute timeout for download + restart
  - Logs to systemd journal as `node-pulse-updater`

### PID File Management (internal/pidfile/)
Handles process tracking and prevents duplicate agent runs:

- **Location**: `/var/run/pulse.pid` (root) or `~/.node-pulse/pulse.pid` (user)
- **Stale PID detection**: `CheckRunning()` automatically detects and cleans stale PID files by sending signal 0 to check process existence
- **Lifecycle**:
  - Foreground mode: Creates PID file → `defer RemovePidFile()` → Ctrl+C triggers graceful shutdown → deferred cleanup runs
  - Daemon mode: Parent checks, child creates PID file → `pulse stop` removes it
  - Systemd mode: **No PID file created** (systemd manages the process itself)
- **Systemd detection**: Checks for `INVOCATION_ID` environment variable (automatically set by systemd for all services)

### Process Separation (No Conflicts)

**Three independent execution paths:**

1. **Foreground** (`pulse start`)
   - Creates PID file
   - Blocks terminal
   - Stop: Ctrl+C (signal handler → graceful shutdown → deferred cleanup)

2. **Daemon** (`pulse start -d`)
   - Creates PID file
   - Detached process (`Setsid: true`)
   - Stop: `pulse stop` (reads PID → SIGTERM → wait 5s → SIGKILL if needed)

3. **Systemd** (`pulse service start`)
   - **No PID file** (systemd tracks it via cgroups)
   - Managed by systemd (auto-restart, boot persistence)
   - Stop: `pulse service stop` (calls `systemctl stop`)
   - `pulse stop` cannot affect it (no PID file to read)

**Protection**: If user runs `pulse stop` when only systemd service is active, it detects this (`systemctl is-active --quiet node-pulse`) and displays helpful guidance to use `pulse service stop` instead.

## Agent Main Loop (cmd/start.go)

The agent runs as a **long-running daemon** (NOT a one-shot process):

1. Load configuration
2. Initialize logger
3. Create HTTP sender with buffer (always enabled)
4. Setup graceful shutdown on SIGINT/SIGTERM
5. **Start background drain goroutine** (continuously attempts to send buffered reports)
6. Collect and buffer metrics immediately on start
7. Enter infinite ticker loop at configured interval (5s, 10s, 30s, or 1m)
8. On each tick:
   - Call `metrics.Collect()` to gather all metrics
   - **Synchronously save to buffer** (Write-Ahead Log pattern)
   - Record stats (for TUI dashboard)
   - Return immediately (no HTTP blocking)
9. **Background goroutine** (runs concurrently):
   - Continuously checks for buffered reports
   - Batches up to `batch_size` reports (default: 5)
   - Attempts HTTP POST to server
   - On success: deletes sent files, triggers cleanup
   - On failure: keeps files, waits random delay (0 to interval), retries
   - Includes random jitter to avoid thundering herd

**Key Design Point**: The agent is a monolithic process that does collection, buffering, and sending all in one binary. Systemd keeps it running (with auto-restart), while the internal ticker controls collection frequency.

## TUI Dashboard (cmd/watch.go)

Built with [Bubble Tea](https://github.com/charmbracelet/bubbletea) and [Lipgloss](https://github.com/charmbracelet/lipgloss):

- Real-time metric display with progress bars
- Hourly statistics (collections, avg CPU/memory, total network)
- Buffer status (shows buffered file count if any)
- Agent info (distro, kernel, interval, endpoint)
- Auto-refreshes at configured interval
- Press `r` to force refresh, `q` to quit

## Self-Update Flow

When the updater timer fires (every 10 minutes):

1. **Check version**: `pulse update` calls `GET /agent/version?version={current}&os=linux&arch=amd64`
2. **Server response**:
   - 204 No Content → Agent is up to date, exit
   - 200 OK with JSON → Update available, proceed
3. **Download**: Fetch new binary from URL in response
4. **Verify**: Calculate SHA256 checksum, compare with expected
5. **Stop agent**: `systemctl stop node-pulse` (graceful shutdown)
6. **Backup**: Copy current `/usr/local/bin/pulse` to `/usr/local/bin/pulse.backup`
7. **Replace**: Atomically move new binary to `/usr/local/bin/pulse`
8. **Start agent**: `systemctl start node-pulse` (loads new version)
9. **Cleanup**: Remove backup if successful
10. **Rollback**: On any failure, restore from backup and restart

**Downtime**: Typically 1-2 seconds during the stop → replace → start cycle.

## Important Platform Constraints

- **Linux-only**: All metrics depend on `/proc` filesystem
- **Architectures**: Built for amd64 and arm64 only
- **Permissions**:
  - Regular user can run `pulse start`, `pulse start -d`, `pulse stop`, and `pulse watch`
  - Root required for `pulse service`, `pulse setup`, and `pulse update` commands (writes to `/etc` and `/var`)

## Testing Notes

Since the agent relies on Linux `/proc` files, development/testing should be done on a Linux system or VM. The TUI watch command is helpful for visual verification during development.

To test the full flow locally:
1. Run `make dev` or `go run . start` in one terminal
2. Run `./build/pulse watch` in another terminal to see live metrics
3. Verify buffering by pointing endpoint to invalid URL and checking buffer directory

## Release Process

This project uses [GoReleaser](https://goreleaser.com/) for multi-architecture builds:

- Configured in `.goreleaser.yaml`
- Creates tar.gz archives with binary + LICENSE + README + nodepulse.yml
- GitHub releases to `node-pulse/agent` repository
- Use `make release` for local snapshot testing before actual release

## Code Style Notes

- Metric collectors return `(*MetricType, error)` - never panic
- Buffer operations are thread-safe (use mutex)
- Config validation happens after loading, before agent starts
- System info is cached globally on first collection (doesn't change at runtime)
- Server ID is validated to ensure alphanumeric start/end, can contain dashes in middle
