package pidfile

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

const (
	daemonPidFile = ".node-pulse/pulse.pid"
)

// GetPidFilePath returns the PID file path based on user privileges
func GetPidFilePath() string {
	if os.Geteuid() == 0 {
		// Root: use /var/run
		return "/var/run/pulse.pid"
	}
	// Normal user: use home directory
	home, err := os.UserHomeDir()
	if err != nil {
		// Fallback to current directory
		return "pulse.pid"
	}
	return filepath.Join(home, daemonPidFile)
}

// IsProcessRunning checks if a process with the given PID is running
func IsProcessRunning(pid int) bool {
	// Send signal 0 to check if process exists
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = process.Signal(syscall.Signal(0))
	return err == nil
}

// ReadPidFile reads the PID from the PID file
func ReadPidFile() (int, error) {
	pidPath := GetPidFilePath()
	data, err := os.ReadFile(pidPath)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil // No PID file, no process running
		}
		return 0, fmt.Errorf("failed to read PID file: %w", err)
	}

	pidStr := strings.TrimSpace(string(data))
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		return 0, fmt.Errorf("invalid PID in file: %w", err)
	}

	return pid, nil
}

// WritePidFile writes the current process PID to the PID file
func WritePidFile(pid int) error {
	pidPath := GetPidFilePath()

	// Create directory if needed
	dir := filepath.Dir(pidPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create PID directory: %w", err)
	}

	// Write PID to file
	if err := os.WriteFile(pidPath, []byte(fmt.Sprintf("%d\n", pid)), 0644); err != nil {
		return fmt.Errorf("failed to write PID file: %w", err)
	}

	return nil
}

// RemovePidFile removes the PID file
func RemovePidFile() error {
	pidPath := GetPidFilePath()
	err := os.Remove(pidPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove PID file: %w", err)
	}
	return nil
}

// CheckRunning checks if the agent is already running
// Returns (isRunning, pid, error)
func CheckRunning() (bool, int, error) {
	pid, err := ReadPidFile()
	if err != nil {
		return false, 0, err
	}

	if pid == 0 {
		return false, 0, nil
	}

	// Check if process is actually running
	if IsProcessRunning(pid) {
		return true, pid, nil
	}

	// Stale PID file - process not running
	// Clean it up
	RemovePidFile()
	return false, 0, nil
}
