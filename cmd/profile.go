// Package cmd — profile commands for managing service groups from localias.yaml.
//
// Author: Thiru K
// Module: github.com/thirukguru/localias/cmd
package cmd

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
	"github.com/thirukguru/localias/internal/daemon"
	"github.com/thirukguru/localias/internal/port"
	"github.com/thirukguru/localias/internal/profile"
)

var profileName string

var profileCmd = &cobra.Command{
	Use:   "profile",
	Short: "Manage service profiles from localias.yaml",
}

var profileStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start all services in a profile",
	RunE:  runProfileStart,
}

var profileStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop all services in a profile",
	RunE:  runProfileStop,
}

var profileListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available profiles",
	RunE:  runProfileList,
}

func init() {
	profileStartCmd.Flags().StringVar(&profileName, "profile", "default", "Profile name to start")
	profileStopCmd.Flags().StringVar(&profileName, "profile", "default", "Profile name to stop")

	profileCmd.AddCommand(profileStartCmd, profileStopCmd, profileListCmd)
	rootCmd.AddCommand(profileCmd)
}

// lipgloss styles for service log prefixes
var prefixStyles = []lipgloss.Style{
	lipgloss.NewStyle().Foreground(lipgloss.Color("#00d7d7")).Bold(true), // cyan
	lipgloss.NewStyle().Foreground(lipgloss.Color("#d7d700")).Bold(true), // yellow
	lipgloss.NewStyle().Foreground(lipgloss.Color("#d700d7")).Bold(true), // magenta
	lipgloss.NewStyle().Foreground(lipgloss.Color("#00d700")).Bold(true), // green
	lipgloss.NewStyle().Foreground(lipgloss.Color("#0087ff")).Bold(true), // blue
	lipgloss.NewStyle().Foreground(lipgloss.Color("#ff5f5f")).Bold(true), // red
}

func runProfileStart(cmd *cobra.Command, args []string) error {
	configPath, err := profile.FindConfig()
	if err != nil {
		return fmt.Errorf("finding config: %w", err)
	}

	cfg, err := profile.LoadConfig(configPath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	p, err := cfg.GetProfile(profileName)
	if err != nil {
		return err
	}

	stDir := GetStateDir()
	socketPath := filepath.Join(stDir, "localias.sock")
	client := daemon.NewClient(socketPath, stDir, slog.Default())

	fmt.Printf("Starting profile %q (%d services)\n\n", profileName, len(p.Services))

	var wg sync.WaitGroup
	processes := make([]*exec.Cmd, 0, len(p.Services))
	var mu sync.Mutex

	for i, svc := range p.Services {
		style := prefixStyles[i%len(prefixStyles)]
		prefix := style.Render("["+svc.Name+"]") + " "

		// Find free port
		appPort, err := port.FindFree(0, 0)
		if err != nil {
			fmt.Printf("%sError finding port: %v\n", prefix, err)
			continue
		}

		// Register route
		result, err := client.Register(svc.Name, appPort, os.Getpid(), svc.Cmd)
		if err != nil {
			fmt.Printf("%sError registering: %v\n", prefix, err)
			continue
		}
		fmt.Printf("%s→ %s (port %d)\n", prefix, result.URL, appPort)

		// Start child process
		parts := strings.Fields(svc.Cmd)
		child := exec.Command(parts[0], parts[1:]...)
		if svc.Dir != "" {
			child.Dir = svc.Dir
		}
		child.Env = append(os.Environ(),
			fmt.Sprintf("PORT=%d", appPort),
			"HOST=127.0.0.1",
		)
		child.Stdout = &prefixWriter{prefix: prefix, out: os.Stdout}
		child.Stderr = &prefixWriter{prefix: prefix, out: os.Stderr}

		if err := child.Start(); err != nil {
			fmt.Printf("%sError starting: %v\n", prefix, err)
			continue
		}

		mu.Lock()
		processes = append(processes, child)
		mu.Unlock()

		wg.Add(1)
		go func(name string) {
			defer wg.Done()
			child.Wait()
			client.Deregister(name)
		}(svc.Name)
	}

	// Handle Ctrl-C
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-sigCh
		fmt.Println("\nStopping all services...")
		mu.Lock()
		for _, p := range processes {
			if p.Process != nil {
				p.Process.Signal(syscall.SIGTERM)
			}
		}
		mu.Unlock()
	}()

	wg.Wait()
	return nil
}

func runProfileStop(cmd *cobra.Command, args []string) error {
	// Load config to get the service names for this profile
	configPath, err := profile.FindConfig()
	if err != nil {
		return fmt.Errorf("finding config: %w", err)
	}
	cfg, err := profile.LoadConfig(configPath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	p, err := cfg.GetProfile(profileName)
	if err != nil {
		return err
	}

	stDir := GetStateDir()
	socketPath := filepath.Join(stDir, "localias.sock")
	client := daemon.NewClient(socketPath, stDir, slog.Default())

	// Get running routes to find PIDs
	result, err := client.List()
	if err != nil {
		return fmt.Errorf("listing routes: %w", err)
	}

	// Build lookup of route name → PID
	pidMap := make(map[string]int)
	for _, r := range result.Routes {
		if r.PID > 0 {
			pidMap[r.Name] = r.PID
		}
	}

	stopped := 0
	for _, svc := range p.Services {
		// Deregister route
		if err := client.Deregister(svc.Name); err != nil {
			fmt.Fprintf(os.Stderr, "  warning: could not deregister %s: %v\n", svc.Name, err)
		}

		// Send SIGTERM to the process
		if pid, ok := pidMap[svc.Name]; ok {
			proc, err := os.FindProcess(pid)
			if err == nil {
				proc.Signal(syscall.SIGTERM)
				fmt.Printf("  ✓ Stopped %s (PID %d)\n", svc.Name, pid)
				stopped++
			}
		} else {
			fmt.Printf("  ✓ Deregistered %s (process not tracked)\n", svc.Name)
			stopped++
		}
	}

	if stopped == 0 {
		fmt.Println("No services were running for this profile")
	} else {
		fmt.Printf("\n✓ Stopped %d services from profile %q\n", stopped, profileName)
	}
	return nil
}

func runProfileList(cmd *cobra.Command, args []string) error {
	configPath, err := profile.FindConfig()
	if err != nil {
		return fmt.Errorf("finding config: %w", err)
	}

	cfg, err := profile.LoadConfig(configPath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	for _, name := range cfg.ListProfiles() {
		p, _ := cfg.GetProfile(name)
		fmt.Printf("  %s (%d services)\n", name, len(p.Services))
		for _, svc := range p.Services {
			fmt.Printf("    - %s: %s\n", svc.Name, svc.Cmd)
		}
	}
	return nil
}

// prefixWriter adds a prefix to each line of output.
type prefixWriter struct {
	prefix string
	out    *os.File
	mu     sync.Mutex
	buf    []byte
}

func (w *prefixWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.buf = append(w.buf, p...)
	for {
		idx := -1
		for i, b := range w.buf {
			if b == '\n' {
				idx = i
				break
			}
		}
		if idx == -1 {
			break
		}
		line := w.buf[:idx+1]
		w.out.WriteString(w.prefix + string(line))
		w.buf = w.buf[idx+1:]
	}
	return len(p), nil
}
