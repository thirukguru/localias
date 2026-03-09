// Package health tests — verifies health checking with mock HTTP server.
//
// Author: Thiru K
// Module: github.com/thirukguru/localias/internal/health
package health

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func getPort(url string) int {
	parts := strings.Split(url, ":")
	port, _ := strconv.Atoi(parts[len(parts)-1])
	return port
}

func TestChecker_HealthyBackend(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	port := getPort(backend.URL)
	checker := NewChecker(slog.Default())

	status := checker.CheckNow("testapp", port)
	if !status.Healthy {
		t.Error("expected healthy backend")
	}
	if status.StatusCode != 200 {
		t.Errorf("expected status 200, got %d", status.StatusCode)
	}
	if status.Latency <= 0 {
		t.Error("expected positive latency")
	}
}

func TestChecker_UnhealthyBackend(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer backend.Close()

	port := getPort(backend.URL)
	checker := NewChecker(slog.Default())

	status := checker.CheckNow("testapp", port)
	if status.Healthy {
		t.Error("expected unhealthy backend (500)")
	}
}

func TestChecker_UnavailableBackend(t *testing.T) {
	checker := NewChecker(slog.Default())

	// Use a port that nothing is listening on
	status := checker.CheckNow("testapp", 19999)
	if status.Healthy {
		t.Error("expected unhealthy for unavailable backend")
	}
	if status.Error == "" {
		t.Error("expected error message for unavailable backend")
	}
}

func TestChecker_StartStopChecking(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	port := getPort(backend.URL)
	checker := NewChecker(slog.Default())
	checker.interval = 50 * time.Millisecond // Short interval for testing

	checker.StartChecking("testapp", port)

	// Wait for at least one check
	time.Sleep(200 * time.Millisecond)

	status, ok := checker.GetStatus("testapp")
	if !ok {
		t.Fatal("expected status for testapp")
	}
	if !status.Healthy {
		t.Error("expected healthy status")
	}

	checker.StopChecking("testapp")

	_, ok = checker.GetStatus("testapp")
	if ok {
		t.Error("expected no status after stopping")
	}
}

func TestChecker_ConsecutiveFailures(t *testing.T) {
	checker := NewChecker(slog.Default())
	checker.interval = 50 * time.Millisecond

	// Check against a non-existent port
	checker.StartChecking("deadapp", 19998)

	time.Sleep(250 * time.Millisecond)

	status, ok := checker.GetStatus("deadapp")
	if !ok {
		t.Fatal("expected status for deadapp")
	}
	if status.ConsecutiveFailures == 0 {
		t.Error("expected consecutive failures > 0")
	}

	checker.StopAll()
}

func TestChecker_GetAllStatuses(t *testing.T) {
	backend1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer backend1.Close()
	backend2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer backend2.Close()

	checker := NewChecker(slog.Default())
	checker.interval = 50 * time.Millisecond

	checker.StartChecking("app1", getPort(backend1.URL))
	checker.StartChecking("app2", getPort(backend2.URL))

	time.Sleep(200 * time.Millisecond)

	statuses := checker.GetAllStatuses()
	if len(statuses) != 2 {
		t.Errorf("expected 2 statuses, got %d", len(statuses))
	}

	checker.StopAll()
}
