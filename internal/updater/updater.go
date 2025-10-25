package updater

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/node-pulse/agent/internal/logger"
)

const (
	// CurrentVersion is the current agent version
	// This will be set at build time via -ldflags
	CurrentVersion = "dev"
)

// VersionInfo represents version information from the update server
type VersionInfo struct {
	Version  string `json:"version"`
	URL      string `json:"url"`      // Download URL for the binary
	Checksum string `json:"checksum"` // SHA256 checksum
}

// Config represents updater configuration
type Config struct {
	UpdateEndpoint string        // URL to check for updates (e.g., https://api.nodepulse.io/agent/version)
	Timeout        time.Duration // HTTP timeout
	BinaryPath     string        // Path to current agent binary (e.g., /usr/local/bin/pulse)
	ServiceName    string        // Systemd service name (e.g., node-pulse)
}

// Updater handles agent updates
type Updater struct {
	config Config
	client *http.Client
}

// New creates a new updater
func New(cfg Config) *Updater {
	// Apply defaults
	if cfg.Timeout == 0 {
		cfg.Timeout = 10 * time.Second
	}
	if cfg.BinaryPath == "" {
		cfg.BinaryPath = "/usr/local/bin/pulse"
	}
	if cfg.ServiceName == "" {
		cfg.ServiceName = "node-pulse"
	}

	return &Updater{
		config: cfg,
		client: &http.Client{
			Timeout: cfg.Timeout,
		},
	}
}

// CheckAndUpdate checks for updates and performs update if available
// Returns (updated bool, error)
func (u *Updater) CheckAndUpdate() (bool, error) {
	logger.Info("Checking for updates", logger.String("current_version", CurrentVersion))

	// Step 1: Check for new version
	versionInfo, needsUpdate, err := u.checkVersion()
	if err != nil {
		return false, fmt.Errorf("failed to check version: %w", err)
	}

	if !needsUpdate {
		logger.Info("Agent is up to date", logger.String("version", CurrentVersion))
		return false, nil
	}

	logger.Info("New version available",
		logger.String("current", CurrentVersion),
		logger.String("new", versionInfo.Version))

	// Step 2: Download new binary
	tmpPath, err := u.downloadBinary(versionInfo)
	if err != nil {
		return false, fmt.Errorf("failed to download binary: %w", err)
	}
	defer os.Remove(tmpPath) // Clean up on error

	// Step 3: Verify checksum
	if err := u.verifyChecksum(tmpPath, versionInfo.Checksum); err != nil {
		return false, fmt.Errorf("checksum verification failed: %w", err)
	}

	logger.Info("Binary downloaded and verified", logger.String("path", tmpPath))

	// Step 4: Replace binary and restart service
	if err := u.replaceBinaryAndRestart(tmpPath); err != nil {
		return false, fmt.Errorf("failed to replace binary: %w", err)
	}

	logger.Info("Update completed successfully", logger.String("version", versionInfo.Version))
	return true, nil
}

// checkVersion queries the update server for version information
// Returns (versionInfo, needsUpdate bool, error)
func (u *Updater) checkVersion() (*VersionInfo, bool, error) {
	// Build URL with current version and platform info
	url := fmt.Sprintf("%s?version=%s&os=%s&arch=%s",
		u.config.UpdateEndpoint,
		CurrentVersion,
		runtime.GOOS,
		runtime.GOARCH)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, false, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", fmt.Sprintf("NodePulse-Agent/%s", CurrentVersion))

	resp, err := u.client.Do(req)
	if err != nil {
		return nil, false, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// 204 No Content = no update available
	if resp.StatusCode == http.StatusNoContent {
		return nil, false, nil
	}

	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var versionInfo VersionInfo
	if err := json.NewDecoder(resp.Body).Decode(&versionInfo); err != nil {
		return nil, false, fmt.Errorf("failed to decode response: %w", err)
	}

	// Validate response
	if versionInfo.Version == "" || versionInfo.URL == "" || versionInfo.Checksum == "" {
		return nil, false, fmt.Errorf("invalid version info response")
	}

	return &versionInfo, true, nil
}

// downloadBinary downloads the new binary to a temporary location
func (u *Updater) downloadBinary(versionInfo *VersionInfo) (string, error) {
	logger.Info("Downloading binary", logger.String("url", versionInfo.URL))

	resp, err := u.client.Get(versionInfo.URL)
	if err != nil {
		return "", fmt.Errorf("failed to download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download failed with status: %d", resp.StatusCode)
	}

	// Create temporary file
	tmpFile, err := os.CreateTemp("", "pulse-update-*")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}
	defer tmpFile.Close()

	// Download to temp file
	if _, err := io.Copy(tmpFile, resp.Body); err != nil {
		os.Remove(tmpFile.Name())
		return "", fmt.Errorf("failed to write binary: %w", err)
	}

	// Make executable
	if err := os.Chmod(tmpFile.Name(), 0755); err != nil {
		os.Remove(tmpFile.Name())
		return "", fmt.Errorf("failed to make binary executable: %w", err)
	}

	return tmpFile.Name(), nil
}

// verifyChecksum verifies the SHA256 checksum of the downloaded binary
func (u *Updater) verifyChecksum(path string, expectedChecksum string) error {
	logger.Info("Verifying checksum")

	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return fmt.Errorf("failed to calculate checksum: %w", err)
	}

	actualChecksum := hex.EncodeToString(hash.Sum(nil))

	if actualChecksum != expectedChecksum {
		return fmt.Errorf("checksum mismatch: expected %s, got %s", expectedChecksum, actualChecksum)
	}

	logger.Info("Checksum verified successfully")
	return nil
}

// replaceBinaryAndRestart replaces the current binary and restarts the service
func (u *Updater) replaceBinaryAndRestart(tmpPath string) error {
	logger.Info("Replacing binary and restarting service")

	// Step 1: Stop the service
	logger.Info("Stopping service", logger.String("service", u.config.ServiceName))
	if err := u.stopService(); err != nil {
		return fmt.Errorf("failed to stop service: %w", err)
	}

	// Step 2: Backup current binary
	backupPath := u.config.BinaryPath + ".backup"
	if err := u.backupBinary(backupPath); err != nil {
		// Try to restart service even if backup fails
		u.startService()
		return fmt.Errorf("failed to backup binary: %w", err)
	}

	// Step 3: Replace binary
	if err := u.replaceBinary(tmpPath); err != nil {
		// Try to restore from backup
		logger.Error("Failed to replace binary, attempting restore", logger.Err(err))
		if restoreErr := os.Rename(backupPath, u.config.BinaryPath); restoreErr != nil {
			logger.Error("Failed to restore backup", logger.Err(restoreErr))
		}
		u.startService()
		return fmt.Errorf("failed to replace binary: %w", err)
	}

	// Step 4: Start the service
	logger.Info("Starting service", logger.String("service", u.config.ServiceName))
	if err := u.startService(); err != nil {
		return fmt.Errorf("failed to start service: %w", err)
	}

	// Step 5: Clean up backup
	os.Remove(backupPath)

	return nil
}

// backupBinary creates a backup of the current binary
func (u *Updater) backupBinary(backupPath string) error {
	src, err := os.Open(u.config.BinaryPath)
	if err != nil {
		return fmt.Errorf("failed to open source: %w", err)
	}
	defer src.Close()

	dst, err := os.Create(backupPath)
	if err != nil {
		return fmt.Errorf("failed to create backup: %w", err)
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return fmt.Errorf("failed to copy: %w", err)
	}

	// Preserve permissions
	if info, err := os.Stat(u.config.BinaryPath); err == nil {
		os.Chmod(backupPath, info.Mode())
	}

	return nil
}

// replaceBinary atomically replaces the current binary
func (u *Updater) replaceBinary(tmpPath string) error {
	// Ensure parent directory exists
	dir := filepath.Dir(u.config.BinaryPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Atomic rename
	if err := os.Rename(tmpPath, u.config.BinaryPath); err != nil {
		return fmt.Errorf("failed to rename: %w", err)
	}

	return nil
}

// stopService stops the systemd service
func (u *Updater) stopService() error {
	cmd := exec.Command("systemctl", "stop", u.config.ServiceName)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("systemctl stop failed: %w (output: %s)", err, string(output))
	}

	// Wait for service to fully stop (max 10 seconds)
	for i := 0; i < 10; i++ {
		if !u.isServiceActive() {
			return nil
		}
		time.Sleep(1 * time.Second)
	}

	return fmt.Errorf("service did not stop in time")
}

// startService starts the systemd service
func (u *Updater) startService() error {
	cmd := exec.Command("systemctl", "start", u.config.ServiceName)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("systemctl start failed: %w (output: %s)", err, string(output))
	}

	// Wait for service to become active (max 10 seconds)
	for i := 0; i < 10; i++ {
		if u.isServiceActive() {
			return nil
		}
		time.Sleep(1 * time.Second)
	}

	return fmt.Errorf("service did not start in time")
}

// isServiceActive checks if the systemd service is active
func (u *Updater) isServiceActive() bool {
	cmd := exec.Command("systemctl", "is-active", "--quiet", u.config.ServiceName)
	return cmd.Run() == nil
}
