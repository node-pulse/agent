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
	"github.com/node-pulse/agent/internal/pidfile"
	"github.com/node-pulse/agent/internal/prometheus"
	"github.com/node-pulse/agent/internal/report"
	"github.com/spf13/cobra"
)

var daemonFlag bool

// startCmd represents the start command
var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Run the Prometheus forwarding agent",
	Long:  `Scrapes node_exporter on localhost:9100 and forwards Prometheus metrics to the dashboard.`,
	RunE:  runAgent,
}

func init() {
	rootCmd.AddCommand(startCmd)
	startCmd.Flags().BoolVarP(&daemonFlag, "daemon", "d", false, "Run in background (for development/debugging only)")
}

func runAgent(cmd *cobra.Command, args []string) error {
	// Check config exists before doing anything
	if err := config.RequireConfig(cfgFile); err != nil {
		return err
	}

	// Handle daemon mode
	if daemonFlag {
		return runInBackground()
	}

	// Check if running under systemd (systemd sets INVOCATION_ID for all services)
	isSystemdManaged := os.Getenv("INVOCATION_ID") != ""

	// Only manage PID file if NOT running under systemd
	if !isSystemdManaged {
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
	}

	// Load configuration
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Initialize logger
	if err := logger.Initialize(cfg.Logging); err != nil {
		return fmt.Errorf("failed to initialize logger: %w", err)
	}
	defer func() {
		if err := logger.Sync(); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to flush logs: %v\n", err)
		}
	}()

	// Create Prometheus scraper
	scraper := prometheus.NewScraper(&prometheus.ScraperConfig{
		Endpoint: cfg.Prometheus.Endpoint,
		Timeout:  cfg.Prometheus.Timeout,
	})

	// Verify node_exporter is accessible on startup
	if err := scraper.Verify(); err != nil {
		logger.Error("Failed to verify Prometheus exporter - is node_exporter running?", logger.Err(err))
		return fmt.Errorf("prometheus exporter verification failed: %w\nPlease ensure node_exporter is running on %s", err, cfg.Prometheus.Endpoint)
	}

	// Create report sender
	sender, err := report.NewSender(cfg)
	if err != nil {
		return fmt.Errorf("failed to create sender: %w", err)
	}
	defer sender.Close()

	// Start background draining goroutine (WAL pattern)
	sender.StartDraining()

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

	// Main scraping loop
	// Use ticker for interval, but align collection timestamps to interval boundaries
	ticker := time.NewTicker(cfg.Agent.Interval)
	defer ticker.Stop()

	logger.Info("Agent started",
		logger.String("server_id", cfg.Agent.ServerID),
		logger.Duration("interval", cfg.Agent.Interval),
		logger.String("prometheus_endpoint", cfg.Prometheus.Endpoint),
		logger.String("server_endpoint", cfg.Server.Endpoint))

	// Scrape immediately on start with aligned timestamp
	collectionTime := time.Now().Truncate(cfg.Agent.Interval)
	if err := scrapeAndSendWithTimestamp(scraper, sender, cfg.Agent.ServerID, collectionTime); err != nil {
		logger.Error("Initial scrape failed", logger.Err(err))
	}

	// Continue with ticker
	for {
		select {
		case <-ctx.Done():
			return nil
		case tickTime := <-ticker.C:
			// Align collection time to interval boundary
			collectionTime := tickTime.Truncate(cfg.Agent.Interval)
			if err := scrapeAndSendWithTimestamp(scraper, sender, cfg.Agent.ServerID, collectionTime); err != nil {
				logger.Error("Scrape failed", logger.Err(err))
			}
		}
	}
}

// scrapeAndSendWithTimestamp scrapes metrics and adds aligned collection timestamp
func scrapeAndSendWithTimestamp(scraper *prometheus.Scraper, sender *report.Sender, serverID string, collectionTime time.Time) error {
	// Scrape Prometheus exporter
	data, err := scraper.Scrape()
	if err != nil {
		return fmt.Errorf("failed to scrape prometheus: %w", err)
	}

	// Add explicit timestamps to metrics (aligned to collection time)
	// This ensures all agents report metrics at the same logical time boundaries
	dataWithTimestamp := prometheus.AddTimestamps(data, collectionTime)

	// Save to buffer (WAL pattern - actual sending happens in background)
	if err := sender.SendPrometheus(dataWithTimestamp, serverID); err != nil {
		return fmt.Errorf("failed to buffer prometheus data: %w", err)
	}

	logger.Debug("Prometheus data scraped and buffered",
		logger.Int("bytes", len(dataWithTimestamp)),
		logger.Time("collection_time", collectionTime))
	return nil
}

// Legacy function kept for backwards compatibility
func scrapeAndSend(scraper *prometheus.Scraper, sender *report.Sender, serverID string) error {
	return scrapeAndSendWithTimestamp(scraper, sender, serverID, time.Now().Truncate(5*time.Second))
}

func runInBackground() error {
	// Check config exists
	if err := config.RequireConfig(cfgFile); err != nil {
		return err
	}

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
	fmt.Println("  nodepulse service install")
	fmt.Println("  nodepulse service start")
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
