package cmd

import (
	"fmt"
	"os"
	"syscall"
	"time"

	"github.com/node-pulse/agent/internal/pidfile"
	"github.com/spf13/cobra"
)

// stopCmd represents the stop command
var stopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the daemon agent",
	Long:  `Stops the agent running in background (started with 'pulse start -d').`,
	RunE:  stopAgent,
}

func init() {
	rootCmd.AddCommand(stopCmd)
}

func stopAgent(cmd *cobra.Command, args []string) error {
	// Check if agent is running
	isRunning, pid, err := pidfile.CheckRunning()
	if err != nil {
		return fmt.Errorf("failed to check if agent is running: %w", err)
	}

	if !isRunning {
		fmt.Println("No agent is running")
		return nil
	}

	// Find the process
	process, err := os.FindProcess(pid)
	if err != nil {
		// Clean up stale PID file
		pidfile.RemovePidFile()
		return fmt.Errorf("failed to find process %d: %w", pid, err)
	}

	fmt.Printf("Stopping agent (PID %d)...\n", pid)

	// Send SIGTERM for graceful shutdown
	if err := process.Signal(syscall.SIGTERM); err != nil {
		// Process might already be dead
		pidfile.RemovePidFile()
		return fmt.Errorf("failed to send SIGTERM: %w", err)
	}

	// Wait for process to exit (max 5 seconds)
	for i := 0; i < 50; i++ {
		time.Sleep(100 * time.Millisecond)
		if !pidfile.IsProcessRunning(pid) {
			// Process stopped
			pidfile.RemovePidFile()
			fmt.Println("Agent stopped successfully")
			return nil
		}
	}

	// Process didn't stop, send SIGKILL
	fmt.Println("Agent didn't stop gracefully, forcing shutdown...")
	if err := process.Signal(syscall.SIGKILL); err != nil {
		// Might already be dead
		pidfile.RemovePidFile()
		return fmt.Errorf("failed to send SIGKILL: %w", err)
	}

	// Wait a bit more
	time.Sleep(500 * time.Millisecond)
	pidfile.RemovePidFile()
	fmt.Println("Agent stopped (forced)")

	return nil
}
