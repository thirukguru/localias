// Package cert — platform-specific trust store integration.
// Adds the localias CA certificate to the system trust store.
//
// Author: Thiru K
// Module: github.com/thirukguru/localias/internal/cert
package cert

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// TrustCA adds the CA certificate to the system trust store.
func TrustCA(caPath string) error {
	switch runtime.GOOS {
	case "darwin":
		return trustDarwin(caPath)
	case "linux":
		return trustLinux(caPath)
	default:
		return fmt.Errorf("unsupported OS: %s. Please manually add %s to your trust store", runtime.GOOS, caPath)
	}
}

// trustDarwin adds the CA to the macOS system keychain.
func trustDarwin(caPath string) error {
	cmd := exec.Command("security", "add-trusted-cert",
		"-d", "-r", "trustRoot",
		"-k", "/Library/Keychains/System.keychain",
		caPath,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("security add-trusted-cert: %w\n%s", err, output)
	}
	return nil
}

// trustLinux adds the CA to the Linux system trust store.
// Detects the distro from /etc/os-release.
func trustLinux(caPath string) error {
	distro := detectLinuxDistro()

	switch distro {
	case "debian", "ubuntu", "pop", "mint", "elementary":
		return trustDebian(caPath)
	case "arch", "manjaro", "endeavouros":
		return trustArch(caPath)
	case "fedora", "rhel", "centos", "rocky", "alma":
		return trustFedora(caPath)
	default:
		// Try debian method as fallback
		return trustDebian(caPath)
	}
}

// trustDebian adds CA to Debian/Ubuntu trust store.
func trustDebian(caPath string) error {
	destDir := "/usr/local/share/ca-certificates"
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("creating cert dir: %w", err)
	}

	dest := filepath.Join(destDir, "localias-ca.crt")
	data, err := os.ReadFile(caPath)
	if err != nil {
		return fmt.Errorf("reading CA cert: %w", err)
	}
	if err := os.WriteFile(dest, data, 0644); err != nil {
		return fmt.Errorf("writing CA cert to %s: %w", dest, err)
	}

	cmd := exec.Command("update-ca-certificates")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("update-ca-certificates: %w\n%s", err, output)
	}
	return nil
}

// trustArch adds CA to Arch Linux trust store.
func trustArch(caPath string) error {
	cmd := exec.Command("trust", "anchor", "--store", caPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("trust anchor: %w\n%s", err, output)
	}
	return nil
}

// trustFedora adds CA to Fedora/RHEL trust store.
func trustFedora(caPath string) error {
	destDir := "/etc/pki/ca-trust/source/anchors"
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("creating cert dir: %w", err)
	}

	dest := filepath.Join(destDir, "localias-ca.crt")
	data, err := os.ReadFile(caPath)
	if err != nil {
		return fmt.Errorf("reading CA cert: %w", err)
	}
	if err := os.WriteFile(dest, data, 0644); err != nil {
		return fmt.Errorf("writing CA cert: %w", err)
	}

	cmd := exec.Command("update-ca-trust")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("update-ca-trust: %w\n%s", err, output)
	}
	return nil
}

// detectLinuxDistro reads /etc/os-release to determine the Linux distro.
func detectLinuxDistro() string {
	data, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return "unknown"
	}
	content := string(data)
	for _, line := range strings.Split(content, "\n") {
		if strings.HasPrefix(line, "ID=") {
			id := strings.TrimPrefix(line, "ID=")
			id = strings.Trim(id, `"`)
			return strings.ToLower(id)
		}
	}
	return "unknown"
}
