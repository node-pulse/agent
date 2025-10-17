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
	Use:   "pulse",
	Short: "NodePulse Agent - Monitor Linux server metrics",
	Long: `NodePulse Agent monitors Linux server health metrics including CPU,
memory, network I/O, and uptime, reporting them to a central server.`,
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
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default: /etc/node-pulse/nodepulse.yml)")
}
