# NodePulse Agent

A lightweight Prometheus forwarder written in Go. It scrapes metrics from `node_exporter` and forwards them to your NodePulse dashboard via HTTP.

**Lightweight & Efficient:**

- Single binary, <15 MB
- <40 MB RAM usage
- Standard 15-second scrape interval

## Features

- **Prometheus Scraping**: Scrapes `node_exporter` on `localhost:9100`
- **Reliable Delivery**: HTTP-based forwarding with automatic buffering
- **Smart Buffering**: Failed reports stored as `.prom` files (48-hour retention)
- **Random Jitter**: Distributes load across scrape interval window
- **Batch Processing**: Sends up to 5 buffered reports per request
- **Service Management**: Easy systemd service installation
- **Cross-Platform**: Builds for both amd64 and arm64 architectures

## Prerequisites

**You must install `node_exporter` first:**

```bash
# Download node_exporter
wget https://github.com/prometheus/node_exporter/releases/download/v1.8.2/node_exporter-1.8.2.linux-amd64.tar.gz
tar -xzf node_exporter-1.8.2.linux-amd64.tar.gz
sudo mv node_exporter-1.8.2.linux-amd64/node_exporter /usr/local/bin/
sudo chmod +x /usr/local/bin/node_exporter

# Create systemd service
sudo tee /etc/systemd/system/node_exporter.service <<EOF
[Unit]
Description=Prometheus Node Exporter
After=network.target

[Service]
Type=simple
ExecStart=/usr/local/bin/node_exporter --web.listen-address=127.0.0.1:9100
Restart=always

[Install]
WantedBy=multi-user.target
EOF

# Start service
sudo systemctl daemon-reload
sudo systemctl enable node_exporter
sudo systemctl start node_exporter

# Verify
curl http://localhost:9100/metrics
```

**Security: Block Port 9100 Externally**

⚠️ **CRITICAL**: Port 9100 exposes sensitive system information!

```bash
# Using UFW (Ubuntu/Debian)
sudo ufw deny 9100/tcp

# Using iptables
sudo iptables -A INPUT -p tcp --dport 9100 ! -s 127.0.0.1 -j DROP
sudo iptables-save | sudo tee /etc/iptables/rules.v4
```

## Installation

### From Binary

Download the latest release for your architecture:

```bash
# For amd64
wget https://github.com/node-pulse/agent/releases/latest/download/nodepulse-linux-amd64.tar.gz
tar -xzf nodepulse-linux-amd64.tar.gz
sudo mv nodepulse /usr/local/bin/
sudo chmod +x /usr/local/bin/nodepulse

# For arm64
wget https://github.com/node-pulse/agent/releases/latest/download/nodepulse-linux-arm64.tar.gz
tar -xzf nodepulse-linux-arm64.tar.gz
sudo mv nodepulse /usr/local/bin/
sudo chmod +x /usr/local/bin/nodepulse
```

### From Source

```bash
git clone https://github.com/node-pulse/agent.git
cd agent
go build -o nodepulse
sudo mv nodepulse /usr/local/bin/
sudo chmod +x /usr/local/bin/nodepulse
```

## Usage

### Initialize Configuration (First Time Setup)

```bash
sudo nodepulse setup --endpoint-url https://dashboard.nodepulse.io/metrics/prometheus --server-id <your-uuid>
```

Setup command:

- Creates necessary directories (`/etc/nodepulse`, `/var/lib/nodepulse`, `/var/log/nodepulse`)
- Generates configuration file with hardcoded defaults
- Uses provided server ID (assigned by dashboard when adding server)

**Server ID**: When you add a server in the dashboard, it will provide a UUID. Pass this as `--server-id`.

### Running the Agent

#### Foreground Mode (Development/Testing)

```bash
nodepulse start
```

Or run without subcommand (equivalent):

```bash
nodepulse --config /etc/nodepulse/nodepulse.yml
```

Runs the agent in the foreground (blocks the terminal). Best for development and testing.
- Stop with: **Ctrl+C** (gracefully shuts down and cleans up)
- Creates PID file to prevent duplicate runs
- Logs to stdout by default

#### Daemon Mode (Background - Development Only)

```bash
nodepulse start -d
```

Runs the agent in the background for quick testing. **Not recommended for production.**
- Detaches from terminal and runs in background
- Creates PID file for process management
- Stop with: `nodepulse stop`

```bash
nodepulse stop
```

Stops the background daemon agent:
- Sends SIGTERM for graceful shutdown (waits up to 5 seconds)
- Sends SIGKILL if process doesn't stop gracefully
- Automatically cleans up PID file
- **Note**: Only stops daemon mode agents, not systemd-managed services

#### Production Mode (Systemd Service)

For production deployments, use systemd service management:

```bash
sudo nodepulse service install
sudo nodepulse service start
```

Benefits:
- Automatic restart on failure
- Starts on system boot
- Managed by systemd (no PID file needed)
- Stop with: `sudo nodepulse service stop`

**Important**: `nodepulse stop` will not stop systemd-managed agents. Use `nodepulse service stop` instead.

### Check Agent Status

```bash
nodepulse status
```

Shows comprehensive agent status including server ID, configuration, service status, buffer state, and logging.

**Example output:**

```
Node Pulse Agent Status
=====================

Server ID:     a1b2c3d4-e5f6-7890-abcd-ef1234567890
Persisted at:  /var/lib/nodepulse/server_id

Config File:   /etc/nodepulse/nodepulse.yml
Endpoint:      https://dashboard.nodepulse.io/metrics/prometheus
Interval:      15s

Agent:         running (via systemd)

Buffer:        3 report(s) pending in /var/lib/nodepulse/buffer

Log File:      /var/log/nodepulse/agent.log
```

### Service Management

#### Install as systemd service

```bash
sudo nodepulse service install
```

#### Start the service

```bash
sudo nodepulse service start
```

#### Check service status

```bash
sudo nodepulse service status
```

#### Stop the service

```bash
sudo nodepulse service stop
```

#### Restart the service

```bash
sudo nodepulse service restart
```

#### Uninstall the service

```bash
sudo nodepulse service uninstall
```

## Configuration

Configuration file at `/etc/nodepulse/nodepulse.yml`:

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
  level: "info"  # Options: debug, info, warn, error
  output: "stdout"  # Options: stdout, file, both
  file:
    path: "/var/log/nodepulse/agent.log"
    max_size_mb: 10
    max_backups: 3
    max_age_days: 7
    compress: true
```

### Configuration Notes

**Hardcoded Defaults:**

Most settings use hardcoded defaults and are **not configurable** during Ansible deployment:
- `interval`: 15s (Prometheus standard)
- `timeout`: 5s
- `prometheus.endpoint`: `http://localhost:9100/metrics`
- `buffer.retention_hours`: 48
- `buffer.batch_size`: 5
- `logging.*`: All logging settings

**Configurable Fields (Ansible Deployment):**

Only **TWO** fields are configurable:
1. `server.endpoint`: Dashboard URL (e.g., `https://dashboard.nodepulse.io/metrics/prometheus`)
2. `agent.server_id`: UUID assigned by dashboard when adding server

### Logging Configuration

The agent supports flexible logging with the following options:

- **level**: Set log verbosity (`debug`, `info`, `warn`, `error`)
  - `debug`: Verbose diagnostic information
  - `info`: General informational messages (default)
  - `warn`: Potentially harmful situations
  - `error`: Error events

- **output**: Choose where logs are written (`stdout`, `file`, `both`)
  - `stdout`: Output to console/terminal (default, recommended for systemd)
  - `file`: Write to log file with automatic rotation
  - `both`: Output to both console and file

- **file**: Log file rotation settings (applies when output is `file` or `both`)
  - `path`: Location of the log file
  - `max_size_mb`: Maximum size in MB before rotating (default: 10)
  - `max_backups`: Maximum number of old log files to keep (default: 3)
  - `max_age_days`: Maximum age in days for old log files (default: 7)
  - `compress`: Compress rotated logs with gzip (default: true)

### Server ID Generation & Persistence

The server ID uniquely identifies your server in the NodePulse system:

- **Dashboard Assignment**: When you add a server in the dashboard, it assigns a UUID
- **Ansible Deployment**: Pass the UUID as `server_id` variable
- **Persistence**: The agent stores the ID in `/var/lib/nodepulse/server_id`
- **Fallback Locations**: `/etc/nodepulse/server_id`, `~/.nodepulse/server_id`, `./server_id`

## Metrics Collected

The agent forwards 100+ metrics from `node_exporter`, including:

### System Information
- Hostname, kernel version, OS distribution
- CPU cores, architecture

### CPU Metrics
- CPU usage per core
- System, user, idle, iowait times
- CPU frequency, thermal throttling

### Memory Metrics
- Total, used, free, available memory
- Swap usage
- Memory pressure

### Disk Metrics
- Disk I/O operations (reads/writes)
- Disk space usage per mount point
- Filesystem info

### Network Metrics
- Network I/O (bytes sent/received)
- Packet counts, errors, drops
- TCP connection states

### System Metrics
- System uptime
- Load average (1m, 5m, 15m)
- Process counts
- File descriptor usage

**For full list**: Run `curl http://localhost:9100/metrics` to see all available metrics.

## Prometheus Text Format

The agent forwards metrics in Prometheus text format:

```
# HELP node_cpu_seconds_total Seconds the CPUs spent in each mode.
# TYPE node_cpu_seconds_total counter
node_cpu_seconds_total{cpu="0",mode="idle"} 123456.78
node_cpu_seconds_total{cpu="0",mode="system"} 1234.56
node_cpu_seconds_total{cpu="0",mode="user"} 2345.67

# HELP node_memory_MemTotal_bytes Memory information field MemTotal_bytes.
# TYPE node_memory_MemTotal_bytes gauge
node_memory_MemTotal_bytes 8.589934592e+09
```

## Buffering Behavior (Write-Ahead Log Pattern)

When HTTP forwarding fails (timeout or error):

1. **Metrics are saved to buffer first** (before sending)
2. **Background goroutine drains buffer continuously** with random jitter
3. **Format**: `/var/lib/nodepulse/buffer/YYYYMMDD-HHMMSS-<server_id>.prom`
4. **Batch processing**: Sends up to 5 reports per request (configurable)
5. **Oldest first**: Processes files in chronological order
6. **Cleanup**: Files older than 48 hours are automatically deleted

**Random Jitter:**
- Distributes load across the scrape interval window
- Prevents thundering herd problem with multiple agents
- Example: With 15s interval, delay is random between 0-15s

## Building

### Using Makefile (Recommended)

```bash
# Build for current platform
make build

# Build for all Linux platforms (amd64 + arm64)
make build-all

# Binaries will be in build/ directory
```

### Build for all platforms using GoReleaser

```bash
# Install goreleaser
go install github.com/goreleaser/goreleaser/v2@latest

# Build snapshot (for testing)
make release

# Binaries will be in dist/ directory
```

### Manual compilation

```bash
# For current platform (outputs to build/)
make build

# Or using go directly
go build -o build/nodepulse .

# For Linux amd64
make build-linux-amd64

# For Linux arm64
make build-linux-arm64
```

## Requirements

- **OS**: Linux (uses `node_exporter`)
- **Architectures**: amd64, arm64
- **Dependencies**: `node_exporter` running on `localhost:9100`
- **Permissions**:
  - Normal user for `nodepulse start`, `nodepulse start -d`, `nodepulse stop`
  - Root (sudo) for `nodepulse service` commands and `nodepulse setup`

## Data Retention

**Dashboard Retention:** 7 days raw data

- Raw Prometheus metrics stored for 7 days
- After 7 days: automatic deletion (drop old partitions)
- Storage estimate: ~3 TB for 1000 servers
- Industry standard for self-hosted monitoring

## Development

### Project Structure

```
agent/
├── cmd/                     # CLI commands
│   ├── root.go             # Root command
│   ├── start.go            # Agent runner (Prometheus scraper)
│   ├── setup.go            # Setup wizard
│   ├── service.go          # Service management
│   ├── status.go           # Status display
│   ├── stop.go             # Stop daemon
│   └── update.go           # Self-updater
├── internal/
│   ├── prometheus/         # Prometheus scraper
│   │   ├── scraper.go
│   │   └── scraper_test.go
│   ├── report/             # HTTP sender & buffer
│   │   ├── sender.go
│   │   ├── buffer.go
│   │   └── buffer_status.go
│   ├── config/             # Configuration
│   │   ├── config.go
│   │   └── serverid.go
│   ├── logger/             # Logging
│   │   └── logger.go
│   └── pidfile/            # PID file management
│       └── pidfile.go
├── .goreleaser.yaml        # Release config
├── nodepulse.yml           # Example config
└── main.go
```

### Testing

The agent needs to run on a Linux system with `node_exporter`:

```bash
# Ensure node_exporter is running
curl http://localhost:9100/metrics

# Run agent directly with go
go run . start

# Or build and run
make build
./build/nodepulse start
```

## License

[MIT](LICENSE)

## Contributing

Contributions are welcome! Please open an issue or submit a pull request.

## Support

For issues and questions, please open an issue on GitHub:
https://github.com/node-pulse/agent/issues
