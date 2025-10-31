package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/node-pulse/agent/internal/logger"
	"github.com/spf13/viper"
)

// Config represents the application configuration
type Config struct {
	Server     ServerConfig      `mapstructure:"server"`
	Agent      AgentConfig       `mapstructure:"agent"`
	Exporters  []ExporterConfig  `mapstructure:"exporters"`
	Buffer     BufferConfig      `mapstructure:"buffer"`
	Logging    logger.Config     `mapstructure:"logging"`
	ConfigFile string            `mapstructure:"-"` // Path to the config file that was loaded (not from config)
}

// ServerConfig represents server connection settings
type ServerConfig struct {
	Endpoint string        `mapstructure:"endpoint"`
	Timeout  time.Duration `mapstructure:"timeout"`
}

// AgentConfig represents agent behavior settings
type AgentConfig struct {
	ServerID string        `mapstructure:"server_id"`
	Interval time.Duration `mapstructure:"interval"`
}

// ExporterConfig configures a single Prometheus exporter
type ExporterConfig struct {
	Name     string        `mapstructure:"name"`     // e.g., "node_exporter", "postgres_exporter"
	Enabled  bool          `mapstructure:"enabled"`  // default: true
	Endpoint string        `mapstructure:"endpoint"` // e.g., "http://localhost:9100/metrics"
	Interval string        `mapstructure:"interval"` // e.g., "15s", "30s", "1m" (parsed as time.Duration)
	Timeout  time.Duration `mapstructure:"timeout"`  // default: 3s
}

// BufferConfig represents buffer settings
// Note: Buffer is always enabled in the new architecture (write-ahead log pattern)
type BufferConfig struct {
	Path           string `mapstructure:"path"`
	RetentionHours int    `mapstructure:"retention_hours"`
	BatchSize      int    `mapstructure:"batch_size"` // Number of reports to send per batch (default: 5)
}

var (
	defaultConfig = Config{
		Server: ServerConfig{
			Endpoint: "https://api.nodepulse.io/metrics/prometheus",
			Timeout:  5 * time.Second,
		},
		Agent: AgentConfig{
			Interval: 15 * time.Second, // Prometheus scraping typically 15s-1m
		},
		Buffer: BufferConfig{
			Path:           "/var/lib/nodepulse/buffer",
			RetentionHours: 48,
			BatchSize:      5,
		},
		Logging: logger.Config{
			Level:  "info",
			Output: "stdout",
			File: logger.FileConfig{
				Path:       "/var/log/nodepulse/agent.log",
				MaxSizeMB:  10,
				MaxBackups: 3,
				MaxAgeDays: 7,
				Compress:   true,
			},
		},
	}
)

// Load reads configuration from file
func Load(configPath string) (*Config, error) {
	v := viper.New()

	// Set defaults
	setDefaults(v)

	// If config path provided, use it
	if configPath != "" {
		v.SetConfigFile(configPath)
	} else {
		// Search for config in standard locations
		v.SetConfigName("nodepulse")
		v.SetConfigType("yaml")
		v.AddConfigPath("/etc/nodepulse/")
		v.AddConfigPath("$HOME/.nodepulse/")
		v.AddConfigPath(".")
	}

	// Read config file
	if err := v.ReadInConfig(); err != nil {
		// If config file not found, use defaults
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("failed to read config: %w", err)
		}
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Store which config file was used
	cfg.ConfigFile = v.ConfigFileUsed()

	// Ensure server ID exists (auto-generate if needed)
	if err := EnsureServerID(&cfg); err != nil {
		return nil, fmt.Errorf("failed to ensure server ID: %w", err)
	}

	// Validate config
	if err := validate(&cfg); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return &cfg, nil
}

// setDefaults sets default configuration values
func setDefaults(v *viper.Viper) {
	v.SetDefault("server.endpoint", defaultConfig.Server.Endpoint)
	v.SetDefault("server.timeout", defaultConfig.Server.Timeout)
	v.SetDefault("agent.interval", defaultConfig.Agent.Interval)
	v.SetDefault("buffer.path", defaultConfig.Buffer.Path)
	v.SetDefault("buffer.retention_hours", defaultConfig.Buffer.RetentionHours)
	v.SetDefault("buffer.batch_size", defaultConfig.Buffer.BatchSize)
	v.SetDefault("logging.level", defaultConfig.Logging.Level)
	v.SetDefault("logging.output", defaultConfig.Logging.Output)
	v.SetDefault("logging.file.path", defaultConfig.Logging.File.Path)
	v.SetDefault("logging.file.max_size_mb", defaultConfig.Logging.File.MaxSizeMB)
	v.SetDefault("logging.file.max_backups", defaultConfig.Logging.File.MaxBackups)
	v.SetDefault("logging.file.max_age_days", defaultConfig.Logging.File.MaxAgeDays)
	v.SetDefault("logging.file.compress", defaultConfig.Logging.File.Compress)
}

// validate validates the configuration
func validate(cfg *Config) error {
	if cfg.Server.Endpoint == "" {
		return fmt.Errorf("server.endpoint is required")
	}

	if cfg.Server.Timeout <= 0 {
		return fmt.Errorf("server.timeout must be positive")
	}

	// Validate server_id format
	// Note: EnsureServerID() should have already set this
	if cfg.Agent.ServerID == "" {
		return fmt.Errorf("agent.server_id is missing (this should not happen)")
	}
	if !isValidServerID(cfg.Agent.ServerID) {
		return fmt.Errorf("agent.server_id must contain only letters, numbers, and dashes, and must start and end with a letter or number")
	}

	if cfg.Agent.Interval <= 0 {
		return fmt.Errorf("agent.interval must be positive")
	}

	// Validate allowed intervals (Prometheus scraping typically 15s-1m)
	allowedIntervals := []time.Duration{
		15 * time.Second,
		30 * time.Second,
		1 * time.Minute,
	}

	valid := false
	for _, allowed := range allowedIntervals {
		if cfg.Agent.Interval == allowed {
			valid = true
			break
		}
	}
	if !valid {
		return fmt.Errorf("agent.interval must be one of: 15s, 30s, 1m")
	}

	// Validate exporters config
	if len(cfg.Exporters) == 0 {
		return fmt.Errorf("no exporters configured - please configure at least one exporter in 'exporters' array")
	}

	// Validate each exporter
	for i, e := range cfg.Exporters {
		if e.Name == "" {
			return fmt.Errorf("exporters[%d]: name is required", i)
		}
		if e.Endpoint == "" {
			return fmt.Errorf("exporters[%d] (%s): endpoint is required", i, e.Name)
		}
		if e.Timeout <= 0 {
			return fmt.Errorf("exporters[%d] (%s): timeout must be positive", i, e.Name)
		}

		// Validate interval if specified
		if e.Interval != "" {
			allowedIntervals := []string{"15s", "30s", "1m"}
			valid := false
			for _, allowed := range allowedIntervals {
				if e.Interval == allowed {
					valid = true
					break
				}
			}
			if !valid {
				return fmt.Errorf("exporters[%d] (%s): interval must be one of: 15s, 30s, 1m", i, e.Name)
			}
		}
	}

	// Buffer is always enabled now
	if cfg.Buffer.Path == "" {
		return fmt.Errorf("buffer.path is required")
	}
	if cfg.Buffer.RetentionHours <= 0 {
		return fmt.Errorf("buffer.retention_hours must be positive")
	}
	if cfg.Buffer.BatchSize <= 0 {
		return fmt.Errorf("buffer.batch_size must be positive")
	}

	return nil
}

// isValidServerID checks if a string is a valid server ID format
// Pattern: ^[a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?$
// Must start and end with alphanumeric, can contain dashes in middle
func isValidServerID(id string) bool {
	if len(id) == 0 {
		return false
	}

	// Single character - must be alphanumeric
	if len(id) == 1 {
		return isAlphanumeric(rune(id[0]))
	}

	// Check first character: must be alphanumeric
	if !isAlphanumeric(rune(id[0])) {
		return false
	}

	// Check last character: must be alphanumeric
	if !isAlphanumeric(rune(id[len(id)-1])) {
		return false
	}

	// Check all characters: only alphanumeric and dash allowed
	for _, c := range id {
		if !isAlphanumeric(c) && c != '-' {
			return false
		}
	}

	return true
}

// isAlphanumeric checks if a character is alphanumeric
func isAlphanumeric(c rune) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}

// EnsureBufferDir creates the buffer directory if it doesn't exist
func (c *Config) EnsureBufferDir() error {
	if err := os.MkdirAll(c.Buffer.Path, 0755); err != nil {
		return fmt.Errorf("failed to create buffer directory: %w", err)
	}

	return nil
}

// ConfigExists checks if a configuration file exists in any of the standard locations
func ConfigExists(configPath string) bool {
	// If explicit config path provided, check only that
	if configPath != "" {
		if _, err := os.Stat(configPath); err == nil {
			return true
		}
		return false
	}

	// Check standard locations
	locations := []string{
		"/etc/nodepulse/nodepulse.yml",
		filepath.Join(os.Getenv("HOME"), ".nodepulse", "nodepulse.yml"),
		"nodepulse.yml",
	}

	for _, loc := range locations {
		if _, err := os.Stat(loc); err == nil {
			return true
		}
	}

	return false
}

// RequireConfig checks if config exists and returns a helpful error if not
func RequireConfig(configPath string) error {
	if !ConfigExists(configPath) {
		// Build error message
		msg := "configuration file not found\n\n" +
			"Please run setup first:\n" +
			"  pulse setup\n\n" +
			"Or specify a config file:\n" +
			"  pulse --config /path/to/nodepulse.yml <command>"

		return errors.New(msg)
	}
	return nil
}
