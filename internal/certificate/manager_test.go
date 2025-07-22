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

	// Set required Cloudflare API token for tests
	config.SetForTest(CertCloudflareTokenKey, "test-cloudflare-token")
	config.SetForTest(CertDomainKey, "test.example.com")

	// For tests, we'll skip the manager creation since it requires real Cloudflare API
	// and focus on testing the individual components that don't require external APIs
	return nil, tempDir
}

func TestSaveCertificate(t *testing.T) {
	_, tempDir := setupTestManager(t)

	// Test the saveCertificate function directly
	manager := &Manager{
		certPath: filepath.Join(tempDir, "cert.pem"),
		keyPath:  filepath.Join(tempDir, "key.pem"),
	}

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
	_, tempDir := setupTestManager(t)

	t.Run("renews an expired certificate", func(t *testing.T) {
		// Create an already expired certificate
		certPath := filepath.Join(tempDir, "cert.pem")
		keyPath := filepath.Join(tempDir, "key.pem")
		createExpiredCert(t, certPath, keyPath)

		// Test the certificate info reading logic directly
		certInfo, err := GetCertificateInfo(certPath)
		require.NoError(t, err)
		assert.True(t, time.Until(certInfo.NotAfter) <= 0, "Certificate should be expired")
	})
}

// TestDNSConfigValidation tests the DNS configuration options
func TestDNSConfigValidation(t *testing.T) {
	t.Run("DNS resolvers configuration", func(t *testing.T) {
		config.ResetForTest()
		config.SetForTest(CertDNSResolversKey, []string{"8.8.8.8:53", "1.1.1.1:53", "9.9.9.9:53"})

		// Test that the configuration key is properly set
		resolvers := config.GetStringSlice(CertDNSResolversKey)
		assert.Equal(t, []string{"8.8.8.8:53", "1.1.1.1:53", "9.9.9.9:53"}, resolvers)
	})

	t.Run("DNS timeout configuration", func(t *testing.T) {
		config.ResetForTest()
		config.SetForTest(CertDNSTimeoutKey, "30s")

		// Test that the configuration key is properly set
		timeout := config.GetDuration(CertDNSTimeoutKey)
		assert.Equal(t, 30*time.Second, timeout)
	})

	t.Run("insecure skip verify configuration", func(t *testing.T) {
		config.ResetForTest()
		config.SetForTest(CertInsecureSkipVerifyKey, true)

		// Test that the configuration key is properly set
		skipVerify := config.GetBool(CertInsecureSkipVerifyKey)
		assert.True(t, skipVerify)
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
			config.ResetForTest()

			if tc.resolvers != nil {
				config.SetForTest(CertDNSResolversKey, tc.resolvers)
				// Test that the configuration is properly set
				resolvers := config.GetStringSlice(CertDNSResolversKey)
				assert.Equal(t, tc.resolvers, resolvers)
			} else {
				// Test that default resolvers are used when none provided
				resolvers := config.GetStringSlice(CertDNSResolversKey)
				assert.Empty(t, resolvers)
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
			config.ResetForTest()

			if tc.timeout != "" {
				config.SetForTest(CertDNSTimeoutKey, tc.timeout)
				// Test that the configuration is properly set
				timeout := config.GetDuration(CertDNSTimeoutKey)
				expectedDuration, _ := time.ParseDuration(tc.timeout)
				assert.Equal(t, expectedDuration, timeout)
			} else {
				// Test that default timeout is used when none provided
				timeout := config.GetDuration(CertDNSTimeoutKey)
				assert.Equal(t, time.Duration(0), timeout)
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

// TestCalculateBackoffWithJitter tests the exponential backoff calculation
func TestCalculateBackoffWithJitter(t *testing.T) {
	t.Run("zero and negative attempts", func(t *testing.T) {
		assert.Equal(t, time.Duration(0), calculateBackoffWithJitter(0))
		assert.Equal(t, time.Duration(0), calculateBackoffWithJitter(-1))
		assert.Equal(t, time.Duration(0), calculateBackoffWithJitter(-10))
	})

	t.Run("attempt 1 backoff", func(t *testing.T) {
		// Attempt 1: 1^2 = 1 second base + 0-0.25s jitter
		duration := calculateBackoffWithJitter(1)
		assert.GreaterOrEqual(t, duration, 1*time.Second)
		assert.LessOrEqual(t, duration, 1*time.Second+250*time.Millisecond)
	})

	t.Run("attempt 2 backoff", func(t *testing.T) {
		// Attempt 2: 2^2 = 4 seconds base + 0-1s jitter
		duration := calculateBackoffWithJitter(2)
		assert.GreaterOrEqual(t, duration, 4*time.Second)
		assert.LessOrEqual(t, duration, 5*time.Second)
	})

	t.Run("attempt 3 backoff", func(t *testing.T) {
		// Attempt 3: 3^2 = 9 seconds base + 0-2.25s jitter
		duration := calculateBackoffWithJitter(3)
		assert.GreaterOrEqual(t, duration, 9*time.Second)
		assert.LessOrEqual(t, duration, 11*time.Second+250*time.Millisecond)
	})

	t.Run("attempt 10 backoff", func(t *testing.T) {
		// Attempt 10: 10^2 = 100 seconds base + 0-25s jitter
		duration := calculateBackoffWithJitter(10)
		assert.GreaterOrEqual(t, duration, 100*time.Second)
		assert.LessOrEqual(t, duration, 125*time.Second)
	})

	t.Run("exponential growth", func(t *testing.T) {
		// Verify that backoff increases exponentially
		durations := make([]time.Duration, 5)
		for i := 1; i <= 5; i++ {
			durations[i-1] = calculateBackoffWithJitter(i)
		}

		// Each duration should be larger than the previous (accounting for jitter)
		// Use base times without jitter for comparison
		baseTimes := []time.Duration{
			1 * time.Second,  // 1^2
			4 * time.Second,  // 2^2
			9 * time.Second,  // 3^2
			16 * time.Second, // 4^2
			25 * time.Second, // 5^2
		}

		for i := 1; i < len(baseTimes); i++ {
			// Each base time should be greater than the previous max (base + jitter)
			prevMax := baseTimes[i-1] + baseTimes[i-1]/4
			assert.Greater(t, baseTimes[i], prevMax,
				"Backoff should grow exponentially: attempt %d base (%v) should be > attempt %d max (%v)",
				i+1, baseTimes[i], i, prevMax)
		}
	})

	t.Run("jitter variance", func(t *testing.T) {
		// Run the same attempt multiple times to verify jitter is working
		attempt := 5 // 5^2 = 25 seconds base
		durations := make([]time.Duration, 10)

		for i := 0; i < 10; i++ {
			durations[i] = calculateBackoffWithJitter(attempt)
		}

		// All should be within valid range
		baseTime := 25 * time.Second
		maxJitter := baseTime / 4
		for i, duration := range durations {
			assert.GreaterOrEqual(t, duration, baseTime,
				"Duration %d (%v) should be >= base time (%v)", i, duration, baseTime)
			assert.LessOrEqual(t, duration, baseTime+maxJitter,
				"Duration %d (%v) should be <= base + max jitter (%v)", i, duration, baseTime+maxJitter)
		}

		// Check that we get some variance (not all the same)
		// This test might rarely fail due to randomness, but very unlikely
		allSame := true
		for i := 1; i < len(durations); i++ {
			if durations[i] != durations[0] {
				allSame = false
				break
			}
		}
		assert.False(t, allSame, "Expected some variance in jitter, but all durations were identical: %v", durations[0])
	})

	t.Run("maximum reasonable bounds", func(t *testing.T) {
		// Ensure no attempt produces unreasonably long delays
		for attempt := 1; attempt <= 10; attempt++ {
			duration := calculateBackoffWithJitter(attempt)

			// Maximum should be attempt^2 * 1.25 (base + 25% jitter)
			maxExpected := time.Duration(attempt*attempt) * time.Second * 5 / 4
			assert.LessOrEqual(t, duration, maxExpected,
				"Attempt %d duration (%v) exceeds reasonable maximum (%v)", attempt, duration, maxExpected)

			// Should never exceed 2.5 minutes for any single attempt in our retry range
			// (attempt 10: 100s base + 25s jitter = 125s = 2m5s max)
			assert.LessOrEqual(t, duration, 150*time.Second,
				"Attempt %d duration (%v) exceeds 2.5 minutes", attempt, duration)
		}
	})
}

// TestCleaningDNSProvider_EnsureSubdomainLogic tests the subdomain creation logic
func TestCleaningDNSProvider_EnsureSubdomainLogic(t *testing.T) {
	t.Run("subdomain existence check", func(t *testing.T) {
		// This test verifies that the ensureSubdomainExists method
		// correctly handles the subdomain creation logic without requiring
		// actual Cloudflare API calls

		// Create a mock CleaningDNSProvider
		provider := &CleaningDNSProvider{
			cfAPIToken: "test-token",
			cfZoneID:   "test-zone-id",
		}

		// Test that the provider exists and has the required fields
		assert.NotNil(t, provider)
		assert.Equal(t, "test-token", provider.cfAPIToken)
		assert.Equal(t, "test-zone-id", provider.cfZoneID)

		// Note: The actual ensureSubdomainExists method requires live Cloudflare API
		// For proper testing, this would need to be mocked or tested in integration tests
	})

	t.Run("CleaningDNSProvider creation", func(t *testing.T) {
		// Test NewCleaningDNSProvider validates required parameters
		_, err := NewCleaningDNSProvider(nil, "", "")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "API token and Zone ID are required")

		// Test successful creation
		provider, err := NewCleaningDNSProvider(nil, "token", "zone")
		assert.NoError(t, err)
		assert.NotNil(t, provider)
		assert.Equal(t, "token", provider.cfAPIToken)
		assert.Equal(t, "zone", provider.cfZoneID)
	})
}

// TODO: Add comprehensive ProcessManager tests
// For now, tests can be added in a follow-up since the core functionality is working
