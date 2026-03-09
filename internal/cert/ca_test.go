// Package cert tests — verifies CA generation and leaf cert signing.
//
// Author: Thiru K
// Module: github.com/thirukguru/localias/internal/cert
package cert

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestEnsureCA_GeneratesNew(t *testing.T) {
	dir := t.TempDir()
	ca, err := EnsureCA(dir)
	if err != nil {
		t.Fatalf("EnsureCA failed: %v", err)
	}

	if ca.Certificate == nil {
		t.Fatal("CA certificate is nil")
	}
	if ca.PrivateKey == nil {
		t.Fatal("CA private key is nil")
	}
	if !ca.Certificate.IsCA {
		t.Error("certificate is not a CA")
	}
	if ca.Certificate.Subject.CommonName != "Localias Local CA" {
		t.Errorf("unexpected CN: %s", ca.Certificate.Subject.CommonName)
	}

	// Verify files exist
	if _, err := os.Stat(filepath.Join(dir, "ca.crt")); err != nil {
		t.Error("ca.crt not created")
	}
	if _, err := os.Stat(filepath.Join(dir, "ca.key")); err != nil {
		t.Error("ca.key not created")
	}
}

func TestEnsureCA_LoadsExisting(t *testing.T) {
	dir := t.TempDir()
	ca1, err := EnsureCA(dir)
	if err != nil {
		t.Fatalf("first EnsureCA failed: %v", err)
	}

	ca2, err := EnsureCA(dir)
	if err != nil {
		t.Fatalf("second EnsureCA failed: %v", err)
	}

	if ca1.Certificate.SerialNumber.Cmp(ca2.Certificate.SerialNumber) != 0 {
		t.Error("CA was regenerated instead of loaded")
	}
}

func TestEnsureCA_ValidFor10Years(t *testing.T) {
	dir := t.TempDir()
	ca, _ := EnsureCA(dir)

	validity := ca.Certificate.NotAfter.Sub(ca.Certificate.NotBefore)
	expectedYears := 10 * 365 * 24 * time.Hour
	if validity < expectedYears-24*time.Hour { // allow 1 day tolerance
		t.Errorf("CA validity %v is less than 10 years", validity)
	}
}

func TestEnsureLeafCert(t *testing.T) {
	dir := t.TempDir()

	certPath, keyPath, err := EnsureLeafCert(dir)
	if err != nil {
		t.Fatalf("EnsureLeafCert failed: %v", err)
	}

	// Verify files exist
	if _, err := os.Stat(certPath); err != nil {
		t.Error("leaf cert not created")
	}
	if _, err := os.Stat(keyPath); err != nil {
		t.Error("leaf key not created")
	}

	// Verify cert is valid
	certPEM, _ := os.ReadFile(certPath)
	block, _ := pem.Decode(certPEM)
	if block == nil {
		t.Fatal("failed to decode leaf cert PEM")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("failed to parse leaf cert: %v", err)
	}

	// Check SANs
	hasLocalhostWildcard := false
	for _, dns := range cert.DNSNames {
		if dns == "*.localhost" {
			hasLocalhostWildcard = true
		}
	}
	if !hasLocalhostWildcard {
		t.Error("leaf cert missing *.localhost SAN")
	}

	// Check 127.0.0.1 IP SAN
	hasLoopback := false
	for _, ip := range cert.IPAddresses {
		if ip.String() == "127.0.0.1" {
			hasLoopback = true
		}
	}
	if !hasLoopback {
		t.Error("leaf cert missing 127.0.0.1 IP SAN")
	}
}

func TestLeafCert_LoadsExisting(t *testing.T) {
	dir := t.TempDir()
	cert1, key1, _ := EnsureLeafCert(dir)
	cert2, key2, _ := EnsureLeafCert(dir)

	if cert1 != cert2 || key1 != key2 {
		t.Error("leaf cert paths changed on second call")
	}
}

func TestLeafCert_SignedByCA(t *testing.T) {
	dir := t.TempDir()

	certPath, keyPath, err := EnsureLeafCert(dir)
	if err != nil {
		t.Fatalf("EnsureLeafCert failed: %v", err)
	}

	// Load CA
	caCertPEM, _ := os.ReadFile(filepath.Join(dir, "ca.crt"))

	// Load leaf cert pair
	_, err = tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		t.Fatalf("failed to load leaf key pair: %v", err)
	}

	// Verify signature chain
	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(caCertPEM) {
		t.Fatal("failed to add CA to pool")
	}

	leafPEM, _ := os.ReadFile(certPath)
	leafBlock, _ := pem.Decode(leafPEM)
	leafCert, _ := x509.ParseCertificate(leafBlock.Bytes)

	opts := x509.VerifyOptions{
		Roots: caPool,
		KeyUsages: []x509.ExtKeyUsage{
			x509.ExtKeyUsageServerAuth,
		},
	}
	if _, err := leafCert.Verify(opts); err != nil {
		t.Fatalf("leaf cert not signed by CA: %v", err)
	}
}

func TestDetectLinuxDistro(t *testing.T) {
	// This test is informational — just ensure no panic
	distro := detectLinuxDistro()
	if distro == "" {
		t.Error("detected empty distro")
	}
}
