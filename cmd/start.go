package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/node-pulse/agent/internal/config"
	"github.com/node-pulse/agent/internal/exporters"
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

	// Create exporter registry
	registry := exporters.NewRegistry()

	// Register built-in exporters
	registry.Register(exporters.NewNodeExporter("", 0))
	// Future: register other exporters here
	// registry.Register(exporters.NewPostgresExporter("", 0))
	// registry.Register(exporters.NewMysqlExporter("", 0))

	// Initialize enabled exporters from config
	activeExporters := []exporters.Exporter{}
	for _, exporterCfg := range cfg.Exporters {
		if !exporterCfg.Enabled {
			continue
		}

		// Create exporter instance with configured endpoint and timeout
		var exp exporters.Exporter
		switch exporterCfg.Name {
		case "node_exporter":
			exp = exporters.NewNodeExporter(exporterCfg.Endpoint, exporterCfg.Timeout)
		default:
			logger.Warn("Unknown exporter type, skipping", logger.String("name", exporterCfg.Name))
			continue
		}

		// Verify exporter is accessible
		if err := exp.Verify(); err != nil {
			logger.Warn("Exporter verification failed, skipping",
				logger.String("name", exporterCfg.Name),
				logger.String("endpoint", exporterCfg.Endpoint),
				logger.Err(err))
			continue
		}

		activeExporters = append(activeExporters, exp)
		logger.Info("Exporter initialized",
			logger.String("name", exporterCfg.Name),
			logger.String("endpoint", exporterCfg.Endpoint))
	}

	if len(activeExporters) == 0 {
		return fmt.Errorf("no active exporters configured - please configure at least one exporter")
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

	// Launch independent scraper goroutine for each exporter (Phase 2)
	var wg sync.WaitGroup

	logger.Info("Agent started",
		logger.String("server_id", cfg.Agent.ServerID),
		logger.Int("exporters", len(activeExporters)),
		logger.String("server_endpoint", cfg.Server.Endpoint))

	for i, exp := range activeExporters {
		exporterCfg := cfg.Exporters[i]
		interval := exporterCfg.ParsedInterval
		timeout := exporterCfg.Timeout

		wg.Add(1)
		go func(exporter exporters.Exporter, scrapeInterval time.Duration, scrapeTimeout time.Duration) {
			defer wg.Done()
			runScraperLoop(ctx, exporter, sender, cfg.Agent.ServerID, scrapeInterval, scrapeTimeout)
		}(exp, interval, timeout)

		logger.Info("Started scraper loop",
			logger.String("exporter", exp.Name()),
			logger.Duration("interval", interval),
			logger.Duration("timeout", timeout))
	}

	// Wait for shutdown signal
	<-ctx.Done()

	// Wait for all scraper goroutines to finish
	logger.Info("Waiting for all scrapers to stop...")
	wg.Wait()

	logger.Info("All scrapers stopped, agent shutdown complete")
	return nil
}

// runScraperLoop runs an independent scrape loop for a single exporter
// Each exporter has its own ticker and runs at its configured interval
func runScraperLoop(ctx context.Context, exporter exporters.Exporter,
	sender *report.Sender, serverID string, interval time.Duration, timeout time.Duration) {

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Scrape immediately on start with aligned timestamp (UTC)
	collectionTime := time.Now().UTC().Truncate(interval)
	scrapeAndBuffer(ctx, exporter, sender, serverID, collectionTime, timeout)

	// Continue with ticker
	for {
		select {
		case <-ctx.Done():
			logger.Info("Scraper loop stopped", logger.String("exporter", exporter.Name()))
			return

		case tickTime := <-ticker.C:
			// Align collection time to interval boundary (UTC)
			collectionTime := tickTime.UTC().Truncate(interval)
			scrapeAndBuffer(ctx, exporter, sender, serverID, collectionTime, timeout)
		}
	}
}

// scrapeAndBuffer performs a single scrape operation for an exporter
func scrapeAndBuffer(ctx context.Context, exporter exporters.Exporter,
	sender *report.Sender, serverID string, collectionTime time.Time, timeout time.Duration) {

	// Create timeout context for scrape
	scrapeCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Scrape metrics
	data, err := exporter.Scrape(scrapeCtx)
	if err != nil {
		logger.Warn("Failed to scrape exporter",
			logger.String("exporter", exporter.Name()),
			logger.Err(err))
		return
	}

	// Add explicit timestamps to metrics (aligned to collection time)
	dataWithTimestamp := prometheus.AddTimestamps(data, collectionTime)

	// Save raw Prometheus text to buffer (WAL pattern)
	if err := sender.BufferPrometheus(dataWithTimestamp, serverID, exporter.Name()); err != nil {
		logger.Error("Failed to buffer metrics",
			logger.String("exporter", exporter.Name()),
			logger.Err(err))
		return
	}

	logger.Debug("Exporter scraped and buffered",
		logger.String("exporter", exporter.Name()),
		logger.Int("bytes", len(dataWithTimestamp)),
		logger.String("collection_time", collectionTime.Format(time.RFC3339)))
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
