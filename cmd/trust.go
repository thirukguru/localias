// Package cmd — trust command to add the localias CA to the system trust store.
//
// Author: Thiru K
// Module: github.com/thirukguru/localias/cmd
package cmd

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/thirukguru/localias/internal/cert"
)

var trustCmd = &cobra.Command{
	Use:   "trust",
	Short: "Add localias CA to system trust store",
	Long:  "Install the localias certificate authority into the system trust store.\nThis allows HTTPS connections to .localhost domains to be trusted by browsers.",
	RunE:  runTrust,
}

func init() {
	rootCmd.AddCommand(trustCmd)
}

func runTrust(cmd *cobra.Command, args []string) error {
	dir := GetStateDir()
	certsDir := filepath.Join(dir, "certs")

	// Make sure CA exists
	_, err := cert.EnsureCA(certsDir)
	if err != nil {
		return fmt.Errorf("ensuring CA: %w", err)
	}

	caPath := filepath.Join(certsDir, "ca.crt")
	if err := cert.TrustCA(caPath); err != nil {
		return fmt.Errorf("trusting CA: %w", err)
	}

	fmt.Println("✓ CA certificate added to system trust store")
	return nil
}
