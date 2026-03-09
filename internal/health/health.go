// Package health provides background health checking for proxy routes.
// Runs a goroutine per route that periodically checks backend availability.
//
// Author: Thiru K
// Module: github.com/thirukguru/localias/internal/health
package health

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

// Status represents the health status of a route.
type Status struct {
	Name               string        `json:"name"`
	Healthy            bool          `json:"healthy"`
	StatusCode         int           `json:"status_code"`
	Latency            time.Duration `json:"latency"`
	LastCheck          time.Time     `json:"last_check"`
	ConsecutiveFailures int          `json:"consecutive_failures"`
	Error              string        `json:"error,omitempty"`
}

// Checker manages background health checks for routes.
type Checker struct {
	mu       sync.RWMutex
	statuses map[string]*Status
	cancels  map[string]context.CancelFunc
	interval time.Duration
	logger   *slog.Logger
	client   *http.Client
}

// NewChecker creates a new health checker.
func NewChecker(logger *slog.Logger) *Checker {
	if logger == nil {
		logger = slog.Default()
	}
	return &Checker{
		statuses: make(map[string]*Status),
		cancels:  make(map[string]context.CancelFunc),
		interval: 10 * time.Second,
		logger:   logger,
		client: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

// StartChecking begins health checks for a route.
func (c *Checker) StartChecking(name string, port int) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Cancel existing checker if any
	if cancel, ok := c.cancels[name]; ok {
		cancel()
	}

	ctx, cancel := context.WithCancel(context.Background())
	c.cancels[name] = cancel
	c.statuses[name] = &Status{Name: name}

	go c.checkLoop(ctx, name, port)
}

// StopChecking stops health checks for a route.
func (c *Checker) StopChecking(name string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if cancel, ok := c.cancels[name]; ok {
		cancel()
		delete(c.cancels, name)
		delete(c.statuses, name)
	}
}

// GetStatus returns the health status for a route.
func (c *Checker) GetStatus(name string) (*Status, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	s, ok := c.statuses[name]
	if !ok {
		return nil, false
	}
	// Return a copy
	copy := *s
	return &copy, true
}

// GetAllStatuses returns all health statuses.
func (c *Checker) GetAllStatuses() map[string]*Status {
	c.mu.RLock()
	defer c.mu.RUnlock()
	result := make(map[string]*Status, len(c.statuses))
	for k, v := range c.statuses {
		copy := *v
		result[k] = &copy
	}
	return result
}

// CheckNow performs an immediate health check for a route.
func (c *Checker) CheckNow(name string, port int) *Status {
	return c.doCheck(name, port)
}

// StopAll stops all health checks.
func (c *Checker) StopAll() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for name, cancel := range c.cancels {
		cancel()
		delete(c.cancels, name)
	}
}

func (c *Checker) checkLoop(ctx context.Context, name string, port int) {
	// Do an initial check immediately
	c.updateStatus(name, c.doCheck(name, port))

	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.updateStatus(name, c.doCheck(name, port))
		}
	}
}

func (c *Checker) doCheck(name string, port int) *Status {
	url := fmt.Sprintf("http://127.0.0.1:%d/", port)
	start := time.Now()

	resp, err := c.client.Get(url)
	latency := time.Since(start)

	status := &Status{
		Name:      name,
		Latency:   latency,
		LastCheck: time.Now(),
	}

	if err != nil {
		status.Healthy = false
		status.Error = err.Error()
		return status
	}
	defer resp.Body.Close()

	status.StatusCode = resp.StatusCode
	status.Healthy = resp.StatusCode >= 200 && resp.StatusCode < 500
	return status
}

func (c *Checker) updateStatus(name string, newStatus *Status) {
	c.mu.Lock()
	defer c.mu.Unlock()

	existing, ok := c.statuses[name]
	if !ok {
		c.statuses[name] = newStatus
		return
	}

	if !newStatus.Healthy {
		existing.ConsecutiveFailures++
	} else {
		existing.ConsecutiveFailures = 0
	}

	existing.Healthy = newStatus.Healthy
	existing.StatusCode = newStatus.StatusCode
	existing.Latency = newStatus.Latency
	existing.LastCheck = newStatus.LastCheck
	existing.Error = newStatus.Error
}
