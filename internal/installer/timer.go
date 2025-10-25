package installer

import (
	"fmt"
	"os"
)

const (
	// Timer unit path
	TimerUnitPath = "/etc/systemd/system/node-pulse-updater.timer"
	// Service unit path for the updater
	UpdaterServiceUnitPath = "/etc/systemd/system/node-pulse-updater.service"
)

// GetTimerUnit returns the systemd timer unit content
// Timer fires at UTC-aligned 10-minute intervals: 00, 10, 20, 30, 40, 50 minutes past each hour
func GetTimerUnit() string {
	return `[Unit]
Description=NodePulse Agent Updater Timer
Documentation=https://docs.nodepulse.io

[Timer]
# Fire at UTC-aligned 10-minute intervals
# OnCalendar accepts multiple values, each on its own line
# *:00,10,20,30,40,50:00 means every hour at minutes 00, 10, 20, 30, 40, 50
OnCalendar=*:00,10,20,30,40,50:00

# Persistent means if the system was powered off when timer should fire,
# it will fire immediately on next boot
Persistent=true

# Randomize delay by up to 1 minute to avoid thundering herd
# (if you have many agents updating at once)
RandomizedDelaySec=60

[Install]
WantedBy=timers.target
`
}

// GetUpdaterServiceUnit returns the systemd service unit content for the updater
func GetUpdaterServiceUnit() string {
	return `[Unit]
Description=NodePulse Agent Updater
Documentation=https://docs.nodepulse.io
After=network-online.target
Wants=network-online.target

[Service]
Type=oneshot
ExecStart=/usr/local/bin/pulse update

# Run as root (needed to replace binary and restart service)
User=root

# Timeout after 5 minutes (should be plenty for download + restart)
TimeoutStartSec=300

# Logging
StandardOutput=journal
StandardError=journal
SyslogIdentifier=node-pulse-updater

[Install]
# This is a oneshot service, no WantedBy needed (triggered by timer)
`
}

// InstallTimer installs the systemd timer and service units
func InstallTimer() error {
	// Write timer unit
	if err := os.WriteFile(TimerUnitPath, []byte(GetTimerUnit()), 0644); err != nil {
		return fmt.Errorf("failed to write timer unit: %w", err)
	}

	// Write updater service unit
	if err := os.WriteFile(UpdaterServiceUnitPath, []byte(GetUpdaterServiceUnit()), 0644); err != nil {
		return fmt.Errorf("failed to write updater service unit: %w", err)
	}

	return nil
}

// UninstallTimer removes the systemd timer and service units
func UninstallTimer() error {
	// Remove timer unit
	if err := os.Remove(TimerUnitPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove timer unit: %w", err)
	}

	// Remove updater service unit
	if err := os.Remove(UpdaterServiceUnitPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove updater service unit: %w", err)
	}

	return nil
}
