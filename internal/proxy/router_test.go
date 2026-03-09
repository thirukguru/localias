// Package proxy tests — verifies route table CRUD, wildcard matching, and concurrent access.
//
// Author: Thiru K
// Module: github.com/thirukguru/localias/internal/proxy
package proxy

import (
	"sync"
	"testing"
)

func TestRouteTable_Register(t *testing.T) {
	rt := NewRouteTable(7777, false, "")

	r, err := rt.Register("myapp", 4231, 1234, "npm run dev")
	if err != nil {
		t.Fatalf("Register returned error: %v", err)
	}
	if r.Name != "myapp" {
		t.Errorf("expected name 'myapp', got %q", r.Name)
	}
	if r.Port != 4231 {
		t.Errorf("expected port 4231, got %d", r.Port)
	}
	if r.URL != "http://myapp.localhost:7777" {
		t.Errorf("expected URL 'http://myapp.localhost:7777', got %q", r.URL)
	}
	if r.PID != 1234 {
		t.Errorf("expected PID 1234, got %d", r.PID)
	}
}

func TestRouteTable_Register_EmptyName(t *testing.T) {
	rt := NewRouteTable(7777, false, "")
	_, err := rt.Register("", 4231, 0, "")
	if err == nil {
		t.Error("expected error for empty name, got nil")
	}
}

func TestRouteTable_Register_HTTPS(t *testing.T) {
	rt := NewRouteTable(7777, true, "")
	r, err := rt.Register("myapp", 4231, 0, "")
	if err != nil {
		t.Fatalf("Register returned error: %v", err)
	}
	if r.URL != "https://myapp.localhost:7777" {
		t.Errorf("expected HTTPS URL, got %q", r.URL)
	}
}

func TestRouteTable_Deregister(t *testing.T) {
	rt := NewRouteTable(7777, false, "")
	rt.Register("myapp", 4231, 0, "")

	rt.Deregister("myapp")
	if rt.Has("myapp") {
		t.Error("route still exists after deregister")
	}
}

func TestRouteTable_Alias(t *testing.T) {
	rt := NewRouteTable(7777, false, "")

	r, err := rt.Alias("redis", 6379, false)
	if err != nil {
		t.Fatalf("Alias returned error: %v", err)
	}
	if !r.Static {
		t.Error("expected static route")
	}
	if r.Port != 6379 {
		t.Errorf("expected port 6379, got %d", r.Port)
	}
}

func TestRouteTable_Alias_Conflict(t *testing.T) {
	rt := NewRouteTable(7777, false, "")
	rt.Alias("redis", 6379, false)

	_, err := rt.Alias("redis", 6380, false)
	if err == nil {
		t.Error("expected conflict error, got nil")
	}
}

func TestRouteTable_Alias_Force(t *testing.T) {
	rt := NewRouteTable(7777, false, "")
	rt.Alias("redis", 6379, false)

	r, err := rt.Alias("redis", 6380, true)
	if err != nil {
		t.Fatalf("Alias with force returned error: %v", err)
	}
	if r.Port != 6380 {
		t.Errorf("expected port 6380, got %d", r.Port)
	}
}

func TestRouteTable_Unalias(t *testing.T) {
	rt := NewRouteTable(7777, false, "")
	rt.Alias("redis", 6379, false)

	err := rt.Unalias("redis")
	if err != nil {
		t.Fatalf("Unalias returned error: %v", err)
	}
	if rt.Has("redis") {
		t.Error("route still exists after unalias")
	}
}

func TestRouteTable_Unalias_NotFound(t *testing.T) {
	rt := NewRouteTable(7777, false, "")
	err := rt.Unalias("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent route, got nil")
	}
}

func TestRouteTable_Unalias_NotStatic(t *testing.T) {
	rt := NewRouteTable(7777, false, "")
	rt.Register("myapp", 4231, 0, "")

	err := rt.Unalias("myapp")
	if err == nil {
		t.Error("expected error for non-static route, got nil")
	}
}

func TestRouteTable_Lookup_ExactMatch(t *testing.T) {
	rt := NewRouteTable(7777, false, "")
	rt.Register("myapp", 4231, 0, "")

	r, ok := rt.Lookup("myapp")
	if !ok {
		t.Fatal("expected to find route 'myapp'")
	}
	if r.Port != 4231 {
		t.Errorf("expected port 4231, got %d", r.Port)
	}
}

func TestRouteTable_Lookup_CaseInsensitive(t *testing.T) {
	rt := NewRouteTable(7777, false, "")
	rt.Register("MyApp", 4231, 0, "")

	r, ok := rt.Lookup("MYAPP")
	if !ok {
		t.Fatal("expected case-insensitive match")
	}
	if r.Port != 4231 {
		t.Errorf("expected port 4231, got %d", r.Port)
	}
}

func TestRouteTable_Lookup_WildcardSubdomain(t *testing.T) {
	rt := NewRouteTable(7777, false, "")
	rt.Register("myapp", 4231, 0, "")

	// tenant1.myapp should fall back to myapp
	r, ok := rt.Lookup("tenant1.myapp")
	if !ok {
		t.Fatal("expected wildcard fallback to 'myapp'")
	}
	if r.Name != "myapp" {
		t.Errorf("expected route name 'myapp', got %q", r.Name)
	}
}

func TestRouteTable_Lookup_WildcardPrefersExact(t *testing.T) {
	rt := NewRouteTable(7777, false, "")
	rt.Register("myapp", 4231, 0, "")
	rt.Register("tenant1.myapp", 4232, 0, "")

	r, ok := rt.Lookup("tenant1.myapp")
	if !ok {
		t.Fatal("expected to find exact match 'tenant1.myapp'")
	}
	if r.Port != 4232 {
		t.Errorf("expected port 4232, got %d", r.Port)
	}
}

func TestRouteTable_Lookup_DeepWildcard(t *testing.T) {
	rt := NewRouteTable(7777, false, "")
	rt.Register("myapp", 4231, 0, "")

	// deep.sub.myapp → sub.myapp → myapp
	r, ok := rt.Lookup("deep.sub.myapp")
	if !ok {
		t.Fatal("expected deep wildcard fallback to 'myapp'")
	}
	if r.Name != "myapp" {
		t.Errorf("expected route name 'myapp', got %q", r.Name)
	}
}

func TestRouteTable_Lookup_NotFound(t *testing.T) {
	rt := NewRouteTable(7777, false, "")
	_, ok := rt.Lookup("nonexistent")
	if ok {
		t.Error("expected no route found")
	}
}

func TestRouteTable_List(t *testing.T) {
	rt := NewRouteTable(7777, false, "")
	rt.Register("app1", 4001, 0, "")
	rt.Register("app2", 4002, 0, "")
	rt.Alias("redis", 6379, false)

	routes := rt.List()
	if len(routes) != 3 {
		t.Errorf("expected 3 routes, got %d", len(routes))
	}
}

func TestRouteTable_Count(t *testing.T) {
	rt := NewRouteTable(7777, false, "")
	if rt.Count() != 0 {
		t.Error("expected 0 routes initially")
	}
	rt.Register("app1", 4001, 0, "")
	if rt.Count() != 1 {
		t.Errorf("expected 1 route, got %d", rt.Count())
	}
}

func TestRouteTable_ConcurrentAccess(t *testing.T) {
	rt := NewRouteTable(7777, false, "")
	var wg sync.WaitGroup

	// Concurrent writes
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			name := "app" + string(rune('a'+i%26))
			rt.Register(name, 4000+i, 0, "")
		}(i)
	}

	// Concurrent reads
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rt.List()
			rt.Lookup("appa")
			rt.Has("appa")
		}()
	}

	wg.Wait()
	// No race condition panic = success
}
