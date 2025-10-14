package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/node-pulse/agent/internal/config"
	"github.com/node-pulse/agent/internal/logger"
	"github.com/node-pulse/agent/internal/metrics"
	"github.com/node-pulse/agent/internal/report"
	"github.com/spf13/cobra"
)

// agentCmd represents the agent command
var agentCmd = &cobra.Command{
	Use:   "agent",
	Short: "Run the monitoring agent",
	Long:  `Runs the agent in foreground, collecting and sending metrics at configured intervals.`,
	RunE:  runAgent,
}

func init() {
	rootCmd.AddCommand(agentCmd)
}

func runAgent(cmd *cobra.Command, args []string) error {
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
