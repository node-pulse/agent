# PRD: `pulse init` Command

## Overview

Create an idempotent initialization command that automates the setup of NodePulse agent on a target machine. The command should support both interactive (TUI) and non-interactive modes for different use cases.

## Prerequisites

- The `pulse` binary must already be installed in `/usr/local/bin/` or be available in `$PATH`
- This can be done via:
  - Manual binary placement (current method)
  - Installation script: `curl -L https://get.nodepulse.sh | sudo bash` (planned for future)
- Root/sudo access is required to create system directories and config files

**Note**: Binary installation is out of scope for this command. `pulse init` only handles configuration and setup after the binary is already installed.

## Motivation

Currently, after installing the binary, setting up the agent requires manual steps:
1. Creating multiple directories (`/etc/node-pulse/`, `/var/lib/node-pulse/buffer/`)
2. Manually creating and editing the config file
3. Handling server ID generation and persistence

This manual process is:
- Error-prone
- Not idempotent (can't safely re-run)
- Difficult to automate
- Poor user experience

## Goals

1. **Idempotent**: Can be run multiple times safely without breaking existing setup
2. **User-friendly**: Interactive TUI for manual setup with beautiful wizard
3. **Simple**: Quick mode (`--yes`) only asks for endpoint and optional server ID, everything else uses sensible defaults
4. **Flexible**: Support both custom server IDs (alphanumeric names like "prod-web-01") and auto-generated UUIDs
5. **Smart**: Detect existing configuration and preserve user data (especially server_id)
6. **Robust**: Proper error handling and permission checks

## Command Signature

```bash
pulse init [--yes|-y]
```

### Flags

- `--yes, -y`: Quick mode - only prompt for endpoint URL, use defaults for everything else

## Features

### 1. Interactive Mode (Default)

When run without `--yes`, launch a beautiful TUI wizard using Bubble Tea.

#### Screens/Steps:

1. **Welcome Screen**
   - Show NodePulse logo/title
   - Brief description of what init will do
   - Permission check (warn if not root/sudo)
   - "Press Enter to continue"

2. **Existing Installation Detection**
   - Check if config exists
   - Check if server_id exists
   - If found, show current settings and ask:
     - "Reconfigure" (update settings)
     - "Repair" (fix directories/permissions only)
     - "Cancel"

3. **Endpoint Configuration**
   - Text input field with validation
   - Example: `https://api.nodepulse.io/metrics`
   - Real-time validation (URL format check)
   - Help text: "Enter the metrics endpoint URL for your control server"

4. **Server ID Configuration**
   - Check if server_id file exists
   - If exists:
     - Show: "Found existing server ID: `xxxx-xxxx-xxxx`"
     - Option: "Keep existing" or "Enter new server ID"
   - If not exists or user wants to change:
     - Prompt: "Enter server ID (leave empty to auto-generate UUID): "
     - If user provides input:
       - Validate format:
         - Only alphanumeric characters and dashes allowed (a-z, A-Z, 0-9, -)
         - Must start and end with alphanumeric character (not dash)
         - No spaces allowed
         - Must be at least 1 character long
       - Use the provided ID
     - If empty: auto-generate UUID v4
     - Show the final server ID that will be used
   - Help text: "A unique identifier for this server. Can be any name like 'prod-web-01' or leave empty to generate a UUID."

5. **Interval Selection**
   - Radio button / dropdown selection
   - Options: 5s, 10s, 30s, 1m
   - Default: 5s
   - Help text: "How often to collect and send metrics"

6. **Buffer Settings**
   - Toggle: Enable buffering (default: yes)
   - If enabled:
     - Path input (default: /var/lib/node-pulse/buffer)
     - Retention hours input (default: 48)
   - Help text: "Buffer failed reports for retry when server is unavailable"

7. **Advanced Settings (Optional)**
   - Collapsible/expandable section
   - Server timeout (default: 3s)
   - Custom config file path

8. **Review & Confirm**
   - Show all settings in a formatted view:
     ```
     Configuration Summary
     ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
     Endpoint:     https://api.nodepulse.io/metrics
     Server ID:    a1b2c3d4-e5f6-7890-abcd-ef1234567890
     Interval:     5s
     Timeout:      3s
     Buffer:       Enabled
     Buffer Path:  /var/lib/node-pulse/buffer
     Retention:    48 hours
     Config Path:  /etc/node-pulse/nodepulse.yml
     ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
     ```
   - Buttons: "Confirm" or "Back to edit"

9. **Installation Progress**
   - Show progress steps with spinners/checkmarks:
     ```
     ✓ Checking permissions
     ✓ Creating /etc/node-pulse/
     ✓ Creating /var/lib/node-pulse/
     ✓ Creating /var/lib/node-pulse/buffer/
     ✓ Persisting server ID
     ✓ Writing configuration file
     ✓ Validating configuration
     ✓ Setting permissions
     ```

10. **Success Screen**
    - Success message with checkmark
    - Show summary:
      ```
      ✓ NodePulse agent initialized successfully!

      Server ID:  a1b2c3d4-e5f6-7890-abcd-ef1234567890
      Config:     /etc/node-pulse/nodepulse.yml

      Next steps:
        1. Start the agent:    pulse agent
        2. Watch live metrics: pulse watch
        3. Install service:    sudo pulse service install
      ```
    - Optional: Ask "Install as systemd service now? [y/N]"

### 2. Quick Mode (`--yes` or `-y`)

Streamlined setup with minimal prompts.

#### Behavior:

- **Prompt for essential settings only:**
  1. Endpoint URL (required)
  2. Server ID (optional - custom name or leave empty to auto-generate UUID)
- Use defaults for all other settings:
  - Interval: 5s
  - Timeout: 3s
  - Buffer: enabled, 48-hour retention
- Detect existing installation:
  - If server_id exists: show it, ask to keep or change
  - If config exists: update endpoint, keep other settings
- Output: Simple text progress messages
- Exit codes:
  - 0: Success
  - 1: Error (with descriptive message)

#### Examples:

```bash
# Auto-generate UUID
sudo pulse init --yes
# Prompts:
#   Enter endpoint URL: https://api.nodepulse.io/metrics
#   Enter server ID (leave empty to auto-generate UUID): [Enter]
# Generates UUID like "a1b2c3d4-e5f6-7890-abcd-ef1234567890"

# Use custom server ID
sudo pulse init --yes
# Prompts:
#   Enter endpoint URL: https://api.nodepulse.io/metrics
#   Enter server ID (leave empty to auto-generate UUID): prod-web-01
# Uses "prod-web-01" as server ID
```

### 3. Idempotent Operations

The command must be safe to run multiple times.

#### Detection Logic:

1. **Config file exists**:
   - Interactive: Ask to reconfigure or repair
   - Quick mode: Update endpoint only, keep other settings

2. **Directories exist**:
   - Skip creation
   - Verify permissions, fix if needed

3. **Server ID exists**:
   - Always keep existing
   - Show which ID is being used

4. **Binary already in place**:
   - Not init's responsibility (manual or install script)

#### Output Messages:

```
✓ Found existing server ID: a1b2c3d4-...
✓ Directory /etc/node-pulse/ already exists
✓ Updated configuration file
⚠ Fixed permissions on /var/lib/node-pulse/server_id
```

## Setup Tasks (In Order)

1. **Permission Check**
   - Verify running as root or with sudo
   - Check write access to `/etc/` and `/var/lib/`
   - Error if insufficient permissions

2. **Directory Creation**
   - Create `/etc/node-pulse/` (mode: 0755)
   - Create `/var/lib/node-pulse/` (mode: 0755)
   - Create `/var/lib/node-pulse/buffer/` (mode: 0755)
   - Skip if already exists
   - Verify/fix permissions if exists

3. **Server ID Management**
   - Check for existing server_id at standard paths:
     - `/var/lib/node-pulse/server_id`
     - `/etc/node-pulse/server_id`
     - `~/.node-pulse/server_id`
   - If found and valid:
     - Interactive: Show it, ask to keep or enter new
     - Quick mode: Show it, ask to keep or enter new
   - Prompt user: "Enter server ID (leave empty to auto-generate UUID): "
   - If user provides input:
     - Validate format:
       - Pattern: `^[a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?$`
       - Must start and end with alphanumeric character
       - Can contain dashes in the middle
       - No spaces allowed
       - Minimum length: 1 character
     - Use provided ID (can be custom name like "prod-web-01" or UUID format)
   - If user leaves empty or no existing ID:
     - Generate new UUID v4
   - Persist to `/var/lib/node-pulse/server_id` (mode: 0600)
   - Display final server ID to user

4. **Configuration File**
   - Generate YAML config from user input or defaults
   - Write to `/etc/node-pulse/nodepulse.yml` (mode: 0644)
   - If file exists:
     - Interactive: show diff, ask to overwrite
     - Quick mode: update endpoint only, preserve other customizations
   - Format example:
     ```yaml
     server:
       endpoint: "https://api.nodepulse.io/metrics"
       timeout: 3s

     agent:
       server_id: "a1b2c3d4-e5f6-7890-abcd-ef1234567890"
       interval: 5s

     buffer:
       enabled: true
       path: "/var/lib/node-pulse/buffer"
       retention_hours: 48
     ```

5. **Validation**
   - Load the written config using existing `config.Load()`
   - Verify all required fields present
   - Validate formats:
     - Server ID: `^[a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?$`
       - Must start and end with alphanumeric
       - Can contain dashes in middle
       - No spaces
     - URL: must be valid http/https endpoint
     - Durations: must be valid time.Duration format
   - Error if validation fails

   **Note**: The existing validation in `internal/config/config.go` will need to be updated to accept alphanumeric+dash server IDs, not just UUID format.

6. **Optional: Systemd Service**
   - At the end, ask user: "Install as systemd service now? [y/N]"
   - If confirmed, run `pulse service install` internally
   - Report success/failure

## Implementation Structure

### New Files

#### `cmd/init.go`
- Cobra command definition
- Flag parsing
- Mode selection (interactive vs non-interactive)
- TUI setup and event loop (Bubble Tea)
- Orchestrate installer calls

#### `internal/installer/installer.go`
- Core installation functions
- Idempotency logic
- File system operations
- Permission management

### Functions Needed

#### `cmd/init.go`
```go
func runInit(cmd *cobra.Command, args []string) error
func runInteractive() error
func runQuickMode() error // Prompts for endpoint and server ID, uses defaults for rest

// TUI models
type initModel struct { ... }
func (m initModel) Init() tea.Cmd
func (m initModel) Update(msg tea.Msg) (tea.Model, tea.Cmd)
func (m initModel) View() string
```

#### `internal/installer/installer.go`
```go
type InstallConfig struct {
    Endpoint string // Required: metrics endpoint URL
    ServerID string // Optional: custom ID (alphanumeric + dashes) or empty to auto-generate UUID
}

func CheckPermissions() error
func DetectExisting() (*ExistingInstall, error)
func CreateDirectories() error
func HandleServerID(customID string) (string, error) // Returns final server ID (validates custom or generates UUID)
func ValidateServerID(id string) error // Validates format: ^[a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?$
func WriteConfigFile(endpoint string, serverID string) error
func ValidateInstallation() error
func FixPermissions() error
```

### Reuse Existing Code

- `internal/config/serverid.go`: Server ID generation and persistence
- `internal/config/config.go`: Config validation (needs update - change `isValidUUID()` to accept alphanumeric+dash format)
- `cmd/service.go`: Service installation (optional at end of init)

### Code Changes Required

Update `internal/config/config.go`:
```go
// Change from:
func isValidUUID(u string) bool {
    // UUID validation logic...
}

// To:
func isValidServerID(id string) bool {
    // Validate: alphanumeric and dashes
    // Pattern: ^[a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?$
    // Must start and end with alphanumeric, no spaces

    if len(id) == 0 {
        return false
    }

    // Check first character: must be alphanumeric
    first := rune(id[0])
    if !isAlphanumeric(first) {
        return false
    }

    // Check last character: must be alphanumeric
    last := rune(id[len(id)-1])
    if !isAlphanumeric(last) {
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

func isAlphanumeric(c rune) bool {
    return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}
```

## Edge Cases & Error Handling

1. **No sudo/root**:
   - Error immediately: "This command requires root privileges. Run with sudo."

2. **Invalid endpoint URL**:
   - Validate format (must be http:// or https://)
   - Interactive: Show error, allow re-entry
   - Quick mode: Show error, allow re-entry

3. **Invalid server ID format**:
   - Must contain only alphanumeric characters (a-z, A-Z, 0-9) and dashes (-)
   - Must start and end with alphanumeric character (not dash)
   - Cannot contain spaces
   - Must be at least 1 character long
   - If invalid: Show error "Invalid server ID. Must start and end with a letter or number. Can contain dashes in the middle. No spaces allowed. (e.g., 'prod-web-01' or 'my-server')"
   - Allow re-entry or option to auto-generate UUID

4. **Directory creation fails**:
   - Show clear error: "Failed to create /etc/node-pulse/: permission denied"
   - Suggest solution: "Ensure you're running with sudo"

5. **Config file locked/in use**:
   - Detect if agent is running
   - Warn: "Agent may be running. Stop it first with: pulse service stop"

6. **Partial installation**:
   - If init fails midway, allow re-running
   - Don't leave system in broken state
   - Rollback on critical failures

7. **Invalid existing server_id file**:
   - Warn: "Found invalid server_id file, will generate new one"
   - Backup old file: `server_id.bak`

8. **Disk space issues**:
   - Check available space before writing
   - Reasonable check: >100MB available

## Testing Strategy

### Manual Testing

1. Fresh installation (no existing files)
   - Auto-generate server ID (leave empty)
   - Provide custom alphanumeric server ID (e.g., "prod-web-01")
   - Provide UUID format server ID
2. Re-run on existing installation
   - Keep existing server ID
   - Change to new custom server ID
   - Change to new auto-generated UUID
3. Quick mode (`--yes`) with various inputs
   - Empty server ID (auto-generate UUID)
   - Custom alphanumeric server ID (e.g., "database-primary")
   - UUID format server ID
4. Permission errors (run without sudo)
5. Invalid inputs
   - Bad URL format
   - Invalid server ID formats:
     - Contains spaces: "my server"
     - Contains special chars: "server@prod", "web#01"
     - Starts with dash: "-server", "-web-01"
     - Ends with dash: "server-", "web-01-"
     - Contains underscore: "server_01"
     - Empty string or just spaces
6. Existing but corrupted config or server_id file

### Automated Testing

Unit tests for:
- `installer.DetectExisting()`
- `installer.WriteConfigFile()`
- `installer.HandleServerID()` - test with valid custom ID, invalid characters, empty string
- `installer.ValidateServerID()` - test various formats:
  - Valid:
    - "prod-web-01", "my-server", "db-primary-2"
    - "a1b2c3d4-e5f6-7890-abcd-ef1234567890" (UUID format)
    - "a", "1", "x" (single character)
    - "server-1", "web01", "DB-PRIMARY"
  - Invalid:
    - "server name" (space)
    - "server@prod" (special char)
    - "" (empty)
    - "server_01" (underscore)
    - "-server" (starts with dash)
    - "server-" (ends with dash)
    - "-" (only dash)
    - "my--server" (consecutive dashes - still valid actually)
- `installer.CheckPermissions()`
- URL validation function - test various formats
- Config validation
- Flag parsing

## Documentation Updates

### README.md

Add section after "Installation":

```markdown
## Quick Start

### Initialize the Agent

Run the interactive setup wizard (recommended for first-time setup):

```bash
sudo pulse init
```

Or use quick mode (minimal prompts, uses defaults):

```bash
sudo pulse init --yes
# Prompts:
#   Enter endpoint URL: https://api.nodepulse.io/metrics
#   Enter server ID (leave empty to auto-generate UUID): [press Enter]
# Auto-generates UUID and completes setup

# Or with custom server ID:
sudo pulse init --yes
# Prompts:
#   Enter endpoint URL: https://api.nodepulse.io/metrics
#   Enter server ID (leave empty to auto-generate UUID): prod-web-server-01
# Uses "prod-web-server-01" as server ID
```

This will:
- Create necessary directories
- Use custom server ID or generate UUID
- Create the configuration file
- Set proper permissions

### Start the Agent

```bash
# Run in foreground
pulse agent

# Or install as a service
sudo pulse service install
sudo pulse service start
```
```

## Success Metrics

- Users can set up agent in <2 minutes (interactive mode)
- Quick mode setup in <30 seconds (enter endpoint + optional server ID)
- Zero manual file editing required
- Can re-run safely (idempotent)
- Server ID is flexible:
  - Can use custom names (e.g., "prod-web-01", "my-server")
  - Can use UUID format if desired
  - Can auto-generate UUID by leaving empty
- Existing server IDs are preserved by default
- Clear error messages guide users to solutions
- Simple enough for automation scripts (pipe inputs to stdin with `--yes`)

## Future Enhancements (Out of Scope)

- **One-line installation script** (`curl -L https://get.nodepulse.sh | sudo bash`): Download, extract, and install the binary, then optionally run `pulse init`
- Auto-detect if running in Docker/container
- Support for Windows (PowerShell equivalent)
- Cloud provider metadata integration (AWS, GCP, Azure instance IDs)
- Config templates for different use cases
- Remote configuration pull from control server
- Wizard to create endpoint if user doesn't have one yet
