package cmd

import (
	"fmt"
	"os"

	"github.com/charmbracelet/lipgloss"
	"github.com/node-pulse/agent/internal/config"
	"github.com/node-pulse/agent/internal/logger"
	"github.com/node-pulse/agent/internal/updater"
	"github.com/spf13/cobra"
)

// updateCmd represents the update command
var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Check for and install agent updates",
	Long: `Checks the update server for a new version of the agent.
If an update is available, downloads, verifies, and installs it.

This command is typically run automatically by systemd timer every 10 minutes.
Manual usage: pulse update`,
	RunE: runUpdate,
}

func init() {
	rootCmd.AddCommand(updateCmd)
}

func runUpdate(cmd *cobra.Command, args []string) error {
	// Initialize logger first (use minimal config for updater)
	logCfg := logger.Config{
		Level:  "info",
		Output: "stdout",
	}
	if err := logger.Initialize(logCfg); err != nil {
		return fmt.Errorf("failed to initialize logger: %w", err)
	}

	// Load configuration to get update endpoint
	cfg, err := config.Load(cfgFile)
	if err != nil {
		// If config doesn't exist, use defaults
		logger.Warn("Failed to load config, using defaults", logger.Err(err))
		cfg = &config.Config{
			Server: config.ServerConfig{
				Endpoint: "https://api.nodepulse.io/metrics",
			},
		}
	}

	// Derive update endpoint from metrics endpoint
	// Example: https://api.nodepulse.io/metrics -> https://api.nodepulse.io/agent/version
	updateEndpoint := deriveUpdateEndpoint(cfg.Server.Endpoint)

	// Create updater
	updaterCfg := updater.Config{
		UpdateEndpoint: updateEndpoint,
		BinaryPath:     "/usr/local/bin/pulse",
		ServiceName:    "node-pulse",
	}

	u := updater.New(updaterCfg)

	// Check and perform update
	updated, err := u.CheckAndUpdate()
	if err != nil {
		// Print user-friendly error
		errorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#EF4444")).Bold(true)
		fmt.Fprintln(os.Stderr, errorStyle.Render("Update failed: ")+err.Error())
		return err
	}

	if updated {
		successStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#10B981")).Bold(true)
		fmt.Println(successStyle.Render("✓ Agent updated successfully"))
	} else {
		infoStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#3B82F6"))
		fmt.Println(infoStyle.Render("Agent is already up to date"))
		fmt.Printf("Current version: %s\n", updater.CurrentVersion)
	}

	return nil
}

// deriveUpdateEndpoint converts a metrics endpoint to an update endpoint
// Example: https://api.nodepulse.io/metrics -> https://api.nodepulse.io/agent/version
func deriveUpdateEndpoint(metricsEndpoint string) string {
	// Simple heuristic: replace /metrics with /agent/version
	// You can make this more sophisticated if needed
	if len(metricsEndpoint) > 8 {
		// Find the base URL (everything before the last path component)
		// This is a simple implementation; you might want to use url.Parse for production
		base := metricsEndpoint

		// Remove /metrics if present
		if len(base) > 8 && base[len(base)-8:] == "/metrics" {
			base = base[:len(base)-8]
		}

		return base + "/agent/version"
	}

	// Fallback to default
	return "https://api.nodepulse.io/agent/version"
}
