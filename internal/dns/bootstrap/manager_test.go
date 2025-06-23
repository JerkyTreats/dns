package bootstrap

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/jerkytreats/dns/internal/config"
)

// createTestBootstrapConfig creates a test bootstrap configuration
func createTestBootstrapConfig() config.BootstrapConfig {
	return config.BootstrapConfig{
		Origin: "internal.test.local",
		Devices: []config.BootstrapDevice{
			{
				Name:          "ns",
				TailscaleName: "omnitron",
				Aliases:       []string{"omnitron", "dns"},
				Description:   "NAS, DNS host",
				Enabled:       true,
			},
			{
				Name:          "dev",
				TailscaleName: "revenantor",
				Aliases:       []string{"macbook"},
				Description:   "MacBook development",
				Enabled:       true,
			},
			{
				Name:          "disabled",
				TailscaleName: "offline-device",
				Aliases:       []string{"offline"},
				Description:   "Offline device",
				Enabled:       false,
			},
		},
	}
}

func setupTestConfig() {
	config.ResetForTest()
	config.SetForTest("dns.internal.origin", "internal.test.local")
	config.SetForTest("dns.internal.bootstrap_devices", []map[string]interface{}{
		{
			"name":           "ns",
			"tailscale_name": "omnitron",
			"aliases":        []string{"omnitron", "dns"},
			"description":    "NAS, DNS host",
			"enabled":        true,
		},
		{
			"name":           "dev",
			"tailscale_name": "revenantor",
			"aliases":        []string{"macbook"},
			"description":    "MacBook development",
			"enabled":        true,
		},
		{
			"name":           "disabled",
			"tailscale_name": "offline-device",
			"aliases":        []string{"offline"},
			"description":    "Offline device",
			"enabled":        false,
		},
	})
}

func TestNewManager(t *testing.T) {
	setupTestConfig()
	defer config.ResetForTest()

	// Test with nil dependencies for now - integration tests will test with real dependencies
	manager, err := NewManager(nil, nil)

	assert.NoError(t, err)
	assert.NotNil(t, manager)
	assert.NotNil(t, manager.ipCache)
	assert.False(t, manager.bootstrapped)
	// Note: config will be loaded from actual config file, not test config
}

func TestIPCaching(t *testing.T) {
	setupTestConfig()
	defer config.ResetForTest()

	manager, err := NewManager(nil, nil)
	assert.NoError(t, err)

	// Test caching functionality
	deviceName := "test-device"
	ip := "100.1.1.1"
	ttl := 100 * time.Millisecond

	// Cache should be empty initially
	cachedIP := manager.getCachedIP(deviceName)
	assert.Empty(t, cachedIP)

	// Cache an IP
	manager.cacheIP(deviceName, ip, ttl)

	// Should retrieve cached IP
	cachedIP = manager.getCachedIP(deviceName)
	assert.Equal(t, ip, cachedIP)

	// Wait for TTL to expire
	time.Sleep(ttl + 50*time.Millisecond)

	// Should return empty after TTL expires
	cachedIP = manager.getCachedIP(deviceName)
	assert.Empty(t, cachedIP)
}

func TestExtractZoneName(t *testing.T) {
	tests := []struct {
		name     string
		origin   string
		expected string
	}{
		{
			name:     "Simple internal zone",
			origin:   "internal.jerkytreats.dev",
			expected: "internal.jerkytreats.dev",
		},
		{
			name:     "Zone with trailing dot",
			origin:   "internal.jerkytreats.dev.",
			expected: "internal.jerkytreats.dev",
		},
		{
			name:     "Single part origin",
			origin:   "internal",
			expected: "internal",
		},
		{
			name:     "Empty origin",
			origin:   "",
			expected: "",
		},
		{
			name:     "Only dot",
			origin:   ".",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractZoneName(tt.origin)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestValidateLocalBootstrapConfig(t *testing.T) {
	tests := []struct {
		name          string
		setupConfig   func()
		expectError   bool
		errorContains string
	}{
		{
			name: "Valid configuration",
			setupConfig: func() {
				config.ResetForTest()
				config.SetForTest("dns.internal.origin", "internal.test.local")
				config.SetForTest("dns.internal.bootstrap_devices", []map[string]interface{}{
					{
						"name":           "ns",
						"tailscale_name": "omnitron",
						"enabled":        true,
					},
				})
			},
			expectError: false,
		},
		{
			name: "Missing origin",
			setupConfig: func() {
				config.ResetForTest()
				config.SetForTest("dns.internal.bootstrap_devices", []map[string]interface{}{
					{
						"name":           "ns",
						"tailscale_name": "omnitron",
						"enabled":        true,
					},
				})
			},
			expectError:   true,
			errorContains: "dns.internal.origin is required",
		},
		{
			name: "No bootstrap devices",
			setupConfig: func() {
				config.ResetForTest()
				config.SetForTest("dns.internal.origin", "internal.test.local")
				config.SetForTest("dns.internal.bootstrap_devices", []map[string]interface{}{})
			},
			expectError:   true,
			errorContains: "at least one bootstrap device must be configured",
		},
		{
			name: "Device missing name",
			setupConfig: func() {
				config.ResetForTest()
				config.SetForTest("dns.internal.origin", "internal.test.local")
				config.SetForTest("dns.internal.bootstrap_devices", []map[string]interface{}{
					{
						"tailscale_name": "omnitron",
						"enabled":        true,
					},
				})
			},
			expectError:   true,
			errorContains: "name is required",
		},
		{
			name: "Device missing tailscale_name",
			setupConfig: func() {
				config.ResetForTest()
				config.SetForTest("dns.internal.origin", "internal.test.local")
				config.SetForTest("dns.internal.bootstrap_devices", []map[string]interface{}{
					{
						"name":    "ns",
						"enabled": true,
					},
				})
			},
			expectError:   true,
			errorContains: "tailscale_name is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupConfig()
			defer config.ResetForTest()

			manager, err := NewManager(nil, nil)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, manager)
				// Additional validation test
				err = manager.ValidateBootstrapConfig()
				assert.NoError(t, err)
			}
		})
	}
}

func TestIsZoneBootstrapped(t *testing.T) {
	setupTestConfig()
	defer config.ResetForTest()

	manager, err := NewManager(nil, nil)
	assert.NoError(t, err)

	// Initially not bootstrapped
	assert.False(t, manager.IsZoneBootstrapped())

	// Set bootstrapped state
	manager.bootstrapped = true
	assert.True(t, manager.IsZoneBootstrapped())
}

func TestDeviceResolutionStructure(t *testing.T) {
	device := config.BootstrapDevice{
		Name:          "test",
		TailscaleName: "test-device",
		Aliases:       []string{"alias1"},
		Enabled:       true,
	}

	resolution := DeviceResolution{
		Device:  device,
		IP:      "100.1.1.1",
		Online:  true,
		Error:   nil,
		Skipped: false,
		Reason:  "",
	}

	assert.Equal(t, device, resolution.Device)
	assert.Equal(t, "100.1.1.1", resolution.IP)
	assert.True(t, resolution.Online)
	assert.NoError(t, resolution.Error)
	assert.False(t, resolution.Skipped)
}

func TestBootstrapResultStructure(t *testing.T) {
	result := BootstrapResult{
		Success:         true,
		TotalDevices:    3,
		ResolvedDevices: 2,
		SkippedDevices:  1,
		FailedDevices:   0,
		Resolutions:     []DeviceResolution{},
		Error:           nil,
	}

	assert.True(t, result.Success)
	assert.Equal(t, 3, result.TotalDevices)
	assert.Equal(t, 2, result.ResolvedDevices)
	assert.Equal(t, 1, result.SkippedDevices)
	assert.Equal(t, 0, result.FailedDevices)
	assert.NotNil(t, result.Resolutions)
	assert.NoError(t, result.Error)
}

func TestBootstrapConfigValidation(t *testing.T) {
	tests := []struct {
		name          string
		config        string
		expectError   bool
		errorContains string
	}{
		{
			name: "Valid configuration",
			config: `
dns:
  internal:
    origin: "internal.test.local"
    bootstrap_devices:
      - name: "ns"
        tailscale_name: "omnitron"
        enabled: true
`,
			expectError: false,
		},
		{
			name: "Missing origin",
			config: `
dns:
  internal:
    bootstrap_devices:
      - name: "ns"
        tailscale_name: "omnitron"
        enabled: true
`,
			expectError:   true,
			errorContains: "dns.internal.origin is required",
		},
		{
			name: "No bootstrap devices",
			config: `
dns:
  internal:
    origin: "internal.test.local"
    bootstrap_devices: []
`,
			expectError:   true,
			errorContains: "at least one bootstrap device must be configured",
		},
		{
			name: "Device missing name",
			config: `
dns:
  internal:
    origin: "internal.test.local"
    bootstrap_devices:
      - tailscale_name: "omnitron"
        enabled: true
`,
			expectError:   true,
			errorContains: "name is required",
		},
		{
			name: "Device missing tailscale_name",
			config: `
dns:
  internal:
    origin: "internal.test.local"
    bootstrap_devices:
      - name: "ns"
        enabled: true
`,
			expectError:   true,
			errorContains: "tailscale_name is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config.ResetForTest()
			defer config.ResetForTest()

			// Create temporary config file
			tmpDir := t.TempDir()
			configFile := filepath.Join(tmpDir, "config.yaml")
			err := os.WriteFile(configFile, []byte(tt.config), 0644)
			assert.NoError(t, err)

			// Initialize config
			err = config.InitConfig(config.WithConfigPath(configFile))
			assert.NoError(t, err)

			bootstrapConfig := config.GetBootstrapConfig()
			manager := &Manager{config: bootstrapConfig}
			err = manager.ValidateBootstrapConfig()

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// NOTE: Integration tests should be added separately to test:
// - EnsureInternalZone with real CoreDNS and Tailscale clients
// - RefreshDeviceIPs with real dependencies
// - ValidateConfiguration with real Tailscale client
// - Full bootstrap flow end-to-end
// - Error handling with real failure scenarios
