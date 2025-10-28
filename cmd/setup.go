package cmd

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/node-pulse/agent/internal/installer"
	"github.com/spf13/cobra"
)

var (
	// Config flags
	flagEndpointURL string
	flagServerID    string
)

// setupCmd represents the setup command
var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Set up Node Pulse agent configuration",
	Long: `Set up the Node Pulse agent by creating necessary directories,
generating server ID, and creating configuration file.

For development/testing only. Production deployments use Ansible.`,
	RunE: runSetup,
}

func init() {
	rootCmd.AddCommand(setupCmd)

	// Only two flags needed - everything else uses hardcoded defaults
	setupCmd.Flags().StringVar(&flagEndpointURL, "endpoint-url", "", "Dashboard endpoint URL (required)")
	setupCmd.Flags().StringVar(&flagServerID, "server-id", "", "Server ID (auto-generated UUID if not provided)")
}

func runSetup(cmd *cobra.Command, args []string) error {
	// Validate that endpoint URL is provided
	if flagEndpointURL == "" {
		return fmt.Errorf("--endpoint-url is required")
	}

	// Validate endpoint URL format
	if err := validateEndpointURL(flagEndpointURL); err != nil {
		return fmt.Errorf("invalid endpoint URL: %w", err)
	}

	// Run setup
	fmt.Println("⚡ Node Pulse Agent Setup")
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
		fmt.Println("  Configuration will be overwritten")
		fmt.Println()
	}

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

	// Build config options with hardcoded defaults
	opts := installer.ConfigOptions{
		// Server options
		Endpoint: flagEndpointURL,
		Timeout:  "5s",

		// Agent options
		ServerID: finalServerID,
		Interval: "15s",

		// Buffer options (hardcoded defaults)
		BufferPath:           "/var/lib/nodepulse/buffer",
		BufferRetentionHours: 48,

		// Logging options (hardcoded defaults)
		LogLevel:      "info",
		LogOutput:     "stdout",
		LogFilePath:   "/var/log/nodepulse/agent.log",
		LogMaxSizeMB:  10,
		LogMaxBackups: 3,
		LogMaxAgeDays: 7,
		LogCompress:   true,
	}

	fmt.Println()
	fmt.Printf("Configuration:\n")
	fmt.Printf("  Endpoint:  %s\n", opts.Endpoint)
	fmt.Printf("  Server ID: %s\n", opts.ServerID)
	fmt.Printf("  Interval:  %s (default)\n", opts.Interval)
	fmt.Println()

	// Perform installation
	return performInstallation(opts)
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
	fmt.Printf("Server ID: %s\n", opts.ServerID)
	fmt.Printf("Config:    %s\n", installer.DefaultConfigPath)
	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Println("  1. Start the agent:    nodepulse start")
	fmt.Println("  2. Install service:    sudo nodepulse service install")
	fmt.Println()

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
