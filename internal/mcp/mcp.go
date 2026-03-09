// Package mcp provides an MCP (Model Context Protocol) server exposing active
// routes via JSON-RPC 2.0 over HTTP SSE transport. Allows AI agents to discover
// and interact with local development services.
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
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"

	"github.com/thirukguru/localias/internal/health"
	"github.com/thirukguru/localias/internal/proxy"
	"github.com/thirukguru/localias/internal/traffic"
)

// Server implements the MCP protocol over HTTP SSE.
type Server struct {
	routes  *proxy.RouteTable
	health  *health.Checker
	traffic *traffic.Logger
	logger  *slog.Logger
	token   string
	idGen   atomic.Int64
}

// NewServer creates a new MCP server.
// stateDir is used to store/load the bearer token.
func NewServer(routes *proxy.RouteTable, h *health.Checker, t *traffic.Logger, logger *slog.Logger, stateDir string) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	token := ensureToken(stateDir, logger)
	return &Server{
		routes:  routes,
		health:  h,
		traffic: t,
		logger:  logger,
		token:   token,
	}
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
	os.MkdirAll(stateDir, 0755)
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
func (s *Server) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.token == "" {
			// No token configured — allow all (fallback)
			next(w, r)
			return
		}

		auth := r.Header.Get("Authorization")
		if auth == "" {
			s.logger.Warn("MCP request rejected: no Authorization header", "remote", r.RemoteAddr)
			http.Error(w, `{"error":"Authorization header required. Use: Authorization: Bearer <token>"}`, http.StatusUnauthorized)
			return
		}

		parts := strings.SplitN(auth, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || parts[1] != s.token {
			s.logger.Warn("MCP request rejected: invalid token", "remote", r.RemoteAddr)
			http.Error(w, `{"error":"Invalid bearer token. Read token from: ~/.localias/mcp-token"}`, http.StatusUnauthorized)
			return
		}

		next(w, r)
	}
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
		resp.Result = s.toolsCall(req.Params)
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

func (s *Server) toolsCall(params json.RawMessage) interface{} {
	var call struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(params, &call); err != nil {
		return map[string]interface{}{"content": []map[string]string{{"type": "text", "text": "Invalid params"}}}
	}

	switch call.Name {
	case "list_routes":
		routes := s.routes.List()
		result := make([]map[string]interface{}, len(routes))
		for i, r := range routes {
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
			result[i] = entry
		}
		data, _ := json.Marshal(result)
		return map[string]interface{}{"content": []map[string]string{{"type": "text", "text": string(data)}}}

	case "get_route":
		var args struct{ Name string `json:"name"` }
		json.Unmarshal(call.Arguments, &args)
		r, ok := s.routes.Lookup(args.Name)
		if !ok {
			return map[string]interface{}{"content": []map[string]string{{"type": "text", "text": "Route not found"}}, "isError": true}
		}
		result := map[string]interface{}{"name": r.Name, "url": r.URL, "backend_port": r.Port}
		if s.traffic != nil {
			entries := s.traffic.List(5, r.Name)
			result["recent_traffic"] = entries
		}
		data, _ := json.Marshal(result)
		return map[string]interface{}{"content": []map[string]string{{"type": "text", "text": string(data)}}}

	case "register_route":
		var args struct {
			Name string `json:"name"`
			Port int    `json:"port"`
		}
		json.Unmarshal(call.Arguments, &args)
		r, err := s.routes.Alias(args.Name, args.Port, false)
		if err != nil {
			return map[string]interface{}{"content": []map[string]string{{"type": "text", "text": err.Error()}}, "isError": true}
		}
		return map[string]interface{}{"content": []map[string]string{{"type": "text", "text": "Registered: " + r.URL}}}

	case "health_check":
		var args struct{ Name string `json:"name"` }
		json.Unmarshal(call.Arguments, &args)
		r, ok := s.routes.Lookup(args.Name)
		if !ok {
			return map[string]interface{}{"content": []map[string]string{{"type": "text", "text": "Route not found"}}, "isError": true}
		}
		if s.health == nil {
			return map[string]interface{}{"content": []map[string]string{{"type": "text", "text": "Health checker not available"}}, "isError": true}
		}
		status := s.health.CheckNow(args.Name, r.Port)
		data, _ := json.Marshal(status)
		return map[string]interface{}{"content": []map[string]string{{"type": "text", "text": string(data)}}}

	default:
		return map[string]interface{}{"content": []map[string]string{{"type": "text", "text": "Unknown tool"}}, "isError": true}
	}
}
