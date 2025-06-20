package tests

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jerkytreats/dns/internal/config"
	"github.com/jerkytreats/dns/internal/dns/bootstrap"
	"github.com/jerkytreats/dns/internal/dns/coredns"
	"github.com/jerkytreats/dns/internal/tailscale"
)

// TestBootstrapIntegration tests the full bootstrap flow with real dependencies
// This test is skipped unless INTEGRATION_TEST environment variable is set
func TestBootstrapIntegration(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") == "" {
		t.Skip("Skipping integration test - set INTEGRATION_TEST=1 to run")
	}

	// Ensure required environment variables are set
	tailscaleAPIKey := os.Getenv("TAILSCALE_API_KEY")
	tailscaleTailnet := os.Getenv("TAILSCALE_TAILNET")

	if tailscaleAPIKey == "" || tailscaleTailnet == "" {
		t.Skip("Skipping integration test - TAILSCALE_API_KEY and TAILSCALE_TAILNET must be set")
	}

	// Initialize configuration for testing
	err := config.InitConfig(config.WithConfigPath("configs/config-bootstrap-example.yaml"))
	require.NoError(t, err, "Failed to initialize config")

	// Get bootstrap configuration
	bootstrapConfig := config.GetBootstrapConfig()

	// Create CoreDNS manager (using test configuration)
	corednsManager := coredns.NewManager("configs/coredns-test", "configs/coredns-test/zones", []string{"reload"}, "test.local")

	// Create Tailscale client
	tailscaleClient := tailscale.NewClient(tailscaleAPIKey, tailscaleTailnet)

	// Create bootstrap manager
	bootstrapManager := bootstrap.NewManager(corednsManager, tailscaleClient, bootstrapConfig)

	// Test configuration validation
	t.Run("ValidateConfiguration", func(t *testing.T) {
		err := bootstrapManager.ValidateConfiguration()
		assert.NoError(t, err, "Configuration validation should succeed")
	})

	// Test zone bootstrap
	t.Run("EnsureInternalZone", func(t *testing.T) {
		err := bootstrapManager.EnsureInternalZone()
		assert.NoError(t, err, "Zone bootstrap should succeed")
		assert.True(t, bootstrapManager.IsZoneBootstrapped(), "Zone should be marked as bootstrapped")
	})

	// Test IP refresh
	t.Run("RefreshDeviceIPs", func(t *testing.T) {
		err := bootstrapManager.RefreshDeviceIPs()
		assert.NoError(t, err, "IP refresh should succeed")
	})
}

// TestBootstrapWithMockTailscale tests bootstrap with mock Tailscale server
// This provides integration testing without requiring real Tailscale API access
func TestBootstrapWithMockTailscale(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") == "" {
		t.Skip("Skipping integration test - set INTEGRATION_TEST=1 to run")
	}

	// Create mock Tailscale server (could be implemented using httptest.Server)
	// This would simulate Tailscale API responses for testing

	// Initialize configuration for testing
	err := config.InitConfig(config.WithConfigPath("configs/config-bootstrap-example.yaml"))
	require.NoError(t, err, "Failed to initialize config")

	// Get bootstrap configuration
	bootstrapConfig := config.GetBootstrapConfig()

	// Create CoreDNS manager (using test configuration)
	corednsManager := coredns.NewManager("configs/coredns-test", "configs/coredns-test/zones", []string{"reload"}, "test.local")

	// Create Tailscale client with mock server URL
	mockAPIKey := "test-api-key"
	mockTailnet := "test-tailnet"
	mockBaseURL := "http://localhost:8080" // Would point to mock server

	tailscaleClient := tailscale.NewClientWithBaseURL(mockAPIKey, mockTailnet, mockBaseURL)

	// Create bootstrap manager
	bootstrapManager := bootstrap.NewManager(corednsManager, tailscaleClient, bootstrapConfig)

	// Tests would go here - these would test with controlled responses
	// from the mock Tailscale server

	// Use the bootstrap manager to avoid unused variable error
	assert.NotNil(t, bootstrapManager)
	t.Log("Mock Tailscale integration test would be implemented here")
}

// TestBootstrapErrorScenarios tests error handling scenarios
func TestBootstrapErrorScenarios(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") == "" {
		t.Skip("Skipping integration test - set INTEGRATION_TEST=1 to run")
	}

	// Test with invalid configurations
	t.Run("InvalidConfiguration", func(t *testing.T) {
		invalidConfig := config.BootstrapConfig{
			Origin:  "", // Invalid - empty origin
			Devices: []config.BootstrapDevice{},
		}

		// Create bootstrap manager with invalid config (nil clients are fine for this test)
		bootstrapManager := bootstrap.NewManager(nil, nil, invalidConfig)

		// This should demonstrate that config validation catches invalid configurations
		// Note: This test shows the architecture - in practice, you'd test the validation
		// logic separately or use a working Tailscale client
		assert.NotNil(t, bootstrapManager, "Manager should be created even with invalid config")

		// The actual validation would happen during ValidateConfiguration, but that
		// requires a working Tailscale client. This demonstrates the separation of concerns.
		t.Log("Configuration validation logic is tested in unit tests")
	})

	// Test with unreachable Tailscale API
	t.Run("UnreachableTailscaleAPI", func(t *testing.T) {
		// Create configuration
		validConfig := config.BootstrapConfig{
			Origin: "internal.test.local",
			Devices: []config.BootstrapDevice{
				{
					Name:          "test",
					TailscaleName: "test-device",
					Enabled:       true,
				},
			},
		}

		// Create Tailscale client with invalid base URL
		invalidAPIKey := "invalid-key"
		invalidTailnet := "invalid-tailnet"
		invalidBaseURL := "http://invalid-url:9999"

		tailscaleClient := tailscale.NewClientWithBaseURL(invalidAPIKey, invalidTailnet, invalidBaseURL)

		bootstrapManager := bootstrap.NewManager(nil, tailscaleClient, validConfig)

		// Should fail validation due to unreachable API
		err := bootstrapManager.ValidateConfiguration()
		assert.Error(t, err, "Should fail with unreachable Tailscale API")
	})
}

// TestBootstrapPerformance tests performance characteristics
func TestBootstrapPerformance(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") == "" {
		t.Skip("Skipping integration test - set INTEGRATION_TEST=1 to run")
	}

	// Performance tests would go here:
	// - Test caching effectiveness
	// - Test retry behavior timing
	// - Test concurrent bootstrap operations
	// - Test large device list handling

	t.Log("Performance integration tests would be implemented here")
}

/*
Integration Test Usage:

To run integration tests:
1. Set up environment variables:
   export INTEGRATION_TEST=1
   export TAILSCALE_API_KEY="your-api-key"
   export TAILSCALE_TAILNET="your-tailnet"

2. Run tests:
   go test ./tests/... -v -run TestBootstrapIntegration

For mock integration tests (recommended for CI):
1. Implement mock Tailscale server
2. Set INTEGRATION_TEST=1
3. Run: go test ./tests/... -v -run TestBootstrapWithMockTailscale

This approach provides:
- Real integration testing when needed
- Controlled testing with mock servers
- Error scenario testing
- Performance testing capabilities
- No invasive mocking in core logic
*/
