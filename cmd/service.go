package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/node-pulse/agent/internal/config"
	"github.com/node-pulse/agent/internal/pidfile"
	"github.com/spf13/cobra"
)

const (
	serviceName     = "node-pulse"
	serviceFile     = "/etc/systemd/system/node-pulse.service"
	binaryPath      = "/usr/local/bin/pulse"
	serviceTemplate = `[Unit]
Description=NodePulse Server Monitor Agent
After=network.target

[Service]
Type=simple
ExecStart=%s start
Restart=always
RestartSec=10s

[Install]
WantedBy=multi-user.target
`
)

// serviceCmd represents the service command
var serviceCmd = &cobra.Command{
	Use:   "service",
	Short: "Manage the NodePulse systemd service",
	Long:  `Install, start, stop, restart, status, or uninstall the NodePulse systemd service.`,
}

var serviceInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Install the systemd service",
	RunE:  installService,
}

var serviceStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the service",
	RunE:  startService,
}

var serviceStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the service",
	RunE:  stopService,
}

var serviceRestartCmd = &cobra.Command{
	Use:   "restart",
	Short: "Restart the service",
	RunE:  restartService,
}

var serviceStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check service status",
	RunE:  statusService,
}

var serviceUninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Uninstall the systemd service",
	RunE:  uninstallService,
}

func init() {
	rootCmd.AddCommand(serviceCmd)
	serviceCmd.AddCommand(serviceInstallCmd)
	serviceCmd.AddCommand(serviceStartCmd)
	serviceCmd.AddCommand(serviceStopCmd)
	serviceCmd.AddCommand(serviceRestartCmd)
	serviceCmd.AddCommand(serviceStatusCmd)
	serviceCmd.AddCommand(serviceUninstallCmd)
}

func installService(cmd *cobra.Command, args []string) error {
	// Check config exists
	if err := config.RequireConfig(cfgFile); err != nil {
		return err
	}

	// Check if running as root
	if os.Geteuid() != 0 {
		return fmt.Errorf("this command must be run as root (use sudo)")
	}

	// Get current executable path
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}

	// Copy binary to /usr/local/bin/pulse if not already there
	if exePath != binaryPath {
		if err := copyFile(exePath, binaryPath); err != nil {
			return fmt.Errorf("failed to copy binary: %w", err)
		}
		if err := os.Chmod(binaryPath, 0755); err != nil {
			return fmt.Errorf("failed to set binary permissions: %w", err)
		}
		fmt.Printf("Installed binary to %s\n", binaryPath)
	}

	// Create service file
	serviceContent := fmt.Sprintf(serviceTemplate, binaryPath)
	if err := os.WriteFile(serviceFile, []byte(serviceContent), 0644); err != nil {
		return fmt.Errorf("failed to write service file: %w", err)
	}
	fmt.Printf("Created service file: %s\n", serviceFile)

	// Reload systemd
	if err := runSystemctl("daemon-reload"); err != nil {
		return fmt.Errorf("failed to reload systemd: %w", err)
	}

	// Enable service
	if err := runSystemctl("enable", serviceName); err != nil {
		return fmt.Errorf("failed to enable service: %w", err)
	}

	fmt.Println("Service installed and enabled successfully!")
	fmt.Println("\nTo start the service, run:")
	fmt.Printf("  sudo pulse service start\n")
	return nil
}

func startService(cmd *cobra.Command, args []string) error {
	// Check config exists
	if err := config.RequireConfig(cfgFile); err != nil {
		return err
	}

	if os.Geteuid() != 0 {
		return fmt.Errorf("this command must be run as root (use sudo)")
	}

	// Check if daemon is already running
	isRunning, pid, err := pidfile.CheckRunning()
	if err != nil {
		fmt.Printf("Warning: failed to check daemon status: %v\n", err)
	} else if isRunning {
		return fmt.Errorf("agent is already running as daemon (PID %d)\nUse 'pulse stop' first", pid)
	}

	if err := runSystemctl("start", serviceName); err != nil {
		return fmt.Errorf("failed to start service: %w", err)
	}

	fmt.Println("Service started successfully!")
	return nil
}

func stopService(cmd *cobra.Command, args []string) error {
	if os.Geteuid() != 0 {
		return fmt.Errorf("this command must be run as root (use sudo)")
	}

	if err := runSystemctl("stop", serviceName); err != nil {
		return fmt.Errorf("failed to stop service: %w", err)
	}

	fmt.Println("Service stopped successfully!")
	return nil
}

func restartService(cmd *cobra.Command, args []string) error {
	// Check config exists
	if err := config.RequireConfig(cfgFile); err != nil {
		return err
	}

	if os.Geteuid() != 0 {
		return fmt.Errorf("this command must be run as root (use sudo)")
	}

	if err := runSystemctl("restart", serviceName); err != nil {
		return fmt.Errorf("failed to restart service: %w", err)
	}

	fmt.Println("Service restarted successfully!")
	return nil
}

func statusService(cmd *cobra.Command, args []string) error {
	// Status doesn't require root
	output, err := exec.Command("systemctl", "status", serviceName).CombinedOutput()
	fmt.Print(string(output))
	return err
}

func uninstallService(cmd *cobra.Command, args []string) error {
	if os.Geteuid() != 0 {
		return fmt.Errorf("this command must be run as root (use sudo)")
	}

	// Stop service if running
	runSystemctl("stop", serviceName)

	// Disable service
	if err := runSystemctl("disable", serviceName); err != nil {
		fmt.Printf("Warning: failed to disable service: %v\n", err)
	}

	// Remove service file
	if err := os.Remove(serviceFile); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove service file: %w", err)
	}

	// Reload systemd
	if err := runSystemctl("daemon-reload"); err != nil {
		return fmt.Errorf("failed to reload systemd: %w", err)
	}

	fmt.Println("Service uninstalled successfully!")
	return nil
}

func runSystemctl(args ...string) error {
	cmd := exec.Command("systemctl", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, string(output))
	}
	return nil
}

func copyFile(src, dst string) error {
	input, err := os.ReadFile(src)
	if err != nil {
		return err
	}

	// Create directory if needed
	dir := filepath.Dir(dst)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	return os.WriteFile(dst, input, 0755)
}
