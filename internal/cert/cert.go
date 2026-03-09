// Package cert — per-app leaf certificate generation.
// Issues certificates signed by the local CA for *.localhost domains.
//
// Author: Thiru K
// Module: github.com/thirukguru/localias/internal/cert
package cert

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"
)

// EnsureLeafCert loads existing leaf cert or generates a new one signed by the CA.
// Returns paths to the cert and key files.
func EnsureLeafCert(certsDir string) (certPath, keyPath string, err error) {
	certPath = filepath.Join(certsDir, "leaf.crt")
	keyPath = filepath.Join(certsDir, "leaf.key")

	// Check if leaf cert exists and is valid
	if isLeafValid(certPath) {
		return certPath, keyPath, nil
	}

	// Load CA
	ca, err := EnsureCA(certsDir)
	if err != nil {
		return "", "", fmt.Errorf("ensuring CA: %w", err)
	}

	// Generate leaf cert
	if err := generateLeaf(ca, certPath, keyPath); err != nil {
		return "", "", fmt.Errorf("generating leaf cert: %w", err)
	}

	return certPath, keyPath, nil
}

// generateLeaf creates a leaf certificate signed by the CA.
func generateLeaf(ca *CA, certPath, keyPath string) error {
	// Generate RSA 2048-bit key
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return fmt.Errorf("generating leaf key: %w", err)
	}

	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return fmt.Errorf("generating serial number: %w", err)
	}

	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"Localias Local Development"},
			CommonName:   "*.localhost",
		},
		NotBefore: time.Now(),
		NotAfter:  time.Now().Add(365 * 24 * time.Hour), // 1 year
		KeyUsage:  x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage: []x509.ExtKeyUsage{
			x509.ExtKeyUsageServerAuth,
		},
		DNSNames: []string{
			"*.localhost",
			"localhost",
			"*.local",
		},
		IPAddresses: []net.IP{
			net.ParseIP("127.0.0.1"),
			net.ParseIP("::1"),
		},
	}

	// Sign with CA
	certDER, err := x509.CreateCertificate(rand.Reader, template, ca.Certificate, &privateKey.PublicKey, ca.PrivateKey)
	if err != nil {
		return fmt.Errorf("creating leaf certificate: %w", err)
	}

	// Encode PEM
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	})

	// Save to files
	if err := os.WriteFile(certPath, certPEM, 0644); err != nil {
		return fmt.Errorf("writing leaf cert: %w", err)
	}
	if err := os.WriteFile(keyPath, keyPEM, 0600); err != nil {
		return fmt.Errorf("writing leaf key: %w", err)
	}

	return nil
}

// isLeafValid checks if the leaf cert exists and won't expire within 30 days.
func isLeafValid(certPath string) bool {
	data, err := os.ReadFile(certPath)
	if err != nil {
		return false
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return false
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return false
	}
	return time.Until(cert.NotAfter) > 30*24*time.Hour
}
