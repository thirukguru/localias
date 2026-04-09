// Package mcp — token store for scoped MCP authentication.
// Manages per-route, per-capability bearer tokens with optional PID-based
// ephemeral lifecycle.
//
// Author: Thiru K
// Module: github.com/thirukguru/localias/internal/mcp
package mcp

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Capability constants for scoped tokens.
const (
	CapRead  = "read"   // list_routes, get_route (filtered)
	CapWrite = "write"  // register_route (scoped)
	CapHealth = "health" // health_check (scoped)
	CapAll   = "*"      // admin — all tools, all routes
)

// ScopedToken represents a bearer token with route and capability restrictions.
type ScopedToken struct {
	Token        string    `json:"token"`
	Routes       []string  `json:"routes"`        // route name patterns; "*" = all
	Capabilities []string  `json:"capabilities"`  // "read", "write", "health", "*"
	PID          int       `json:"pid,omitempty"` // 0 = persistent, >0 = ephemeral
	Label        string    `json:"label,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

// IsAdmin returns true if this token has full access.
func (t *ScopedToken) IsAdmin() bool {
	for _, c := range t.Capabilities {
		if c == CapAll {
			return true
		}
	}
	for _, r := range t.Routes {
		if r == "*" {
			// Check if all caps are present
			if t.HasCapability(CapRead) && t.HasCapability(CapWrite) && t.HasCapability(CapHealth) {
				return true
			}
		}
	}
	return false
}

// HasCapability checks if the token has a specific capability.
func (t *ScopedToken) HasCapability(cap string) bool {
	for _, c := range t.Capabilities {
		if c == CapAll || c == cap {
			return true
		}
	}
	return false
}

// CanAccessRoute checks if the token is allowed to access a specific route.
func (t *ScopedToken) CanAccessRoute(routeName string) bool {
	routeName = strings.ToLower(routeName)
	for _, r := range t.Routes {
		if r == "*" || strings.ToLower(r) == routeName {
			return true
		}
	}
	return false
}

// TokenStore manages scoped MCP tokens with file-backed persistence.
type TokenStore struct {
	mu     sync.RWMutex
	tokens map[string]*ScopedToken // token string → ScopedToken
	path   string                  // path to mcp-tokens.json
	logger *slog.Logger
}

// NewTokenStore creates a token store, loading existing tokens from disk
// and pruning any ephemeral tokens whose PIDs are dead.
func NewTokenStore(stateDir string, logger *slog.Logger) *TokenStore {
	if logger == nil {
		logger = slog.Default()
	}
	ts := &TokenStore{
		tokens: make(map[string]*ScopedToken),
		path:   filepath.Join(stateDir, "mcp-tokens.json"),
		logger: logger,
	}
	ts.load()
	ts.Prune()
	return ts
}

// Create generates a new scoped token and persists it.
func (ts *TokenStore) Create(routes, capabilities []string, pid int, label string) *ScopedToken {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		ts.logger.Error("failed to generate scoped token", "error", err)
		return nil
	}
	tokenStr := hex.EncodeToString(b)

	st := &ScopedToken{
		Token:        tokenStr,
		Routes:       routes,
		Capabilities: capabilities,
		PID:          pid,
		Label:        label,
		CreatedAt:    time.Now(),
	}

	ts.tokens[tokenStr] = st
	ts.persistLocked()

	ts.logger.Info("scoped MCP token created",
		"routes", routes,
		"capabilities", capabilities,
		"pid", pid,
		"label", label,
		"prefix", tokenStr[:8]+"...",
	)

	return st
}

// Resolve looks up a bearer token and returns the ScopedToken if valid.
// Returns nil if the token is not found or the associated PID is dead.
func (ts *TokenStore) Resolve(bearerToken string) *ScopedToken {
	ts.mu.RLock()
	st, ok := ts.tokens[bearerToken]
	ts.mu.RUnlock()

	if !ok {
		return nil
	}

	// Check PID liveness for ephemeral tokens
	if st.PID > 0 && !isPIDAlive(st.PID) {
		ts.mu.Lock()
		delete(ts.tokens, bearerToken)
		ts.persistLocked()
		ts.mu.Unlock()
		ts.logger.Info("ephemeral MCP token pruned (PID dead)", "pid", st.PID, "prefix", bearerToken[:8]+"...")
		return nil
	}

	return st
}

// Revoke removes tokens matching the given prefix.
// Returns the number of tokens revoked.
func (ts *TokenStore) Revoke(prefix string) int {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	count := 0
	for token := range ts.tokens {
		if strings.HasPrefix(token, prefix) {
			delete(ts.tokens, token)
			count++
		}
	}

	if count > 0 {
		ts.persistLocked()
		ts.logger.Info("MCP tokens revoked", "prefix", prefix, "count", count)
	}

	return count
}

// Prune removes all ephemeral tokens whose PIDs are no longer alive.
func (ts *TokenStore) Prune() {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	pruned := 0
	for token, st := range ts.tokens {
		if st.PID > 0 && !isPIDAlive(st.PID) {
			delete(ts.tokens, token)
			pruned++
		}
	}

	if pruned > 0 {
		ts.persistLocked()
		ts.logger.Info("pruned dead ephemeral MCP tokens", "count", pruned)
	}
}

// List returns all current tokens (for admin display).
func (ts *TokenStore) List() []*ScopedToken {
	ts.mu.RLock()
	defer ts.mu.RUnlock()

	result := make([]*ScopedToken, 0, len(ts.tokens))
	for _, st := range ts.tokens {
		result = append(result, st)
	}
	return result
}

// Count returns the number of tokens in the store.
func (ts *TokenStore) Count() int {
	ts.mu.RLock()
	defer ts.mu.RUnlock()
	return len(ts.tokens)
}

// load reads tokens from disk.
func (ts *TokenStore) load() {
	data, err := os.ReadFile(ts.path)
	if err != nil {
		if !os.IsNotExist(err) {
			ts.logger.Warn("failed to read MCP tokens file", "error", err)
		}
		return
	}

	var tokens map[string]*ScopedToken
	if err := json.Unmarshal(data, &tokens); err != nil {
		ts.logger.Warn("failed to parse MCP tokens file", "error", err)
		return
	}

	ts.tokens = tokens
	ts.logger.Info("loaded MCP tokens", "count", len(tokens))
}

// persistLocked writes tokens to disk. Must be called with mu held.
func (ts *TokenStore) persistLocked() {
	data, err := json.MarshalIndent(ts.tokens, "", "  ")
	if err != nil {
		ts.logger.Error("failed to marshal MCP tokens", "error", err)
		return
	}
	if err := os.WriteFile(ts.path, data, 0600); err != nil {
		fmt.Fprintf(os.Stderr, "localias: failed to persist MCP tokens: %v\n", err)
	}
}

// isPIDAlive checks if a process with the given PID is still running.
func isPIDAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || err == syscall.EPERM
}
