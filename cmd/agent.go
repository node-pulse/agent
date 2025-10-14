package cmd

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/node-pulse/agent/internal/config"
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
		log.Println("Shutting down agent...")
		cancel()
	}()

	// Main collection loop
	ticker := time.NewTicker(cfg.Agent.Interval)
	defer ticker.Stop()

	log.Printf("Agent started (interval: %s, endpoint: %s)\n", cfg.Agent.Interval, cfg.Server.Endpoint)

	// Collect and send immediately on start
	if err := collectAndSend(sender); err != nil {
		log.Printf("Error: %v\n", err)
	}

	// Then continue with ticker
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := collectAndSend(sender); err != nil {
				log.Printf("Error: %v\n", err)
			}
		}
	}
}

func collectAndSend(sender *report.Sender) error {
	// Collect metrics
	report, err := metrics.Collect()
	if err != nil {
		return fmt.Errorf("failed to collect metrics: %w", err)
	}

	// Send report
	if err := sender.Send(report); err != nil {
		return fmt.Errorf("failed to send report: %w", err)
	}

	log.Println("Report sent successfully")
	return nil
}
