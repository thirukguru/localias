// Package cmd — tunnel command for SSH reverse tunneling.
//
// Author: Thiru K
// Module: github.com/thirukguru/localias/cmd
package cmd

import (
	"fmt"
	"log/slog"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/thirukguru/localias/internal/daemon"
	"github.com/thirukguru/localias/internal/tunnel"
)

var tunnelCmd = &cobra.Command{
	Use:   "tunnel <name>",
	Short: "Expose a local service via SSH tunnel",
	Long:  "Open an SSH reverse tunnel to a relay server to expose a local service publicly.",
	Args:  cobra.ExactArgs(1),
	RunE:  runTunnel,
}

func init() {
	rootCmd.AddCommand(tunnelCmd)
}

func runTunnel(cmd *cobra.Command, args []string) error {
	name := args[0]
	stDir := GetStateDir()
	socketPath := filepath.Join(stDir, "localias.sock")
	client := daemon.NewClient(socketPath, stDir, slog.Default())

	// Find the route to get the local port
	result, err := client.List()
	if err != nil {
		return fmt.Errorf("listing routes: %w", err)
	}

	for _, r := range result.Routes {
		if r.Name == name {
			return tunnel.Start(name, r.Port)
		}
	}

	return fmt.Errorf("route %q not found. Register it first with 'localias run' or 'localias alias'", name)
}
