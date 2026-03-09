// Package port tests — verifies free port finding, range validation, and env var override.
//
// Author: Thiru K
// Module: github.com/thirukguru/localias/internal/port
package port

import (
	"fmt"
	"net"
	"os"
	"testing"
)

func TestFindFree_DefaultRange(t *testing.T) {
	ResetUsed()
	port, err := FindFree(0, 0)
	if err != nil {
		t.Fatalf("FindFree(0,0) returned error: %v", err)
	}
	if port < DefaultRangeStart || port > DefaultRangeEnd {
		t.Errorf("port %d outside default range %d-%d", port, DefaultRangeStart, DefaultRangeEnd)
	}
}

func TestFindFree_CustomRange(t *testing.T) {
	ResetUsed()
	port, err := FindFree(9100, 9110)
	if err != nil {
		t.Fatalf("FindFree(9100,9110) returned error: %v", err)
	}
	if port < 9100 || port > 9110 {
		t.Errorf("port %d outside range 9100-9110", port)
	}
}

func TestFindFree_InvalidRange(t *testing.T) {
	ResetUsed()
	_, err := FindFree(5000, 4000)
	if err == nil {
		t.Error("expected error for invalid range, got nil")
	}
}

func TestFindFree_PortIsAvailable(t *testing.T) {
	ResetUsed()
	port, err := FindFree(0, 0)
	if err != nil {
		t.Fatalf("FindFree returned error: %v", err)
	}
	// Verify the port is actually usable
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		t.Fatalf("assigned port %d is not available: %v", port, err)
	}
	ln.Close()
}

func TestFindFree_SkipsOccupiedPorts(t *testing.T) {
	ResetUsed()
	// Occupy port 9200
	ln, err := net.Listen("tcp", "127.0.0.1:9200")
	if err != nil {
		t.Skipf("cannot occupy port 9200: %v", err)
	}
	defer ln.Close()

	port, err := FindFree(9200, 9210)
	if err != nil {
		t.Fatalf("FindFree returned error: %v", err)
	}
	if port == 9200 {
		t.Error("FindFree returned occupied port 9200")
	}
}

func TestFindFree_NoFreePorts(t *testing.T) {
	ResetUsed()
	// Occupy a small range
	var listeners []net.Listener
	for p := 9300; p <= 9302; p++ {
		ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", p))
		if err != nil {
			t.Skipf("cannot occupy port %d: %v", p, err)
		}
		listeners = append(listeners, ln)
	}
	defer func() {
		for _, ln := range listeners {
			ln.Close()
		}
	}()

	_, err := FindFree(9300, 9302)
	if err == nil {
		t.Error("expected error when no ports are free, got nil")
	}
}

func TestFindFree_MultipleCalls_NoDuplicates(t *testing.T) {
	ResetUsed()
	seen := make(map[int]bool)
	for i := 0; i < 5; i++ {
		port, err := FindFree(9400, 9410)
		if err != nil {
			t.Fatalf("FindFree call %d returned error: %v", i, err)
		}
		if seen[port] {
			t.Errorf("duplicate port %d on call %d", port, i)
		}
		seen[port] = true
	}
}

func TestRelease(t *testing.T) {
	ResetUsed()
	port, err := FindFree(9500, 9500)
	if err != nil {
		t.Fatalf("FindFree returned error: %v", err)
	}
	Release(port)
	port2, err := FindFree(9500, 9500)
	if err != nil {
		t.Fatalf("FindFree after Release returned error: %v", err)
	}
	if port != port2 {
		t.Errorf("expected same port %d after release, got %d", port, port2)
	}
}

func TestFindFreeFromEnv(t *testing.T) {
	ResetUsed()
	os.Setenv("LOCALIAS_APP_PORT", "8888")
	defer os.Unsetenv("LOCALIAS_APP_PORT")

	port, err := FindFreeFromEnv()
	if err != nil {
		t.Fatalf("FindFreeFromEnv returned error: %v", err)
	}
	if port != 8888 {
		t.Errorf("expected port 8888, got %d", port)
	}
}

func TestFindFreeFromEnv_Invalid(t *testing.T) {
	ResetUsed()
	os.Setenv("LOCALIAS_APP_PORT", "notanumber")
	defer os.Unsetenv("LOCALIAS_APP_PORT")

	_, err := FindFreeFromEnv()
	if err == nil {
		t.Error("expected error for invalid LOCALIAS_APP_PORT, got nil")
	}
}

func TestFindFreeFromEnv_OutOfRange(t *testing.T) {
	ResetUsed()
	os.Setenv("LOCALIAS_APP_PORT", "99999")
	defer os.Unsetenv("LOCALIAS_APP_PORT")

	_, err := FindFreeFromEnv()
	if err == nil {
		t.Error("expected error for out-of-range LOCALIAS_APP_PORT, got nil")
	}
}

func TestFindFreeFromEnv_FallsBack(t *testing.T) {
	ResetUsed()
	os.Unsetenv("LOCALIAS_APP_PORT")

	port, err := FindFreeFromEnv()
	if err != nil {
		t.Fatalf("FindFreeFromEnv returned error: %v", err)
	}
	if port < DefaultRangeStart || port > DefaultRangeEnd {
		t.Errorf("port %d outside default range", port)
	}
}
