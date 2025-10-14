# NodePulse Agent

A lightweight Linux server monitoring agent that collects and reports system metrics including CPU, memory, network I/O, and uptime.

## Features

- **Real-time Metrics**: CPU usage, memory usage, network I/O, and system uptime
- **Configurable Intervals**: 5s, 10s, 30s, or 1 minute collection intervals
- **Reliable Delivery**: HTTP-based reporting with automatic buffering on failure
- **Smart Buffering**: Failed reports are stored in hourly JSONL files (48-hour retention)
- **Service Management**: Easy systemd service installation and management
- **Live View**: Built-in TUI for viewing metrics in real-time
- **Cross-Platform**: Builds for both amd64 and arm64 architectures

## Installation

### From Binary

Download the latest release for your architecture:

```bash
# For amd64
wget https://github.com/node-pulse/agent/releases/latest/download/pulse-linux-amd64.tar.gz
tar -xzf pulse-linux-amd64.tar.gz
sudo mv pulse /usr/local/bin/

# For arm64
wget https://github.com/node-pulse/agent/releases/latest/download/pulse-linux-arm64.tar.gz
tar -xzf pulse-linux-arm64.tar.gz
sudo mv pulse /usr/local/bin/
```

### From Source

```bash
git clone https://github.com/node-pulse/agent.git
cd agent
go build -o pulse
sudo mv pulse /usr/local/bin/
```

## Configuration

Create a configuration file at `/etc/node-pulse/nodepulse.yml`:

```yaml
server:
  endpoint: "https://api.nodepulse.io/metrics"
  timeout: 3s

agent:
  interval: 5s # Options: 5s, 10s, 30s, 1m

buffer:
  enabled: true
  path: "/var/lib/node-pulse/buffer"
  retention_hours: 48
```

Or use the included `nodepulse.yml` as a template:

```bash
sudo mkdir -p /etc/node-pulse
sudo cp nodepulse.yml /etc/node-pulse/
sudo nano /etc/node-pulse/nodepulse.yml  # Edit your endpoint
```

## Usage

### Run Agent in Foreground

```bash
pulse agent
```

### View Live Metrics

```bash
pulse view
```

Press `q` to quit the live view.

### Service Management

#### Install as systemd service

```bash
sudo pulse service install
```

#### Start the service

```bash
sudo pulse service start
```

#### Check service status

```bash
sudo pulse service status
```

#### Stop the service

```bash
sudo pulse service stop
```

#### Restart the service

```bash
sudo pulse service restart
```

#### Uninstall the service

```bash
sudo pulse service uninstall
```

## Metrics Collected

### CPU

- Usage percentage (calculated from `/proc/stat`)

### Memory

- Used MB
- Total MB
- Usage percentage (calculated from `/proc/meminfo`)

### Network

- Upload bytes (delta since last collection)
- Download bytes (delta since last collection)
- Collected from `/proc/net/dev` (excludes loopback interface)

### Uptime

- System uptime in days (from `/proc/uptime`)

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
  "cpu": null,
  "memory": { ... },
  "network": { ... },
  "uptime": { ... }
}
```

## Buffering Behavior

When HTTP reporting fails (timeout or error):

1. The report is appended to an hourly JSONL file in the buffer directory
2. Format: `/var/lib/node-pulse/buffer/2025-10-13-14.jsonl`
3. On next successful send, buffered reports are sent (oldest first)
4. Files older than 48 hours are automatically deleted

## Building

### Build for current platform

```bash
go build -o pulse
```

### Build for all platforms using GoReleaser

```bash
# Install goreleaser
go install github.com/goreleaser/goreleaser/v2@latest

# Build snapshot (for testing)
goreleaser release --snapshot --clean

# Binaries will be in dist/
```

### Manual cross-compilation

```bash
# For amd64
GOOS=linux GOARCH=amd64 go build -o pulse-linux-amd64

# For arm64
GOOS=linux GOARCH=arm64 go build -o pulse-linux-arm64
```

## Requirements

- **OS**: Linux (uses `/proc` filesystem)
- **Architectures**: amd64, arm64
- **Permissions**:
  - Normal user for `pulse agent` and `pulse view`
  - Root (sudo) for `pulse service` commands

## Development

### Project Structure

```
agent/
├── cmd/                  # CLI commands
│   ├── root.go          # Root command
│   ├── agent.go         # Agent runner
│   ├── view.go          # TUI view
│   └── service.go       # Service management
├── internal/
│   ├── metrics/         # Metrics collection
│   │   ├── cpu.go
│   │   ├── memory.go
│   │   ├── network.go
│   │   ├── uptime.go
│   │   └── report.go
│   ├── report/          # HTTP sender & buffer
│   │   ├── sender.go
│   │   └── buffer.go
│   └── config/          # Configuration
│       └── config.go
├── .goreleaser.yaml     # Release config
├── nodepulse.yml        # Default config
└── main.go
```

### Testing

The agent needs to run on a Linux system to collect metrics. You can test locally:

```bash
# Run in foreground
go run . agent

# Or build and run
go build -o pulse
./pulse agent
```

View metrics in another terminal:

```bash
./pulse view
```

## License

[MIT](LICENSE)

## Contributing

Contributions are welcome! Please open an issue or submit a pull request.

## Support

For issues and questions, please open an issue on GitHub:
https://github.com/node-pulse/agent/issues
