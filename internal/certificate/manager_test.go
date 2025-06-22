package certificate

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-acme/lego/v4/certificate"
	"github.com/jerkytreats/dns/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testCertEmailKey    = "certificate.email"
	testCertCADirURLKey = "certificate.ca_dir_url"
	testCertCertFileKey = "server.tls.cert_file"
	testCertKeyFileKey  = "server.tls.key_file"
)

func setupTestManager(t *testing.T) (*Manager, string) {
	tempDir, err := os.MkdirTemp("", "cert-manager-test-")
	require.NoError(t, err)
	t.Cleanup(func() { os.RemoveAll(tempDir) })

	config.ResetForTest()
	config.SetForTest(testCertEmailKey, "temp@temp.com")
	config.SetForTest(testCertCADirURLKey, "https://acme-staging-v02.api.letsencrypt.org/directory")
	config.SetForTest(testCertCertFileKey, filepath.Join(tempDir, "cert.pem"))
	config.SetForTest(testCertKeyFileKey, filepath.Join(tempDir, "key.pem"))
	config.SetForTest("dns.coredns.zones_path", tempDir) // for the dns provider

	manager, err := NewManager()
	require.NoError(t, err)

	return manager, tempDir
}

func TestSaveCertificate(t *testing.T) {
	manager, tempDir := setupTestManager(t)

	// In a real scenario, this would come from lego
	dummyCerts := &certificate.Resource{
		Certificate: []byte("test certificate"),
		PrivateKey:  []byte("test key"),
	}

	err := manager.saveCertificate(dummyCerts)
	require.NoError(t, err)

	certContent, err := os.ReadFile(filepath.Join(tempDir, "cert.pem"))
	require.NoError(t, err)
	assert.Equal(t, "test certificate", string(certContent))

	keyContent, err := os.ReadFile(filepath.Join(tempDir, "key.pem"))
	require.NoError(t, err)
	assert.Equal(t, "test key", string(keyContent))
}

func TestCheckAndRenew(t *testing.T) {
	manager, _ := setupTestManager(t)
	domain := "example.com"

	t.Run("does not renew a valid certificate", func(t *testing.T) {
		t.Skip("Skipping test: requires real domain for DNS challenge")
		// The mock server provides a cert valid for one year.
		err := manager.ObtainCertificate(domain)
		require.NoError(t, err)

		// Check renewal with a 30-day threshold
		// This should log "Certificate not due for renewal"
		// Testing logs is complex, so we'll just ensure no error occurs for now.
		manager.checkAndRenew(domain, 30*24*time.Hour)
	})

	t.Run("renews an expired certificate", func(t *testing.T) {
		t.Skip("Skipping test: requires real domain for DNS challenge")
		// Create an already expired certificate
		certPath := config.GetString(testCertCertFileKey)
		keyPath := config.GetString(testCertKeyFileKey)
		createExpiredCert(t, certPath, keyPath)

		// This should log "Certificate is due for renewal"
		// Again, just checking for no errors.
		manager.checkAndRenew(domain, 30*24*time.Hour)
	})
}

// TestDNSConfigValidation tests the new DNS configuration options
func TestDNSConfigValidation(t *testing.T) {
	t.Run("DNS resolvers configuration", func(t *testing.T) {
		tempDir, err := os.MkdirTemp("", "cert-manager-dns-test-")
		require.NoError(t, err)
		t.Cleanup(func() { os.RemoveAll(tempDir) })

		config.ResetForTest()
		// Set required config
		config.SetForTest(testCertEmailKey, "test@example.com")
		config.SetForTest(testCertCertFileKey, filepath.Join(tempDir, "cert.pem"))
		config.SetForTest(testCertKeyFileKey, filepath.Join(tempDir, "key.pem"))
		config.SetForTest("dns.coredns.zones_path", tempDir)

		// Test with custom DNS resolvers
		config.SetForTest(CertDNSResolversKey, []string{"8.8.8.8:53", "1.1.1.1:53", "9.9.9.9:53"})

		manager, err := NewManager()
		require.NoError(t, err)
		assert.NotNil(t, manager)
		assert.NotNil(t, manager.legoClient)
	})

	t.Run("DNS timeout configuration", func(t *testing.T) {
		tempDir, err := os.MkdirTemp("", "cert-manager-dns-timeout-test-")
		require.NoError(t, err)
		t.Cleanup(func() { os.RemoveAll(tempDir) })

		config.ResetForTest()
		// Set required config
		config.SetForTest(testCertEmailKey, "test@example.com")
		config.SetForTest(testCertCertFileKey, filepath.Join(tempDir, "cert.pem"))
		config.SetForTest(testCertKeyFileKey, filepath.Join(tempDir, "key.pem"))
		config.SetForTest("dns.coredns.zones_path", tempDir)

		// Test with custom DNS timeout
		config.SetForTest(CertDNSTimeoutKey, "30s")

		manager, err := NewManager()
		require.NoError(t, err)
		assert.NotNil(t, manager)
		assert.NotNil(t, manager.legoClient)
	})

	t.Run("default DNS configuration", func(t *testing.T) {
		tempDir, err := os.MkdirTemp("", "cert-manager-dns-default-test-")
		require.NoError(t, err)
		t.Cleanup(func() { os.RemoveAll(tempDir) })

		config.ResetForTest()
		// Set required config only, no DNS config
		config.SetForTest(testCertEmailKey, "test@example.com")
		config.SetForTest(testCertCertFileKey, filepath.Join(tempDir, "cert.pem"))
		config.SetForTest(testCertKeyFileKey, filepath.Join(tempDir, "key.pem"))
		config.SetForTest("dns.coredns.zones_path", tempDir)

		// Should use default DNS resolvers and timeout
		manager, err := NewManager()
		require.NoError(t, err)
		assert.NotNil(t, manager)
		assert.NotNil(t, manager.legoClient)
	})

	t.Run("insecure skip verify configuration", func(t *testing.T) {
		tempDir, err := os.MkdirTemp("", "cert-manager-insecure-test-")
		require.NoError(t, err)
		t.Cleanup(func() { os.RemoveAll(tempDir) })

		config.ResetForTest()
		// Set required config
		config.SetForTest(testCertEmailKey, "test@example.com")
		config.SetForTest(testCertCertFileKey, filepath.Join(tempDir, "cert.pem"))
		config.SetForTest(testCertKeyFileKey, filepath.Join(tempDir, "key.pem"))
		config.SetForTest("dns.coredns.zones_path", tempDir)

		// Test with insecure skip verify enabled
		config.SetForTest(CertInsecureSkipVerifyKey, true)

		manager, err := NewManager()
		require.NoError(t, err)
		assert.NotNil(t, manager)
		assert.NotNil(t, manager.legoClient)
	})
}

// TestNewManagerConfigKeys tests that the new configuration keys are properly registered
func TestNewManagerConfigKeys(t *testing.T) {
	t.Run("DNS configuration keys exist", func(t *testing.T) {
		// Test that the new configuration keys are defined
		assert.Equal(t, "certificate.dns_resolvers", CertDNSResolversKey)
		assert.Equal(t, "certificate.dns_timeout", CertDNSTimeoutKey)
		assert.Equal(t, "certificate.insecure_skip_verify", CertInsecureSkipVerifyKey)
	})
}

// TestDNSResolverValidation tests various DNS resolver configurations
func TestDNSResolverValidation(t *testing.T) {
	testCases := []struct {
		name      string
		resolvers []string
		expectErr bool
	}{
		{
			name:      "valid IPv4 resolvers",
			resolvers: []string{"8.8.8.8:53", "1.1.1.1:53"},
			expectErr: false,
		},
		{
			name:      "valid IPv6 resolvers",
			resolvers: []string{"[2001:4860:4860::8888]:53", "[2606:4700:4700::1111]:53"},
			expectErr: false,
		},
		{
			name:      "mixed IPv4 and IPv6",
			resolvers: []string{"8.8.8.8:53", "[2001:4860:4860::8888]:53"},
			expectErr: false,
		},
		{
			name:      "hostname resolvers",
			resolvers: []string{"dns.google:53", "one.one.one.one:53"},
			expectErr: false,
		},
		{
			name:      "empty list uses defaults",
			resolvers: []string{},
			expectErr: false,
		},
		{
			name:      "nil list uses defaults",
			resolvers: nil,
			expectErr: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tempDir, err := os.MkdirTemp("", "cert-manager-resolver-test-")
			require.NoError(t, err)
			t.Cleanup(func() { os.RemoveAll(tempDir) })

			config.ResetForTest()
			// Set required config
			config.SetForTest(testCertEmailKey, "test@example.com")
			config.SetForTest(testCertCertFileKey, filepath.Join(tempDir, "cert.pem"))
			config.SetForTest(testCertKeyFileKey, filepath.Join(tempDir, "key.pem"))
			config.SetForTest("dns.coredns.zones_path", tempDir)

			if tc.resolvers != nil {
				config.SetForTest(CertDNSResolversKey, tc.resolvers)
			}

			manager, err := NewManager()
			if tc.expectErr {
				assert.Error(t, err)
				assert.Nil(t, manager)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, manager)
				assert.NotNil(t, manager.legoClient)
			}
		})
	}
}

// TestDNSTimeoutValidation tests various DNS timeout configurations
func TestDNSTimeoutValidation(t *testing.T) {
	testCases := []struct {
		name      string
		timeout   string
		expectErr bool
	}{
		{
			name:      "valid timeout seconds",
			timeout:   "10s",
			expectErr: false,
		},
		{
			name:      "valid timeout minutes",
			timeout:   "2m",
			expectErr: false,
		},
		{
			name:      "valid timeout mixed",
			timeout:   "1m30s",
			expectErr: false,
		},
		{
			name:      "zero timeout uses default",
			timeout:   "0s",
			expectErr: false,
		},
		{
			name:      "empty timeout uses default",
			timeout:   "",
			expectErr: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tempDir, err := os.MkdirTemp("", "cert-manager-timeout-test-")
			require.NoError(t, err)
			t.Cleanup(func() { os.RemoveAll(tempDir) })

			config.ResetForTest()
			// Set required config
			config.SetForTest(testCertEmailKey, "test@example.com")
			config.SetForTest(testCertCertFileKey, filepath.Join(tempDir, "cert.pem"))
			config.SetForTest(testCertKeyFileKey, filepath.Join(tempDir, "key.pem"))
			config.SetForTest("dns.coredns.zones_path", tempDir)

			if tc.timeout != "" {
				config.SetForTest(CertDNSTimeoutKey, tc.timeout)
			}

			manager, err := NewManager()
			if tc.expectErr {
				assert.Error(t, err)
				assert.Nil(t, manager)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, manager)
				assert.NotNil(t, manager.legoClient)
			}
		})
	}
}

// createExpiredCert creates a dummy cert file with an expiration in the past.
func createExpiredCert(t *testing.T, certPath, keyPath string) {
	template := x509.Certificate{
		SerialNumber:          big.NewInt(1),
		NotBefore:             time.Now().Add(-2 * time.Hour),
		NotAfter:              time.Now().Add(-1 * time.Hour), // Expired
		BasicConstraintsValid: true,
	}

	priv, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	require.NoError(t, err)

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	require.NoError(t, os.WriteFile(certPath, certPEM, 0644))

	// Also write a dummy key file
	require.NoError(t, os.WriteFile(keyPath, []byte("dummy expired key"), 0600))
}
