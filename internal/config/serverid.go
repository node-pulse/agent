package config

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	serverIDFileName = "server_id"
)

// EnsureServerID ensures a server ID exists, generating one if needed
// Priority:
// 1. Config file value (if valid)
// 2. Persisted file value
// 3. Auto-generate new UUID and persist it
func EnsureServerID(cfg *Config) error {
	// If config has a valid server ID that's not the placeholder, use it
	if cfg.Agent.ServerID != "" && cfg.Agent.ServerID != "00000000-0000-0000-0000-000000000000" {
		if isValidServerID(cfg.Agent.ServerID) {
			return nil
		}
	}

	// Try to load from persisted file
	serverIDPath := GetServerIDPath()
	if persistedID, err := loadServerID(serverIDPath); err == nil && isValidServerID(persistedID) {
		cfg.Agent.ServerID = persistedID
		return nil
	}

	// Generate new UUID
	newID, err := generateUUID()
	if err != nil {
		return fmt.Errorf("failed to generate server ID: %w", err)
	}

	// Persist it
	if err := saveServerID(serverIDPath, newID); err != nil {
		return fmt.Errorf("failed to save server ID: %w", err)
	}

	cfg.Agent.ServerID = newID
	return nil
}

// GetServerIDPath returns the path where server_id is persisted
func GetServerIDPath() string {
	// Try standard locations
	locations := []string{
		"/var/lib/node-pulse/server_id",
		"/etc/node-pulse/server_id",
		filepath.Join(os.Getenv("HOME"), ".node-pulse", "server_id"),
		"./server_id", // Fallback to current directory
	}

	// Use first writable location
	for _, path := range locations {
		dir := filepath.Dir(path)
		if isWritable(dir) {
			return path
		}
	}

	// Last resort: current directory
	return "./server_id"
}

// loadServerID loads server ID from file
func loadServerID(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	id := strings.TrimSpace(string(data))
	if !isValidServerID(id) {
		return "", fmt.Errorf("invalid server ID format in file")
	}

	return id, nil
}

// saveServerID saves server ID to file
func saveServerID(path string, id string) error {
	// Create directory if needed
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	// Write with restricted permissions
	return os.WriteFile(path, []byte(id+"\n"), 0600)
}

// GenerateUUID generates a new UUID v4
func GenerateUUID() (string, error) {
	uuid := make([]byte, 16)
	if _, err := rand.Read(uuid); err != nil {
		return "", err
	}

	// Set version (4) and variant bits
	uuid[6] = (uuid[6] & 0x0f) | 0x40 // Version 4
	uuid[8] = (uuid[8] & 0x3f) | 0x80 // Variant is 10

	// Format as 8-4-4-4-12
	return fmt.Sprintf("%s-%s-%s-%s-%s",
		hex.EncodeToString(uuid[0:4]),
		hex.EncodeToString(uuid[4:6]),
		hex.EncodeToString(uuid[6:8]),
		hex.EncodeToString(uuid[8:10]),
		hex.EncodeToString(uuid[10:16]),
	), nil
}

// generateUUID is an internal alias for GenerateUUID (for backwards compatibility)
func generateUUID() (string, error) {
	return GenerateUUID()
}

// isWritable checks if a directory is writable
func isWritable(dir string) bool {
	info, err := os.Stat(dir)
	if err != nil {
		return false
	}

	if !info.IsDir() {
		return false
	}

	// Try to create a test file
	testFile := filepath.Join(dir, ".write-test")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		return false
	}
	os.Remove(testFile)

	return true
}
