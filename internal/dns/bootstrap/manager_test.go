package bootstrap

import (
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

func TestNewManager(t *testing.T) {
	config := createTestBootstrapConfig()

	// Test with nil dependencies for now - integration tests will test with real dependencies
	manager := NewManager(nil, nil, config)

	assert.NotNil(t, manager)
	assert.Equal(t, config, manager.config)
	assert.NotNil(t, manager.ipCache)
	assert.False(t, manager.bootstrapped)
}

func TestIPCaching(t *testing.T) {
	config := createTestBootstrapConfig()
	manager := NewManager(nil, nil, config)

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
			expected: "internal",
		},
		{
			name:     "Zone with trailing dot",
			origin:   "internal.jerkytreats.dev.",
			expected: "internal",
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
		config        config.BootstrapConfig
		expectError   bool
		errorContains string
	}{
		{
			name: "Valid configuration",
			config: config.BootstrapConfig{
				Origin: "internal.test.local",
				Devices: []config.BootstrapDevice{
					{
						Name:          "ns",
						TailscaleName: "omnitron",
						Enabled:       true,
					},
				},
			},
			expectError: false,
		},
		{
			name: "Missing origin",
			config: config.BootstrapConfig{
				Origin: "",
				Devices: []config.BootstrapDevice{
					{
						Name:          "ns",
						TailscaleName: "omnitron",
						Enabled:       true,
					},
				},
			},
			expectError:   true,
			errorContains: "dns.internal.origin is required",
		},
		{
			name: "No bootstrap devices",
			config: config.BootstrapConfig{
				Origin:  "internal.test.local",
				Devices: []config.BootstrapDevice{},
			},
			expectError:   true,
			errorContains: "at least one bootstrap device must be configured",
		},
		{
			name: "Device missing name",
			config: config.BootstrapConfig{
				Origin: "internal.test.local",
				Devices: []config.BootstrapDevice{
					{
						Name:          "",
						TailscaleName: "omnitron",
						Enabled:       true,
					},
				},
			},
			expectError:   true,
			errorContains: "name is required",
		},
		{
			name: "Device missing tailscale_name",
			config: config.BootstrapConfig{
				Origin: "internal.test.local",
				Devices: []config.BootstrapDevice{
					{
						Name:          "ns",
						TailscaleName: "",
						Enabled:       true,
					},
				},
			},
			expectError:   true,
			errorContains: "tailscale_name is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager := NewManager(nil, nil, tt.config)

			err := manager.validateLocalBootstrapConfig()

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

func TestIsZoneBootstrapped(t *testing.T) {
	config := createTestBootstrapConfig()
	manager := NewManager(nil, nil, config)

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

// NOTE: Integration tests should be added separately to test:
// - EnsureInternalZone with real CoreDNS and Tailscale clients
// - RefreshDeviceIPs with real dependencies
// - ValidateConfiguration with real Tailscale client
// - Full bootstrap flow end-to-end
// - Error handling with real failure scenarios
