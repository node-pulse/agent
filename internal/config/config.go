package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/viper"
)

// Config represents the application configuration
type Config struct {
	Server ServerConfig `mapstructure:"server"`
	Agent  AgentConfig  `mapstructure:"agent"`
	Buffer BufferConfig `mapstructure:"buffer"`
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

// BufferConfig represents buffer settings
type BufferConfig struct {
	Enabled        bool   `mapstructure:"enabled"`
	Path           string `mapstructure:"path"`
	RetentionHours int    `mapstructure:"retention_hours"`
}

var (
	defaultConfig = Config{
		Server: ServerConfig{
			Endpoint: "https://api.nodepulse.io/metrics",
			Timeout:  3 * time.Second,
		},
		Agent: AgentConfig{
			Interval: 5 * time.Second,
		},
		Buffer: BufferConfig{
			Enabled:        true,
			Path:           "/var/lib/node-pulse/buffer",
			RetentionHours: 48,
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
		v.AddConfigPath("/etc/node-pulse/")
		v.AddConfigPath("$HOME/.node-pulse/")
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
	v.SetDefault("buffer.enabled", defaultConfig.Buffer.Enabled)
	v.SetDefault("buffer.path", defaultConfig.Buffer.Path)
	v.SetDefault("buffer.retention_hours", defaultConfig.Buffer.RetentionHours)
}

// validate validates the configuration
func validate(cfg *Config) error {
	if cfg.Server.Endpoint == "" {
		return fmt.Errorf("server.endpoint is required")
	}

	if cfg.Server.Timeout <= 0 {
		return fmt.Errorf("server.timeout must be positive")
	}

	// Validate server_id is a valid UUID format
	// Note: EnsureServerID() should have already set this
	if cfg.Agent.ServerID == "" {
		return fmt.Errorf("agent.server_id is missing (this should not happen)")
	}
	if !isValidUUID(cfg.Agent.ServerID) {
		return fmt.Errorf("agent.server_id must be a valid UUID format")
	}

	if cfg.Agent.Interval <= 0 {
		return fmt.Errorf("agent.interval must be positive")
	}

	// Validate allowed intervals
	allowedIntervals := []time.Duration{
		5 * time.Second,
		10 * time.Second,
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
		return fmt.Errorf("agent.interval must be one of: 5s, 10s, 30s, 1m")
	}

	if cfg.Buffer.Enabled {
		if cfg.Buffer.Path == "" {
			return fmt.Errorf("buffer.path is required when buffer is enabled")
		}
		if cfg.Buffer.RetentionHours <= 0 {
			return fmt.Errorf("buffer.retention_hours must be positive")
		}
	}

	return nil
}

// isValidUUID checks if a string is a valid UUID format
func isValidUUID(u string) bool {
	// Basic UUID validation: 8-4-4-4-12 format
	if len(u) != 36 {
		return false
	}
	if u[8] != '-' || u[13] != '-' || u[18] != '-' || u[23] != '-' {
		return false
	}
	for i, c := range u {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			continue
		}
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

// EnsureBufferDir creates the buffer directory if it doesn't exist
func (c *Config) EnsureBufferDir() error {
	if !c.Buffer.Enabled {
		return nil
	}

	if err := os.MkdirAll(c.Buffer.Path, 0755); err != nil {
		return fmt.Errorf("failed to create buffer directory: %w", err)
	}

	return nil
}

// GetBufferFilePath returns the path for the current hour's buffer file
func (c *Config) GetBufferFilePath(t time.Time) string {
	filename := fmt.Sprintf("%s.jsonl", t.Format("2006-01-02-15"))
	return filepath.Join(c.Buffer.Path, filename)
}
