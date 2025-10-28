package installer

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/node-pulse/agent/internal/config"
	"gopkg.in/yaml.v3"
)

const (
	DefaultConfigPath      = "/etc/nodepulse/nodepulse.yml"
	DefaultServerIDPath    = "/var/lib/nodepulse/server_id"
	DefaultBufferPath      = "/var/lib/nodepulse/buffer"
	DefaultConfigDir       = "/etc/nodepulse"
	DefaultStateDir        = "/var/lib/nodepulse"
)

// InstallConfig holds the configuration for installation
type InstallConfig struct {
	Endpoint string // Required: metrics endpoint URL
	ServerID string // Optional: custom ID (alphanumeric + dashes) or empty to auto-generate UUID
}

// ConfigOptions holds all configurable options for the config file
type ConfigOptions struct {
	// Server options
	Endpoint string
	Timeout  string

	// Agent options
	ServerID string
	Interval string

	// Buffer options (buffer is always enabled in new architecture)
	BufferPath           string
	BufferRetentionHours int
	BufferBatchSize      int

	// Logging options
	LogLevel      string
	LogOutput     string
	LogFilePath   string
	LogMaxSizeMB  int
	LogMaxBackups int
	LogMaxAgeDays int
	LogCompress   bool
}

// ExistingInstall represents an existing installation
type ExistingInstall struct {
	HasConfig   bool
	HasServerID bool
	ServerID    string
	ConfigPath  string
	Endpoint    string // Existing endpoint from config file
}

// CheckPermissions verifies the user has sufficient permissions
func CheckPermissions() error {
	// Check if running as root
	if os.Geteuid() != 0 {
		return fmt.Errorf("this command requires root privileges. Please run with sudo")
	}

	// Check write access to /etc
	if err := checkWritable("/etc"); err != nil {
		return fmt.Errorf("no write access to /etc: %w", err)
	}

	// Check write access to /var/lib
	if err := checkWritable("/var/lib"); err != nil {
		return fmt.Errorf("no write access to /var/lib: %w", err)
	}

	return nil
}

// checkWritable tests if a directory is writable
func checkWritable(dir string) error {
	testFile := filepath.Join(dir, ".pulse-write-test")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		return err
	}
	os.Remove(testFile)
	return nil
}

// DetectExisting checks for existing installation
func DetectExisting() (*ExistingInstall, error) {
	existing := &ExistingInstall{
		ConfigPath: DefaultConfigPath,
	}

	// Check for existing config
	if _, err := os.Stat(DefaultConfigPath); err == nil {
		existing.HasConfig = true

		// Try to read the endpoint from existing config
		if cfg, err := config.Load(DefaultConfigPath); err == nil {
			existing.Endpoint = cfg.Server.Endpoint
		}
	}

	// Check for existing server_id
	serverIDPath := config.GetServerIDPath()
	if data, err := os.ReadFile(serverIDPath); err == nil {
		existing.HasServerID = true
		existing.ServerID = string(data)
	}

	return existing, nil
}

// CreateDirectories creates necessary directories
func CreateDirectories() error {
	dirs := []string{
		DefaultConfigDir,
		DefaultStateDir,
		DefaultBufferPath,
	}

	for _, dir := range dirs {
		// Check if already exists
		if info, err := os.Stat(dir); err == nil {
			if !info.IsDir() {
				return fmt.Errorf("%s exists but is not a directory", dir)
			}
			// Directory exists, verify/fix permissions
			if err := os.Chmod(dir, 0755); err != nil {
				return fmt.Errorf("failed to fix permissions on %s: %w", dir, err)
			}
			continue
		}

		// Create directory
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	return nil
}

// HandleServerID validates custom ID or generates UUID
func HandleServerID(customID string) (string, error) {
	// If empty, generate UUID
	if customID == "" {
		return config.GenerateUUID()
	}

	// Validate custom ID
	if err := ValidateServerID(customID); err != nil {
		return "", err
	}

	return customID, nil
}

// ValidateServerID validates server ID format
// Pattern: ^[a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?$
func ValidateServerID(id string) error {
	if len(id) == 0 {
		return fmt.Errorf("server ID cannot be empty")
	}

	// Single character - must be alphanumeric
	if len(id) == 1 {
		if !isAlphanumeric(rune(id[0])) {
			return fmt.Errorf("server ID must be alphanumeric")
		}
		return nil
	}

	// Check first character: must be alphanumeric
	if !isAlphanumeric(rune(id[0])) {
		return fmt.Errorf("server ID must start with a letter or number")
	}

	// Check last character: must be alphanumeric
	if !isAlphanumeric(rune(id[len(id)-1])) {
		return fmt.Errorf("server ID must end with a letter or number")
	}

	// Check all characters: only alphanumeric and dash allowed
	for _, c := range id {
		if !isAlphanumeric(c) && c != '-' {
			return fmt.Errorf("server ID can only contain letters, numbers, and dashes (invalid character: %c)", c)
		}
	}

	return nil
}

// isAlphanumeric checks if a character is alphanumeric
func isAlphanumeric(c rune) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}

// DefaultConfigOptions returns default configuration options
func DefaultConfigOptions() ConfigOptions {
	return ConfigOptions{
		// Server defaults
		Timeout: "3s",

		// Agent defaults
		Interval: "5s",

		// Buffer defaults (always enabled)
		BufferPath:           DefaultBufferPath,
		BufferRetentionHours: 48,
		BufferBatchSize:      5,

		// Logging defaults
		LogLevel:      "info",
		LogOutput:     "stdout",
		LogFilePath:   "/var/log/nodepulse/agent.log",
		LogMaxSizeMB:  10,
		LogMaxBackups: 3,
		LogMaxAgeDays: 7,
		LogCompress:   true,
	}
}

// WriteConfigFile writes the configuration file
func WriteConfigFile(opts ConfigOptions) error {
	// Create config structure
	configData := map[string]interface{}{
		"server": map[string]interface{}{
			"endpoint": opts.Endpoint,
			"timeout":  opts.Timeout,
		},
		"agent": map[string]interface{}{
			"server_id": opts.ServerID,
			"interval":  opts.Interval,
		},
		"buffer": map[string]interface{}{
			"path":            opts.BufferPath,
			"retention_hours": opts.BufferRetentionHours,
			"batch_size":      opts.BufferBatchSize,
		},
		"logging": map[string]interface{}{
			"level":  opts.LogLevel,
			"output": opts.LogOutput,
			"file": map[string]interface{}{
				"path":          opts.LogFilePath,
				"max_size_mb":   opts.LogMaxSizeMB,
				"max_backups":   opts.LogMaxBackups,
				"max_age_days":  opts.LogMaxAgeDays,
				"compress":      opts.LogCompress,
			},
		},
	}

	// Marshal to YAML
	data, err := yaml.Marshal(configData)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// Write to file
	if err := os.WriteFile(DefaultConfigPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// PersistServerID saves server ID to file
func PersistServerID(serverID string) error {
	serverIDPath := DefaultServerIDPath

	// Create directory if needed
	dir := filepath.Dir(serverIDPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Write with restricted permissions
	if err := os.WriteFile(serverIDPath, []byte(serverID+"\n"), 0600); err != nil {
		return fmt.Errorf("failed to write server ID file: %w", err)
	}

	return nil
}

// ValidateInstallation validates the installation
func ValidateInstallation() error {
	// Load config using existing config loader
	cfg, err := config.Load(DefaultConfigPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Verify server ID is set
	if cfg.Agent.ServerID == "" {
		return fmt.Errorf("server ID is not set in config")
	}

	// Verify endpoint is set
	if cfg.Server.Endpoint == "" {
		return fmt.Errorf("endpoint is not set in config")
	}

	return nil
}

// FixPermissions ensures proper permissions on files and directories
func FixPermissions() error {
	// Fix directory permissions
	dirs := map[string]os.FileMode{
		DefaultConfigDir:  0755,
		DefaultStateDir:   0755,
		DefaultBufferPath: 0755,
	}

	for dir, mode := range dirs {
		if _, err := os.Stat(dir); err == nil {
			if err := os.Chmod(dir, mode); err != nil {
				return fmt.Errorf("failed to fix permissions on %s: %w", dir, err)
			}
		}
	}

	// Fix file permissions
	files := map[string]os.FileMode{
		DefaultConfigPath:   0644,
		DefaultServerIDPath: 0600,
	}

	for file, mode := range files {
		if _, err := os.Stat(file); err == nil {
			if err := os.Chmod(file, mode); err != nil {
				return fmt.Errorf("failed to fix permissions on %s: %w", file, err)
			}
		}
	}

	return nil
}
