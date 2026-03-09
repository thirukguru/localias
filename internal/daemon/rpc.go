// Package daemon — JSON-RPC 2.0 server over Unix domain socket.
// Provides the RPC interface between CLI commands and the background daemon process.
//
// Author: Thiru K
// Module: github.com/thirukguru/localias/internal/daemon
package daemon

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"os"
	"sync"
)

// RPCRequest is a JSON-RPC 2.0 request.
type RPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// RPCResponse is a JSON-RPC 2.0 response.
type RPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

// RPCError is a JSON-RPC 2.0 error object.
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// RegisterParams are the parameters for the "register" RPC method.
type RegisterParams struct {
	Name string `json:"name"`
	Port int    `json:"port"`
	PID  int    `json:"pid"`
	Cmd  string `json:"cmd"`
}

// RegisterResult is the result of the "register" RPC method.
type RegisterResult struct {
	URL string `json:"url"`
}

// AliasParams are the parameters for the "alias" RPC method.
type AliasParams struct {
	Name  string `json:"name"`
	Port  int    `json:"port"`
	Force bool   `json:"force"`
}

// UnaliasParams are the parameters for the "unalias" RPC method.
type UnaliasParams struct {
	Name string `json:"name"`
}

// DeregisterParams are the parameters for the "deregister" RPC method.
type DeregisterParams struct {
	Name string `json:"name"`
}

// ListResult is the result of the "list" RPC method.
type ListResult struct {
	Routes []RouteInfo `json:"routes"`
}

// RouteInfo represents a route in RPC responses.
type RouteInfo struct {
	Name   string `json:"name"`
	Port   int    `json:"port"`
	URL    string `json:"url"`
	PID    int    `json:"pid,omitempty"`
	Cmd    string `json:"cmd,omitempty"`
	Static bool   `json:"static"`
}

// HealthParams are the parameters for the "health" RPC method.
type HealthParams struct {
	Name string `json:"name"`
}

// HealthResult is the result of the "health" RPC method.
type HealthResult struct {
	Status    string `json:"status"`
	Latency   string `json:"latency"`
	LastCheck string `json:"last_check"`
}

// TrafficParams are the parameters for the "traffic" RPC method.
type TrafficParams struct {
	Limit int    `json:"limit"`
	Route string `json:"route"`
}

// RPCHandler is a function that handles an RPC method call.
type RPCHandler func(params json.RawMessage) (interface{}, error)

// RPCServer is a JSON-RPC 2.0 server over Unix domain socket.
type RPCServer struct {
	socketPath string
	listener   net.Listener
	handlers   map[string]RPCHandler
	mu         sync.RWMutex
	logger     *slog.Logger
	wg         sync.WaitGroup
}

// NewRPCServer creates a new RPC server.
func NewRPCServer(socketPath string, logger *slog.Logger) *RPCServer {
	if logger == nil {
		logger = slog.Default()
	}
	return &RPCServer{
		socketPath: socketPath,
		handlers:   make(map[string]RPCHandler),
		logger:     logger,
	}
}

// Handle registers an RPC method handler.
func (s *RPCServer) Handle(method string, handler RPCHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.handlers[method] = handler
}

// Start begins listening on the Unix socket and handling RPC requests.
func (s *RPCServer) Start(ctx context.Context) error {
	// Remove existing socket file
	os.Remove(s.socketPath)

	ln, err := net.Listen("unix", s.socketPath)
	if err != nil {
		return fmt.Errorf("listening on %s: %w", s.socketPath, err)
	}
	s.listener = ln
	os.Chmod(s.socketPath, 0660)

	s.logger.Info("RPC server started", "socket", s.socketPath)

	// Stop listener when context is cancelled
	go func() {
		<-ctx.Done()
		ln.Close()
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				s.wg.Wait()
				return nil
			default:
				s.logger.Error("accept error", "error", err)
				continue
			}
		}
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.handleConn(conn)
		}()
	}
}

// Stop closes the listener and waits for all connections to finish.
func (s *RPCServer) Stop() {
	if s.listener != nil {
		s.listener.Close()
	}
	s.wg.Wait()
	os.Remove(s.socketPath)
}

// handleConn processes a single connection's RPC requests.
func (s *RPCServer) handleConn(conn net.Conn) {
	defer conn.Close()

	scanner := bufio.NewScanner(conn)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024) // 1MB buffer

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var req RPCRequest
		if err := json.Unmarshal(line, &req); err != nil {
			s.writeError(conn, 0, -32700, "Parse error")
			continue
		}

		if req.JSONRPC != "2.0" {
			s.writeError(conn, req.ID, -32600, "Invalid Request: jsonrpc must be '2.0'")
			continue
		}

		s.mu.RLock()
		handler, ok := s.handlers[req.Method]
		s.mu.RUnlock()

		if !ok {
			s.writeError(conn, req.ID, -32601, fmt.Sprintf("Method not found: %s", req.Method))
			continue
		}

		result, err := handler(req.Params)
		if err != nil {
			s.writeError(conn, req.ID, -32000, err.Error())
			continue
		}

		resultJSON, err := json.Marshal(result)
		if err != nil {
			s.writeError(conn, req.ID, -32603, "Internal error: failed to marshal result")
			continue
		}

		resp := RPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  resultJSON,
		}
		respJSON, _ := json.Marshal(resp)
		conn.Write(append(respJSON, '\n'))
	}
}

// writeError writes a JSON-RPC error response.
func (s *RPCServer) writeError(conn net.Conn, id int, code int, message string) {
	resp := RPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error: &RPCError{
			Code:    code,
			Message: message,
		},
	}
	data, _ := json.Marshal(resp)
	conn.Write(append(data, '\n'))
}
