package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

var (
	cfgFile string
)

// rootCmd represents the base command
var rootCmd = &cobra.Command{
	Use:   "nodepulse",
	Short: "NodePulse Agent - Prometheus forwarder for server metrics",
	Long: `NodePulse Agent scrapes Prometheus metrics from node_exporter and forwards them to a central dashboard.

When called without a subcommand, it runs in foreground mode (equivalent to 'nodepulse start').`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// If no subcommand provided, default to 'start' command
		// This allows systemd to call: /opt/nodepulse/nodepulse --config /path
		return runAgent(cmd, args)
	},
}

// Execute runs the root command
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		// Cobra already printed the error and usage, just exit with code 1
		os.Exit(1)
	}
}

func init() {
	// Global flags
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default: /etc/nodepulse/nodepulse.yml)")
}
