package tailscale

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jerkytreats/dns/internal/config"
)

// setupTestConfig sets up the required configuration for tests
func setupTestConfig(t *testing.T) {
	// Reset config for clean test state
	config.ResetForTest()

	// Set required Tailscale configuration values
	config.SetForTest(TailscaleAPIKeyKey, "test-api-key")
	config.SetForTest(TailscaleTailnetKey, "test-tailnet")
	config.SetForTest(config.TailscaleBaseURLKey, "")
}

// setupMockTailscaleAPI creates a test HTTP server that mocks the Tailscale API
func setupMockTailscaleAPI(t *testing.T) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify authorization header
		auth := r.Header.Get("Authorization")
		if auth != "Bearer test-api-key" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		switch r.URL.Path {
		case "/api/v2/tailnet/test-tailnet/devices":
			mockDevices := DevicesResponse{
				Devices: []Device{
					{
						Name:      "omnitron",
						Hostname:  "omnitron.test-tailnet.ts.net",
						Addresses: []string{"100.65.225.93", "fd7a:115c:a1e0::1"},
						Online:    true,
						ID:        "dev1",
					},
					{
						Name:      "revenantor",
						Hostname:  "revenantor.test-tailnet.ts.net",
						Addresses: []string{"100.115.251.3", "fd7a:115c:a1e0::2"},
						Online:    true,
						ID:        "dev2",
					},
					{
						Name:      "offline-device",
						Hostname:  "offline.test-tailnet.ts.net",
						Addresses: []string{"100.1.1.1"},
						Online:    false,
						ID:        "dev3",
					},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(mockDevices)
		default:
			http.NotFound(w, r)
		}
	}))
}

func TestTailscaleClient_ListDevices(t *testing.T) {
	setupTestConfig(t)
	server := setupMockTailscaleAPI(t)
	defer server.Close()

	// Override the base URL to point to our mock server
	config.SetForTest(config.TailscaleBaseURLKey, server.URL)

	client, err := NewClient()
	if err != nil {
		t.Errorf("NewClient() error = %v", err)
		return
	}

	devices, err := client.ListDevices()
	if err != nil {
		t.Errorf("ListDevices() error = %v", err)
		return
	}

	if len(devices) != 3 {
		t.Errorf("Expected 3 devices, got %d", len(devices))
	}

	// Check first device
	if devices[0].Name != "omnitron" {
		t.Errorf("Expected first device name 'omnitron', got '%s'", devices[0].Name)
	}

	if !devices[0].Online {
		t.Errorf("Expected first device to be online")
	}

	// Check offline device
	if devices[2].Online {
		t.Errorf("Expected third device to be offline")
	}
}

func TestTailscaleClient_GetDevice(t *testing.T) {
	setupTestConfig(t)
	server := setupMockTailscaleAPI(t)
	defer server.Close()

	// Override the base URL to point to our mock server
	config.SetForTest(config.TailscaleBaseURLKey, server.URL)

	client, err := NewClient()
	if err != nil {
		t.Errorf("NewClient() error = %v", err)
		return
	}

	tests := []struct {
		name         string
		deviceName   string
		expectError  bool
		expectedName string
	}{
		{
			name:         "Get device by name",
			deviceName:   "omnitron",
			expectError:  false,
			expectedName: "omnitron",
		},
		{
			name:         "Get device by hostname",
			deviceName:   "revenantor.test-tailnet.ts.net",
			expectError:  false,
			expectedName: "revenantor",
		},
		{
			name:         "Get device by short hostname",
			deviceName:   "revenantor",
			expectError:  false,
			expectedName: "revenantor",
		},
		{
			name:        "Get non-existent device",
			deviceName:  "non-existent",
			expectError: true,
		},
		{
			name:         "Get device by case-insensitive name",
			deviceName:   "OMNITRON", // uppercase, should match "omnitron"
			expectError:  false,
			expectedName: "omnitron",
		},
		{
			name:         "Get device by case-insensitive hostname",
			deviceName:   "REVENANTOR.test-tailnet.ts.net", // uppercase hostname
			expectError:  false,
			expectedName: "revenantor",
		},
		{
			name:         "Get device by case-insensitive short hostname",
			deviceName:   "REVENANTOR", // uppercase short hostname
			expectError:  false,
			expectedName: "revenantor",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			device, err := client.GetDevice(tt.deviceName)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if device.Name != tt.expectedName {
				t.Errorf("Expected device name '%s', got '%s'", tt.expectedName, device.Name)
			}
		})
	}
}

func TestTailscaleClient_GetDeviceIP(t *testing.T) {
	setupTestConfig(t)
	server := setupMockTailscaleAPI(t)
	defer server.Close()

	// Override the base URL to point to our mock server
	config.SetForTest(config.TailscaleBaseURLKey, server.URL)

	client, err := NewClient()
	if err != nil {
		t.Errorf("NewClient() error = %v", err)
		return
	}

	tests := []struct {
		name          string
		deviceName    string
		expectError   bool
		expectedIP    string
		errorContains string
	}{
		{
			name:        "Get IP for online device",
			deviceName:  "omnitron",
			expectError: false,
			expectedIP:  "100.65.225.93",
		},
		{
			name:          "Get IP for offline device",
			deviceName:    "offline-device",
			expectError:   true,
			errorContains: "offline",
		},
		{
			name:          "Get IP for non-existent device",
			deviceName:    "non-existent",
			expectError:   true,
			errorContains: "not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ip, err := client.GetDeviceIP(tt.deviceName)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error, got nil")
					return
				}
				if tt.errorContains != "" && !contains(err.Error(), tt.errorContains) {
					t.Errorf("Expected error to contain '%s', got '%s'", tt.errorContains, err.Error())
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if ip != tt.expectedIP {
				t.Errorf("Expected IP '%s', got '%s'", tt.expectedIP, ip)
			}
		})
	}
}

func TestTailscaleClient_IsDeviceOnline(t *testing.T) {
	setupTestConfig(t)
	server := setupMockTailscaleAPI(t)
	defer server.Close()

	// Override the base URL to point to our mock server
	config.SetForTest(config.TailscaleBaseURLKey, server.URL)

	client, err := NewClient()
	if err != nil {
		t.Errorf("NewClient() error = %v", err)
		return
	}

	tests := []struct {
		name           string
		deviceName     string
		expectError    bool
		expectedOnline bool
	}{
		{
			name:           "Check online device",
			deviceName:     "omnitron",
			expectError:    false,
			expectedOnline: true,
		},
		{
			name:           "Check offline device",
			deviceName:     "offline-device",
			expectError:    false,
			expectedOnline: false,
		},
		{
			name:        "Check non-existent device",
			deviceName:  "non-existent",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			online, err := client.IsDeviceOnline(tt.deviceName)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if online != tt.expectedOnline {
				t.Errorf("Expected online status %v, got %v", tt.expectedOnline, online)
			}
		})
	}
}

func TestTailscaleClient_HandleAPIErrors(t *testing.T) {
	setupTestConfig(t)
	server := setupMockTailscaleAPI(t)
	defer server.Close()

	// Override the base URL to point to our mock server
	config.SetForTest(config.TailscaleBaseURLKey, server.URL)

	client, err := NewClient()
	if err != nil {
		t.Errorf("NewClient() error = %v", err)
		return
	}

	// Test with invalid API key by temporarily changing it
	originalAPIKey := client.apiKey
	client.apiKey = "invalid-key"

	_, err = client.ListDevices()
	if err == nil {
		t.Errorf("Expected error with invalid API key, got nil")
	}

	// Restore original API key
	client.apiKey = originalAPIKey

	// Test with invalid base URL
	client.baseURL = "http://invalid-url-that-does-not-exist.com"

	_, err = client.ListDevices()
	if err == nil {
		t.Errorf("Expected error with invalid base URL, got nil")
	}
}

func TestTailscaleClient_ValidateConnection(t *testing.T) {
	setupTestConfig(t)
	server := setupMockTailscaleAPI(t)
	defer server.Close()

	// Override the base URL to point to our mock server
	config.SetForTest(config.TailscaleBaseURLKey, server.URL)

	client, err := NewClient()
	if err != nil {
		t.Errorf("NewClient() error = %v", err)
		return
	}

	// Test successful validation
	err = client.ValidateConnection()
	if err != nil {
		t.Errorf("Expected successful connection validation, got error: %v", err)
	}
}

// Helper function to check if string contains substring
func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestTailscaleClient_GetDeviceByIP(t *testing.T) {
	setupTestConfig(t)
	server := setupMockTailscaleAPI(t)
	defer server.Close()

	// Override the base URL to point to our mock server
	config.SetForTest(config.TailscaleBaseURLKey, server.URL)

	client, err := NewClient()
	if err != nil {
		t.Errorf("NewClient() error = %v", err)
		return
	}

	tests := []struct {
		name          string
		ip            string
		expectError   bool
		expectedName  string
		errorContains string
	}{
		{
			name:         "Find device by Tailscale IP",
			ip:           "100.65.225.93",
			expectError:  false,
			expectedName: "omnitron",
		},
		{
			name:         "Find device by IPv6 address",
			ip:           "fd7a:115c:a1e0::2",
			expectError:  false,
			expectedName: "revenantor",
		},
		{
			name:         "Find offline device by IP",
			ip:           "100.1.1.1",
			expectError:  false,
			expectedName: "offline-device",
		},
		{
			name:          "IP not found",
			ip:            "100.99.99.99",
			expectError:   true,
			errorContains: "no device found with IP address",
		},
		{
			name:          "Invalid IP format",
			ip:            "invalid-ip",
			expectError:   true,
			errorContains: "no device found with IP address",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			device, err := client.GetDeviceByIP(tt.ip)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error, got nil")
					return
				}
				if tt.errorContains != "" && !contains(err.Error(), tt.errorContains) {
					t.Errorf("Expected error to contain '%s', got '%s'", tt.errorContains, err.Error())
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if device.Name != tt.expectedName {
				t.Errorf("Expected device name '%s', got '%s'", tt.expectedName, device.Name)
			}
		})
	}
}

func TestTailscaleClient_GetTailscaleIP(t *testing.T) {
	setupTestConfig(t)
	server := setupMockTailscaleAPI(t)
	defer server.Close()

	// Override the base URL to point to our mock server
	config.SetForTest(config.TailscaleBaseURLKey, server.URL)

	client, err := NewClient()
	if err != nil {
		t.Errorf("NewClient() error = %v", err)
		return
	}

	tests := []struct {
		name       string
		device     *Device
		expectedIP string
	}{
		{
			name: "Device with Tailscale IP",
			device: &Device{
				Name:      "test-device",
				Addresses: []string{"100.65.225.93", "fd7a:115c:a1e0::1"},
			},
			expectedIP: "100.65.225.93",
		},
		{
			name: "Device with multiple IPs, Tailscale first",
			device: &Device{
				Name:      "test-device",
				Addresses: []string{"100.115.251.3", "192.168.1.100", "fd7a:115c:a1e0::2"},
			},
			expectedIP: "100.115.251.3",
		},
		{
			name: "Device with no Tailscale IP",
			device: &Device{
				Name:      "test-device",
				Addresses: []string{"192.168.1.100", "fd7a:115c:a1e0::1"},
			},
			expectedIP: "",
		},
		{
			name: "Device with no addresses",
			device: &Device{
				Name:      "test-device",
				Addresses: []string{},
			},
			expectedIP: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ip := client.GetTailscaleIP(tt.device)
			if ip != tt.expectedIP {
				t.Errorf("Expected IP '%s', got '%s'", tt.expectedIP, ip)
			}
		})
	}
}

func TestGetTailscaleIPFromAddresses(t *testing.T) {
	tests := []struct {
		name       string
		addresses  []string
		expectedIP string
	}{
		{
			name:       "Single Tailscale IP",
			addresses:  []string{"100.65.225.93"},
			expectedIP: "100.65.225.93",
		},
		{
			name:       "Mixed addresses with Tailscale first",
			addresses:  []string{"100.115.251.3", "192.168.1.100", "fd7a:115c:a1e0::2"},
			expectedIP: "100.115.251.3",
		},
		{
			name:       "Mixed addresses with Tailscale not first",
			addresses:  []string{"192.168.1.100", "100.65.225.93", "fd7a:115c:a1e0::1"},
			expectedIP: "100.65.225.93",
		},
		{
			name:       "No Tailscale IP",
			addresses:  []string{"192.168.1.100", "fd7a:115c:a1e0::1"},
			expectedIP: "",
		},
		{
			name:       "Empty addresses",
			addresses:  []string{},
			expectedIP: "",
		},
		{
			name:       "Nil addresses",
			addresses:  nil,
			expectedIP: "",
		},
		{
			name:       "Edge case: address starting with 100 but not Tailscale",
			addresses:  []string{"1001.1.1.1", "100.65.225.93"},
			expectedIP: "100.65.225.93",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ip := GetTailscaleIPFromAddresses(tt.addresses)
			if ip != tt.expectedIP {
				t.Errorf("Expected IP '%s', got '%s'", tt.expectedIP, ip)
			}
		})
	}
}
