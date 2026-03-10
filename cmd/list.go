// Package cmd — list command showing active routes in a formatted table.
// Now with --health support that calls the health RPC to check each route.
//
// Author: Thiru K
// Module: github.com/thirukguru/localias/cmd
package cmd

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/thirukguru/localias/internal/daemon"
)

var (
	listHealth bool
	listJSON   bool
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List active routes",
	RunE:  runList,
}

func init() {
	listCmd.Flags().BoolVar(&listHealth, "health", false, "Run health checks before displaying")
	listCmd.Flags().BoolVar(&listJSON, "json", false, "Output as JSON array")
	rootCmd.AddCommand(listCmd)
}

type listEntry struct {
	Name    string `json:"name"`
	URL     string `json:"url"`
	Port    int    `json:"port"`
	Type    string `json:"type"`
	Health  string `json:"health,omitempty"`
	Latency string `json:"latency,omitempty"`
}

func runList(cmd *cobra.Command, args []string) error {
	stDir := GetStateDir()
	socketPath := filepath.Join(stDir, "localias.sock")
	client := daemon.NewClient(socketPath, stDir, slog.Default())

	result, err := client.List()
	if err != nil {
		return fmt.Errorf("listing routes: %w", err)
	}

	entries := make([]listEntry, len(result.Routes))
	for i, r := range result.Routes {
		routeType := "dynamic"
		if r.Static {
			routeType = "static"
		}
		entries[i] = listEntry{
			Name: r.Name,
			URL:  r.URL,
			Port: r.Port,
			Type: routeType,
		}

		// Health check each route if --health flag is set
		if listHealth {
			hr, err := client.Health(r.Name)
			if err == nil {
				entries[i].Health = hr.Status
				entries[i].Latency = hr.Latency
			} else {
				entries[i].Health = "unknown"
			}
		}
	}

	if listJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(entries)
	}

	if len(entries) == 0 {
		fmt.Println("No active routes. Use 'localias run <cmd>' or 'localias alias <name> <port>' to add routes.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	if listHealth {
		fmt.Fprintf(w, "NAME\tURL\tBACKEND\tTYPE\tHEALTH\tLATENCY\n")
		fmt.Fprintf(w, "────\t───\t───────\t────\t──────\t───────\n")
		for _, e := range entries {
			healthIcon := "⚪"
			switch e.Health {
			case "healthy":
				healthIcon = "🟢"
			case "unhealthy":
				healthIcon = "🔴"
			case "degraded":
				healthIcon = "🟡"
			}
			fmt.Fprintf(w, "%s\t%s\t:%d\t%s\t%s %s\t%s\n",
				e.Name, e.URL, e.Port, e.Type, healthIcon, e.Health, e.Latency)
		}
	} else {
		fmt.Fprintf(w, "NAME\tURL\tBACKEND\tTYPE\n")
		fmt.Fprintf(w, "────\t───\t───────\t────\n")
		for _, e := range entries {
			fmt.Fprintf(w, "%s\t%s\t:%d\t%s\n",
				e.Name, e.URL, e.Port, e.Type)
		}
	}
	w.Flush()

	return nil
}
