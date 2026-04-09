// Package proxy — core reverse proxy server.
// Combines the router, handler, WebSocket, and loop detection into a single
// HTTP/1.1 + HTTP/2 reverse proxy server.
//
// Author: Thiru K
// Module: github.com/thirukguru/localias/internal/proxy
package proxy

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"golang.org/x/net/http2"
)

// ServerConfig holds configuration for the proxy server.
type ServerConfig struct {
	Port       int
	HTTPS      bool
	TLSCert    string
	TLSKey     string
	StateDir   string
	Logger     *slog.Logger
	Traffic    TrafficRecorder
	OnRouteChange func() // called when routes change, for hosts sync etc.
}

// Server is the main reverse proxy server.
type Server struct {
	config     ServerConfig
	routes     *RouteTable
	handler    *Handler
	httpServer *http.Server
	logger     *slog.Logger
}

// NewServer creates a new proxy server with the given configuration.
func NewServer(cfg ServerConfig) *Server {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.Port == 0 {
		cfg.Port = 7777
	}

	persistPath := ""
	if cfg.StateDir != "" {
		persistPath = cfg.StateDir + "/routes.json"
	}

	routes := NewRouteTable(cfg.Port, cfg.HTTPS, persistPath)
	handler := NewHandler(routes, cfg.Logger, cfg.Traffic)

	s := &Server{
		config:  cfg,
		routes:  routes,
		handler: handler,
		logger:  cfg.Logger,
	}

	return s
}

// Routes returns the server's route table.
func (s *Server) Routes() *RouteTable {
	return s.routes
}

// SetDashboard sets the dashboard handler for localias.localhost.
func (s *Server) SetDashboard(h http.Handler) {
	s.handler.SetDashboard(h)
}

// SetMCP sets the MCP server handler for mcp.localhost.
func (s *Server) SetMCP(h http.Handler) {
	s.handler.SetMCP(h)
}

// Start starts the proxy server. It blocks until the server is stopped.
func (s *Server) Start(ctx context.Context) error {
	addr := fmt.Sprintf("127.0.0.1:%d", s.config.Port)

	s.httpServer = &http.Server{
		Addr:              addr,
		Handler:           s.handler,
		ReadTimeout:       30 * time.Second,
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	// Configure TLS if enabled
	if s.config.HTTPS {
		tlsCfg, err := s.buildTLSConfig()
		if err != nil {
			return fmt.Errorf("TLS config: %w", err)
		}
		s.httpServer.TLSConfig = tlsCfg

		// Enable HTTP/2
		if err := http2.ConfigureServer(s.httpServer, &http2.Server{}); err != nil {
			return fmt.Errorf("HTTP/2 config: %w", err)
		}
	}

	// Listen
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", addr, err)
	}

	// Graceful shutdown on context cancellation
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		s.logger.Info("shutting down proxy server")
		s.httpServer.Shutdown(shutdownCtx)
	}()

	scheme := "http"
	if s.config.HTTPS {
		scheme = "https"
	}
	s.logger.Info("proxy server started",
		"addr", addr,
		"scheme", scheme,
	)

	if s.config.HTTPS {
		return s.httpServer.ServeTLS(ln, s.config.TLSCert, s.config.TLSKey)
	}
	return s.httpServer.Serve(ln)
}

// Stop gracefully shuts down the server.
func (s *Server) Stop() error {
	if s.httpServer == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return s.httpServer.Shutdown(ctx)
}

// buildTLSConfig creates a TLS configuration using the configured cert and key.
func (s *Server) buildTLSConfig() (*tls.Config, error) {
	cert, err := tls.LoadX509KeyPair(s.config.TLSCert, s.config.TLSKey)
	if err != nil {
		return nil, fmt.Errorf("loading TLS cert: %w", err)
	}

	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
		NextProtos:   []string{"h2", "http/1.1"},
	}, nil
}

// ListenAddr returns the address the server is configured to listen on.
func (s *Server) ListenAddr() string {
	return fmt.Sprintf("127.0.0.1:%d", s.config.Port)
}
