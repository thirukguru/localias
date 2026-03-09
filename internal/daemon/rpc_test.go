// Package daemon tests — verifies RPC register/deregister/list over in-process Unix socket.
//
// Author: Thiru K
// Module: github.com/thirukguru/localias/internal/daemon
package daemon

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/thirukguru/localias/internal/proxy"
)

func setupTestRPC(t *testing.T) (*RPCServer, *Client, *proxy.RouteTable, context.CancelFunc) {
	t.Helper()

	dir := t.TempDir()
	socketPath := filepath.Join(dir, "test.sock")
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))

	routes := proxy.NewRouteTable(7777, false, "")

	server := NewRPCServer(socketPath, logger)

	// Register handlers
	server.Handle("register", func(params json.RawMessage) (interface{}, error) {
		var p RegisterParams
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, err
		}
		r, err := routes.Register(p.Name, p.Port, p.PID, p.Cmd)
		if err != nil {
			return nil, err
		}
		return RegisterResult{URL: r.URL}, nil
	})

	server.Handle("deregister", func(params json.RawMessage) (interface{}, error) {
		var p DeregisterParams
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, err
		}
		routes.Deregister(p.Name)
		return struct{}{}, nil
	})

	server.Handle("list", func(params json.RawMessage) (interface{}, error) {
		list := routes.List()
		result := ListResult{Routes: make([]RouteInfo, len(list))}
		for i, r := range list {
			result.Routes[i] = RouteInfo{
				Name:   r.Name,
				Port:   r.Port,
				URL:    r.URL,
				PID:    r.PID,
				Cmd:    r.Cmd,
				Static: r.Static,
			}
		}
		return result, nil
	})

	ctx, cancel := context.WithCancel(context.Background())

	// Start server in background
	go server.Start(ctx)

	// Wait for socket to be created
	for i := 0; i < 20; i++ {
		if _, err := os.Stat(socketPath); err == nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	client := &Client{
		socketPath: socketPath,
		stateDir:   dir,
		logger:     logger,
	}

	return server, client, routes, cancel
}

func TestRPC_Register(t *testing.T) {
	_, client, _, cancel := setupTestRPC(t)
	defer cancel()

	result, err := client.Register("myapp", 4231, 1234, "npm run dev")
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}
	if result.URL != "http://myapp.localhost:7777" {
		t.Errorf("expected URL 'http://myapp.localhost:7777', got %q", result.URL)
	}
}

func TestRPC_Deregister(t *testing.T) {
	_, client, routes, cancel := setupTestRPC(t)
	defer cancel()

	client.Register("myapp", 4231, 1234, "")

	err := client.Deregister("myapp")
	if err != nil {
		t.Fatalf("Deregister failed: %v", err)
	}
	if routes.Has("myapp") {
		t.Error("route still exists after deregister")
	}
}

func TestRPC_List_Empty(t *testing.T) {
	_, client, _, cancel := setupTestRPC(t)
	defer cancel()

	result, err := client.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(result.Routes) != 0 {
		t.Errorf("expected 0 routes, got %d", len(result.Routes))
	}
}

func TestRPC_List_WithRoutes(t *testing.T) {
	_, client, _, cancel := setupTestRPC(t)
	defer cancel()

	client.Register("app1", 4001, 100, "cmd1")
	client.Register("app2", 4002, 200, "cmd2")

	result, err := client.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(result.Routes) != 2 {
		t.Errorf("expected 2 routes, got %d", len(result.Routes))
	}
}

func TestRPC_Register_Deregister_List(t *testing.T) {
	_, client, _, cancel := setupTestRPC(t)
	defer cancel()

	// Register → list → deregister → list
	client.Register("myapp", 4231, 0, "")

	list1, _ := client.List()
	if len(list1.Routes) != 1 {
		t.Errorf("expected 1 route after register, got %d", len(list1.Routes))
	}

	client.Deregister("myapp")

	list2, _ := client.List()
	if len(list2.Routes) != 0 {
		t.Errorf("expected 0 routes after deregister, got %d", len(list2.Routes))
	}
}

func TestRPC_InvalidMethod(t *testing.T) {
	_, client, _, cancel := setupTestRPC(t)
	defer cancel()

	err := client.Call("nonexistent", nil, nil)
	if err == nil {
		t.Error("expected error for invalid method, got nil")
	}
}

func TestRPC_MultipleClients(t *testing.T) {
	_, client, _, cancel := setupTestRPC(t)
	defer cancel()

	// Simulate multiple concurrent clients
	done := make(chan error, 5)
	for i := 0; i < 5; i++ {
		go func(i int) {
			_, err := client.Register("app"+string(rune('a'+i)), 4000+i, i, "")
			done <- err
		}(i)
	}
	for i := 0; i < 5; i++ {
		if err := <-done; err != nil {
			t.Errorf("concurrent register %d failed: %v", i, err)
		}
	}

	result, _ := client.List()
	if len(result.Routes) != 5 {
		t.Errorf("expected 5 routes, got %d", len(result.Routes))
	}
}

func TestDaemon_PIDFile(t *testing.T) {
	dir := t.TempDir()
	d := NewDaemon(dir, slog.Default())

	err := d.WritePID()
	if err != nil {
		t.Fatalf("WritePID failed: %v", err)
	}

	pid, err := d.ReadPID()
	if err != nil {
		t.Fatalf("ReadPID failed: %v", err)
	}
	if pid != os.Getpid() {
		t.Errorf("expected PID %d, got %d", os.Getpid(), pid)
	}
}

func TestDaemon_IsRunning(t *testing.T) {
	dir := t.TempDir()
	d := NewDaemon(dir, slog.Default())

	// No PID file → not running
	if d.IsRunning() {
		t.Error("expected not running without PID file")
	}

	// Write our own PID → should be running
	d.WritePID()
	if !d.IsRunning() {
		t.Error("expected running after writing own PID")
	}
}

func TestDaemon_Cleanup(t *testing.T) {
	dir := t.TempDir()
	d := NewDaemon(dir, slog.Default())

	d.WritePID()
	d.WritePort(7777)

	d.Cleanup()

	if _, err := os.Stat(d.PIDFile()); !os.IsNotExist(err) {
		t.Error("PID file still exists after cleanup")
	}
	if _, err := os.Stat(d.PortFile()); !os.IsNotExist(err) {
		t.Error("port file still exists after cleanup")
	}
}

func TestClient_IsConnectable(t *testing.T) {
	dir := t.TempDir()
	socketPath := filepath.Join(dir, "test.sock")
	client := NewClient(socketPath, dir, slog.Default())

	// No socket → not connectable
	if client.IsConnectable() {
		t.Error("expected not connectable without socket")
	}
}
