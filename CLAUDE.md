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
go run . agent
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
- **cmd/agent.go**: Main agent loop - collects metrics at configured intervals and sends to server
- **cmd/watch.go**: TUI dashboard using [Bubble Tea](https://github.com/charmbracelet/bubbletea) for real-time metric visualization
- **cmd/service.go**: systemd service management (install/start/stop/restart/status/uninstall)
- **cmd/setup.go**: Interactive setup wizard for first-time configuration (command: `pulse setup`)
- **cmd/status.go**: Shows comprehensive agent status including server ID, config, service status, buffer state, and logs

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

## Agent Main Loop (cmd/agent.go)

1. Load configuration
2. Initialize logger
3. Create HTTP sender (with optional buffer)
4. Setup graceful shutdown on SIGINT/SIGTERM
5. Collect and send immediately on start
6. Enter ticker loop at configured interval (5s, 10s, 30s, or 1m)
7. On each tick:
   - Call `metrics.Collect()` to gather all metrics
   - Record stats (for TUI dashboard)
   - Call `sender.Send()` to POST to server
   - On success: record success, trigger background buffer flush
   - On failure: record failure, report is auto-buffered

## TUI Dashboard (cmd/watch.go)

Built with [Bubble Tea](https://github.com/charmbracelet/bubbletea) and [Lipgloss](https://github.com/charmbracelet/lipgloss):

- Real-time metric display with progress bars
- Hourly statistics (collections, avg CPU/memory, total network)
- Buffer status (shows buffered file count if any)
- Agent info (distro, kernel, interval, endpoint)
- Auto-refreshes at configured interval
- Press `r` to force refresh, `q` to quit

## Important Platform Constraints

- **Linux-only**: All metrics depend on `/proc` filesystem
- **Architectures**: Built for amd64 and arm64 only
- **Permissions**:
  - Regular user can run `pulse agent` and `pulse watch`
  - Root required for `pulse service` commands and writing to `/etc` and `/var`

## Testing Notes

Since the agent relies on Linux `/proc` files, development/testing should be done on a Linux system or VM. The TUI watch command is helpful for visual verification during development.

To test the full flow locally:
1. Run `make dev` or `go run . agent` in one terminal
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
