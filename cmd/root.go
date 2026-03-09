// Package cmd contains all CLI commands for localias, built with cobra.
// This file defines the root command and global flags.
//
// Author: Thiru K
// Module: github.com/thirukguru/localias/cmd
package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/spf13/cobra"
)

var (
	// stateDir is the directory for localias state files.
	stateDir string
	// proxyPort is the port the reverse proxy listens on.
	proxyPort int
)

// rootCmd represents the base command when called without any subcommands.
var rootCmd = &cobra.Command{
	Use:   "localias [name] [flags] -- <cmd> [args...]",
	Short: "Local reverse proxy — stable .localhost URLs for development",
	Long: `Localias replaces port numbers with stable named .localhost URLs.

Instead of remembering http://localhost:4231, use http://myapp.localhost:7777.
Supports WebSocket proxying, HTTPS with auto-generated certificates,
health checks, traffic logging, and a built-in dashboard.

Shorthand: localias myapp -- npm run dev`,
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE:          runShorthand,
}

// Execute runs the root command.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&stateDir, "state-dir", "", "Override state directory (default: auto-detected)")
	rootCmd.PersistentFlags().IntVar(&proxyPort, "port", 0, "Proxy port (default: 7777, or LOCALIAS_PORT env)")
}

// GetStateDir returns the resolved state directory path.
// Priority: --state-dir flag > LOCALIAS_STATE_DIR env > default based on proxy port.
func GetStateDir() string {
	if stateDir != "" {
		return stateDir
	}
	if envDir := os.Getenv("LOCALIAS_STATE_DIR"); envDir != "" {
		return envDir
	}
	port := GetProxyPort()
	if port < 1024 {
		return "/tmp/localias"
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "/tmp/localias"
	}
	return filepath.Join(home, ".localias")
}

// GetProxyPort returns the resolved proxy port.
// Priority: --port flag > LOCALIAS_PORT env > default 7777.
func GetProxyPort() int {
	if proxyPort > 0 {
		return proxyPort
	}
	if envPort := os.Getenv("LOCALIAS_PORT"); envPort != "" {
		if p, err := strconv.Atoi(envPort); err == nil && p > 0 {
			return p
		}
	}
	return 7777
}

// IsHTTPSEnabled checks if HTTPS is enabled via env var.
func IsHTTPSEnabled() bool {
	if httpsEnabled {
		return true
	}
	v := os.Getenv("LOCALIAS_HTTPS")
	return v == "1" || v == "true"
}

// EnsureStateDir creates the state directory if it doesn't exist.
func EnsureStateDir() error {
	dir := GetStateDir()
	return os.MkdirAll(dir, 0755)
}

// runShorthand handles `localias <name> -- <cmd>` shorthand.
func runShorthand(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return cmd.Help()
	}
	// First arg is the explicit name, rest is the command
	explicitName := args[0]
	if len(args) < 2 {
		return fmt.Errorf("usage: localias <name> -- <cmd> [args...]")
	}
	cmdArgs := args[1:]
	// Delegate to runRunWithName
	return runRunWithName(explicitName, cmdArgs)
}
