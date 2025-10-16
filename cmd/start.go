package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"github.com/node-pulse/agent/internal/config"
	"github.com/node-pulse/agent/internal/logger"
	"github.com/node-pulse/agent/internal/metrics"
	"github.com/node-pulse/agent/internal/pidfile"
	"github.com/node-pulse/agent/internal/report"
	"github.com/spf13/cobra"
)

var daemonFlag bool

// startCmd represents the start command
var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Run the monitoring agent",
	Long:  `Runs the agent in foreground, collecting and sending metrics at configured intervals.`,
	RunE:  runAgent,
}

func init() {
	rootCmd.AddCommand(startCmd)
	startCmd.Flags().BoolVarP(&daemonFlag, "daemon", "d", false, "Run in background (for development/debugging only)")
}

func runAgent(cmd *cobra.Command, args []string) error {
	// Handle daemon mode
	if daemonFlag {
		return runInBackground()
	}

	// Check if agent is already running
	isRunning, existingPid, err := pidfile.CheckRunning()
	if err != nil {
		return fmt.Errorf("failed to check if agent is running: %w", err)
	}
	if isRunning {
		return fmt.Errorf("agent is already running with PID %d", existingPid)
	}

	// Write PID file for this process
	if err := pidfile.WritePidFile(os.Getpid()); err != nil {
		return fmt.Errorf("failed to write PID file: %w", err)
	}
	defer pidfile.RemovePidFile()

	// Load configuration
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Initialize logger
	if err := logger.Initialize(cfg.Logging); err != nil {
		return fmt.Errorf("failed to initialize logger: %w", err)
	}
	defer logger.Sync()

	// Create report sender
	sender, err := report.NewSender(cfg)
	if err != nil {
		return fmt.Errorf("failed to create sender: %w", err)
	}
	defer sender.Close()

	// Setup signal handling for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigChan
		logger.Info("Shutting down agent...")
		cancel()
	}()

	// Main collection loop
	ticker := time.NewTicker(cfg.Agent.Interval)
	defer ticker.Stop()

	logger.Info("Agent started",
		logger.String("server_id", cfg.Agent.ServerID),
		logger.Duration("interval", cfg.Agent.Interval),
		logger.String("endpoint", cfg.Server.Endpoint))

	// Collect and send immediately on start
	if err := collectAndSend(sender, cfg.Agent.ServerID); err != nil {
		logger.Error("Collection and send failed", logger.Err(err))
	}

	// Then continue with ticker
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := collectAndSend(sender, cfg.Agent.ServerID); err != nil {
				logger.Error("Collection and send failed", logger.Err(err))
			}
		}
	}
}

func collectAndSend(sender *report.Sender, serverID string) error {
	// Collect metrics
	metricsReport, err := metrics.Collect(serverID)
	if err != nil {
		return fmt.Errorf("failed to collect metrics: %w", err)
	}

	// Record collection in stats
	stats := metrics.GetGlobalStats()
	stats.RecordCollection(metricsReport)

	// Send report
	if err := sender.Send(metricsReport); err != nil {
		// Record failure
		stats.RecordFailure()
		return fmt.Errorf("failed to send report: %w", err)
	}

	// Record success
	stats.RecordSuccess()
	logger.Info("Report sent successfully")
	return nil
}

func runInBackground() error {
	// Check if agent is already running
	isRunning, existingPid, err := pidfile.CheckRunning()
	if err != nil {
		return fmt.Errorf("failed to check if agent is running: %w", err)
	}
	if isRunning {
		fmt.Printf("Agent is already running with PID %d\n", existingPid)
		return nil
	}

	// Print warning
	fmt.Println("WARNING: Running in daemon mode (-d) is for development and debugging only.")
	fmt.Println("For production use, install as a systemd service:")
	fmt.Println("  pulse service install")
	fmt.Println("  pulse service start")
	fmt.Println()

	// Build command arguments without the daemon flag
	args := []string{"start"}

	// Add config flag if it was provided
	if cfgFile != "" {
		args = append(args, "--config", cfgFile)
	}

	// Get the current executable path
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}

	// Create the command
	cmd := exec.Command(executable, args...)

	// Detach from parent process
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid: true,
	}

	// Start the process
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start background process: %w", err)
	}

	fmt.Printf("Agent started in background with PID %d\n", cmd.Process.Pid)
	fmt.Println("To stop: pulse stop")

	return nil
}
