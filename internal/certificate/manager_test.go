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
