# NodePulse Agent - Installation Details

This document lists all files and directories created by the NodePulse agent installation.

## Installed Files and Directories

### Binary
- `/usr/local/bin/pulse` - Main agent executable (~15 MB)

### Configuration
- `/etc/node-pulse/` - Configuration directory
  - `/etc/node-pulse/nodepulse.yml` - Agent configuration file (permissions: 0644)

### Data/State
- `/var/lib/node-pulse/` - Data directory
  - `/var/lib/node-pulse/server_id` - Persisted server UUID (permissions: 0600)
  - `/var/lib/node-pulse/buffer/` - Failed metrics buffer directory
    - `/var/lib/node-pulse/buffer/*.jsonl` - Hourly JSONL buffer files (auto-cleaned after 48h)

### Logs (Optional)
- `/var/log/node-pulse/` - Log directory (created if logging to file is enabled)
  - `/var/log/node-pulse/agent.log` - Main log file
  - `/var/log/node-pulse/agent.log.*.gz` - Rotated compressed logs

### Runtime (Daemon Mode)
- `/tmp/nodepulse.pid` - PID file for daemon mode (auto-removed on stop)

### Systemd Service (Production Mode)
- `/etc/systemd/system/node-pulse.service` - Main systemd service unit
- `/etc/systemd/system/node-pulse-updater.service` - Auto-updater service unit
- `/etc/systemd/system/node-pulse-updater.timer` - Auto-updater timer (runs every 10 minutes)

## Installation Modes

### Quick Install (Recommended)
```bash
curl -fsSL https://get.nodepulse.sh | sudo bash
sudo pulse setup
sudo pulse service install
sudo pulse service start
```

**Creates:**
- Binary: `/usr/local/bin/pulse`
- Config: `/etc/node-pulse/nodepulse.yml`
- State: `/var/lib/node-pulse/server_id`
- State: `/var/lib/node-pulse/buffer/`
- Service: `/etc/systemd/system/node-pulse.service`
- Timer: `/etc/systemd/system/node-pulse-updater.{service,timer}`

### Manual Installation
```bash
# Download and install binary
wget https://github.com/node-pulse/agent/releases/latest/download/pulse-linux-amd64.tar.gz
tar -xzf pulse-linux-amd64.tar.gz
sudo mv pulse /usr/local/bin/
sudo chmod +x /usr/local/bin/pulse

# Setup configuration
sudo pulse setup
```

**Creates:**
- Binary: `/usr/local/bin/pulse`
- Config: `/etc/node-pulse/nodepulse.yml`
- State: `/var/lib/node-pulse/server_id`
- State: `/var/lib/node-pulse/buffer/`

## Uninstallation

### Complete Uninstall (All Files)

```bash
# Stop and disable service
sudo pulse service stop
sudo pulse service uninstall

# Remove binary
sudo rm -f /usr/local/bin/pulse

# Remove configuration
sudo rm -rf /etc/node-pulse

# Remove data and buffer
sudo rm -rf /var/lib/node-pulse

# Remove logs (optional)
sudo rm -rf /var/log/node-pulse

# Remove PID file (if exists)
sudo rm -f /tmp/nodepulse.pid
```

### Partial Uninstall (Keep Configuration)

If you want to preserve configuration for later reinstallation:

```bash
# Stop and disable service
sudo pulse service stop
sudo pulse service uninstall

# Remove binary only
sudo rm -f /usr/local/bin/pulse

# Keep: /etc/node-pulse and /var/lib/node-pulse
```

### Uninstall Service Only

To switch from systemd service to daemon mode:

```bash
# Stop and uninstall service
sudo pulse service stop
sudo pulse service uninstall

# Binary and config remain for daemon mode
pulse start -d
```

## File Permissions

| Path | Type | Permissions | Owner |
|------|------|-------------|-------|
| `/usr/local/bin/pulse` | Binary | 0755 | root |
| `/etc/node-pulse/` | Directory | 0755 | root |
| `/etc/node-pulse/nodepulse.yml` | File | 0644 | root |
| `/var/lib/node-pulse/` | Directory | 0755 | root |
| `/var/lib/node-pulse/server_id` | File | 0600 | root |
| `/var/lib/node-pulse/buffer/` | Directory | 0755 | root |
| `/var/log/node-pulse/` | Directory | 0755 | root |
| `/var/log/node-pulse/agent.log` | File | 0644 | root |
| `/etc/systemd/system/*.service` | File | 0644 | root |
| `/etc/systemd/system/*.timer` | File | 0644 | root |

## Disk Usage

Typical disk usage:
- Binary: ~15 MB
- Config: <1 KB
- Server ID: <100 bytes
- Buffer: 0-50 MB (depends on connection failures, auto-cleaned after 48h)
- Logs: 0-30 MB (auto-rotated, keeps 3 backups)

**Total:** ~15-95 MB

## Network Usage

- **Normal operation:** ~1 KB every 5 seconds (configurable interval)
- **Buffer flush:** Variable (depends on buffered data)
- **Auto-updater:** ~15 MB every 10 minutes (only if update available)

## Process Information

- **Process name:** `pulse`
- **Memory usage:** <40 MB RAM
- **CPU usage:** <1% (spikes briefly during metric collection)
- **Service name:** `node-pulse`
- **Timer name:** `node-pulse-updater.timer`

## Automated Maintenance

The agent performs automatic maintenance:

1. **Buffer cleanup:** Deletes JSONL files older than 48 hours
2. **Log rotation:** Rotates logs when they exceed 10 MB (keeps 3 backups, 7 days retention)
3. **Auto-updates:** Checks for updates every 10 minutes via systemd timer

## Troubleshooting

### Check installation status
```bash
pulse status
```

### Verify files exist
```bash
ls -lh /usr/local/bin/pulse
ls -lh /etc/node-pulse/nodepulse.yml
ls -lh /var/lib/node-pulse/server_id
systemctl list-unit-files | grep node-pulse
```

### Check service status
```bash
sudo pulse service status
systemctl status node-pulse
systemctl status node-pulse-updater.timer
```

### View logs
```bash
# If logging to file
sudo tail -f /var/log/node-pulse/agent.log

# If using systemd (default: stdout)
sudo journalctl -u node-pulse -f
```

## Security Considerations

1. **Server ID file** (`/var/lib/node-pulse/server_id`) has restrictive permissions (0600) to prevent unauthorized access
2. **Config file** can contain sensitive endpoint URLs - ensure proper file permissions
3. **Binary** runs as root when using systemd service (required for system metrics collection)
4. **Auto-updater** runs as root - only downloads from official GitHub releases with signature verification

## See Also

- [README.md](README.md) - Main documentation
- [Agent Configuration](https://docs.nodepulse.io/agent/configuration)
- [Systemd Service Management](https://docs.nodepulse.io/agent/systemd)
