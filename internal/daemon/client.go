// Package daemon — JSON-RPC 2.0 client for communicating with the daemon.
// Used by CLI commands to interact with the background proxy process.
// Includes auto-start logic to ensure the daemon is running.
//
// Author: Thiru K
// Module: github.com/thirukguru/localias/internal/daemon
package daemon

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"sync/atomic"
	"time"
)

// Client is a JSON-RPC 2.0 client that connects to the daemon via Unix socket.
type Client struct {
	socketPath string
	stateDir   string
	logger     *slog.Logger
	idCounter  atomic.Int64
}

// NewClient creates a new RPC client.
func NewClient(socketPath, stateDir string, logger *slog.Logger) *Client {
	if logger == nil {
		logger = slog.Default()
	}
	return &Client{
		socketPath: socketPath,
		stateDir:   stateDir,
		logger:     logger,
	}
}

// Call invokes an RPC method on the daemon and returns the result.
func (c *Client) Call(method string, params interface{}, result interface{}) error {
	conn, err := c.connect()
	if err != nil {
		return fmt.Errorf("connecting to daemon: %w", err)
	}
	defer conn.Close()

	// Serialize params
	var paramsJSON json.RawMessage
	if params != nil {
		data, err := json.Marshal(params)
		if err != nil {
			return fmt.Errorf("marshaling params: %w", err)
		}
		paramsJSON = data
	}

	// Build request
	id := int(c.idCounter.Add(1))
	req := RPCRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  paramsJSON,
	}

	// Send request
	reqJSON, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshaling request: %w", err)
	}
	if _, err := conn.Write(append(reqJSON, '\n')); err != nil {
		return fmt.Errorf("writing request: %w", err)
	}

	// Read response
	scanner := bufio.NewScanner(conn)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return fmt.Errorf("reading response: %w", err)
		}
		return fmt.Errorf("no response from daemon")
	}

	var resp RPCResponse
	if err := json.Unmarshal(scanner.Bytes(), &resp); err != nil {
		return fmt.Errorf("parsing response: %w", err)
	}

	if resp.Error != nil {
		return fmt.Errorf("RPC error (%d): %s", resp.Error.Code, resp.Error.Message)
	}

	if result != nil && resp.Result != nil {
		if err := json.Unmarshal(resp.Result, result); err != nil {
			return fmt.Errorf("parsing result: %w", err)
		}
	}

	return nil
}

// Register registers a route with the daemon.
func (c *Client) Register(name string, port, pid int, cmd string) (*RegisterResult, error) {
	params := RegisterParams{
		Name: name,
		Port: port,
		PID:  pid,
		Cmd:  cmd,
	}
	var result RegisterResult
	if err := c.Call("register", params, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// Deregister removes a route from the daemon.
func (c *Client) Deregister(name string) error {
	params := DeregisterParams{Name: name}
	return c.Call("deregister", params, nil)
}

// Alias registers a static route with the daemon.
func (c *Client) Alias(name string, port int, force bool) error {
	params := AliasParams{
		Name:  name,
		Port:  port,
		Force: force,
	}
	return c.Call("alias", params, nil)
}

// Unalias removes a static route from the daemon.
func (c *Client) Unalias(name string) error {
	params := UnaliasParams{Name: name}
	return c.Call("unalias", params, nil)
}

// List retrieves all routes from the daemon.
func (c *Client) List() (*ListResult, error) {
	var result ListResult
	if err := c.Call("list", nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// Health retrieves health status for a route.
func (c *Client) Health(name string) (*HealthResult, error) {
	params := HealthParams{Name: name}
	var result HealthResult
	if err := c.Call("health", params, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// StopDaemon tells the daemon to shut down.
func (c *Client) StopDaemon() error {
	return c.Call("stop", nil, nil)
}

// MCPTokenCreate creates a scoped MCP token via the daemon.
func (c *Client) MCPTokenCreate(routes, capabilities []string, pid int, label string) (*MCPTokenCreateResult, error) {
	params := MCPTokenCreateParams{
		Routes:       routes,
		Capabilities: capabilities,
		PID:          pid,
		Label:        label,
	}
	var result MCPTokenCreateResult
	if err := c.Call("mcp.token.create", params, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// MCPTokenList lists all scoped MCP tokens via the daemon.
func (c *Client) MCPTokenList() (*MCPTokenListResult, error) {
	var result MCPTokenListResult
	if err := c.Call("mcp.token.list", nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// MCPTokenRevoke revokes MCP tokens matching the given prefix.
func (c *Client) MCPTokenRevoke(prefix string) (*MCPTokenRevokeResult, error) {
	params := MCPTokenRevokeParams{Prefix: prefix}
	var result MCPTokenRevokeResult
	if err := c.Call("mcp.token.revoke", params, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// connect establishes a connection to the daemon, auto-starting it if necessary.
func (c *Client) connect() (net.Conn, error) {
	// Try direct connection first
	conn, err := net.DialTimeout("unix", c.socketPath, 2*time.Second)
	if err == nil {
		return conn, nil
	}

	c.logger.Info("daemon not running, starting...")
	if err := c.autoStart(); err != nil {
		return nil, fmt.Errorf("auto-starting daemon: %w", err)
	}

	// Retry with backoff
	for i := 0; i < 5; i++ {
		time.Sleep(time.Duration(200*(i+1)) * time.Millisecond)
		conn, err = net.DialTimeout("unix", c.socketPath, 2*time.Second)
		if err == nil {
			return conn, nil
		}
	}

	return nil, fmt.Errorf("could not connect to daemon after auto-start")
}

// autoStart launches the daemon as a background process.
func (c *Client) autoStart() error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolving executable: %w", err)
	}

	args := []string{"proxy", "start", "--state-dir", c.stateDir}
	cmd := exec.Command(exe, args...)
	cmd.Stdout = nil
	cmd.Stderr = nil

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting daemon process: %w", err)
	}

	// Don't wait for it — let it run in background
	go cmd.Wait()

	return nil
}

// IsConnectable checks if the daemon socket is connectable.
func (c *Client) IsConnectable() bool {
	conn, err := net.DialTimeout("unix", c.socketPath, 1*time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}
