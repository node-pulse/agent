package cmd

import (
	"fmt"
	"os/exec"
	"path/filepath"

	"github.com/node-pulse/agent/internal/config"
	"github.com/spf13/cobra"
)

// statusCmd represents the status command
var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Display agent status and configuration",
	Long:  `Shows comprehensive status including server ID, configuration, service status, buffer state, and logging.`,
	RunE:  runStatus,
}

func init() {
	rootCmd.AddCommand(statusCmd)
}

func runStatus(cmd *cobra.Command, args []string) error {
	// Check config exists
	if err := config.RequireConfig(cfgFile); err != nil {
		return err
	}

	// Load configuration (this will auto-generate UUID if needed)
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Display status information
	fmt.Println("Node Pulse Agent Status")
	fmt.Println("=====================")
	fmt.Println()

	// Server ID
	serverIDPath := config.GetServerIDPath()
	fmt.Printf("Server ID:     %s\n", cfg.Agent.ServerID)
	fmt.Printf("Persisted at:  %s\n", serverIDPath)
	fmt.Println()

	// Configuration
	configFileUsed := cfg.ConfigFile
	if configFileUsed == "" {
		configFileUsed = "using defaults (no config file found)"
	}
	fmt.Printf("Config File:   %s\n", configFileUsed)
	fmt.Printf("Endpoint:      %s\n", cfg.Server.Endpoint)
	fmt.Printf("Interval:      %s\n", cfg.Agent.Interval)
	fmt.Println()

	// Agent/Service Status
	serviceStatus := getServiceStatus()
	fmt.Printf("Agent:         %s\n", serviceStatus)
	fmt.Println()

	// Buffer Status
	if cfg.Buffer.Enabled {
		bufferCount, err := countBufferFiles(cfg.Buffer.Path)
		if err != nil {
			fmt.Printf("Buffer:        enabled (error checking: %v)\n", err)
		} else if bufferCount > 0 {
			fmt.Printf("Buffer:        %d report(s) pending in %s\n", bufferCount, cfg.Buffer.Path)
		} else {
			fmt.Printf("Buffer:        enabled, no pending reports\n")
		}
	} else {
		fmt.Printf("Buffer:        disabled\n")
	}
	fmt.Println()

	// Logging
	if cfg.Logging.Output == "file" || cfg.Logging.Output == "both" {
		fmt.Printf("Log File:      %s\n", cfg.Logging.File.Path)
	} else {
		fmt.Printf("Log Output:    %s\n", cfg.Logging.Output)
	}

	return nil
}

// getServiceStatus checks if the systemd service is running
func getServiceStatus() string {
	// Try to check systemd status
	cmd := exec.Command("systemctl", "is-active", "node-pulse")
	output, err := cmd.Output()

	if err == nil && string(output) == "active\n" {
		return "running (via systemd)"
	}

	// Check if service exists but is not active
	cmd = exec.Command("systemctl", "is-enabled", "node-pulse")
	_, err = cmd.Output()

	if err == nil {
		return "stopped (systemd service installed)"
	}

	return "not installed as systemd service"
}

// countBufferFiles counts the number of .jsonl files in the buffer directory
func countBufferFiles(bufferPath string) (int, error) {
	pattern := filepath.Join(bufferPath, "*.jsonl")
	files, err := filepath.Glob(pattern)
	if err != nil {
		return 0, err
	}
	return len(files), nil
}
