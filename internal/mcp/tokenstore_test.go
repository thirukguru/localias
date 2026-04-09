package mcp

import (
	"log/slog"
	"os"
	"testing"
)

func TestTokenStore_CreateAndResolve(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	ts := NewTokenStore(dir, logger)

	st := ts.Create([]string{"frontend", "api"}, []string{CapRead, CapHealth}, 0, "test token")
	if st == nil {
		t.Fatal("expected non-nil scoped token")
	}
	if len(st.Token) != 64 { // 32 bytes = 64 hex chars
		t.Errorf("expected 64-char hex token, got %d chars", len(st.Token))
	}

	// Resolve it
	resolved := ts.Resolve(st.Token)
	if resolved == nil {
		t.Fatal("expected to resolve created token")
	}
	if resolved.Label != "test token" {
		t.Errorf("expected label 'test token', got %q", resolved.Label)
	}
}

func TestTokenStore_ResolveUnknown(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	ts := NewTokenStore(dir, slog.Default())

	if ts.Resolve("nonexistent") != nil {
		t.Error("expected nil for unknown token")
	}
}

func TestTokenStore_Revoke(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	ts := NewTokenStore(dir, slog.Default())

	st := ts.Create([]string{"app1"}, []string{CapRead}, 0, "")
	prefix := st.Token[:8]

	count := ts.Revoke(prefix)
	if count != 1 {
		t.Errorf("expected 1 revoked, got %d", count)
	}

	if ts.Resolve(st.Token) != nil {
		t.Error("expected nil after revoke")
	}
}

func TestTokenStore_RevokeNonExistent(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	ts := NewTokenStore(dir, slog.Default())

	count := ts.Revoke("zzzzzz")
	if count != 0 {
		t.Errorf("expected 0 revoked, got %d", count)
	}
}

func TestTokenStore_EphemeralPIDToken(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	ts := NewTokenStore(dir, slog.Default())

	// Create with current PID → should resolve
	st := ts.Create([]string{"myapp"}, []string{CapRead}, os.Getpid(), "ephemeral")
	if ts.Resolve(st.Token) == nil {
		t.Error("expected to resolve token for live PID")
	}

	// Create with a dead PID → should be pruned on Resolve
	deadSt := ts.Create([]string{"myapp"}, []string{CapRead}, 99999999, "dead")
	if ts.Resolve(deadSt.Token) != nil {
		t.Error("expected nil for dead PID token")
	}
}

func TestTokenStore_Prune(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	ts := NewTokenStore(dir, slog.Default())

	// Create tokens: one alive, one dead
	ts.Create([]string{"live"}, []string{CapRead}, os.Getpid(), "live")
	ts.Create([]string{"dead"}, []string{CapRead}, 99999999, "dead")

	if ts.Count() != 2 {
		t.Fatalf("expected 2 tokens before prune, got %d", ts.Count())
	}

	ts.Prune()

	if ts.Count() != 1 {
		t.Errorf("expected 1 token after prune, got %d", ts.Count())
	}
}

func TestTokenStore_Persistence(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))

	// Create tokens then reload
	ts1 := NewTokenStore(dir, logger)
	st := ts1.Create([]string{"app1"}, []string{CapRead, CapWrite}, 0, "persistent")

	// Load from same dir
	ts2 := NewTokenStore(dir, logger)
	resolved := ts2.Resolve(st.Token)
	if resolved == nil {
		t.Fatal("expected token to persist across store instances")
	}
	if resolved.Label != "persistent" {
		t.Errorf("expected label 'persistent', got %q", resolved.Label)
	}
}

func TestTokenStore_List(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	ts := NewTokenStore(dir, slog.Default())

	ts.Create([]string{"a"}, []string{CapRead}, 0, "")
	ts.Create([]string{"b"}, []string{CapRead}, 0, "")
	ts.Create([]string{"c"}, []string{CapRead}, 0, "")

	list := ts.List()
	if len(list) != 3 {
		t.Errorf("expected 3 tokens, got %d", len(list))
	}
}

func TestScopedToken_HasCapability(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		caps     []string
		check    string
		expected bool
	}{
		{"has read", []string{CapRead, CapHealth}, CapRead, true},
		{"no write", []string{CapRead, CapHealth}, CapWrite, false},
		{"admin all", []string{CapAll}, CapWrite, true},
		{"admin all health", []string{CapAll}, CapHealth, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := &ScopedToken{Capabilities: tt.caps}
			if got := st.HasCapability(tt.check); got != tt.expected {
				t.Errorf("HasCapability(%q) = %v, want %v", tt.check, got, tt.expected)
			}
		})
	}
}

func TestScopedToken_CanAccessRoute(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		routes   []string
		check    string
		expected bool
	}{
		{"exact match", []string{"frontend", "api"}, "frontend", true},
		{"no match", []string{"frontend", "api"}, "database", false},
		{"wildcard", []string{"*"}, "anything", true},
		{"case insensitive", []string{"Frontend"}, "frontend", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := &ScopedToken{Routes: tt.routes}
			if got := st.CanAccessRoute(tt.check); got != tt.expected {
				t.Errorf("CanAccessRoute(%q) = %v, want %v", tt.check, got, tt.expected)
			}
		})
	}
}

func TestScopedToken_IsAdmin(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		routes   []string
		caps     []string
		expected bool
	}{
		{"cap all", []string{"app"}, []string{CapAll}, true},
		{"all routes all caps", []string{"*"}, []string{CapRead, CapWrite, CapHealth}, true},
		{"limited scope", []string{"app"}, []string{CapRead}, false},
		{"all routes read only", []string{"*"}, []string{CapRead}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := &ScopedToken{Routes: tt.routes, Capabilities: tt.caps}
			if got := st.IsAdmin(); got != tt.expected {
				t.Errorf("IsAdmin() = %v, want %v", got, tt.expected)
			}
		})
	}
}
