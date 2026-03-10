// Package cmd — dashboard command to open the web dashboard.
//
// Author: Thiru K
// Module: github.com/thirukguru/localias/cmd
package cmd

import (
	"fmt"
	"os/exec"
	"runtime"

	"github.com/spf13/cobra"
)

var dashboardCmd = &cobra.Command{
	Use:   "dashboard",
	Short: "Open the web dashboard in your browser",
	RunE: func(cmd *cobra.Command, args []string) error {
		port := GetProxyPort()
		url := fmt.Sprintf("http://localias.localhost:%d", port)
		fmt.Printf("Opening dashboard: %s\n", url)
		return openBrowser(url)
	},
}

func init() {
	rootCmd.AddCommand(dashboardCmd)
}

func openBrowser(url string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", url).Start()
	case "linux":
		return exec.Command("xdg-open", url).Start()
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	default:
		return fmt.Errorf("unsupported OS for browser open: %s", runtime.GOOS)
	}
}
