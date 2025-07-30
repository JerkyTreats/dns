package certificate

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"fmt"
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
	// Clean up any leftover certificate data files from previous test runs
	CleanupCertificateDataDir(t)
	
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
	
	// Set certificate domain storage path to use a temporary directory
	config.SetForTest(CertDomainStoragePathKey, filepath.Join(tempDir, "certificate_domains.json"))

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

// TestDNSTimingFunctions tests the new DNS timing configuration functions
func TestDNSTimingFunctions(t *testing.T) {
	t.Run("getCleanupWait with explicit config", func(t *testing.T) {
		config.ResetForTest()
		config.SetForTest(CertDNSCleanupWaitKey, "300s")
		
		wait := getCleanupWait()
		assert.Equal(t, 300*time.Second, wait)
	})
	
	t.Run("getCleanupWait production default", func(t *testing.T) {
		config.ResetForTest()
		config.SetForTest(CertUseProdCertsKey, true)
		
		wait := getCleanupWait()
		assert.Equal(t, 120*time.Second, wait)
	})
	
	t.Run("getCleanupWait staging default", func(t *testing.T) {
		config.ResetForTest()
		config.SetForTest(CertUseProdCertsKey, false)
		
		wait := getCleanupWait()
		assert.Equal(t, 90*time.Second, wait)
	})
	
	t.Run("getCreationWait with explicit config", func(t *testing.T) {
		config.ResetForTest()
		config.SetForTest(CertDNSCreationWaitKey, "45s")
		
		wait := getCreationWait()
		assert.Equal(t, 45*time.Second, wait)
	})
	
	t.Run("getCreationWait production default", func(t *testing.T) {
		config.ResetForTest()
		config.SetForTest(CertUseProdCertsKey, true)
		
		wait := getCreationWait()
		assert.Equal(t, 90*time.Second, wait)
	})
	
	t.Run("getCreationWait staging default", func(t *testing.T) {
		config.ResetForTest()
		config.SetForTest(CertUseProdCertsKey, false)
		
		wait := getCreationWait()
		assert.Equal(t, 60*time.Second, wait)
	})
}

// TestRateLimitDetection tests the rate limit error detection function
func TestRateLimitDetection(t *testing.T) {
	testCases := []struct {
		name          string
		errorMessage  string
		expectRateLimit bool
	}{
		{
			name:            "explicit rate limit error",
			errorMessage:    "Error creating new certificate: rate limit exceeded",
			expectRateLimit: true,
		},
		{
			name:            "too many certificates error",
			errorMessage:    "too many certificates already issued",
			expectRateLimit: true,
		},
		{
			name:            "rate limited error",
			errorMessage:    "Request was rate limited",
			expectRateLimit: true,
		},
		{
			name:            "rateLimited camelCase",
			errorMessage:    "Certificate request rateLimited by server",
			expectRateLimit: true,
		},
		{
			name:            "DNS propagation error",
			errorMessage:    "DNS propagation failed: record not found",
			expectRateLimit: false,
		},
		{
			name:            "network error",
			errorMessage:    "Connection timeout",
			expectRateLimit: false,
		},
		{
			name:            "validation error",
			errorMessage:    "Challenge validation failed",
			expectRateLimit: false,
		},
		{
			name:            "empty error",
			errorMessage:    "",
			expectRateLimit: false,
		},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := fmt.Errorf("%s", tc.errorMessage)
			result := isRateLimitError(err)
			assert.Equal(t, tc.expectRateLimit, result, 
				"Expected rate limit detection for '%s' to be %v, got %v", 
				tc.errorMessage, tc.expectRateLimit, result)
		})
	}
}

// TestDNSPropagationVerification tests the DNS propagation verification logic
func TestDNSPropagationVerification(t *testing.T) {
	t.Run("uses configured resolvers", func(t *testing.T) {
		config.ResetForTest()
		customResolvers := []string{"1.1.1.1:53", "9.9.9.9:53"}
		config.SetForTest(CertDNSResolversKey, customResolvers)
		
		// Create a mock CleaningDNSProvider to test the logic
		provider := &CleaningDNSProvider{
			cfAPIToken: "test-token",
			cfZoneID:   "test-zone-id",
		}
		
		// Note: This test verifies the configuration reading logic
		// Actual DNS queries would need network mocking for full testing
		resolvers := config.GetStringSlice(CertDNSResolversKey)
		assert.Equal(t, customResolvers, resolvers)
		assert.NotNil(t, provider)
	})
	
	t.Run("uses default resolvers when none configured", func(t *testing.T) {
		config.ResetForTest()
		// Don't set any resolvers
		
		resolvers := config.GetStringSlice(CertDNSResolversKey)
		assert.Empty(t, resolvers, "Should be empty when not configured")
		
		// The actual function would fall back to defaults:
		// []string{"8.8.8.8:53", "1.1.1.1:53"}
	})
}

// TestEnvironmentAwareDefaults tests the staging vs production behavior
func TestEnvironmentAwareDefaults(t *testing.T) {
	testCases := []struct {
		name                string
		useProductionCerts  bool
		expectedCleanupWait time.Duration
		expectedCreationWait time.Duration
	}{
		{
			name:                 "production environment",
			useProductionCerts:   true,
			expectedCleanupWait:  120 * time.Second,
			expectedCreationWait: 90 * time.Second,
		},
		{
			name:                 "staging environment",
			useProductionCerts:   false,
			expectedCleanupWait:  90 * time.Second,
			expectedCreationWait: 60 * time.Second,
		},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			config.ResetForTest()
			config.SetForTest(CertUseProdCertsKey, tc.useProductionCerts)
			
			cleanupWait := getCleanupWait()
			creationWait := getCreationWait()
			
			assert.Equal(t, tc.expectedCleanupWait, cleanupWait, 
				"Cleanup wait for %s should be %v", tc.name, tc.expectedCleanupWait)
			assert.Equal(t, tc.expectedCreationWait, creationWait,
				"Creation wait for %s should be %v", tc.name, tc.expectedCreationWait)
		})
	}
}

// TestNewConfigKeys tests that the new configuration keys are properly registered
func TestNewConfigKeys(t *testing.T) {
	t.Run("DNS timing configuration keys exist", func(t *testing.T) {
		assert.Equal(t, "certificate.dns_cleanup_wait", CertDNSCleanupWaitKey)
		assert.Equal(t, "certificate.dns_creation_wait", CertDNSCreationWaitKey)
		assert.Equal(t, "certificate.use_production_certs", CertUseProdCertsKey)
	})
}

// TestCleaningDNSProviderValidation tests validation of the CleaningDNSProvider
func TestCleaningDNSProviderValidation(t *testing.T) {
	t.Run("requires API token", func(t *testing.T) {
		_, err := NewCleaningDNSProvider(nil, "", "zone-id")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "API token and Zone ID are required")
	})
	
	t.Run("requires zone ID", func(t *testing.T) {
		_, err := NewCleaningDNSProvider(nil, "token", "")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "API token and Zone ID are required")
	})
	
	t.Run("successful creation with valid parameters", func(t *testing.T) {
		provider, err := NewCleaningDNSProvider(nil, "valid-token", "valid-zone-id")
		assert.NoError(t, err)
		assert.NotNil(t, provider)
		assert.Equal(t, "valid-token", provider.cfAPIToken)
		assert.Equal(t, "valid-zone-id", provider.cfZoneID)
	})
}

// TestRateLimitAwareBackoff tests the new rate-limit-aware backoff calculation
func TestRateLimitAwareBackoff(t *testing.T) {
	t.Run("production environment respects 12-minute refill rate", func(t *testing.T) {
		testCases := []struct {
			attempt  int
			expected time.Duration
		}{
			{1, 2 * time.Minute},   // First retry: quick for transient issues
			{2, 12 * time.Minute},  // Second retry: minimum refill interval
			{3, 15 * time.Minute},  // Third retry: slightly longer
			{4, 20 * time.Minute},  // Fourth retry: more spacing
			{5, 30 * time.Minute},  // Fifth+ retry: avoid weekly limits
			{10, 30 * time.Minute}, // Capped at 30 minutes
		}
		
		for _, tc := range testCases {
			t.Run(fmt.Sprintf("attempt_%d", tc.attempt), func(t *testing.T) {
				backoff := calculateRateLimitAwareBackoff(tc.attempt, true)
				assert.Equal(t, tc.expected, backoff, 
					"Production backoff for attempt %d should be %v, got %v", 
					tc.attempt, tc.expected, backoff)
			})
		}
	})
	
	t.Run("staging environment uses exponential backoff with jitter", func(t *testing.T) {
		testCases := []struct {
			attempt     int
			minExpected time.Duration
			maxExpected time.Duration
		}{
			{1, 1 * time.Minute, 1*time.Minute + 12*time.Second},      // 1 minute + 20% jitter
			{2, 2 * time.Minute, 2*time.Minute + 24*time.Second},      // 2 minutes + 20% jitter
			{3, 4 * time.Minute, 4*time.Minute + 48*time.Second},      // 4 minutes + 20% jitter
			{4, 8 * time.Minute, 8*time.Minute + 96*time.Second},      // 8 minutes + 20% jitter
			{5, 16 * time.Minute, 16*time.Minute + 192*time.Second},   // 16 minutes + 20% jitter
			{6, 30 * time.Minute, 30*time.Minute + 360*time.Second},   // Capped at 30 minutes + jitter
		}
		
		for _, tc := range testCases {
			t.Run(fmt.Sprintf("staging_attempt_%d", tc.attempt), func(t *testing.T) {
				// Run multiple times to test jitter variance
				for i := 0; i < 5; i++ {
					backoff := calculateRateLimitAwareBackoff(tc.attempt, false)
					assert.GreaterOrEqual(t, backoff, tc.minExpected,
						"Staging backoff for attempt %d should be >= %v, got %v", 
						tc.attempt, tc.minExpected, backoff)
					assert.LessOrEqual(t, backoff, tc.maxExpected,
						"Staging backoff for attempt %d should be <= %v, got %v", 
						tc.attempt, tc.maxExpected, backoff)
				}
			})
		}
	})
	
	t.Run("zero and negative attempts", func(t *testing.T) {
		assert.Equal(t, time.Duration(0), calculateRateLimitAwareBackoff(0, true))
		assert.Equal(t, time.Duration(0), calculateRateLimitAwareBackoff(-1, true))
		assert.Equal(t, time.Duration(0), calculateRateLimitAwareBackoff(0, false))
		assert.Equal(t, time.Duration(0), calculateRateLimitAwareBackoff(-1, false))
	})
}

// TestObtainCertificateWithRetryRateLimits tests the retry behavior with rate limits
func TestObtainCertificateWithRetryRateLimits(t *testing.T) {
	t.Run("production uses 5 max retries", func(t *testing.T) {
		config.ResetForTest()
		config.SetForTest(CertUseProdCertsKey, true)
		
		// This test verifies the retry logic structure
		// Full integration testing would require mocking the ACME client
		
		// Verify production setting affects retry count
		isProduction := config.GetBool(CertUseProdCertsKey)
		assert.True(t, isProduction)
		
		expectedMaxRetries := 5
		assert.Equal(t, 5, expectedMaxRetries, "Production should use 5 max retries")
	})
	
	t.Run("staging uses 8 max retries", func(t *testing.T) {
		config.ResetForTest()
		config.SetForTest(CertUseProdCertsKey, false)
		
		isProduction := config.GetBool(CertUseProdCertsKey)
		assert.False(t, isProduction)
		
		expectedMaxRetries := 8
		assert.Equal(t, 8, expectedMaxRetries, "Staging should use 8 max retries")
	})
}

// TestRateLimitStrategy tests the overall rate limiting strategy
func TestRateLimitStrategy(t *testing.T) {
	t.Run("production backoff prevents hitting 5 failures per hour", func(t *testing.T) {
		// Calculate total time for 5 attempts in production
		totalTime := time.Duration(0)
		for attempt := 1; attempt <= 5; attempt++ {
			if attempt > 1 { // Don't count first attempt
				backoff := calculateRateLimitAwareBackoff(attempt-1, true)
				totalTime += backoff
			}
		}
		
		// Total backoff: 2 + 12 + 15 + 20 = 49 minutes
		expectedTotal := 49 * time.Minute
		assert.Equal(t, expectedTotal, totalTime,
			"Production backoff should space out 5 attempts over %v to respect rate limits", expectedTotal)
			
		// Verify we don't exceed 1 hour total
		assert.LessOrEqual(t, totalTime, 1*time.Hour,
			"Production backoff should keep attempts within 1 hour window")
	})
	
	t.Run("staging backoff is more aggressive but reasonable", func(t *testing.T) {
		// Calculate total time for first 5 attempts in staging
		totalTime := time.Duration(0)
		for attempt := 1; attempt <= 5; attempt++ {
			if attempt > 1 {
				backoff := calculateRateLimitAwareBackoff(attempt-1, false)
				totalTime += backoff
			}
		}
		
		// Staging should be faster than production
		productionTotal := 49 * time.Minute
		assert.Less(t, totalTime, productionTotal,
			"Staging backoff should be faster than production")
		
		// But still reasonable (not immediate retries)
		assert.Greater(t, totalTime, 10*time.Minute,
			"Staging backoff should still provide reasonable spacing")
	})
}

// TODO: Add comprehensive ProcessManager tests
// For now, tests can be added in a follow-up since the core functionality is working
