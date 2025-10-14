package cmd

import (
	"fmt"

	"github.com/node-pulse/agent/internal/config"
	"github.com/spf13/cobra"
)

// currentServerCmd represents the current-server command
var currentServerCmd = &cobra.Command{
	Use:   "current-server",
	Short: "Display the current server ID",
	Long:  `Shows the UUID that identifies this server in metric reports.`,
	RunE:  runCurrentServer,
}

func init() {
	rootCmd.AddCommand(currentServerCmd)
}

func runCurrentServer(cmd *cobra.Command, args []string) error {
	// Load configuration (this will auto-generate UUID if needed)
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Display server ID
	fmt.Printf("Server ID: %s\n", cfg.Agent.ServerID)

	// Show where it's persisted
	serverIDPath := config.GetServerIDPath()
	fmt.Printf("Persisted at: %s\n", serverIDPath)

	return nil
}
