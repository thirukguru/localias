// Package proxy implements the core reverse proxy for localias.
// This file provides the route table — a thread-safe mapping of names to backend ports
// with support for wildcard subdomain matching.
//
// Author: Thiru K
// Module: github.com/thirukguru/localias/internal/proxy
package proxy

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
)

// validNameRegex allows only lowercase alphanumeric, hyphens, and dots.
var validNameRegex = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]*$`)

// Route represents a single proxy route mapping a name to a backend port.
type Route struct {
	Name      string    `json:"name"`
	Port      int       `json:"port"`
	URL       string    `json:"url"`
	PID       int       `json:"pid,omitempty"`
	Cmd       string    `json:"cmd,omitempty"`
	Static    bool      `json:"static"`
	CreatedAt time.Time `json:"created_at"`
}

// RouteTable manages the mapping of names to backend ports.
// It is safe for concurrent use.
type RouteTable struct {
	mu        sync.RWMutex
	routes    map[string]*Route
	proxyPort int
	https     bool
	persPath  string // path to routes.json for persistence
}

// NewRouteTable creates a new empty route table.
func NewRouteTable(proxyPort int, https bool, persistPath string) *RouteTable {
	rt := &RouteTable{
		routes:    make(map[string]*Route),
		proxyPort: proxyPort,
		https:     https,
		persPath:  persistPath,
	}
	rt.loadPersisted()
	return rt
}

// Register adds or updates a route in the table.
func (rt *RouteTable) Register(name string, port int, pid int, cmd string) (*Route, error) {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	name = strings.ToLower(name)
	if name == "" {
		return nil, fmt.Errorf("route name cannot be empty")
	}
	if !validNameRegex.MatchString(name) {
		return nil, fmt.Errorf("route name %q contains invalid characters (only a-z, 0-9, hyphens, and dots allowed)", name)
	}

	scheme := "http"
	if rt.https {
		scheme = "https"
	}

	r := &Route{
		Name:      name,
		Port:      port,
		URL:       fmt.Sprintf("%s://%s.localhost:%d", scheme, name, rt.proxyPort),
		PID:       pid,
		Cmd:       cmd,
		Static:    false,
		CreatedAt: time.Now(),
	}
	rt.routes[name] = r
	return r, nil
}

// Alias adds a static route (for Docker, external processes, etc.).
func (rt *RouteTable) Alias(name string, port int, force bool) (*Route, error) {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	name = strings.ToLower(name)
	if name == "" {
		return nil, fmt.Errorf("route name cannot be empty")
	}
	if !validNameRegex.MatchString(name) {
		return nil, fmt.Errorf("route name %q contains invalid characters (only a-z, 0-9, hyphens, and dots allowed)", name)
	}

	if existing, ok := rt.routes[name]; ok && !force {
		return nil, fmt.Errorf("route %q already exists (port %d), use --force to overwrite", name, existing.Port)
	}

	scheme := "http"
	if rt.https {
		scheme = "https"
	}

	r := &Route{
		Name:      name,
		Port:      port,
		URL:       fmt.Sprintf("%s://%s.localhost:%d", scheme, name, rt.proxyPort),
		Static:    true,
		CreatedAt: time.Now(),
	}
	rt.routes[name] = r
	rt.persistLocked()
	return r, nil
}

// Deregister removes a route by name.
func (rt *RouteTable) Deregister(name string) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	name = strings.ToLower(name)
	delete(rt.routes, name)
}

// Unalias removes a static route by name.
func (rt *RouteTable) Unalias(name string) error {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	name = strings.ToLower(name)
	r, ok := rt.routes[name]
	if !ok {
		return fmt.Errorf("route %q not found", name)
	}
	if !r.Static {
		return fmt.Errorf("route %q is not a static alias", name)
	}
	delete(rt.routes, name)
	rt.persistLocked()
	return nil
}

// Lookup finds a route by name, supporting wildcard subdomain fallback.
// For example, "tenant1.myapp" checks "tenant1.myapp" first, then "myapp".
func (rt *RouteTable) Lookup(name string) (*Route, bool) {
	rt.mu.RLock()
	defer rt.mu.RUnlock()

	name = strings.ToLower(name)

	// Exact match first
	if r, ok := rt.routes[name]; ok {
		return r, true
	}

	// Wildcard fallback: strip leading subdomain segments
	parts := strings.SplitN(name, ".", 2)
	for len(parts) == 2 {
		parent := parts[1]
		if r, ok := rt.routes[parent]; ok {
			return r, true
		}
		parts = strings.SplitN(parent, ".", 2)
	}

	return nil, false
}

// List returns all registered routes.
func (rt *RouteTable) List() []*Route {
	rt.mu.RLock()
	defer rt.mu.RUnlock()

	routes := make([]*Route, 0, len(rt.routes))
	for _, r := range rt.routes {
		routes = append(routes, r)
	}
	return routes
}

// Has checks if a route name is registered.
func (rt *RouteTable) Has(name string) bool {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	_, ok := rt.routes[strings.ToLower(name)]
	return ok
}

// Count returns the number of registered routes.
func (rt *RouteTable) Count() int {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	return len(rt.routes)
}

// persistLocked saves static routes to routes.json. Must be called with lock held.
func (rt *RouteTable) persistLocked() {
	if rt.persPath == "" {
		return
	}
	staticRoutes := make(map[string]*Route)
	for name, r := range rt.routes {
		if r.Static {
			staticRoutes[name] = r
		}
	}
	data, err := json.MarshalIndent(staticRoutes, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "localias: failed to marshal routes: %v\n", err)
		return
	}
	if err := os.WriteFile(rt.persPath, data, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "localias: failed to persist routes: %v\n", err)
	}
}

// loadPersisted loads static routes from routes.json.
func (rt *RouteTable) loadPersisted() {
	if rt.persPath == "" {
		return
	}
	data, err := os.ReadFile(rt.persPath)
	if err != nil {
		return
	}
	var routes map[string]*Route
	if err := json.Unmarshal(data, &routes); err != nil {
		return
	}
	for name, r := range routes {
		r.Static = true
		rt.routes[name] = r
	}
}

// LoadStaticRoutes reloads static routes from the given file path.
// Used by the fsnotify watcher to reload on external changes.
func (rt *RouteTable) LoadStaticRoutes(filePath string) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("reading routes file: %w", err)
	}
	var routes map[string]*Route
	if err := json.Unmarshal(data, &routes); err != nil {
		return fmt.Errorf("parsing routes file: %w", err)
	}
	rt.mu.Lock()
	defer rt.mu.Unlock()
	// Remove old static routes
	for name, r := range rt.routes {
		if r.Static {
			delete(rt.routes, name)
		}
	}
	// Add new static routes
	for name, r := range routes {
		r.Static = true
		rt.routes[name] = r
	}
	return nil
}
