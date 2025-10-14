package metrics

import (
	"bufio"
	"os"
	"runtime"
	"strings"
)

// SystemInfo represents static system information
type SystemInfo struct {
	Hostname     string `json:"hostname"`
	Kernel       string `json:"kernel"`
	KernelVer    string `json:"kernel_version"`
	Distro       string `json:"distro"`
	DistroVer    string `json:"distro_version"`
	Architecture string `json:"architecture"`
	CPUCores     int    `json:"cpu_cores"`
}

var cachedSystemInfo *SystemInfo

// CollectSystemInfo collects static system information
// This is cached after first call since it doesn't change
func CollectSystemInfo() (*SystemInfo, error) {
	if cachedSystemInfo != nil {
		return cachedSystemInfo, nil
	}

	info := &SystemInfo{
		Architecture: runtime.GOARCH,
		CPUCores:     runtime.NumCPU(),
	}

	// Get hostname
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}
	info.Hostname = hostname

	// Get kernel info from /proc/version
	if kernel, version := readKernelInfo(); kernel != "" {
		info.Kernel = kernel
		info.KernelVer = version
	}

	// Get distro info from /etc/os-release
	if distro, version := readOSRelease(); distro != "" {
		info.Distro = distro
		info.DistroVer = version
	}

	cachedSystemInfo = info
	return info, nil
}

// readKernelInfo reads kernel information from /proc/version
func readKernelInfo() (string, string) {
	data, err := os.ReadFile("/proc/version")
	if err != nil {
		return "Linux", "unknown"
	}

	line := string(data)
	// Example: "Linux version 5.15.0-89-generic (buildd@lcy02-amd64-026) ..."
	if strings.HasPrefix(line, "Linux version ") {
		parts := strings.Fields(line)
		if len(parts) >= 3 {
			return "Linux", parts[2] // e.g., "5.15.0-89-generic"
		}
	}

	return "Linux", "unknown"
}

// readOSRelease reads distribution information from /etc/os-release
func readOSRelease() (string, string) {
	file, err := os.Open("/etc/os-release")
	if err != nil {
		// Try alternative location
		file, err = os.Open("/usr/lib/os-release")
		if err != nil {
			return "Linux", "unknown"
		}
	}
	defer file.Close()

	var name, version, prettyName string
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "NAME=") {
			name = strings.Trim(strings.TrimPrefix(line, "NAME="), "\"")
		} else if strings.HasPrefix(line, "VERSION=") {
			version = strings.Trim(strings.TrimPrefix(line, "VERSION="), "\"")
		} else if strings.HasPrefix(line, "PRETTY_NAME=") {
			prettyName = strings.Trim(strings.TrimPrefix(line, "PRETTY_NAME="), "\"")
		}
	}

	// Prefer NAME field, fallback to PRETTY_NAME
	if name == "" {
		name = prettyName
	}
	if name == "" {
		name = "Linux"
	}

	if version == "" {
		version = "unknown"
	}

	return name, version
}

// ResetSystemInfoCache clears the cached system info (useful for testing)
func ResetSystemInfoCache() {
	cachedSystemInfo = nil
}
