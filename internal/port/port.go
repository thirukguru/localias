// Package port provides utilities for finding free TCP ports in a specified range.
// It is used by localias to assign available ports to local development servers.
//
// Author: Thiru K
// Module: github.com/thirukguru/localias/internal/port
package port

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"sync"
)

const (
	// DefaultRangeStart is the beginning of the default port allocation range.
	DefaultRangeStart = 4000
	// DefaultRangeEnd is the end of the default port allocation range.
	DefaultRangeEnd = 4999
)

// usedPorts tracks ports that have been assigned in this process to avoid conflicts
// when multiple ports are allocated before any are actually bound.
var (
	usedPorts   = make(map[int]bool)
	usedPortsMu sync.Mutex
)

// FindFree finds an available TCP port within the range [start, end].
// If start and end are both 0, the default range (4000-4999) is used.
// It returns the port number or an error if no port is available.
func FindFree(start, end int) (int, error) {
	if start == 0 && end == 0 {
		start = DefaultRangeStart
		end = DefaultRangeEnd
	}
	if start > end {
		return 0, fmt.Errorf("invalid port range: %d-%d", start, end)
	}

	usedPortsMu.Lock()
	defer usedPortsMu.Unlock()

	for port := start; port <= end; port++ {
		if usedPorts[port] {
			continue
		}
		if isPortAvailable(port) {
			usedPorts[port] = true
			return port, nil
		}
	}
	return 0, fmt.Errorf("no free port found in range %d-%d", start, end)
}

// FindFreeFromEnv checks LOCALIAS_APP_PORT env var first.
// If set, it validates and returns that port. Otherwise it calls FindFree.
func FindFreeFromEnv() (int, error) {
	if envPort := os.Getenv("LOCALIAS_APP_PORT"); envPort != "" {
		p, err := strconv.Atoi(envPort)
		if err != nil {
			return 0, fmt.Errorf("invalid LOCALIAS_APP_PORT %q: %w", envPort, err)
		}
		if p < 1 || p > 65535 {
			return 0, fmt.Errorf("LOCALIAS_APP_PORT %d out of valid range (1-65535)", p)
		}
		return p, nil
	}
	return FindFree(0, 0)
}

// Release marks a port as no longer in use, allowing it to be reassigned.
func Release(port int) {
	usedPortsMu.Lock()
	defer usedPortsMu.Unlock()
	delete(usedPorts, port)
}

// ResetUsed clears all tracked used ports. Primarily for testing.
func ResetUsed() {
	usedPortsMu.Lock()
	defer usedPortsMu.Unlock()
	usedPorts = make(map[int]bool)
}

// isPortAvailable checks if a TCP port is available by attempting to listen on it.
func isPortAvailable(port int) bool {
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return false
	}
	ln.Close()
	return true
}
