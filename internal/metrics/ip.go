package metrics

import (
	"bufio"
	"net"
	"os"
	"strings"

	"github.com/node-pulse/agent/internal/logger"
)

// collectIPAddresses collects IPv4 and IPv6 addresses for the server
// Returns (ipv4, ipv6) where either can be empty string if not available
func collectIPAddresses() (string, string) {
	var ipv4, ipv6 string

	// Try method 1: Parse network interfaces
	if v4, v6 := getIPFromInterfaces(); v4 != "" || v6 != "" {
		ipv4, ipv6 = v4, v6
		return ipv4, ipv6
	}

	// Try method 2: Parse /proc/net/route and /proc/net/ipv6_route for default routes
	if v4, v6 := getIPFromRoutes(); v4 != "" || v6 != "" {
		ipv4, ipv6 = v4, v6
		return ipv4, ipv6
	}

	logger.Debug("Could not determine IP addresses from any method")
	return ipv4, ipv6
}

// getIPFromInterfaces gets the IP addresses from network interfaces
func getIPFromInterfaces() (string, string) {
	var ipv4, ipv6 string

	ifaces, err := net.Interfaces()
	if err != nil {
		logger.Debug("Failed to get network interfaces", logger.Err(err))
		return ipv4, ipv6
	}

	// Prioritize non-loopback, up interfaces
	for _, iface := range ifaces {
		// Skip down interfaces and loopback
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			default:
				continue
			}

			// Skip loopback addresses
			if ip.IsLoopback() {
				continue
			}

			// Check if IPv4
			if ip.To4() != nil && ipv4 == "" {
				ipv4 = ip.String()
			}

			// Check if IPv6 (and not IPv4-mapped)
			if ip.To4() == nil && ipv6 == "" {
				// Skip link-local addresses (fe80::/10)
				if !ip.IsLinkLocalUnicast() {
					ipv6 = ip.String()
				}
			}

			// If we found both, we're done
			if ipv4 != "" && ipv6 != "" {
				return ipv4, ipv6
			}
		}
	}

	return ipv4, ipv6
}

// getIPFromRoutes gets IP addresses by finding default route interfaces
func getIPFromRoutes() (string, string) {
	var ipv4, ipv6 string

	// Get IPv4 from default route
	if iface := getDefaultRouteInterface("/proc/net/route"); iface != "" {
		ipv4 = getInterfaceIP(iface, false)
	}

	// Get IPv6 from default route
	if iface := getDefaultIPv6RouteInterface(); iface != "" {
		ipv6 = getInterfaceIP(iface, true)
	}

	return ipv4, ipv6
}

// getDefaultRouteInterface finds the interface name for the default route from /proc/net/route
func getDefaultRouteInterface(path string) string {
	file, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	// Skip header
	scanner.Scan()

	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 8 {
			continue
		}

		// Check if destination is 00000000 (default route)
		if fields[1] == "00000000" {
			return fields[0] // Interface name
		}
	}

	return ""
}

// getDefaultIPv6RouteInterface finds the interface name for the default IPv6 route
func getDefaultIPv6RouteInterface() string {
	file, err := os.Open("/proc/net/ipv6_route")
	if err != nil {
		return ""
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 10 {
			continue
		}

		// Check if destination is 00000000000000000000000000000000 (default route)
		if fields[0] == "00000000000000000000000000000000" {
			return fields[9] // Interface name
		}
	}

	return ""
}

// getInterfaceIP gets the IP address for a specific interface
func getInterfaceIP(ifaceName string, ipv6 bool) string {
	iface, err := net.InterfaceByName(ifaceName)
	if err != nil {
		return ""
	}

	addrs, err := iface.Addrs()
	if err != nil {
		return ""
	}

	for _, addr := range addrs {
		var ip net.IP
		switch v := addr.(type) {
		case *net.IPNet:
			ip = v.IP
		case *net.IPAddr:
			ip = v.IP
		default:
			continue
		}

		if ipv6 {
			// Looking for IPv6
			if ip.To4() == nil && !ip.IsLoopback() && !ip.IsLinkLocalUnicast() {
				return ip.String()
			}
		} else {
			// Looking for IPv4
			if ip.To4() != nil && !ip.IsLoopback() {
				return ip.String()
			}
		}
	}

	return ""
}
