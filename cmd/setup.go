package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/node-pulse/agent/internal/installer"
	"github.com/spf13/cobra"
)

var (
	quickMode bool
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
	setupCmd.Flags().BoolVarP(&quickMode, "yes", "y", false, "Quick mode - minimal prompts, use defaults")
}

func runSetup(cmd *cobra.Command, args []string) error {
	// Run appropriate mode
	if quickMode {
		// Quick mode: run checks before prompts
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
			fmt.Println()

			if !promptYesNo("Continue and update configuration?", true) {
				fmt.Println("\nInstallation cancelled")
				return nil
			}
			fmt.Println()
		}

		return runQuickMode(existing)
	}

	// Interactive mode: TUI handles all checks
	return runInteractive()
}

func runQuickMode(existing *installer.ExistingInstall) error {
	fmt.Println("🚀 Quick Mode Setup")
	fmt.Println()

	// Prompt for endpoint
	defaultEndpoint := ""
	endpointPrompt := "Enter endpoint URL"
	if existing.Endpoint != "" {
		defaultEndpoint = strings.TrimSpace(existing.Endpoint)
		fmt.Printf("Existing endpoint: %s\n", defaultEndpoint)
		endpointPrompt = "Enter endpoint URL (leave empty to keep existing)"
	}

	endpoint, err := promptString(endpointPrompt, "", func(s string) error {
		if s == "" && defaultEndpoint == "" {
			return fmt.Errorf("endpoint is required")
		}
		if s != "" && !strings.HasPrefix(s, "http://") && !strings.HasPrefix(s, "https://") {
			return fmt.Errorf("endpoint must start with http:// or https://")
		}
		return nil
	})
	if err != nil {
		return err
	}

	// Use existing endpoint if user pressed Enter
	if endpoint == "" && defaultEndpoint != "" {
		endpoint = defaultEndpoint
		fmt.Println("Keeping existing endpoint")
	}

	// Prompt for server ID
	defaultServerID := ""
	serverIDPrompt := "Enter server ID (leave empty to auto-generate UUID)"
	if existing.HasServerID {
		defaultServerID = strings.TrimSpace(existing.ServerID)
		fmt.Printf("Existing server ID: %s\n", defaultServerID)
		serverIDPrompt = "Enter server ID (leave empty to keep existing)"
	}

	serverID, err := promptString(serverIDPrompt, "", func(s string) error {
		if s == "" {
			return nil // Empty is OK, will auto-generate or use existing
		}
		return installer.ValidateServerID(s)
	})
	if err != nil {
		return err
	}

	// Handle server ID
	var finalServerID string
	if serverID == "" {
		if existing.HasServerID {
			finalServerID = defaultServerID
			fmt.Println("Keeping existing server ID")
		} else {
			fmt.Print("Generating server ID... ")
			finalServerID, err = installer.HandleServerID("")
			if err != nil {
				fmt.Println("✗")
				return err
			}
			fmt.Printf("✓\n  %s\n", finalServerID)
		}
	} else {
		finalServerID = serverID
		fmt.Println("Server ID set")
	}

	// Create config options with defaults
	opts := installer.DefaultConfigOptions()
	opts.Endpoint = endpoint
	opts.ServerID = finalServerID

	// Perform installation
	return performInstallation(opts)
}

func runInteractive() error {
	// Run TUI wizard - it handles all checks internally
	p := tea.NewProgram(
		newSetupTUIModel(),
		tea.WithAltScreen(),
	)

	if _, err := p.Run(); err != nil {
		fmt.Println("TUI error:")
		return err
	}

	return nil
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
	fmt.Println("  2. Watch live metrics: pulse watch")
	fmt.Println("  3. Install service:    sudo pulse service install")
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

// promptString prompts for a string input with validation
func promptString(prompt, defaultVal string, validate func(string) error) (string, error) {
	reader := bufio.NewReader(os.Stdin)

	for {
		if defaultVal != "" {
			fmt.Printf("%s [%s]: ", prompt, defaultVal)
		} else {
			fmt.Printf("%s: ", prompt)
		}

		input, err := reader.ReadString('\n')
		if err != nil {
			fmt.Println("Failed to read input:")
			return "", err
		}

		input = strings.TrimSpace(input)

		// Use default if empty and default exists
		if input == "" && defaultVal != "" {
			input = defaultVal
		}

		// Validate
		if validate != nil {
			if err := validate(input); err != nil {
				fmt.Printf("❌ %v\n", err)
				continue
			}
		}

		return input, nil
	}
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
