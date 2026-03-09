// Package proxy tests — verifies loop detection returns 508 with correct JSON body.
//
// Author: Thiru K
// Module: github.com/thirukguru/localias/internal/proxy
package proxy

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLoopDetection_Returns508(t *testing.T) {
	rt := NewRouteTable(7777, false, "")
	rt.Register("myapp", 4231, 0, "")

	handler := NewHandler(rt, slog.Default(), nil)

	req := httptest.NewRequest("GET", "http://myapp.localhost:7777/test", nil)
	req.Host = "myapp.localhost:7777"
	req.Header.Set("X-Localias-Forwarded", "true")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusLoopDetected {
		t.Errorf("expected status 508, got %d", rr.Code)
	}

	var loopErr LoopError
	if err := json.NewDecoder(rr.Body).Decode(&loopErr); err != nil {
		t.Fatalf("failed to decode loop error: %v", err)
	}
	if loopErr.Error != "loop_detected" {
		t.Errorf("expected error 'loop_detected', got %q", loopErr.Error)
	}
	if loopErr.Fix == "" {
		t.Error("expected non-empty fix message")
	}
}

func TestLoopDetection_NormalRequest(t *testing.T) {
	rt := NewRouteTable(7777, false, "")
	// Don't register a route so we get a 502 (no backend), but NOT a 508
	handler := NewHandler(rt, slog.Default(), nil)

	req := httptest.NewRequest("GET", "http://myapp.localhost:7777/test", nil)
	req.Host = "myapp.localhost:7777"
	// No X-Localias-Forwarded header

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code == http.StatusLoopDetected {
		t.Error("got 508 for normal request without forwarded header")
	}
}

func TestIsLoopRequest(t *testing.T) {
	tests := []struct {
		name     string
		header   string
		expected bool
	}{
		{"no header", "", false},
		{"with header", "true", true},
		{"any value", "1", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			if tt.header != "" {
				req.Header.Set("X-Localias-Forwarded", tt.header)
			}
			if got := IsLoopRequest(req); got != tt.expected {
				t.Errorf("IsLoopRequest = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestHandler_NoRoute(t *testing.T) {
	rt := NewRouteTable(7777, false, "")
	handler := NewHandler(rt, slog.Default(), nil)

	req := httptest.NewRequest("GET", "http://unknown.localhost:7777/", nil)
	req.Host = "unknown.localhost:7777"

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadGateway {
		t.Errorf("expected status 502 for unknown route, got %d", rr.Code)
	}
}

func TestHandler_ProxiesToBackend(t *testing.T) {
	// Start a test backend
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Test", "passed")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("hello from backend"))
	}))
	defer backend.Close()

	// Extract port from backend URL
	backendPort := 0
	for i := len(backend.URL) - 1; i >= 0; i-- {
		if backend.URL[i] == ':' {
			p := backend.URL[i+1:]
			for _, c := range p {
				backendPort = backendPort*10 + int(c-'0')
			}
			break
		}
	}

	rt := NewRouteTable(7777, false, "")
	rt.Register("testapp", backendPort, 0, "")

	handler := NewHandler(rt, slog.Default(), nil)

	req := httptest.NewRequest("GET", "http://testapp.localhost:7777/api/test", nil)
	req.Host = "testapp.localhost:7777"

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}
	if rr.Body.String() != "hello from backend" {
		t.Errorf("unexpected body: %q", rr.Body.String())
	}
}
