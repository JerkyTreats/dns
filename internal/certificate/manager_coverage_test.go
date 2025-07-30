package certificate

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jerkytreats/dns/internal/config"
)

// createTestCertificate creates a test certificate with the given domains
func createTestCertificate(baseDomain string, sanDomains []string, certPath, keyPath string) error {
	// Generate private key
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return err
	}

	// Create certificate template
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: baseDomain,
		},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(90 * 24 * time.Hour), // 90 days
		KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  nil,
	}

	// Add DNS names (base domain + SAN domains)
	allDomains := []string{baseDomain}
	allDomains = append(allDomains, sanDomains...)
	template.DNSNames = allDomains

	// Create certificate
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return err
	}

	// Create directory if it doesn't exist
	if err := os.MkdirAll(filepath.Dir(certPath), 0755); err != nil {
		return err
	}

	// Write certificate
	certOut, err := os.Create(certPath)
	if err != nil {
		return err
	}
	defer certOut.Close()

	if err := pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: certDER}); err != nil {
		return err
	}

	// Write private key
	keyOut, err := os.Create(keyPath)
	if err != nil {
		return err
	}
	defer keyOut.Close()

	privateKeyDER, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		return err
	}

	return pem.Encode(keyOut, &pem.Block{Type: "PRIVATE KEY", Bytes: privateKeyDER})
}

func TestCheckDomainCoverage(t *testing.T) {
	// Clean up any leftover certificate data files from previous test runs
	CleanupCertificateDataDir(t)
	
	// Setup test environment
	tempDir := t.TempDir()
	certPath := filepath.Join(tempDir, "test.crt")
	keyPath := filepath.Join(tempDir, "test.key")

	// Set required config values
	config.SetForTest(CertEmailKey, "test@example.com")
	config.SetForTest(CertDomainKey, "internal.jerkytreats.dev")
	config.SetForTest(CertCertFileKey, certPath)
	config.SetForTest(CertKeyFileKey, keyPath)
	config.SetForTest(CertCADirURLKey, "https://acme-staging-v02.api.letsencrypt.org/directory")
	config.SetForTest(CertRenewalEnabledKey, false)
	config.SetForTest(CertRenewalRenewBeforeKey, "720h")
	config.SetForTest(CertRenewalCheckIntervalKey, "24h")
	config.SetForTest(CertDNSResolversKey, []string{"8.8.8.8:53"})
	config.SetForTest(CertDNSTimeoutKey, "10s")
	config.SetForTest(CertCloudflareTokenKey, "dummy-token")
	config.SetForTest(CertDNSCleanupWaitKey, "90s")
	config.SetForTest(CertDNSCreationWaitKey, "60s")
	config.SetForTest(CertUseProdCertsKey, false)

	// Create test certificate with multiple SAN domains
	baseDomain := "internal.jerkytreats.dev"
	sanDomains := []string{
		"dns.internal.jerkytreats.dev",
		"api.internal.jerkytreats.dev",
		"test.internal.jerkytreats.dev",
	}

	if err := createTestCertificate(baseDomain, sanDomains, certPath, keyPath); err != nil {
		t.Fatalf("Failed to create test certificate: %v", err)
	}

	// Create manager with test certificate
	manager := &Manager{
		certPath:      certPath,
		keyPath:       keyPath,
		domainStorage: NewCertificateDomainStorage(),
	}

	tests := []struct {
		name           string
		domain         string
		expectedResult bool
		shouldError    bool
	}{
		{
			name:           "Base domain is covered",
			domain:         "internal.jerkytreats.dev",
			expectedResult: true,
			shouldError:    false,
		},
		{
			name:           "SAN domain is covered",
			domain:         "dns.internal.jerkytreats.dev",
			expectedResult: true,
			shouldError:    false,
		},
		{
			name:           "Another SAN domain is covered",
			domain:         "api.internal.jerkytreats.dev",
			expectedResult: true,
			shouldError:    false,
		},
		{
			name:           "Domain not in certificate",
			domain:         "new.internal.jerkytreats.dev",
			expectedResult: false,
			shouldError:    false,
		},
		{
			name:           "Completely different domain",
			domain:         "external.example.com",
			expectedResult: false,
			shouldError:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			covered, err := manager.CheckDomainCoverage(tt.domain)

			if tt.shouldError && err == nil {
				t.Errorf("Expected error but got none")
			}
			if !tt.shouldError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			if covered != tt.expectedResult {
				t.Errorf("Expected coverage %v for domain %s, got %v", tt.expectedResult, tt.domain, covered)
			}
		})
	}
}

func TestCheckDomainCoverageNoCertificate(t *testing.T) {
	// Clean up any leftover certificate data files from previous test runs
	CleanupCertificateDataDir(t)
	
	// Test behavior when certificate doesn't exist
	tempDir := t.TempDir()
	certPath := filepath.Join(tempDir, "nonexistent.crt")
	keyPath := filepath.Join(tempDir, "nonexistent.key")
	
	// Set certificate domain storage path to use a temporary directory
	config.SetForTest(CertDomainStoragePathKey, filepath.Join(tempDir, "certificate_domains.json"))

	manager := &Manager{
		certPath:      certPath,
		keyPath:       keyPath,
		domainStorage: NewCertificateDomainStorage(),
	}

	covered, err := manager.CheckDomainCoverage("test.internal.jerkytreats.dev")
	if err != nil {
		t.Errorf("Expected no error when certificate doesn't exist, got: %v", err)
	}
	if covered {
		t.Errorf("Expected domain not to be covered when certificate doesn't exist")
	}
}

func TestSyncStorageWithCertificate(t *testing.T) {
	// Setup test environment
	tempDir := t.TempDir()
	certPath := filepath.Join(tempDir, "test.crt")
	keyPath := filepath.Join(tempDir, "test.key")

	// Create test certificate
	baseDomain := "internal.jerkytreats.dev"
	sanDomains := []string{
		"dns.internal.jerkytreats.dev",
		"api.internal.jerkytreats.dev",
	}

	if err := createTestCertificate(baseDomain, sanDomains, certPath, keyPath); err != nil {
		t.Fatalf("Failed to create test certificate: %v", err)
	}

	// Create manager
	manager := &Manager{
		certPath:      certPath,
		keyPath:       keyPath,
		domainStorage: NewCertificateDomainStorage(),
	}

	// Sync storage with certificate
	if err := manager.SyncStorageWithCertificate(); err != nil {
		t.Fatalf("Failed to sync storage: %v", err)
	}

	// Verify storage contains correct domains
	domains, err := manager.domainStorage.LoadDomains()
	if err != nil {
		t.Fatalf("Failed to load domains from storage: %v", err)
	}

	if domains.BaseDomain != baseDomain {
		t.Errorf("Expected base domain %s, got %s", baseDomain, domains.BaseDomain)
	}

	if len(domains.SANDomains) != len(sanDomains) {
		t.Errorf("Expected %d SAN domains, got %d", len(sanDomains), len(domains.SANDomains))
	}

	// Verify all SAN domains are present
	sanMap := make(map[string]bool)
	for _, domain := range domains.SANDomains {
		sanMap[domain] = true
	}

	for _, expectedDomain := range sanDomains {
		if !sanMap[expectedDomain] {
			t.Errorf("Expected SAN domain %s not found in storage", expectedDomain)
		}
	}
}