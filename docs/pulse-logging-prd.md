# Node Pulse Agent Logging System - PRD

## Overview

The Node Pulse agent currently uses basic `log.Println()` statements that output to stdout/stderr. As a system monitoring agent that runs as a daemon, it requires a robust, configurable logging system for debugging, troubleshooting, and operational visibility.

## Background

Current state:

- Simple `log.Println()` statements scattered throughout the codebase
- No log levels (everything is treated as INFO)
- No structured logging
- No log file persistence
- No log rotation
- Difficult to debug production issues

## Goals

1. Implement structured logging with multiple severity levels
2. Support both file-based and console logging
3. Add log rotation to prevent disk space issues
4. Make logging configurable via `nodepulse.yml`
5. Maintain minimal performance overhead
6. Provide operational visibility into agent behavior

## Non-Goals

1. Remote log shipping (syslog, log aggregation services) - future enhancement
2. Log encryption - not needed for v1
3. Custom log formats (JSON, etc.) - can be added later
4. Per-component log level configuration - keep it simple for v1

## Requirements

### Functional Requirements

#### FR1: Log Levels

- Support standard log levels: DEBUG, INFO, WARN, ERROR
- Default level: INFO
- Configurable via config file
- Lower levels include higher severity logs (e.g., INFO includes WARN and ERROR)

#### FR2: Log Outputs

- **Console output**: Write to stdout/stderr
- **File output**: Write to a log file with rotation
- **Both**: Write to both console and file simultaneously
- Configurable via config file

#### FR3: Log Rotation

- Rotate logs based on file size
- Keep a configurable number of backup files
- Keep logs for a configurable number of days
- Use timestamp-based naming for rotated files

#### FR4: Configuration

Add new section to `nodepulse.yml`:

```yaml
logging:
  level: "info" # debug, info, warn, error
  output: "both" # stdout, file, both
  file:
    path: "/var/log/nodepulse/agent.log"
    max_size_mb: 10
    max_backups: 3
    max_age_days: 7
    compress: true # gzip old log files
```

#### FR5: Log Content

Each log entry should include:

- Timestamp (ISO 8601 format with timezone)
- Log level
- Message
- Optional structured fields (key-value pairs)

Example format:

```
2025-10-14T15:04:05-04:00 INFO  Agent started (server_id=abc-123, interval=5s)
2025-10-14T15:04:10-04:00 ERROR Failed to send report: connection timeout
2025-10-14T15:04:15-04:00 DEBUG Collected metrics: cpu=45.2%, memory=2.1GB
```

### Non-Functional Requirements

#### NFR1: Performance

- Logging should add < 1ms overhead per call
- Use buffered I/O for file writes
- Async logging for non-critical messages (INFO, DEBUG)

#### NFR2: Reliability

- Never crash the agent due to logging errors
- If log file cannot be written, fall back to stderr
- Handle disk full scenarios gracefully

#### NFR3: Security

- Log files should have restricted permissions (0640)
- Do not log sensitive data (API keys, tokens)
- Sanitize any user-provided input before logging

#### NFR4: Compatibility

- Work on Linux, macOS, and Windows
- Handle different filesystem permissions
- Work with systemd, Docker, and bare-metal deployments

## Technical Design

### Library Selection

Use **[lumberjack](https://github.com/natefinch/lumberjack)** for log rotation:

- Lightweight and battle-tested
- Automatic rotation based on size, age, and count
- No external dependencies
- Works well with Go's standard logger

Use **[logrus](https://github.com/sirupsen/logrus)** or **[zap](https://go.uber.org/zap)** for structured logging:

| Feature            | logrus | zap                     |
| ------------------ | ------ | ----------------------- |
| Performance        | Good   | Excellent (2-3x faster) |
| API complexity     | Simple | More complex            |
| Structured logging | Yes    | Yes                     |
| Community          | Large  | Large                   |
| Maintenance        | Active | Active                  |

**Recommendation**: Use **zap** for better performance, as the agent needs to be efficient.

### Architecture

```
┌─────────────────┐
│   Application   │
│    Code         │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│  Logger (zap)   │
│  - Level filter │
│  - Formatting   │
└────┬────────┬───┘
     │        │
     ▼        ▼
┌─────────┐ ┌──────────────┐
│ Console │ │ File Writer  │
│ Output  │ │ (lumberjack) │
└─────────┘ └──────────────┘
                    │
                    ▼
            ┌───────────────┐
            │  Log Files    │
            │  - agent.log  │
            │  - agent.log.1│
            │  - agent.log.2│
            └───────────────┘
```

### File Structure

```
internal/
  logger/
    logger.go         # Logger initialization and configuration
    config.go         # Logger config struct
    logger_test.go    # Unit tests
```

### Key Components

#### 1. Logger Configuration

```go
type LogConfig struct {
    Level  string     `yaml:"level"`
    Output string     `yaml:"output"`
    File   FileConfig `yaml:"file"`
}

type FileConfig struct {
    Path       string `yaml:"path"`
    MaxSizeMB  int    `yaml:"max_size_mb"`
    MaxBackups int    `yaml:"max_backups"`
    MaxAgeDays int    `yaml:"max_age_days"`
    Compress   bool   `yaml:"compress"`
}
```

#### 2. Logger Initialization

```go
func InitLogger(cfg LogConfig) (*zap.Logger, error)
```

#### 3. Global Logger Access

```go
var logger *zap.Logger

func Info(msg string, fields ...zap.Field)
func Error(msg string, fields ...zap.Field)
func Debug(msg string, fields ...zap.Field)
func Warn(msg string, fields ...zap.Field)
```

### Migration Strategy

1. Create new logger package
2. Initialize logger in `main.go` after loading config
3. Replace all `log.Println()` calls with structured logger calls
4. Add structured fields where appropriate
5. Remove old `log` package imports

Example migration:

```go
// Before
log.Printf("Agent started (server_id: %s, interval: %s)\n", serverID, interval)

// After
logger.Info("Agent started",
    zap.String("server_id", serverID),
    zap.Duration("interval", interval))
```

## Implementation Plan

### Phase 1: Core Logging Infrastructure (Day 1)

- [ ] Add dependencies: `go.uber.org/zap` and `gopkg.in/natefinch/lumberjack.v2`
- [ ] Create `internal/logger` package
- [ ] Implement logger initialization with config
- [ ] Add logging config to `nodepulse.yml`
- [ ] Update config loading to include logging section
- [ ] Unit tests for logger initialization

### Phase 2: Integration (Day 1-2)

- [ ] Initialize logger in `main.go`
- [ ] Replace `log.Println` calls in `cmd/agent.go`
- [ ] Replace logging in `internal/report/sender.go`
- [ ] Replace logging in `internal/report/buffer.go`
- [ ] Add structured fields for key operations

### Phase 3: Enhancement (Day 2)

- [ ] Add debug logging for metrics collection
- [ ] Add debug logging for HTTP requests/responses
- [ ] Add error logging with stack traces for critical failures
- [ ] Ensure proper log levels throughout

### Phase 4: Testing & Documentation (Day 2-3)

- [ ] Integration tests for file rotation
- [ ] Test logging in different output modes (stdout, file, both)
- [ ] Test with different log levels
- [ ] Update README with logging configuration
- [ ] Add logging best practices to documentation

## Configuration Examples

### Development (verbose logging to console)

```yaml
logging:
  level: "debug"
  output: "stdout"
```

### Production (structured logs to file with rotation)

```yaml
logging:
  level: "info"
  output: "both"
  file:
    path: "/var/log/nodepulse/agent.log"
    max_size_mb: 10
    max_backups: 5
    max_age_days: 7
    compress: true
```

### Minimal (errors only)

```yaml
logging:
  level: "error"
  output: "stdout"
```

## Success Metrics

1. **Adoption**: All log statements use structured logging (0 `log.Println` calls remaining)
2. **Performance**: No measurable performance regression in agent metrics collection
3. **Reliability**: No crashes due to logging failures in 30 days of testing
4. **Usability**: Support team can debug 90% of issues using log files alone

## Security Considerations

1. **File Permissions**: Ensure log files are created with 0640 permissions
2. **Directory Permissions**: Ensure log directory has proper ownership
3. **Sensitive Data**: Never log API keys, tokens, or credentials
4. **Disk Space**: Log rotation prevents disk exhaustion attacks
5. **Input Validation**: Sanitize any user input before logging to prevent log injection

## Open Questions

1. **Q**: Should we support JSON structured logs for easier parsing?
   **A**: Not in v1. Can be added as an optional format later.

2. **Q**: Should we add request ID tracking for correlating related log entries?
   **A**: Yes, if we implement retry logic. Add as a future enhancement.

3. **Q**: Should we log all HTTP request/response bodies?
   **A**: Only in DEBUG mode, and sanitize sensitive fields.

4. **Q**: Should we support syslog output?
   **A**: Not in v1. Can be added later if users request it.

5. **Q**: How do we handle logging during initialization before config is loaded?
   **A**: Use a default console logger, then swap to configured logger after config load.

## Dependencies

- `go.uber.org/zap` - Structured, fast logging
- `gopkg.in/natefinch/lumberjack.v2` - Log rotation

## References

- [zap Documentation](https://pkg.go.dev/go.uber.org/zap)
- [lumberjack Documentation](https://pkg.go.dev/gopkg.in/natefinch/lumberjack.v2)
- [Twelve-Factor App: Logs](https://12factor.net/logs)
- [Go Logging Best Practices](https://www.honeybadger.io/blog/golang-logging/)

## Appendix: Log Message Guidelines

### Good Log Messages

```go
✓ logger.Info("Agent started", zap.String("server_id", id), zap.Duration("interval", interval))
✓ logger.Error("Failed to send report", zap.Error(err), zap.Int("retry_count", retries))
✓ logger.Debug("Collected metrics", zap.Float64("cpu_percent", cpu), zap.Uint64("memory_bytes", mem))
```

### Bad Log Messages

```go
✗ logger.Info("Starting...")  // Too vague
✗ logger.Error("Error")       // No context
✗ logger.Debug(fmt.Sprintf("CPU: %f", cpu))  // Use structured fields instead
```

### Log Level Guidelines

- **DEBUG**: Detailed diagnostic information for troubleshooting

  - Metrics values before/after collection
  - HTTP request/response details
  - Buffer file operations

- **INFO**: General informational messages about normal operation

  - Agent startup/shutdown
  - Successful report sends
  - Configuration changes

- **WARN**: Potentially harmful situations that don't prevent operation

  - Retry attempts
  - Deprecated configuration options
  - Non-critical errors with fallbacks

- **ERROR**: Error events that might still allow operation to continue
  - Failed report sends (will retry)
  - Invalid configuration values (using defaults)
  - File I/O errors with recovery

---

**Document Version**: 1.0
**Last Updated**: 2025-10-14
**Author**: NodePulse Team
**Status**: Draft → Ready for Review
