// Package dashboard provides an embedded HTTP dashboard for localias.
// Serves a single-page app at localias.localhost:7777 with routes, traffic,
// profiles, and settings tabs.
//
// Author: Thiru K
// Module: github.com/thirukguru/localias/internal/dashboard
package dashboard

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/thirukguru/localias/internal/health"
	"github.com/thirukguru/localias/internal/proxy"
	"github.com/thirukguru/localias/internal/traffic"
)

// Dashboard serves the embedded web dashboard and API endpoints.
type Dashboard struct {
	routes  *proxy.RouteTable
	traffic *traffic.Logger
	health  *health.Checker
	logger  *slog.Logger
	port    int
	https   bool
}

// Config holds dashboard configuration.
type Config struct {
	Routes  *proxy.RouteTable
	Traffic *traffic.Logger
	Health  *health.Checker
	Logger  *slog.Logger
	Port    int
	HTTPS   bool
}

// New creates a new Dashboard.
func New(cfg Config) *Dashboard {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	return &Dashboard{
		routes:  cfg.Routes,
		traffic: cfg.Traffic,
		health:  cfg.Health,
		logger:  cfg.Logger,
		port:    cfg.Port,
		https:   cfg.HTTPS,
	}
}

// Handler returns the HTTP handler for the dashboard.
func (d *Dashboard) Handler() http.Handler {
	mux := http.NewServeMux()

	// API endpoints
	mux.HandleFunc("GET /api/routes", d.handleRoutes)
	mux.HandleFunc("GET /api/traffic", d.handleTraffic)
	mux.HandleFunc("GET /api/traffic/stream", d.handleTrafficStream)
	mux.HandleFunc("GET /api/health", d.handleHealth)

	// Serve static files
	mux.HandleFunc("/", d.handleStatic)

	return mux
}

// handleRoutes returns all routes with health info.
func (d *Dashboard) handleRoutes(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	routes := d.routes.List()
	type routeResponse struct {
		Name    string `json:"name"`
		URL     string `json:"url"`
		Port    int    `json:"port"`
		Static  bool   `json:"static"`
		Healthy *bool  `json:"healthy,omitempty"`
		Latency string `json:"latency,omitempty"`
	}

	var result []routeResponse
	for _, route := range routes {
		rr := routeResponse{
			Name:   route.Name,
			URL:    route.URL,
			Port:   route.Port,
			Static: route.Static,
		}
		if d.health != nil {
			if status, ok := d.health.GetStatus(route.Name); ok {
				rr.Healthy = &status.Healthy
				rr.Latency = status.Latency.String()
			}
		}
		result = append(result, rr)
	}

	if err := json.NewEncoder(w).Encode(result); err != nil {
		d.logger.Error("failed to encode routes response", "error", err)
	}
}

// handleTraffic returns recent traffic entries.
func (d *Dashboard) handleTraffic(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	if d.traffic == nil {
		if err := json.NewEncoder(w).Encode([]struct{}{}); err != nil {
			d.logger.Error("failed to encode empty traffic response", "error", err)
		}
		return
	}

	route := r.URL.Query().Get("route")
	entries := d.traffic.List(100, route)
	if entries == nil {
		entries = []traffic.Entry{}
	}
	if err := json.NewEncoder(w).Encode(entries); err != nil {
		d.logger.Error("failed to encode traffic response", "error", err)
	}
}

// handleTrafficStream provides SSE stream of new traffic entries.
func (d *Dashboard) handleTrafficStream(w http.ResponseWriter, r *http.Request) {
	if d.traffic == nil {
		http.Error(w, "Traffic logging not enabled", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	ch := d.traffic.Subscribe()
	defer d.traffic.Unsubscribe(ch)

	for {
		select {
		case <-r.Context().Done():
			return
		case entry := <-ch:
			data, _ := json.Marshal(entry)
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
	}
}

// handleHealth returns health status for all routes.
func (d *Dashboard) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	if d.health == nil {
		if err := json.NewEncoder(w).Encode(map[string]interface{}{}); err != nil {
			d.logger.Error("failed to encode empty health response", "error", err)
		}
		return
	}

	if err := json.NewEncoder(w).Encode(d.health.GetAllStatuses()); err != nil {
		d.logger.Error("failed to encode health response", "error", err)
	}
}

// handleStatic serves the embedded dashboard HTML.
func (d *Dashboard) handleStatic(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	if path == "/" || path == "" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(indexHTML))
		return
	}
	if path == "/app.js" {
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
		w.Write([]byte(appJS))
		return
	}
	if path == "/style.css" {
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
		w.Write([]byte(styleCSS))
		return
	}
	http.NotFound(w, r)
}
