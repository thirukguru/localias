// Package proxy — HTTP handler for the reverse proxy.
// Routes incoming requests based on the Host header to the appropriate backend.
//
// Author: Thiru K
// Module: github.com/thirukguru/localias/internal/proxy
package proxy

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync/atomic"
	"time"
)

// TrafficRecorder is an interface for recording traffic entries.
// Implemented by internal/traffic.Logger.
type TrafficRecorder interface {
	Record(entry TrafficEntry)
}

// TrafficEntry represents a single proxied request for traffic logging.
type TrafficEntry struct {
	ID         string            `json:"id"`
	Timestamp  time.Time         `json:"timestamp"`
	Route      string            `json:"route"`
	Method     string            `json:"method"`
	Path       string            `json:"path"`
	Status     int               `json:"status"`
	Latency    time.Duration     `json:"latency"`
	ReqSize    int64             `json:"req_size"`
	ResSize    int64             `json:"res_size"`
	ReqHeaders map[string]string `json:"req_headers,omitempty"`
	ResHeaders map[string]string `json:"res_headers,omitempty"`
}

// Handler is the main HTTP handler for the reverse proxy.
type Handler struct {
	routes    *RouteTable
	logger    *slog.Logger
	traffic   TrafficRecorder
	dashboard http.Handler
	reqCount  atomic.Uint64
}

// NewHandler creates a new proxy handler.
func NewHandler(routes *RouteTable, logger *slog.Logger, traffic TrafficRecorder) *Handler {
	return &Handler{
		routes:  routes,
		logger:  logger,
		traffic: traffic,
	}
}

// SetDashboard sets the dashboard handler for localias.localhost requests.
func (h *Handler) SetDashboard(dashboard http.Handler) {
	h.dashboard = dashboard
}

// ServeHTTP routes incoming requests to the appropriate backend.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	// Extract the route name from the Host header
	host := r.Host
	// Remove port suffix
	if idx := strings.LastIndex(host, ":"); idx > 0 {
		host = host[:idx]
	}
	// Remove .localhost suffix to get the route name
	name := strings.TrimSuffix(host, ".localhost")

	// Check for dashboard route
	if name == "localias" {
		if h.dashboard != nil {
			h.dashboard.ServeHTTP(w, r)
			return
		}
		http.Error(w, "Dashboard not configured", http.StatusNotFound)
		return
	}

	// Check for loop detection
	if r.Header.Get("X-Localias-Forwarded") != "" {
		handleLoop(w, name)
		return
	}

	// Lookup the route
	route, ok := h.routes.Lookup(name)
	if !ok {
		h.logger.Warn("no route found", "host", r.Host, "name", name)
		http.Error(w, fmt.Sprintf("No route found for %q. Use 'localias alias %s <port>' to register.", name, name), http.StatusBadGateway)
		return
	}

	// Check for WebSocket upgrade
	if isWebSocketUpgrade(r) {
		h.proxyWebSocket(w, r, route)
		return
	}

	// Build the backend URL
	backendURL := &url.URL{
		Scheme: "http",
		Host:   fmt.Sprintf("127.0.0.1:%d", route.Port),
	}

	// Create reverse proxy
	proxy := &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			req.URL.Scheme = backendURL.Scheme
			req.URL.Host = backendURL.Host
			req.Host = backendURL.Host

			// Set forwarding headers
			if clientIP := r.RemoteAddr; clientIP != "" {
				if idx := strings.LastIndex(clientIP, ":"); idx > 0 {
					clientIP = clientIP[:idx]
				}
				req.Header.Set("X-Forwarded-For", clientIP)
				req.Header.Set("X-Real-IP", clientIP)
			}
			req.Header.Set("X-Forwarded-Host", r.Host)
			req.Header.Set("X-Forwarded-Proto", "http")
			req.Header.Set("X-Localias-Forwarded", "true")
		},
		ErrorHandler: func(rw http.ResponseWriter, req *http.Request, err error) {
			h.logger.Error("proxy error", "route", name, "error", err)
			http.Error(rw, fmt.Sprintf("Backend unavailable: %v", err), http.StatusBadGateway)
		},
	}

	// Wrap response writer to capture status code and size
	rw := &responseRecorder{ResponseWriter: w, statusCode: http.StatusOK}
	proxy.ServeHTTP(rw, r)

	// Record traffic
	latency := time.Since(start)
	if h.traffic != nil {
		id := h.reqCount.Add(1)

		// Capture request headers
		reqHeaders := make(map[string]string, len(r.Header))
		for k := range r.Header {
			reqHeaders[k] = r.Header.Get(k)
		}

		// Capture response headers
		resHeaders := make(map[string]string, len(rw.Header()))
		for k := range rw.Header() {
			resHeaders[k] = rw.Header().Get(k)
		}

		h.traffic.Record(TrafficEntry{
			ID:         fmt.Sprintf("req-%d", id),
			Timestamp:  start,
			Route:      route.Name,
			Method:     r.Method,
			Path:       r.URL.Path,
			Status:     rw.statusCode,
			Latency:    latency,
			ReqSize:    r.ContentLength,
			ResSize:    rw.bytesWritten,
			ReqHeaders: reqHeaders,
			ResHeaders: resHeaders,
		})
	}

	h.logger.Info("proxied",
		"route", route.Name,
		"method", r.Method,
		"path", r.URL.Path,
		"status", rw.statusCode,
		"latency", latency,
	)
}

// responseRecorder wraps http.ResponseWriter to capture status code and bytes written.
type responseRecorder struct {
	http.ResponseWriter
	statusCode   int
	bytesWritten int64
	wroteHeader  bool
}

func (rr *responseRecorder) WriteHeader(code int) {
	if !rr.wroteHeader {
		rr.statusCode = code
		rr.wroteHeader = true
	}
	rr.ResponseWriter.WriteHeader(code)
}

func (rr *responseRecorder) Write(b []byte) (int, error) {
	if !rr.wroteHeader {
		rr.wroteHeader = true
	}
	n, err := rr.ResponseWriter.Write(b)
	rr.bytesWritten += int64(n)
	return n, err
}

// Flush implements http.Flusher for SSE support.
func (rr *responseRecorder) Flush() {
	if f, ok := rr.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}
