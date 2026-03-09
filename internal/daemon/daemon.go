// Package daemon manages the background proxy process lifecycle.
// Handles daemonization, PID file management, signal handling, and graceful shutdown.
//
// Author: Thiru K
// Module: github.com/thirukguru/localias/internal/daemon
package daemon

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// Daemon manages the background proxy process.
type Daemon struct {
	stateDir string
	logger   *slog.Logger
}

// NewDaemon creates a new daemon manager.
func NewDaemon(stateDir string, logger *slog.Logger) *Daemon {
	if logger == nil {
		logger = slog.Default()
	}
	return &Daemon{
		stateDir: stateDir,
		logger:   logger,
	}
}

// PIDFile returns the path to the PID file.
func (d *Daemon) PIDFile() string {
	return filepath.Join(d.stateDir, "localias.pid")
}

// SocketPath returns the path to the Unix socket.
func (d *Daemon) SocketPath() string {
	return filepath.Join(d.stateDir, "localias.sock")
}

// LogFile returns the path to the daemon log file.
func (d *Daemon) LogFile() string {
	return filepath.Join(d.stateDir, "localias.log")
}

// PortFile returns the path to the port file.
func (d *Daemon) PortFile() string {
	return filepath.Join(d.stateDir, "localias.port")
}

// WritePID writes the current process PID to the PID file.
func (d *Daemon) WritePID() error {
	if err := os.MkdirAll(d.stateDir, 0755); err != nil {
		return fmt.Errorf("creating state dir: %w", err)
	}
	return os.WriteFile(d.PIDFile(), []byte(strconv.Itoa(os.Getpid())), 0644)
}

// WritePort writes the proxy port to the port file.
func (d *Daemon) WritePort(port int) error {
	return os.WriteFile(d.PortFile(), []byte(strconv.Itoa(port)), 0644)
}

// ReadPID reads the daemon PID from the PID file.
func (d *Daemon) ReadPID() (int, error) {
	data, err := os.ReadFile(d.PIDFile())
	if err != nil {
		return 0, fmt.Errorf("reading PID file: %w", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, fmt.Errorf("parsing PID: %w", err)
	}
	return pid, nil
}

// IsRunning checks if the daemon process is running.
func (d *Daemon) IsRunning() bool {
	pid, err := d.ReadPID()
	if err != nil {
		return false
	}
	return isProcessRunning(pid)
}

// Stop sends SIGTERM to the daemon process.
func (d *Daemon) Stop() error {
	pid, err := d.ReadPID()
	if err != nil {
		return fmt.Errorf("daemon not running: %w", err)
	}

	proc, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("finding process %d: %w", pid, err)
	}

	if err := proc.Signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf("sending SIGTERM to %d: %w", pid, err)
	}

	d.logger.Info("sent SIGTERM to daemon", "pid", pid)
	return nil
}

// Cleanup removes PID file, socket, and port file.
func (d *Daemon) Cleanup() {
	os.Remove(d.PIDFile())
	os.Remove(d.SocketPath())
	os.Remove(d.PortFile())
}

// Daemonize starts the current binary as a background daemon process.
func (d *Daemon) Daemonize(args []string) error {
	if err := os.MkdirAll(d.stateDir, 0755); err != nil {
		return fmt.Errorf("creating state dir: %w", err)
	}

	// Open log file
	logFile, err := os.OpenFile(d.LogFile(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("opening log file: %w", err)
	}

	// Get the current executable path
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolving executable: %w", err)
	}

	cmd := exec.Command(exe, args...)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid: true,
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting daemon: %w", err)
	}

	d.logger.Info("daemon started", "pid", cmd.Process.Pid)
	return nil
}

// WaitForSignal blocks until SIGTERM or SIGINT is received.
// Calls the cleanup function before returning.
func (d *Daemon) WaitForSignal(cleanup func()) {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)

	sig := <-sigCh
	d.logger.Info("received signal, shutting down", "signal", sig)

	if cleanup != nil {
		cleanup()
	}
	d.Cleanup()
}

// isProcessRunning checks if a process with the given PID is running.
func isProcessRunning(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// On Unix, FindProcess always succeeds. Check with signal 0.
	err = proc.Signal(syscall.Signal(0))
	return err == nil
}
