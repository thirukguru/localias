// Package traffic provides an in-memory ring buffer for recording the last N
// proxied HTTP requests. Used by the dashboard and MCP server to display traffic.
//
// Author: Thiru K
// Module: github.com/thirukguru/localias/internal/traffic
package traffic

import (
	"sync"
	"time"
)

const DefaultCapacity = 1000

// Entry represents a single proxied request record.
type Entry struct {
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

// Logger is a ring buffer that stores the last N traffic entries.
type Logger struct {
	mu       sync.RWMutex
	entries  []Entry
	capacity int
	head     int
	count    int
	// subscribers for SSE streaming
	subMu sync.RWMutex
	subs  map[chan Entry]struct{}
}

// NewLogger creates a new traffic logger with the given capacity.
func NewLogger(capacity int) *Logger {
	if capacity <= 0 {
		capacity = DefaultCapacity
	}
	return &Logger{
		entries:  make([]Entry, capacity),
		capacity: capacity,
		subs:     make(map[chan Entry]struct{}),
	}
}

// Record adds a new traffic entry to the ring buffer and notifies subscribers.
func (l *Logger) Record(entry Entry) {
	l.mu.Lock()
	l.entries[l.head] = entry
	l.head = (l.head + 1) % l.capacity
	if l.count < l.capacity {
		l.count++
	}
	l.mu.Unlock()

	// Notify subscribers (non-blocking)
	l.subMu.RLock()
	for ch := range l.subs {
		select {
		case ch <- entry:
		default:
			// Drop if subscriber is slow
		}
	}
	l.subMu.RUnlock()
}

// List returns the last N entries, ordered oldest to newest.
// If limit is 0, returns all entries.
func (l *Logger) List(limit int, routeFilter string) []Entry {
	l.mu.RLock()
	defer l.mu.RUnlock()

	if l.count == 0 {
		return nil
	}

	// Collect all entries in order
	var result []Entry
	start := (l.head - l.count + l.capacity) % l.capacity
	for i := 0; i < l.count; i++ {
		idx := (start + i) % l.capacity
		e := l.entries[idx]
		if routeFilter != "" && e.Route != routeFilter {
			continue
		}
		result = append(result, e)
	}

	// Apply limit (take from the end for newest entries)
	if limit > 0 && len(result) > limit {
		result = result[len(result)-limit:]
	}

	return result
}

// Count returns the number of entries in the buffer.
func (l *Logger) Count() int {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.count
}

// Subscribe creates a channel that receives new traffic entries.
func (l *Logger) Subscribe() chan Entry {
	ch := make(chan Entry, 100)
	l.subMu.Lock()
	l.subs[ch] = struct{}{}
	l.subMu.Unlock()
	return ch
}

// Unsubscribe removes a subscription channel.
func (l *Logger) Unsubscribe(ch chan Entry) {
	l.subMu.Lock()
	delete(l.subs, ch)
	l.subMu.Unlock()
	close(ch)
}
