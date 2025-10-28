# NodePulse Agent - Installation Inventory

This document lists everything that gets installed on a server when deploying the NodePulse agent via Ansible.

## Files & Directories Created

### 1. Binary
| Path | Owner | Permissions | Description |
|------|-------|-------------|-------------|
| `/opt/nodepulse/nodepulse` | nodepulse:nodepulse | 755 | Agent binary (downloaded from releases) |

### 2. Configuration
| Path | Owner | Permissions | Description |
|------|-------|-------------|-------------|
| `/etc/nodepulse/` | nodepulse:nodepulse | 755 | Configuration directory |
| `/etc/nodepulse/nodepulse.yml` | nodepulse:nodepulse | 640 | Agent configuration file |

### 3. Data
| Path | Owner | Permissions | Description |
|------|-------|-------------|-------------|
| `/var/lib/nodepulse/` | nodepulse:nodepulse | 755 | Data directory |
| `/var/lib/nodepulse/buffer/` | nodepulse:nodepulse | 755 | Buffer directory for failed sends |
| `/var/lib/nodepulse/server_id` | nodepulse:nodepulse | 600 | Persisted server UUID (created at runtime) |

### 4. Logs
| Path | Owner | Permissions | Description |
|------|-------|-------------|-------------|
| `/var/log/nodepulse/` | nodepulse:nodepulse | 755 | Log directory |
| `/var/log/nodepulse/agent.log` | nodepulse:nodepulse | 640 | Agent log file (created at runtime) |

### 5. Systemd
| Path | Owner | Permissions | Description |
|------|-------|-------------|-------------|
| `/etc/systemd/system/nodepulse.service` | root:root | 644 | Systemd service unit file |

### 6. Logrotate
| Path | Owner | Permissions | Description |
|------|-------|-------------|-------------|
| `/etc/logrotate.d/nodepulse` | root:root | 644 | Log rotation configuration |

### 7. System User & Group
| Resource | Type | Properties |
|----------|------|------------|
| `nodepulse` | User | System user, no home directory, `/usr/sbin/nologin` shell |
| `nodepulse` | Group | System group |

## Runtime Files (Created by Agent)

These files are created when the agent runs:

| Path | Description |
|------|-------------|
| `/var/lib/nodepulse/server_id` | Persisted server UUID |
| `/var/lib/nodepulse/buffer/*.prom` | Buffered Prometheus metrics (when dashboard is unreachable) |
| `/var/log/nodepulse/agent.log` | Active log file |
| `/var/log/nodepulse/agent.log.*.gz` | Rotated compressed logs |

## What's NOT Installed (Changes from v0.0.x)

### Removed from v1.x:
- ❌ No auto-updater timer (`node-pulse-updater.timer`)
- ❌ No auto-updater service (`node-pulse-updater.service`)
- ❌ No PID files in `/tmp/` or `/var/run/` (systemd manages the process)
- ❌ No binary in `/usr/local/bin/pulse` (moved to `/opt/nodepulse/nodepulse`)

### Comparison with v0.0.x:

| Item | v0.0.x | v0.1.0 |
|------|------|------|
| Binary location | `/usr/local/bin/pulse` | `/opt/nodepulse/nodepulse` |
| Config directory | `/etc/node-pulse/` | `/etc/nodepulse/` |
| Data directory | `/var/lib/node-pulse/` | `/var/lib/nodepulse/` |
| Log directory | `/var/log/node-pulse/` | `/var/log/nodepulse/` |
| Service name | `node-pulse.service` | `nodepulse.service` |
| Buffer format | `.jsonl` (JSON Lines) | `.prom` (Prometheus text) |
| Auto-updater | Yes (timer + service) | No (manual update only) |
| System user | `root` (no dedicated user) | `nodepulse` (dedicated system user) |

## Uninstallation

Use the Ansible uninstall playbook to remove all files:

```bash
# Complete uninstall
ansible-playbook -i inventory.yml uninstall-agent.yml

# Keep configuration and server_id
ansible-playbook -i inventory.yml uninstall-agent.yml -e "keep_config=true"

# Keep logs
ansible-playbook -i inventory.yml uninstall-agent.yml -e "keep_logs=true"

# Keep buffered metrics
ansible-playbook -i inventory.yml uninstall-agent.yml -e "keep_buffer=true"
```

### What Gets Removed:

**Always removed:**
- Binary: `/opt/nodepulse/nodepulse`
- Installation directory: `/opt/nodepulse/`
- Systemd service: `/etc/systemd/system/nodepulse.service`
- Logrotate config: `/etc/logrotate.d/nodepulse`
- System user: `nodepulse`
- System group: `nodepulse`

**Conditionally removed (based on flags):**
- Configuration: `/etc/nodepulse/` (unless `keep_config=true`)
- Data directory: `/var/lib/nodepulse/` (unless `keep_config=true`)
- Buffer: `/var/lib/nodepulse/buffer/` (unless `keep_buffer=true` or `keep_config=true`)
- Logs: `/var/log/nodepulse/` (unless `keep_logs=true`)

## Disk Space Usage

Typical disk space usage per agent:

| Component | Size | Notes |
|-----------|------|-------|
| Binary | ~15 MB | Single static binary |
| Configuration | ~1 KB | YAML config file |
| Logs (active) | ~10 MB | With rotation (max_size_mb: 10) |
| Logs (rotated) | ~30 MB | 3 backups × 10 MB compressed |
| Buffer (normal) | ~0 KB | Empty when dashboard is reachable |
| Buffer (max) | Varies | Depends on dashboard downtime |

**Buffer growth calculation:**
- Scrape interval: 15s
- Metrics size: ~50 KB per scrape (Prometheus text format)
- 1 hour downtime: 240 scrapes × 50 KB = ~12 MB
- 48 hours (max retention): 7,680 scrapes × 50 KB = ~384 MB

**Total disk usage (normal operation):** ~60 MB per server

## Network Requirements

**Outbound connections required:**
- Dashboard endpoint: HTTPS to `{{ dashboard_endpoint }}/metrics/prometheus`
- Port: 443 (HTTPS)
- Frequency: Every 15 seconds (default interval)

**Inbound connections:**
- None required (agent is push-based, not pull-based)

## Security Considerations

1. **Dedicated system user**: Agent runs as `nodepulse` user (not root)
2. **No shell access**: User has `/usr/sbin/nologin` shell
3. **Restricted permissions**: Config files are 640 (owner + group read)
4. **Log rotation**: Prevents disk exhaustion
5. **Buffer cleanup**: Old buffer files deleted after 48 hours
6. **Systemd isolation**: Service runs in its own cgroup

## Systemd Service Details

**Service file:** `/etc/systemd/system/nodepulse.service`

**Key properties:**
- Type: `simple` (foreground process)
- ExecStart: `/opt/nodepulse/nodepulse start`
- Restart: `always` (auto-restart on failure)
- User: `nodepulse` (runs as dedicated system user)
- WorkingDirectory: `/var/lib/nodepulse`

**Systemd management:**
```bash
# Start service
sudo systemctl start nodepulse

# Stop service
sudo systemctl stop nodepulse

# Restart service
sudo systemctl restart nodepulse

# Check status
sudo systemctl status nodepulse

# View logs
sudo journalctl -u nodepulse -f
```

## Post-Installation Verification

After Ansible deployment, verify installation:

```bash
# Check binary exists
ls -lh /opt/nodepulse/nodepulse

# Check configuration
cat /etc/nodepulse/nodepulse.yml

# Check service status
sudo systemctl status nodepulse

# Check logs
sudo tail -f /var/log/nodepulse/agent.log

# Check buffer (should be empty normally)
ls -lh /var/lib/nodepulse/buffer/

# Check process
ps aux | grep nodepulse
```

## References

- Ansible Deployment: `flagship/ansible/playbooks/nodepulse/deploy-agent.yml`
- Ansible Uninstall: `flagship/ansible/playbooks/nodepulse/uninstall-agent.yml`
- Agent Documentation: `../README.md`
- Buffer Mechanism: `buffer-mechanism.md`
