// Package cmd — hosts command to manage /etc/hosts entries for localias routes.
//
// Author: Thiru K
// Module: github.com/thirukguru/localias/cmd
package cmd

import (
	"bufio"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/thirukguru/localias/internal/daemon"
)

const hostsMarkerStart = "# BEGIN localias managed block"
const hostsMarkerEnd = "# END localias managed block"

var hostsCmd = &cobra.Command{
	Use:   "hosts",
	Short: "Manage /etc/hosts entries",
}

var hostsSyncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Write active routes to /etc/hosts",
	Long:  "Add all active route names as .localhost entries in /etc/hosts.\nRequires sudo/root privileges.",
	RunE:  runHostsSync,
}

var hostsCleanCmd = &cobra.Command{
	Use:   "clean",
	Short: "Remove localias entries from /etc/hosts",
	RunE:  runHostsClean,
}

func init() {
	hostsCmd.AddCommand(hostsSyncCmd, hostsCleanCmd)
	rootCmd.AddCommand(hostsCmd)
}

func runHostsSync(cmd *cobra.Command, args []string) error {
	stDir := GetStateDir()
	socketPath := filepath.Join(stDir, "localias.sock")
	client := daemon.NewClient(socketPath, stDir, slog.Default())

	result, err := client.List()
	if err != nil {
		return fmt.Errorf("listing routes: %w", err)
	}

	if len(result.Routes) == 0 {
		fmt.Println("No active routes to sync")
		return nil
	}

	// Read existing /etc/hosts
	content, err := os.ReadFile("/etc/hosts")
	if err != nil {
		return fmt.Errorf("reading /etc/hosts: %w", err)
	}

	// Remove existing localias block
	cleaned := removeHostsBlock(string(content))

	// Build new block
	var block strings.Builder
	block.WriteString(hostsMarkerStart + "\n")
	for _, r := range result.Routes {
		if !isValidHostname(r.Name) {
			continue
		}
		block.WriteString(fmt.Sprintf("127.0.0.1  %s.localhost\n", r.Name))
	}
	block.WriteString(hostsMarkerEnd + "\n")

	newContent := cleaned + "\n" + block.String()

	if err := os.WriteFile("/etc/hosts", []byte(newContent), 0644); err != nil {
		return fmt.Errorf("writing /etc/hosts (try with sudo): %w", err)
	}

	fmt.Printf("✓ Synced %d routes to /etc/hosts\n", len(result.Routes))
	return nil
}

func runHostsClean(cmd *cobra.Command, args []string) error {
	content, err := os.ReadFile("/etc/hosts")
	if err != nil {
		return fmt.Errorf("reading /etc/hosts: %w", err)
	}

	cleaned := removeHostsBlock(string(content))

	if err := os.WriteFile("/etc/hosts", []byte(cleaned), 0644); err != nil {
		return fmt.Errorf("writing /etc/hosts (try with sudo): %w", err)
	}

	fmt.Println("✓ Removed localias entries from /etc/hosts")
	return nil
}

func removeHostsBlock(content string) string {
	var result strings.Builder
	scanner := bufio.NewScanner(strings.NewReader(content))
	inBlock := false

	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == hostsMarkerStart {
			inBlock = true
			continue
		}
		if strings.TrimSpace(line) == hostsMarkerEnd {
			inBlock = false
			continue
		}
		if !inBlock {
			result.WriteString(line + "\n")
		}
	}

	return strings.TrimRight(result.String(), "\n") + "\n"
}

// isValidHostname checks if a name is safe to use as a hostname.
// Only allows lowercase alphanumeric characters, hyphens, and dots.
func isValidHostname(name string) bool {
	if len(name) == 0 || len(name) > 253 {
		return false
	}
	for _, c := range name {
		if !((c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' || c == '.') {
			return false
		}
	}
	return true
}
