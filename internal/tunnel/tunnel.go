// Package tunnel provides SSH reverse tunnel for exposing local services.
// Connects to a relay server via SSH and opens a remote port forward.
//
// Author: Thiru K
// Module: github.com/thirukguru/localias/internal/tunnel
package tunnel

import (
	"fmt"
	"os"
	"os/exec"
)

// Start initiates an SSH reverse tunnel to the specified relay.
// Uses the LOCALIAS_TUNNEL_HOST env var, or prints instructions.
func Start(name string, localPort int) error {
	host := os.Getenv("LOCALIAS_TUNNEL_HOST")
	if host == "" {
		fmt.Println("⚠ Tunnel requires a relay server.")
		fmt.Println("")
		fmt.Println("Set LOCALIAS_TUNNEL_HOST to your relay:")
		fmt.Println("  export LOCALIAS_TUNNEL_HOST=relay.example.com")
		fmt.Println("")
		fmt.Println("Then run:")
		fmt.Printf("  localias tunnel %s\n", name)
		return nil
	}

	// Build SSH command: ssh -R 0:localhost:PORT relay_host
	remotePort := "0" // Let server choose
	sshArgs := []string{
		"-R", fmt.Sprintf("%s:localhost:%d", remotePort, localPort),
		"-N",        // No remote command
		"-o", "StrictHostKeyChecking=no",
		"-o", "ServerAliveInterval=30",
		host,
	}

	fmt.Printf("→ Opening tunnel for %s via %s...\n", name, host)
	fmt.Printf("  Local: 127.0.0.1:%d\n", localPort)

	cmd := exec.Command("ssh", sshArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("SSH tunnel failed: %w", err)
	}

	return nil
}
