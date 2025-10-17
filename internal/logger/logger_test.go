package logger

import (
	"os"
	"path/filepath"
	"testing"

	"go.uber.org/zap/zapcore"
)

func TestValidateConfig(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{
			name: "valid stdout config",
			cfg: Config{
				Level:  "info",
				Output: "stdout",
			},
			wantErr: false,
		},
		{
			name: "valid console config",
			cfg: Config{
				Level:  "info",
				Output: "console",
			},
			wantErr: false,
		},
		{
			name: "valid file config",
			cfg: Config{
				Level:  "info",
				Output: "file",
				File: FileConfig{
					Path:       "/tmp/test.log",
					MaxSizeMB:  10,
					MaxBackups: 3,
					MaxAgeDays: 7,
					Compress:   true,
				},
			},
			wantErr: false,
		},
		{
			name: "valid both config",
			cfg: Config{
				Level:  "info",
				Output: "both",
				File: FileConfig{
					Path:       "/tmp/test.log",
					MaxSizeMB:  10,
					MaxBackups: 3,
					MaxAgeDays: 7,
					Compress:   true,
				},
			},
			wantErr: false,
		},
		{
			name: "invalid output type",
			cfg: Config{
				Level:  "info",
				Output: "invalid",
			},
			wantErr: true,
		},
		{
			name: "file output with empty path",
			cfg: Config{
				Level:  "info",
				Output: "file",
				File: FileConfig{
					Path:       "",
					MaxSizeMB:  10,
					MaxBackups: 3,
					MaxAgeDays: 7,
				},
			},
			wantErr: true,
		},
		{
			name: "file output with zero max size",
			cfg: Config{
				Level:  "info",
				Output: "file",
				File: FileConfig{
					Path:       "/tmp/test.log",
					MaxSizeMB:  0,
					MaxBackups: 3,
					MaxAgeDays: 7,
				},
			},
			wantErr: true,
		},
		{
			name: "file output with negative max size",
			cfg: Config{
				Level:  "info",
				Output: "file",
				File: FileConfig{
					Path:       "/tmp/test.log",
					MaxSizeMB:  -1,
					MaxBackups: 3,
					MaxAgeDays: 7,
				},
			},
			wantErr: true,
		},
		{
			name: "file output with negative max backups",
			cfg: Config{
				Level:  "info",
				Output: "file",
				File: FileConfig{
					Path:       "/tmp/test.log",
					MaxSizeMB:  10,
					MaxBackups: -1,
					MaxAgeDays: 7,
				},
			},
			wantErr: true,
		},
		{
			name: "file output with negative max age",
			cfg: Config{
				Level:  "info",
				Output: "file",
				File: FileConfig{
					Path:       "/tmp/test.log",
					MaxSizeMB:  10,
					MaxBackups: 3,
					MaxAgeDays: -1,
				},
			},
			wantErr: true,
		},
		{
			name: "file output with zero backups is valid",
			cfg: Config{
				Level:  "info",
				Output: "file",
				File: FileConfig{
					Path:       "/tmp/test.log",
					MaxSizeMB:  10,
					MaxBackups: 0,
					MaxAgeDays: 7,
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateConfig(tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestParseLevel(t *testing.T) {
	tests := []struct {
		name      string
		level     string
		want      zapcore.Level
		wantErr   bool
	}{
		{
			name:    "debug level",
			level:   "debug",
			want:    zapcore.DebugLevel,
			wantErr: false,
		},
		{
			name:    "info level",
			level:   "info",
			want:    zapcore.InfoLevel,
			wantErr: false,
		},
		{
			name:    "warn level",
			level:   "warn",
			want:    zapcore.WarnLevel,
			wantErr: false,
		},
		{
			name:    "warning level (alias)",
			level:   "warning",
			want:    zapcore.WarnLevel,
			wantErr: false,
		},
		{
			name:    "error level",
			level:   "error",
			want:    zapcore.ErrorLevel,
			wantErr: false,
		},
		{
			name:    "invalid level",
			level:   "invalid",
			want:    zapcore.InfoLevel,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseLevel(tt.level)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseLevel() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("parseLevel() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestInitialize(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{
			name: "stdout output",
			cfg: Config{
				Level:  "info",
				Output: "stdout",
			},
			wantErr: false,
		},
		{
			name: "console output",
			cfg: Config{
				Level:  "info",
				Output: "console",
			},
			wantErr: false,
		},
		{
			name: "invalid log level",
			cfg: Config{
				Level:  "invalid",
				Output: "stdout",
			},
			wantErr: true,
		},
		{
			name: "invalid output type",
			cfg: Config{
				Level:  "info",
				Output: "invalid",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Initialize(tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("Initialize() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestInitializeWithFile(t *testing.T) {
	// Create temp directory for test logs
	tempDir := t.TempDir()
	logFile := filepath.Join(tempDir, "test.log")

	cfg := Config{
		Level:  "info",
		Output: "file",
		File: FileConfig{
			Path:       logFile,
			MaxSizeMB:  10,
			MaxBackups: 3,
			MaxAgeDays: 7,
			Compress:   true,
		},
	}

	err := Initialize(cfg)
	if err != nil {
		t.Fatalf("Initialize() failed: %v", err)
	}

	// Write a test log
	Info("test message", String("key", "value"))

	// Sync to ensure log is written
	if err := Sync(); err != nil {
		t.Errorf("Sync() failed: %v", err)
	}

	// Check if log file was created
	if _, err := os.Stat(logFile); os.IsNotExist(err) {
		t.Errorf("Log file was not created at %s", logFile)
	}
}

func TestInitializeWithBoth(t *testing.T) {
	// Create temp directory for test logs
	tempDir := t.TempDir()
	logFile := filepath.Join(tempDir, "test.log")

	cfg := Config{
		Level:  "debug",
		Output: "both",
		File: FileConfig{
			Path:       logFile,
			MaxSizeMB:  10,
			MaxBackups: 3,
			MaxAgeDays: 7,
			Compress:   false,
		},
	}

	err := Initialize(cfg)
	if err != nil {
		t.Fatalf("Initialize() failed: %v", err)
	}

	// Write test logs at different levels
	Debug("debug message")
	Info("info message")
	Warn("warn message")
	Error("error message")

	// Sync to ensure logs are written
	// Note: Sync() may fail on stdout with "bad file descriptor", which is expected
	Sync()

	// Check if log file exists
	if _, err := os.Stat(logFile); os.IsNotExist(err) {
		t.Errorf("Log file was not created at %s", logFile)
	}
}

func TestFallbackMechanism(t *testing.T) {
	// Try to create logger with file in non-existent directory with no permissions
	// This should fall back gracefully
	cfg := Config{
		Level:  "info",
		Output: "both",
		File: FileConfig{
			Path:       "/root/impossible/test.log", // Typically no permissions
			MaxSizeMB:  10,
			MaxBackups: 3,
			MaxAgeDays: 7,
			Compress:   true,
		},
	}

	// This should NOT fail - it should fall back to stdout only
	err := Initialize(cfg)
	if err != nil {
		t.Fatalf("Initialize() should not fail when file creation fails in 'both' mode, got error: %v", err)
	}

	// Logger should still work
	Info("test message after fallback")
}

func TestLoggerHelperFunctions(t *testing.T) {
	// Initialize with stdout to avoid file creation
	cfg := Config{
		Level:  "debug",
		Output: "stdout",
	}

	err := Initialize(cfg)
	if err != nil {
		t.Fatalf("Initialize() failed: %v", err)
	}

	// Test all helper functions (should not panic)
	t.Run("String helper", func(t *testing.T) {
		field := String("key", "value")
		if field.Key != "key" {
			t.Errorf("String() key = %v, want 'key'", field.Key)
		}
	})

	t.Run("Int helper", func(t *testing.T) {
		field := Int("count", 42)
		if field.Key != "count" {
			t.Errorf("Int() key = %v, want 'count'", field.Key)
		}
	})

	t.Run("Bool helper", func(t *testing.T) {
		field := Bool("enabled", true)
		if field.Key != "enabled" {
			t.Errorf("Bool() key = %v, want 'enabled'", field.Key)
		}
	})

	t.Run("Float64 helper", func(t *testing.T) {
		field := Float64("value", 3.14)
		if field.Key != "value" {
			t.Errorf("Float64() key = %v, want 'value'", field.Key)
		}
	})

	t.Run("Any helper", func(t *testing.T) {
		field := Any("data", map[string]string{"foo": "bar"})
		if field.Key != "data" {
			t.Errorf("Any() key = %v, want 'data'", field.Key)
		}
	})
}

func TestGetLogger(t *testing.T) {
	// Initialize logger
	cfg := Config{
		Level:  "info",
		Output: "stdout",
	}

	err := Initialize(cfg)
	if err != nil {
		t.Fatalf("Initialize() failed: %v", err)
	}

	// Get logger instance
	l := GetLogger()
	if l == nil {
		t.Error("GetLogger() returned nil")
	}

	// Get sugared logger instance
	sl := GetSugaredLogger()
	if sl == nil {
		t.Error("GetSugaredLogger() returned nil")
	}
}

func TestSugaredLogger(t *testing.T) {
	// Initialize with stdout
	cfg := Config{
		Level:  "debug",
		Output: "stdout",
	}

	err := Initialize(cfg)
	if err != nil {
		t.Fatalf("Initialize() failed: %v", err)
	}

	// Test sugared logger functions (should not panic)
	Debugf("debug message: %s", "test")
	Infof("info message: %d", 42)
	Warnf("warn message: %v", true)
	Errorf("error message: %f", 3.14)
}
