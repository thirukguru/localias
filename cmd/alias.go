// Package cmd — alias command for managing static routes.
// Static routes map a name to a port for Docker containers, external processes, etc.
//
// Author: Thiru K
// Module: github.com/thirukguru/localias/cmd
package cmd

import (
	"fmt"
	"log/slog"
	"path/filepath"
	"strconv"

	"github.com/spf13/cobra"
	"github.com/thirukguru/localias/internal/daemon"
)

var (
	aliasForce  bool
	aliasRemove string
)

var aliasCmd = &cobra.Command{
	Use:   "alias <name> <port>",
	Short: "Create a static route alias",
	Long:  "Register a static route mapping a name to a backend port.\nUseful for Docker containers, databases, or any externally managed process.",
	Args:  cobra.RangeArgs(0, 2),
	RunE:  runAlias,
}

func init() {
	aliasCmd.Flags().BoolVarP(&aliasForce, "force", "f", false, "Overwrite existing route")
	aliasCmd.Flags().StringVar(&aliasRemove, "remove", "", "Remove a static route by name")
	rootCmd.AddCommand(aliasCmd)
}

func runAlias(cmd *cobra.Command, args []string) error {
	stDir := GetStateDir()
	socketPath := filepath.Join(stDir, "localias.sock")
	client := daemon.NewClient(socketPath, stDir, slog.Default())

	// Handle --remove
	if aliasRemove != "" {
		if err := client.Unalias(aliasRemove); err != nil {
			return fmt.Errorf("removing alias: %w", err)
		}
		fmt.Printf("✓ Removed alias %q\n", aliasRemove)
		return nil
	}

	if len(args) < 2 {
		return fmt.Errorf("usage: localias alias <name> <port>")
	}

	name := args[0]
	portNum, err := strconv.Atoi(args[1])
	if err != nil {
		return fmt.Errorf("invalid port %q: %w", args[1], err)
	}

	if err := client.Alias(name, portNum, aliasForce); err != nil {
		return fmt.Errorf("setting alias: %w", err)
	}

	proxyP := GetProxyPort()
	scheme := "http"
	if httpsEnabled {
		scheme = "https"
	}
	fmt.Printf("✓ %s → %s://%s.localhost:%d\n", name, scheme, name, proxyP)
	return nil
}
