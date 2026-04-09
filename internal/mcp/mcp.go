// Package mcp provides an MCP (Model Context Protocol) server exposing active
// routes via JSON-RPC 2.0 over HTTP SSE transport. Allows AI agents to discover
// and interact with local development services.
//
// Author: Thiru K
// Module: github.com/thirukguru/localias/internal/mcp
package mcp

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"

	"github.com/thirukguru/localias/internal/health"
	"github.com/thirukguru/localias/internal/proxy"
	"github.com/thirukguru/localias/internal/traffic"
)

// contextKey is an unexported type for context keys in this package.
type contextKey int

const scopedTokenKey contextKey = iota

// Server implements the MCP protocol over HTTP SSE.
type Server struct {
	routes     *proxy.RouteTable
	health     *health.Checker
	traffic    *traffic.Logger
	logger     *slog.Logger
	adminToken string
	tokenStore *TokenStore
	idGen      atomic.Int64
}

// NewServer creates a new MCP server.
// stateDir is used to store/load the bearer token and scoped tokens.
func NewServer(routes *proxy.RouteTable, h *health.Checker, t *traffic.Logger, logger *slog.Logger, stateDir string) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	adminToken := ensureToken(stateDir, logger)
	tokenStore := NewTokenStore(stateDir, logger)
	return &Server{
		routes:     routes,
		health:     h,
		traffic:    t,
		logger:     logger,
		adminToken: adminToken,
		tokenStore: tokenStore,
	}
}

// TokenStore returns the token store for external use (e.g., RPC handlers).
func (s *Server) TokenStore() *TokenStore {
	return s.tokenStore
}

// TokenPath returns the path to the MCP token file.
func TokenPath(stateDir string) string {
	return filepath.Join(stateDir, "mcp-token")
}

// ensureToken loads or generates the bearer token.
func ensureToken(stateDir string, logger *slog.Logger) string {
	tokenFile := TokenPath(stateDir)

	// Try to read existing token
	if data, err := os.ReadFile(tokenFile); err == nil {
		token := strings.TrimSpace(string(data))
		if len(token) >= 32 {
			logger.Info("MCP token loaded", "path", tokenFile)
			return token
		}
	}

	// Generate new 32-byte hex token
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		logger.Error("failed to generate MCP token", "error", err)
		return ""
	}
	token := hex.EncodeToString(b)

	// Save with restrictive permissions (owner-only read)
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		logger.Error("failed to create state dir for MCP token", "error", err)
		return ""
	}
	if err := os.WriteFile(tokenFile, []byte(token+"\n"), 0600); err != nil {
		logger.Error("failed to write MCP token", "error", err)
	}
	logger.Info("MCP token generated", "path", tokenFile)
	return token
}

// Handler returns the HTTP handler for the MCP endpoints.
// All endpoints require a valid Bearer token.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /mcp", s.requireAuth(s.handleSSE))
	mux.HandleFunc("POST /mcp/message", s.requireAuth(s.handleMessage))
	return mux
}

// requireAuth wraps an HTTP handler with bearer token validation.
// Resolves the token as admin (global token) or scoped (TokenStore) and
// injects the ScopedToken into the request context.
func (s *Server) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	// Pre-build admin ScopedToken for the global token
	adminScoped := &ScopedToken{
		Token:        s.adminToken,
		Routes:       []string{"*"},
		Capabilities: []string{CapAll},
		Label:        "admin",
	}

	return func(w http.ResponseWriter, r *http.Request) {
		if s.adminToken == "" {
			// No token configured — allow all (fallback) with admin scope
			ctx := context.WithValue(r.Context(), scopedTokenKey, adminScoped)
			next(w, r.WithContext(ctx))
			return
		}

		auth := r.Header.Get("Authorization")
		if auth == "" {
			s.logger.Warn("MCP request rejected: no Authorization header", "remote", r.RemoteAddr)
			http.Error(w, `{"error":"Authorization header required. Use: Authorization: Bearer <token>"}`, http.StatusUnauthorized)
			return
		}

		bearer := extractBearer(auth)
		if bearer == "" {
			s.logger.Warn("MCP request rejected: malformed Authorization header", "remote", r.RemoteAddr)
			http.Error(w, `{"error":"Invalid Authorization header format. Use: Bearer <token>"}`, http.StatusUnauthorized)
			return
		}

		// Try admin token first (backward compat)
		if bearer == s.adminToken {
			ctx := context.WithValue(r.Context(), scopedTokenKey, adminScoped)
			next(w, r.WithContext(ctx))
			return
		}

		// Try scoped token
		scoped := s.tokenStore.Resolve(bearer)
		if scoped != nil {
			ctx := context.WithValue(r.Context(), scopedTokenKey, scoped)
			next(w, r.WithContext(ctx))
			return
		}

		s.logger.Warn("MCP request rejected: invalid token", "remote", r.RemoteAddr)
		http.Error(w, `{"error":"Invalid bearer token."}`, http.StatusUnauthorized)
	}
}

// extractBearer extracts the bearer token from an Authorization header value.
func extractBearer(auth string) string {
	parts := strings.SplitN(auth, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return strings.TrimSpace(parts[1])
}

// tokenFromContext retrieves the ScopedToken from the request context.
func tokenFromContext(ctx context.Context) *ScopedToken {
	if st, ok := ctx.Value(scopedTokenKey).(*ScopedToken); ok {
		return st
	}
	return nil
}

// JSON-RPC types
type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id"`
	Result  interface{} `json:"result,omitempty"`
	Error   *rpcError   `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// handleSSE provides the SSE output endpoint.
func (s *Server) handleSSE(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "SSE not supported", http.StatusInternalServerError)
		return
	}

	// Send initial capabilities
	init := map[string]interface{}{
		"protocolVersion": "2024-11-05",
		"capabilities": map[string]interface{}{
			"tools": map[string]interface{}{},
		},
		"serverInfo": map[string]interface{}{
			"name":    "localias",
			"version": "1.0.0",
		},
	}
	data, _ := json.Marshal(init)
	fmt.Fprintf(w, "data: %s\n\n", data)
	flusher.Flush()

	// Keep connection alive
	<-r.Context().Done()
}

// handleMessage processes incoming JSON-RPC 2.0 requests.
func (s *Server) handleMessage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1MB limit

	var req rpcRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		json.NewEncoder(w).Encode(rpcResponse{
			JSONRPC: "2.0",
			ID:      nil,
			Error:   &rpcError{Code: -32700, Message: "Parse error"},
		})
		return
	}

	var resp rpcResponse
	resp.JSONRPC = "2.0"
	resp.ID = req.ID

	switch req.Method {
	case "initialize":
		resp.Result = map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"capabilities":   map[string]interface{}{"tools": map[string]interface{}{}},
			"serverInfo":     map[string]interface{}{"name": "localias", "version": "1.0.0"},
		}
	case "tools/list":
		resp.Result = s.toolsList()
	case "tools/call":
		token := tokenFromContext(r.Context())
		resp.Result = s.toolsCall(req.Params, token)
	default:
		resp.Error = &rpcError{Code: -32601, Message: "Method not found: " + req.Method}
	}

	json.NewEncoder(w).Encode(resp)
}

func (s *Server) toolsList() interface{} {
	return map[string]interface{}{
		"tools": []map[string]interface{}{
			{
				"name":        "list_routes",
				"description": "List all active proxy routes with health and latency info",
				"inputSchema": map[string]interface{}{"type": "object", "properties": map[string]interface{}{}},
			},
			{
				"name":        "get_route",
				"description": "Get details for a specific route including recent traffic",
				"inputSchema": map[string]interface{}{
					"type":       "object",
					"properties": map[string]interface{}{"name": map[string]string{"type": "string", "description": "Route name"}},
					"required":   []string{"name"},
				},
			},
			{
				"name":        "register_route",
				"description": "Register a new static alias route",
				"inputSchema": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"name": map[string]string{"type": "string"},
						"port": map[string]string{"type": "number"},
					},
					"required": []string{"name", "port"},
				},
			},
			{
				"name":        "health_check",
				"description": "Run an immediate health check for a route",
				"inputSchema": map[string]interface{}{
					"type":       "object",
					"properties": map[string]interface{}{"name": map[string]string{"type": "string"}},
					"required":   []string{"name"},
				},
			},
		},
	}
}

func (s *Server) toolsCall(params json.RawMessage, token *ScopedToken) interface{} {
	var call struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(params, &call); err != nil {
		return map[string]interface{}{"content": []map[string]string{{"type": "text", "text": "Invalid params"}}}
	}

	switch call.Name {
	case "list_routes":
		if !token.HasCapability(CapRead) {
			return mcpError("Permission denied: requires 'read' capability")
		}
		routes := s.routes.List()
		var result []map[string]interface{}
		for _, r := range routes {
			if !token.CanAccessRoute(r.Name) {
				continue // filter to scoped routes
			}
			entry := map[string]interface{}{
				"name":         r.Name,
				"url":          r.URL,
				"backend_port": r.Port,
			}
			if s.health != nil {
				if status, ok := s.health.GetStatus(r.Name); ok {
					entry["healthy"] = status.Healthy
					entry["latency"] = status.Latency.String()
				}
			}
			result = append(result, entry)
		}
		if result == nil {
			result = []map[string]interface{}{}
		}
		data, _ := json.Marshal(result)
		return map[string]interface{}{"content": []map[string]string{{"type": "text", "text": string(data)}}}

	case "get_route":
		if !token.HasCapability(CapRead) {
			return mcpError("Permission denied: requires 'read' capability")
		}
		var args struct{ Name string `json:"name"` }
		if err := json.Unmarshal(call.Arguments, &args); err != nil {
			return mcpError("Invalid arguments: " + err.Error())
		}
		if !token.CanAccessRoute(args.Name) {
			return mcpError("Permission denied: route not in scope")
		}
		r, ok := s.routes.Lookup(args.Name)
		if !ok {
			return mcpError("Route not found")
		}
		result := map[string]interface{}{"name": r.Name, "url": r.URL, "backend_port": r.Port}
		if s.traffic != nil {
			entries := s.traffic.List(5, r.Name)
			result["recent_traffic"] = entries
		}
		data, _ := json.Marshal(result)
		return map[string]interface{}{"content": []map[string]string{{"type": "text", "text": string(data)}}}

	case "register_route":
		if !token.HasCapability(CapWrite) {
			return mcpError("Permission denied: requires 'write' capability")
		}
		var args struct {
			Name string `json:"name"`
			Port int    `json:"port"`
		}
		if err := json.Unmarshal(call.Arguments, &args); err != nil {
			return mcpError("Invalid arguments: " + err.Error())
		}
		if !token.CanAccessRoute(args.Name) {
			return mcpError("Permission denied: route name not in scope")
		}
		r, err := s.routes.Alias(args.Name, args.Port, false)
		if err != nil {
			return mcpError(err.Error())
		}
		return map[string]interface{}{"content": []map[string]string{{"type": "text", "text": "Registered: " + r.URL}}}

	case "health_check":
		if !token.HasCapability(CapHealth) {
			return mcpError("Permission denied: requires 'health' capability")
		}
		var args struct{ Name string `json:"name"` }
		if err := json.Unmarshal(call.Arguments, &args); err != nil {
			return mcpError("Invalid arguments: " + err.Error())
		}
		if !token.CanAccessRoute(args.Name) {
			return mcpError("Permission denied: route not in scope")
		}
		r, ok := s.routes.Lookup(args.Name)
		if !ok {
			return mcpError("Route not found")
		}
		if s.health == nil {
			return mcpError("Health checker not available")
		}
		status := s.health.CheckNow(args.Name, r.Port)
		data, _ := json.Marshal(status)
		return map[string]interface{}{"content": []map[string]string{{"type": "text", "text": string(data)}}}

	default:
		return mcpError("Unknown tool")
	}
}

// mcpError is a helper for returning MCP tool errors.
func mcpError(msg string) map[string]interface{} {
	return map[string]interface{}{
		"content": []map[string]string{{"type": "text", "text": msg}},
		"isError": true,
	}
}
