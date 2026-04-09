// Package cmd — MCP token management commands.
// Allows creating, listing, and revoking scoped MCP tokens.
//
// Author: Thiru K
// Module: github.com/thirukguru/localias/cmd
package cmd

import (
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/thirukguru/localias/internal/daemon"
)

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Manage the MCP server for AI agent integration",
}

var mcpTokenCmd = &cobra.Command{
	Use:   "token",
	Short: "Manage scoped MCP tokens",
}

// Flags for mcp token create
var (
	tokenRoutes       string
	tokenCapabilities string
	tokenLabel        string
)

var mcpTokenCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a scoped MCP token",
	Long: `Create a new MCP token scoped to specific routes and capabilities.

Examples:
  localias mcp token create --routes frontend,api --capabilities read,health
  localias mcp token create --routes myapp --capabilities read --label "CI agent"
  localias mcp token create --routes '*' --capabilities read,write,health`,
	RunE: runMCPTokenCreate,
}

var mcpTokenListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all scoped MCP tokens",
	RunE:  runMCPTokenList,
}

var mcpTokenRevokeCmd = &cobra.Command{
	Use:   "revoke <token-prefix>",
	Short: "Revoke MCP tokens matching a prefix",
	Args:  cobra.ExactArgs(1),
	RunE:  runMCPTokenRevoke,
}

func init() {
	mcpTokenCreateCmd.Flags().StringVar(&tokenRoutes, "routes", "", "Comma-separated route names (required, use '*' for all)")
	mcpTokenCreateCmd.Flags().StringVar(&tokenCapabilities, "capabilities", "read,health", "Comma-separated capabilities: read, write, health")
	mcpTokenCreateCmd.Flags().StringVar(&tokenLabel, "label", "", "Human-readable label for the token")
	mcpTokenCreateCmd.MarkFlagRequired("routes")

	mcpTokenCmd.AddCommand(mcpTokenCreateCmd, mcpTokenListCmd, mcpTokenRevokeCmd)
	mcpCmd.AddCommand(mcpTokenCmd)
	rootCmd.AddCommand(mcpCmd)
}

func runMCPTokenCreate(cmd *cobra.Command, args []string) error {
	stDir := GetStateDir()
	socketPath := filepath.Join(stDir, "localias.sock")
	client := daemon.NewClient(socketPath, stDir, slog.Default())

	routes := strings.Split(tokenRoutes, ",")
	caps := strings.Split(tokenCapabilities, ",")

	// Validate capabilities
	validCaps := map[string]bool{"read": true, "write": true, "health": true, "*": true}
	for _, c := range caps {
		c = strings.TrimSpace(c)
		if !validCaps[c] {
			return fmt.Errorf("invalid capability %q (valid: read, write, health, *)", c)
		}
	}

	result, err := client.MCPTokenCreate(routes, caps, 0, tokenLabel)
	if err != nil {
		return fmt.Errorf("creating token: %w", err)
	}

	fmt.Printf("✓ Scoped MCP token created\n\n")
	fmt.Printf("  Token:        %s\n", result.Token)
	fmt.Printf("  Routes:       %s\n", strings.Join(routes, ", "))
	fmt.Printf("  Capabilities: %s\n", strings.Join(caps, ", "))
	if tokenLabel != "" {
		fmt.Printf("  Label:        %s\n", tokenLabel)
	}
	fmt.Printf("\n  Use with: Authorization: Bearer %s\n", result.Token)

	return nil
}

func runMCPTokenList(cmd *cobra.Command, args []string) error {
	stDir := GetStateDir()
	socketPath := filepath.Join(stDir, "localias.sock")
	client := daemon.NewClient(socketPath, stDir, slog.Default())

	result, err := client.MCPTokenList()
	if err != nil {
		return fmt.Errorf("listing tokens: %w", err)
	}

	if len(result.Tokens) == 0 {
		fmt.Println("No scoped MCP tokens. Use 'localias mcp token create' to create one.")
		return nil
	}

	fmt.Printf("Scoped MCP tokens (%d):\n\n", len(result.Tokens))
	for _, t := range result.Tokens {
		label := t.Label
		if label == "" {
			label = "(no label)"
		}
		pidStr := ""
		if t.PID > 0 {
			pidStr = fmt.Sprintf("  PID: %d (ephemeral)", t.PID)
		}
		fmt.Printf("  %s…  routes=[%s]  caps=[%s]  %s%s\n",
			t.Prefix,
			strings.Join(t.Routes, ","),
			strings.Join(t.Capabilities, ","),
			label,
			pidStr,
		)
	}

	return nil
}

func runMCPTokenRevoke(cmd *cobra.Command, args []string) error {
	prefix := args[0]

	stDir := GetStateDir()
	socketPath := filepath.Join(stDir, "localias.sock")
	client := daemon.NewClient(socketPath, stDir, slog.Default())

	result, err := client.MCPTokenRevoke(prefix)
	if err != nil {
		return fmt.Errorf("revoking tokens: %w", err)
	}

	if result.Revoked == 0 {
		fmt.Printf("No tokens matching prefix %q found.\n", prefix)
	} else {
		fmt.Printf("✓ Revoked %d token(s) matching %q\n", result.Revoked, prefix)
	}

	return nil
}
