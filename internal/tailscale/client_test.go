package tailscale

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

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
	server := setupMockTailscaleAPI(t)
	defer server.Close()

	client, err := NewClient("test-api-key", "test-tailnet", server.URL)
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
	server := setupMockTailscaleAPI(t)
	defer server.Close()

	client, err := NewClient("test-api-key", "test-tailnet", server.URL)
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
	server := setupMockTailscaleAPI(t)
	defer server.Close()

	client, err := NewClient("test-api-key", "test-tailnet", server.URL)
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
	server := setupMockTailscaleAPI(t)
	defer server.Close()

	client, err := NewClient("test-api-key", "test-tailnet", server.URL)
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
	// First create a client with a working mock server for initialization
	workingServer := setupMockTailscaleAPI(t)
	defer workingServer.Close()

	client, err := NewClient("test-api-key", "test-tailnet", workingServer.URL)
	if err != nil {
		t.Errorf("NewClient() error = %v", err)
		return
	}

	// Test server that returns HTTP errors for specific test cases
	errorServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Header.Get("Test-Case") {
		case "unauthorized":
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
		case "server-error":
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		case "invalid-json":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte("invalid json"))
		default:
			http.Error(w, "Not Found", http.StatusNotFound)
		}
	}))
	defer errorServer.Close()

	tests := []struct {
		name      string
		testCase  string
		expectErr bool
	}{
		{
			name:      "Unauthorized error",
			testCase:  "unauthorized",
			expectErr: true,
		},
		{
			name:      "Server error",
			testCase:  "server-error",
			expectErr: true,
		},
		{
			name:      "Invalid JSON response",
			testCase:  "invalid-json",
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a new client instance for this test case
			testClient := &Client{
				apiKey:  client.apiKey,
				tailnet: client.tailnet,
				baseURL: errorServer.URL,
				client: &http.Client{
					Transport: &testTransport{testCase: tt.testCase},
				},
			}

			_, err := testClient.ListDevices()

			if tt.expectErr && err == nil {
				t.Errorf("Expected error, got nil")
			}

			if !tt.expectErr && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

func TestTailscaleClient_ValidateConnection(t *testing.T) {
	server := setupMockTailscaleAPI(t)
	defer server.Close()

	client, err := NewClient("test-api-key", "test-tailnet", server.URL)
	if err != nil {
		t.Errorf("NewClient() error = %v", err)
		return
	}

	err = client.ValidateConnection()
	if err != nil {
		t.Errorf("ValidateConnection() error = %v", err)
	}

	// Test with invalid URL
	badClient, err := NewClient("test-api-key", "test-tailnet", "http://invalid-url")
	if err != nil {
		// Expected error for invalid URL
		return
	}
	err = badClient.ValidateConnection()
	if err == nil {
		t.Errorf("Expected error for invalid URL, got nil")
	}
}

// Helper function to check if string contains substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > len(substr) && (s[:len(substr)] == substr ||
			s[len(s)-len(substr):] == substr ||
			containsSubstring(s, substr))))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// testTransport is a custom RoundTripper for testing HTTP errors
type testTransport struct {
	testCase string
}

func (t *testTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Set("Test-Case", t.testCase)
	return http.DefaultTransport.RoundTrip(req)
}
