package sync_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jerkytreats/dns/internal/config"
	"github.com/jerkytreats/dns/internal/tailscale/sync"
)

// createTestSyncConfig creates a test sync configuration
func createTestSyncConfig() config.SyncConfig {
	return config.SyncConfig{
		Origin: "internal.test.local",
		Devices: []config.SyncDevice{
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
	config.SetForTest("dns.internal.sync_devices", []map[string]interface{}{
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
	manager, err := sync.NewManager(nil, nil)

	assert.NoError(t, err)
	assert.NotNil(t, manager)
	// Note: config will be loaded from actual config file, not test config
}

func TestValidateSyncConfig(t *testing.T) {
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
				config.SetForTest("dns.internal.sync_devices", []map[string]interface{}{
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
				config.SetForTest("dns.internal.sync_devices", []map[string]interface{}{
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
			name: "No sync devices",
			setupConfig: func() {
				config.ResetForTest()
				config.SetForTest("dns.internal.origin", "internal.test.local")
				config.SetForTest("dns.internal.sync_devices", []map[string]interface{}{})
			},
			expectError:   true,
			errorContains: "at least one sync device must be configured",
		},
		{
			name: "Device missing name",
			setupConfig: func() {
				config.ResetForTest()
				config.SetForTest("dns.internal.origin", "internal.test.local")
				config.SetForTest("dns.internal.sync_devices", []map[string]interface{}{
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
				config.SetForTest("dns.internal.sync_devices", []map[string]interface{}{
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

			manager, err := sync.NewManager(nil, nil)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, manager)

				// Initially, zone should not be synced
				assert.False(t, manager.IsZoneSynced())

				// TODO: Add more advanced tests with mock CoreDNS manager
			}
		})
	}
}

func TestDeviceResolutionStructure(t *testing.T) {
	// This test simply verifies the structure of the DeviceResolution type
	// to ensure all expected fields are present.
	resolution := sync.DeviceResolution{
		Device: config.SyncDevice{
			Name:          "test-device",
			TailscaleName: "test-tailscale",
			Enabled:       true,
		},
		IP:      "100.1.1.1",
		Online:  true,
		Error:   nil,
		Skipped: false,
		Reason:  "",
	}
	assert.Equal(t, "test-device", resolution.Device.Name)
	assert.Equal(t, "100.1.1.1", resolution.IP)
	assert.True(t, resolution.Online)
}

func TestSyncResultStructure(t *testing.T) {
	// This test verifies the structure of the SyncResult type.
	result := sync.SyncResult{
		Success:         true,
		TotalDevices:    1,
		ResolvedDevices: 1,
		SkippedDevices:  0,
		FailedDevices:   0,
		Resolutions:     []sync.DeviceResolution{},
		Error:           nil,
	}
	assert.True(t, result.Success)
	assert.Equal(t, 1, result.TotalDevices)
}

func TestSyncConfigValidation(t *testing.T) {
	// This test validates that the SyncConfig struct has the expected fields.
	// This helps catch accidental changes to the config structure.
	conf := config.SyncConfig{
		Origin:  "internal.test.local",
		Devices: []config.SyncDevice{},
	}
	assert.Equal(t, "internal.test.local", conf.Origin)
}

// NOTE: Integration tests should be added separately to test:
// - EnsureInternalZone with real CoreDNS and Tailscale clients
// - RefreshDeviceIPs with real dependencies
// - ValidateConfiguration with real Tailscale client
// - Full sync flow end-to-end
// - Error handling with real failure scenarios
