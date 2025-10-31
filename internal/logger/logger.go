package logger

import (
	"fmt"
	"os"
	"path/filepath"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

// Config holds the logging configuration
type Config struct {
	Level  string     `mapstructure:"level"`
	Output string     `mapstructure:"output"`
	File   FileConfig `mapstructure:"file"`
}

// FileConfig holds file-specific logging configuration
type FileConfig struct {
	Path       string `mapstructure:"path"`
	MaxSizeMB  int    `mapstructure:"max_size_mb"`
	MaxBackups int    `mapstructure:"max_backups"`
	MaxAgeDays int    `mapstructure:"max_age_days"`
	Compress   bool   `mapstructure:"compress"`
}

var (
	// Global logger instance
	logger *zap.Logger
	sugar  *zap.SugaredLogger
)

func init() {
	// Initialize with a default development logger
	// This will be replaced when Initialize() is called
	defaultLogger, _ := zap.NewDevelopment()
	logger = defaultLogger
	sugar = logger.Sugar()
}

// Initialize sets up the global logger with the provided configuration
func Initialize(cfg Config) error {
	// Validate configuration
	if err := validateConfig(cfg); err != nil {
		return fmt.Errorf("invalid logger config: %w", err)
	}

	// Parse log level
	level, err := parseLevel(cfg.Level)
	if err != nil {
		return fmt.Errorf("invalid log level: %w", err)
	}

	// Create encoder config
	encoderConfig := zapcore.EncoderConfig{
		TimeKey:        "time",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "caller",
		MessageKey:     "msg",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.CapitalLevelEncoder,
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeDuration: zapcore.StringDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}

	// Create encoder (console format for readability)
	encoder := zapcore.NewConsoleEncoder(encoderConfig)

	// Create writers based on output configuration
	var cores []zapcore.Core
	fileWriterFailed := false

	switch cfg.Output {
	case "stdout", "console":
		cores = append(cores, zapcore.NewCore(encoder, zapcore.AddSync(os.Stdout), level))

	case "file":
		fileWriter, err := createFileWriter(cfg.File)
		if err != nil {
			// For file-only mode, fall back to stderr with a warning
			fmt.Fprintf(os.Stderr, "WARNING: Failed to create log file, falling back to stderr: %v\n", err)
			cores = append(cores, zapcore.NewCore(encoder, zapcore.AddSync(os.Stderr), level))
			fileWriterFailed = true
		} else {
			cores = append(cores, zapcore.NewCore(encoder, zapcore.AddSync(fileWriter), level))
		}

	case "both":
		// Console output (always add this first)
		cores = append(cores, zapcore.NewCore(encoder, zapcore.AddSync(os.Stdout), level))

		// File output (attempt, but don't fail if it doesn't work)
		fileWriter, err := createFileWriter(cfg.File)
		if err != nil {
			// Log warning but continue with console only
			fmt.Fprintf(os.Stderr, "WARNING: Failed to create log file, using stdout only: %v\n", err)
			fileWriterFailed = true
		} else {
			cores = append(cores, zapcore.NewCore(encoder, zapcore.AddSync(fileWriter), level))
		}

	default:
		return fmt.Errorf("invalid output type: %s (must be 'stdout', 'file', or 'both')", cfg.Output)
	}

	// Combine cores and create logger
	core := zapcore.NewTee(cores...)
	newLogger := zap.New(core, zap.AddCaller(), zap.AddStacktrace(zapcore.ErrorLevel))

	// Replace global logger
	logger = newLogger
	sugar = logger.Sugar()

	// Log warning about file writer failure using the new logger
	if fileWriterFailed {
		logger.Warn("Log file creation failed, using fallback output")
	}

	return nil
}

// createFileWriter creates a lumberjack writer for log rotation
func createFileWriter(cfg FileConfig) (*lumberjack.Logger, error) {
	// Ensure directory exists
	logDir := filepath.Dir(cfg.Path)
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create log directory: %w", err)
	}

	return &lumberjack.Logger{
		Filename:   cfg.Path,
		MaxSize:    cfg.MaxSizeMB,
		MaxBackups: cfg.MaxBackups,
		MaxAge:     cfg.MaxAgeDays,
		Compress:   cfg.Compress,
	}, nil
}

// validateConfig validates the logger configuration
func validateConfig(cfg Config) error {
	// Validate output type
	switch cfg.Output {
	case "stdout", "console", "file", "both":
		// Valid
	default:
		return fmt.Errorf("output must be 'stdout', 'file', or 'both', got: %s", cfg.Output)
	}

	// Validate file config if file output is used
	if cfg.Output == "file" || cfg.Output == "both" {
		if cfg.File.Path == "" {
			return fmt.Errorf("file.path is required when output is 'file' or 'both'")
		}
		if cfg.File.MaxSizeMB <= 0 {
			return fmt.Errorf("file.max_size_mb must be positive, got: %d", cfg.File.MaxSizeMB)
		}
		if cfg.File.MaxBackups < 0 {
			return fmt.Errorf("file.max_backups cannot be negative, got: %d", cfg.File.MaxBackups)
		}
		if cfg.File.MaxAgeDays < 0 {
			return fmt.Errorf("file.max_age_days cannot be negative, got: %d", cfg.File.MaxAgeDays)
		}
	}

	return nil
}

// parseLevel converts string level to zapcore.Level
func parseLevel(level string) (zapcore.Level, error) {
	switch level {
	case "debug":
		return zapcore.DebugLevel, nil
	case "info":
		return zapcore.InfoLevel, nil
	case "warn", "warning":
		return zapcore.WarnLevel, nil
	case "error":
		return zapcore.ErrorLevel, nil
	default:
		return zapcore.InfoLevel, fmt.Errorf("unknown level: %s", level)
	}
}

// Sync flushes any buffered log entries
func Sync() error {
	if logger != nil {
		return logger.Sync()
	}
	return nil
}

// GetLogger returns the global logger instance (useful for passing to other libraries)
func GetLogger() *zap.Logger {
	return logger
}

// GetSugaredLogger returns the sugared logger (for printf-style logging)
func GetSugaredLogger() *zap.SugaredLogger {
	return sugar
}

// Package-level logging functions for easy use

// Debug logs a debug message with structured fields
func Debug(msg string, fields ...zap.Field) {
	logger.Debug(msg, fields...)
}

// Info logs an info message with structured fields
func Info(msg string, fields ...zap.Field) {
	logger.Info(msg, fields...)
}

// Warn logs a warning message with structured fields
func Warn(msg string, fields ...zap.Field) {
	logger.Warn(msg, fields...)
}

// Error logs an error message with structured fields
func Error(msg string, fields ...zap.Field) {
	logger.Error(msg, fields...)
}

// Fatal logs a fatal message and exits
func Fatal(msg string, fields ...zap.Field) {
	logger.Fatal(msg, fields...)
}

// Debugf logs a debug message with printf-style formatting (using sugared logger)
func Debugf(template string, args ...interface{}) {
	sugar.Debugf(template, args...)
}

// Infof logs an info message with printf-style formatting (using sugared logger)
func Infof(template string, args ...interface{}) {
	sugar.Infof(template, args...)
}

// Warnf logs a warning message with printf-style formatting (using sugared logger)
func Warnf(template string, args ...interface{}) {
	sugar.Warnf(template, args...)
}

// Errorf logs an error message with printf-style formatting (using sugared logger)
func Errorf(template string, args ...interface{}) {
	sugar.Errorf(template, args...)
}

// Fatalf logs a fatal message with printf-style formatting and exits
func Fatalf(template string, args ...interface{}) {
	sugar.Fatalf(template, args...)
}

// Field helper functions for convenience (re-export from zap)

// String creates a string field
func String(key, val string) zap.Field {
	return zap.String(key, val)
}

// Int creates an int field
func Int(key string, val int) zap.Field {
	return zap.Int(key, val)
}

// Int64 creates an int64 field
func Int64(key string, val int64) zap.Field {
	return zap.Int64(key, val)
}

// Uint64 creates a uint64 field
func Uint64(key string, val uint64) zap.Field {
	return zap.Uint64(key, val)
}

// Float64 creates a float64 field
func Float64(key string, val float64) zap.Field {
	return zap.Float64(key, val)
}

// Bool creates a bool field
func Bool(key string, val bool) zap.Field {
	return zap.Bool(key, val)
}

// Duration creates a duration field
func Duration(key string, val interface{}) zap.Field {
	return zap.Any(key, val)
}

// Err creates an error field
func Err(err error) zap.Field {
	return zap.Error(err)
}

// Any creates a field with any type
func Any(key string, val interface{}) zap.Field {
	return zap.Any(key, val)
}
