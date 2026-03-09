// Package cmd — run command that starts a dev server with a named .localhost URL.
// Infers project name, assigns a free port, injects env vars, and registers a route.
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
	"strings"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/thirukguru/localias/internal/daemon"
	"github.com/thirukguru/localias/internal/inject"
	"github.com/thirukguru/localias/internal/port"
	"github.com/thirukguru/localias/internal/project"
)

var shareLAN bool

var runCmd = &cobra.Command{
	Use:                "run [flags] -- <cmd> [args...]",
	Short:              "Run a command with a named .localhost URL",
	Long:               "Run a development command, automatically assigning a port and creating a .localhost URL.",
	DisableFlagParsing: false,
	Args:               cobra.MinimumNArgs(1),
	RunE:               runRun,
}

func init() {
	runCmd.Flags().BoolVar(&shareLAN, "share-lan", false, "Share on LAN via mDNS")
	rootCmd.AddCommand(runCmd)
}

func runRun(cmd *cobra.Command, args []string) error {
	// Check if localias is disabled
	if v := os.Getenv("LOCALIAS"); v == "0" || v == "skip" {
		return runDirect(args)
	}

	dir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	// Infer project name
	name, err := project.InferName(dir)
	if err != nil {
		return fmt.Errorf("inferring project name: %w", err)
	}

	return runWithName(name, args)
}

// runRunWithName runs a command with an explicit name (for shorthand).
func runRunWithName(name string, args []string) error {
	if v := os.Getenv("LOCALIAS"); v == "0" || v == "skip" {
		return runDirect(args)
	}
	return runWithName(name, args)
}

// runWithName is the shared implementation for run and shorthand.
func runWithName(name string, args []string) error {
	// Find free port
	appPort, err := port.FindFreeFromEnv()
	if err != nil {
		return fmt.Errorf("finding free port: %w", err)
	}

	proxyP := GetProxyPort()
	stDir := GetStateDir()

	scheme := "http"
	if IsHTTPSEnabled() {
		scheme = "https"
	}
	appURL := fmt.Sprintf("%s://%s.localhost:%d", scheme, name, proxyP)

	// Connect to daemon
	socketPath := stDir + "/localias.sock"
	client := daemon.NewClient(socketPath, stDir, slog.Default())

	// Register route
	result, err := client.Register(name, appPort, os.Getpid(), strings.Join(args, " "))
	if err != nil {
		return fmt.Errorf("registering route: %w", err)
	}

	// Print URL
	fmt.Printf("→ %s\n", result.URL)
	fmt.Printf("  Backend: 127.0.0.1:%d\n", appPort)

	// Write localias-routes.json to cwd
	writeRoutesJSON(client)

	// Auto-sync hosts if enabled
	if v := os.Getenv("LOCALIAS_SYNC_HOSTS"); v == "1" || v == "true" {
		syncHostsQuietly(client)
	}

	// Detect framework and inject flags
	injectedArgs := inject.InjectFlags(args, appPort)

	dir, _ := os.Getwd()

	// Build child command
	child := exec.Command(injectedArgs[0], injectedArgs[1:]...)
	child.Dir = dir
	child.Stdout = os.Stdout
	child.Stderr = os.Stderr
	child.Stdin = os.Stdin

	// Set env vars
	child.Env = append(os.Environ(),
		fmt.Sprintf("PORT=%d", appPort),
		"HOST=127.0.0.1",
		fmt.Sprintf("LOCALIAS_URL=%s", appURL),
	)

	// Forward signals to child
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		sig := <-sigCh
		if child.Process != nil {
			child.Process.Signal(sig)
		}
	}()

	// Run child process
	err = child.Run()

	// Deregister route on exit
	if deregErr := client.Deregister(name); deregErr != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to deregister route: %v\n", deregErr)
	}

	// Update routes.json after deregister
	writeRoutesJSON(client)

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		return fmt.Errorf("running command: %w", err)
	}

	return nil
}

func runDirect(args []string) error {
	child := exec.Command(args[0], args[1:]...)
	child.Stdout = os.Stdout
	child.Stderr = os.Stderr
	child.Stdin = os.Stdin
	return child.Run()
}

// writeRoutesJSON writes a localias-routes.json to the current directory.
func writeRoutesJSON(client *daemon.Client) {
	result, err := client.List()
	if err != nil {
		return
	}
	data := "[\n"
	for i, r := range result.Routes {
		data += fmt.Sprintf("  {\"name\": %q, \"url\": %q, \"port\": %d}", r.Name, r.URL, r.Port)
		if i < len(result.Routes)-1 {
			data += ","
		}
		data += "\n"
	}
	data += "]\n"
	os.WriteFile("localias-routes.json", []byte(data), 0644)
}

// syncHostsQuietly silently syncs routes to /etc/hosts.
func syncHostsQuietly(client *daemon.Client) {
	result, err := client.List()
	if err != nil {
		return
	}
	if len(result.Routes) == 0 {
		return
	}
	// Build hosts block
	block := "# BEGIN localias managed block\n"
	for _, r := range result.Routes {
		block += fmt.Sprintf("127.0.0.1  %s.localhost\n", r.Name)
	}
	block += "# END localias managed block\n"
	// Try to write — will fail silently without sudo
	content, err := os.ReadFile("/etc/hosts")
	if err != nil {
		return
	}
	cleaned := removeLocaliasBlock(string(content))
	os.WriteFile("/etc/hosts", []byte(cleaned+"\n"+block), 0644)
}

// removeLocaliasBlock removes the localias managed block from hosts content.
func removeLocaliasBlock(content string) string {
	var result strings.Builder
	inBlock := false
	for _, line := range strings.Split(content, "\n") {
		if strings.TrimSpace(line) == "# BEGIN localias managed block" {
			inBlock = true
			continue
		}
		if strings.TrimSpace(line) == "# END localias managed block" {
			inBlock = false
			continue
		}
		if !inBlock {
			result.WriteString(line + "\n")
		}
	}
	return strings.TrimRight(result.String(), "\n") + "\n"
}
