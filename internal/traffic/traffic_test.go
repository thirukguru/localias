// Package traffic tests — verifies ring buffer behavior, eviction, and filtering.
//
// Author: Thiru K
// Module: github.com/thirukguru/localias/internal/traffic
package traffic

import (
	"fmt"
	"testing"
	"time"
)

func makeEntry(id string, route string) Entry {
	return Entry{
		ID:        id,
		Timestamp: time.Now(),
		Route:     route,
		Method:    "GET",
		Path:      "/test",
		Status:    200,
		Latency:   5 * time.Millisecond,
	}
}

func TestLogger_Record_And_List(t *testing.T) {
	logger := NewLogger(10)

	logger.Record(makeEntry("1", "app1"))
	logger.Record(makeEntry("2", "app2"))
	logger.Record(makeEntry("3", "app1"))

	entries := logger.List(0, "")
	if len(entries) != 3 {
		t.Errorf("expected 3 entries, got %d", len(entries))
	}
}

func TestLogger_Count(t *testing.T) {
	logger := NewLogger(10)

	if logger.Count() != 0 {
		t.Error("expected 0 count initially")
	}

	logger.Record(makeEntry("1", "app1"))
	if logger.Count() != 1 {
		t.Errorf("expected 1, got %d", logger.Count())
	}
}

func TestLogger_RingBufferEviction(t *testing.T) {
	logger := NewLogger(5)

	for i := 0; i < 10; i++ {
		logger.Record(makeEntry(fmt.Sprintf("%d", i), "app"))
	}

	if logger.Count() != 5 {
		t.Errorf("expected 5 after eviction, got %d", logger.Count())
	}

	entries := logger.List(0, "")
	if len(entries) != 5 {
		t.Errorf("expected 5 entries, got %d", len(entries))
	}

	// Should contain entries 5-9 (newest)
	if entries[0].ID != "5" {
		t.Errorf("expected oldest entry ID '5', got %q", entries[0].ID)
	}
	if entries[4].ID != "9" {
		t.Errorf("expected newest entry ID '9', got %q", entries[4].ID)
	}
}

func TestLogger_RouteFilter(t *testing.T) {
	logger := NewLogger(10)

	logger.Record(makeEntry("1", "app1"))
	logger.Record(makeEntry("2", "app2"))
	logger.Record(makeEntry("3", "app1"))
	logger.Record(makeEntry("4", "app2"))

	app1Entries := logger.List(0, "app1")
	if len(app1Entries) != 2 {
		t.Errorf("expected 2 app1 entries, got %d", len(app1Entries))
	}

	app2Entries := logger.List(0, "app2")
	if len(app2Entries) != 2 {
		t.Errorf("expected 2 app2 entries, got %d", len(app2Entries))
	}
}

func TestLogger_Limit(t *testing.T) {
	logger := NewLogger(10)

	for i := 0; i < 8; i++ {
		logger.Record(makeEntry(fmt.Sprintf("%d", i), "app"))
	}

	entries := logger.List(3, "")
	if len(entries) != 3 {
		t.Errorf("expected 3 entries with limit, got %d", len(entries))
	}

	// Should be the 3 newest
	if entries[0].ID != "5" {
		t.Errorf("expected oldest limited entry '5', got %q", entries[0].ID)
	}
}

func TestLogger_EmptyList(t *testing.T) {
	logger := NewLogger(10)
	entries := logger.List(0, "")
	if entries != nil {
		t.Errorf("expected nil for empty logger, got %v", entries)
	}
}

func TestLogger_Subscribe(t *testing.T) {
	logger := NewLogger(10)

	ch := logger.Subscribe()
	defer logger.Unsubscribe(ch)

	// Record an entry
	entry := makeEntry("1", "app1")
	logger.Record(entry)

	// Should receive the entry
	select {
	case received := <-ch:
		if received.ID != "1" {
			t.Errorf("expected ID '1', got %q", received.ID)
		}
	case <-time.After(1 * time.Second):
		t.Error("timeout waiting for subscriber notification")
	}
}

func TestLogger_DefaultCapacity(t *testing.T) {
	logger := NewLogger(0) // Should use default
	if logger.capacity != DefaultCapacity {
		t.Errorf("expected default capacity %d, got %d", DefaultCapacity, logger.capacity)
	}
}
