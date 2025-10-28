package cmd

import (
	"bufio"
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/node-pulse/agent/internal/installer"
	"github.com/spf13/cobra"
)

var (
	quickMode bool

	// Config flags for quick mode
	flagEndpointURL     string
	flagServerID        string
	flagInterval        string
	flagTimeout         string
	flagBufferDir       string
	flagBufferRetention int
	flagLogLevel        string
	flagLogOutput       string
	flagLogFile         string
	flagLogMaxSize      int
	flagLogMaxBackups   int
	flagLogMaxAge       int
	flagLogCompress     bool
)

// setupCmd represents the setup command
var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Set up Node Pulse agent configuration",
	Long: `Set up the Node Pulse agent by creating necessary directories,
generating server ID, and creating configuration file.

Run interactively with the full setup wizard, or use --yes for quick mode
with minimal prompts.`,
	RunE: runSetup,
}

func init() {
	rootCmd.AddCommand(setupCmd)
	setupCmd.Flags().BoolVarP(&quickMode, "yes", "y", false, "Quick mode - non-interactive setup with flags")

	// Server configuration flags
	setupCmd.Flags().StringVar(&flagEndpointURL, "endpoint-url", "", "Metrics endpoint URL (required with --yes)")
	setupCmd.Flags().StringVar(&flagTimeout, "timeout", "3s", "HTTP request timeout")

	// Agent configuration flags
	setupCmd.Flags().StringVar(&flagServerID, "server-id", "", "Server ID (auto-generated UUID if not provided)")
	setupCmd.Flags().StringVar(&flagInterval, "interval", "5s", "Metric collection interval (5s, 10s, 30s, or 1m)")

	// Buffer configuration flags (buffer is always enabled)
	setupCmd.Flags().StringVar(&flagBufferDir, "buffer-dir", "/var/lib/node-pulse/buffer", "Buffer directory path")
	setupCmd.Flags().IntVar(&flagBufferRetention, "buffer-retention", 48, "Buffer retention in hours")

	// Logging configuration flags
	setupCmd.Flags().StringVar(&flagLogLevel, "log-level", "info", "Log level (debug, info, warn, error)")
	setupCmd.Flags().StringVar(&flagLogOutput, "log-output", "stdout", "Log output destination (stdout, file, both)")
	setupCmd.Flags().StringVar(&flagLogFile, "log-file", "/var/log/node-pulse/agent.log", "Log file path")
	setupCmd.Flags().IntVar(&flagLogMaxSize, "log-max-size", 10, "Max log file size in MB")
	setupCmd.Flags().IntVar(&flagLogMaxBackups, "log-max-backups", 3, "Max number of old log files to keep")
	setupCmd.Flags().IntVar(&flagLogMaxAge, "log-max-age", 7, "Max age in days to keep old log files")
	setupCmd.Flags().BoolVar(&flagLogCompress, "log-compress", true, "Compress old log files")
}

func runSetup(cmd *cobra.Command, args []string) error {
	// Run appropriate mode
	if quickMode {
		// Validate that endpoint URL is provided in quick mode
		if flagEndpointURL == "" {
			return fmt.Errorf("--endpoint-url is required when using --yes flag")
		}

		// Validate endpoint URL format
		if err := validateEndpointURL(flagEndpointURL); err != nil {
			return fmt.Errorf("invalid endpoint URL: %w", err)
		}

		// Quick mode: run checks before installation
		fmt.Println("⚡ Node Pulse Agent Setup (Quick Mode)")
		fmt.Println()

		// Check permissions
		fmt.Print("Checking permissions... ")
		if err := installer.CheckPermissions(); err != nil {
			fmt.Println("✗")
			return err
		}
		fmt.Println("✓")

		// Detect existing installation
		existing, err := installer.DetectExisting()
		if err != nil {
			return fmt.Errorf("failed to detect existing installation: %w", err)
		}

		// Handle existing installation
		if existing.HasConfig || existing.HasServerID {
			fmt.Println("\n⚠ Existing installation detected")
			if existing.HasConfig {
				fmt.Printf("  Config: %s\n", existing.ConfigPath)
			}
			if existing.HasServerID {
				fmt.Printf("  Server ID: %s\n", strings.TrimSpace(existing.ServerID))
			}
			fmt.Println("  Configuration will be overwritten with provided flags")
			fmt.Println()
		}

		return runQuickMode(existing)
	}

	// Interactive mode: TUI handles all checks
	return runInteractive()
}

func runQuickMode(existing *installer.ExistingInstall) error {
	fmt.Println("🚀 Building Configuration from Flags")
	fmt.Println()

	// Handle server ID
	var finalServerID string
	if flagServerID != "" {
		// Use provided server ID
		if err := installer.ValidateServerID(flagServerID); err != nil {
			return fmt.Errorf("invalid server ID: %w", err)
		}
		finalServerID = flagServerID
		fmt.Printf("Using provided server ID: %s\n", finalServerID)
	} else if existing.HasServerID {
		// Keep existing server ID
		finalServerID = strings.TrimSpace(existing.ServerID)
		fmt.Printf("Keeping existing server ID: %s\n", finalServerID)
	} else {
		// Auto-generate UUID
		fmt.Print("Generating server ID... ")
		var err error
		finalServerID, err = installer.HandleServerID("")
		if err != nil {
			fmt.Println("✗")
			return err
		}
		fmt.Printf("✓\n  %s\n", finalServerID)
	}

	// Build config options from flags
	opts := installer.ConfigOptions{
		// Server options
		Endpoint: flagEndpointURL,
		Timeout:  flagTimeout,

		// Agent options
		ServerID: finalServerID,
		Interval: flagInterval,

		// Buffer options (always enabled)
		BufferPath:           flagBufferDir,
		BufferRetentionHours: flagBufferRetention,

		// Logging options
		LogLevel:      flagLogLevel,
		LogOutput:     flagLogOutput,
		LogFilePath:   flagLogFile,
		LogMaxSizeMB:  flagLogMaxSize,
		LogMaxBackups: flagLogMaxBackups,
		LogMaxAgeDays: flagLogMaxAge,
		LogCompress:   flagLogCompress,
	}

	fmt.Println()
	fmt.Printf("Configuration summary:\n")
	fmt.Printf("  Endpoint:     %s\n", opts.Endpoint)
	fmt.Printf("  Server ID:    %s\n", opts.ServerID)
	fmt.Printf("  Interval:     %s\n", opts.Interval)
	fmt.Printf("  Timeout:      %s\n", opts.Timeout)
	fmt.Printf("  Buffer path:  %s\n", opts.BufferPath)
	fmt.Printf("  Log level:    %s\n", opts.LogLevel)
	fmt.Println()

	// Perform installation
	return performInstallation(opts)
}

func runInteractive() error {
	// Interactive mode removed in v2.0
	// Users should use quick mode with --yes flag and provide configuration flags
	fmt.Println("❌ Interactive TUI mode has been removed in Node Pulse Agent v2.0")
	fmt.Println()
	fmt.Println("Please use quick mode instead:")
	fmt.Println("  pulse setup --yes --endpoint-url <url> --server-id <uuid>")
	fmt.Println()
	fmt.Println("Example:")
	fmt.Println("  pulse setup --yes --endpoint-url https://dashboard.nodepulse.io/metrics/prometheus --server-id 550e8400-e29b-41d4-a716-446655440000")
	fmt.Println()
	fmt.Println("Or manually create /etc/node-pulse/nodepulse.yml")
	return fmt.Errorf("interactive mode not available")
}

func performInstallation(opts installer.ConfigOptions) error {
	fmt.Println()
	fmt.Println("Installing...")
	fmt.Println()

	// Create directories
	fmt.Print("Creating directories... ")
	if err := installer.CreateDirectories(); err != nil {
		fmt.Println("✗")
		return err
	}
	fmt.Println("✓")

	// Persist server ID
	fmt.Print("Persisting server ID... ")
	if err := installer.PersistServerID(opts.ServerID); err != nil {
		fmt.Println("✗")
		return err
	}
	fmt.Println("✓")

	// Write config file
	fmt.Print("Writing configuration file... ")
	if err := installer.WriteConfigFile(opts); err != nil {
		fmt.Println("✗")
		return err
	}
	fmt.Println("✓")

	// Fix permissions
	fmt.Print("Setting permissions... ")
	if err := installer.FixPermissions(); err != nil {
		fmt.Println("✗")
		return err
	}
	fmt.Println("✓")

	// Validate installation
	fmt.Print("Validating installation... ")
	if err := installer.ValidateInstallation(); err != nil {
		fmt.Println("✗")
		return err
	}
	fmt.Println("✓")

	// Success
	fmt.Println()
	fmt.Println("✓ Node Pulse agent set up successfully!")
	fmt.Println()
	fmt.Println("Server ID:")
	fmt.Printf("  %s\n", opts.ServerID)
	fmt.Println("Config:")
	fmt.Printf("  %s\n", installer.DefaultConfigPath)
	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Println("  1. Start the agent:    pulse start")
	fmt.Println("  2. Install service:    sudo pulse service install")
	fmt.Println()

	// Ask about service installation
	if promptYesNo("Install as systemd service now?", false) {
		fmt.Println()
		// Run service install command
		serviceCmd := rootCmd.Commands()[0] // Get first command (should be service)
		for _, cmd := range rootCmd.Commands() {
			if cmd.Name() == "service" {
				serviceCmd = cmd
				break
			}
		}

		installCmd := serviceCmd.Commands()[0]
		for _, cmd := range serviceCmd.Commands() {
			if cmd.Name() == "install" {
				installCmd = cmd
				break
			}
		}

		if err := installCmd.RunE(installCmd, []string{}); err != nil {
			fmt.Println("Failed to install service:")
			fmt.Printf("  %v\n", err)
		}
	}

	return nil
}

// validateEndpointURL validates endpoint URL format
func validateEndpointURL(endpointURL string) error {
	// Parse and validate URL
	parsedURL, err := url.Parse(endpointURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %v", err)
	}
	// Check scheme is http or https
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return fmt.Errorf("URL must use http:// or https:// scheme")
	}
	// Check host is not empty
	if parsedURL.Host == "" {
		return fmt.Errorf("URL must include a valid host")
	}
	return nil
}

// promptYesNo prompts for a yes/no answer
func promptYesNo(prompt string, defaultYes bool) bool {
	reader := bufio.NewReader(os.Stdin)

	suffix := " [y/N]: "
	if defaultYes {
		suffix = " [Y/n]: "
	}

	for {
		fmt.Print(prompt + suffix)

		input, err := reader.ReadString('\n')
		if err != nil {
			return defaultYes
		}

		input = strings.TrimSpace(strings.ToLower(input))

		if input == "" {
			return defaultYes
		}

		if input == "y" || input == "yes" {
			return true
		}
		if input == "n" || input == "no" {
			return false
		}

		fmt.Println("Please enter 'y' or 'n'")
	}
}
