package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jerkytreats/dns/internal/config"
	"github.com/jerkytreats/dns/internal/dns/coredns"
	"github.com/jerkytreats/dns/internal/proxy"
	"github.com/jerkytreats/dns/internal/tailscale"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockProxyManager is a mock implementation of the proxy manager for testing
type mockProxyManager struct {
	rules map[string]*proxy.ProxyRule
}

func newMockProxyManager() proxy.ProxyManagerInterface {
	return &mockProxyManager{
		rules: make(map[string]*proxy.ProxyRule),
	}
}

func (m *mockProxyManager) AddRule(proxyRule *proxy.ProxyRule) error {
	m.rules[proxyRule.Hostname] = proxyRule
	return nil
}

func (m *mockProxyManager) RemoveRule(hostname string) error {
	delete(m.rules, hostname)
	return nil
}

func (m *mockProxyManager) ListRules() []*proxy.ProxyRule {
	rules := make([]*proxy.ProxyRule, 0, len(m.rules))
	for _, rule := range m.rules {
		rules = append(rules, rule)
	}
	return rules
}

func (m *mockProxyManager) IsEnabled() bool {
	return true
}

func (m *mockProxyManager) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"enabled":     true,
		"total_rules": len(m.rules),
	}
}

// mockTailscaleClient is a mock implementation of the tailscale client for testing
type mockTailscaleClient struct {
	devices []tailscale.Device
}

func newMockTailscaleClient() tailscale.TailscaleClientInterface {
	return &mockTailscaleClient{
		devices: []tailscale.Device{
			{
				Name:     "test-device",
				Hostname: "test-device.local",
				Addresses: []string{
					"100.64.1.1",
					"192.168.1.100",
				},
			},
		},
	}
}

func (m *mockTailscaleClient) GetDeviceByIP(ip string) (*tailscale.Device, error) {
	// Return the first device for any IP in tests
	if len(m.devices) > 0 {
		return &m.devices[0], nil
	}
	return nil, fmt.Errorf("device not found")
}

func (m *mockTailscaleClient) GetTailscaleIP(device *tailscale.Device) string {
	// Return the first Tailscale IP (100.64.x.x) from the device
	for _, addr := range device.Addresses {
		if strings.HasPrefix(addr, "100.64.") {
			return addr
		}
	}
	return ""
}

func (m *mockTailscaleClient) GetTailscaleIPFromSourceIP(sourceIP string) (string, error) {
	device, err := m.GetDeviceByIP(sourceIP)
	if err != nil {
		return "", err
	}
	return m.GetTailscaleIP(device), nil
}

func (m *mockTailscaleClient) ListDevices() ([]tailscale.Device, error) {
	return m.devices, nil
}

func (m *mockTailscaleClient) GetDevice(name string) (*tailscale.Device, error) {
	for _, device := range m.devices {
		if device.Name == name {
			return &device, nil
		}
	}
	return nil, fmt.Errorf("device not found")
}

func (m *mockTailscaleClient) GetDeviceIP(name string) (string, error) {
	for _, device := range m.devices {
		if device.Name == name {
			for _, addr := range device.Addresses {
				if strings.HasPrefix(addr, "100.64.") {
					return addr, nil
				}
			}
		}
	}
	return "", fmt.Errorf("device not found")
}

func (m *mockTailscaleClient) IsDeviceOnline(name string) (bool, error) {
	return true, nil
}

func (m *mockTailscaleClient) ValidateConnection() error {
	return nil
}

func (m *mockTailscaleClient) GetCurrentDeviceIP() (string, error) {
	return "100.64.1.1", nil
}

func (m *mockTailscaleClient) GetCurrentDeviceIPByName(name string) (string, error) {
	return "100.64.1.1", nil
}

func setupTestHandler(t *testing.T) *RecordHandler {
	return setupTestHandlerWithProxy(t, nil, nil)
}

func setupTestHandlerWithProxy(t *testing.T, proxyManager proxy.ProxyManagerInterface, tailscaleClient tailscale.TailscaleClientInterface) *RecordHandler {
	// Create temp directory for test
	tempDir, err := os.MkdirTemp("", "coredns-test-*")
	require.NoError(t, err)

	// Create template file
	templatePath := filepath.Join(tempDir, "Corefile.template")
	templateContent := `. {
    errors
    log
}
{{range .Domains}}
{{if .Enabled}}
# Configuration for {{.Domain}}
{{.Domain}}:{{.Port}} {
    file {{.ZoneFile}} {{.Domain}}
    errors
    log
}
{{end}}
{{end}}
`
	err = os.WriteFile(templatePath, []byte(templateContent), 0644)
	require.NoError(t, err)

	// Set up test configuration
	config.SetForTest("dns.coredns.config_path", filepath.Join(tempDir, "Corefile"))
	config.SetForTest("dns.coredns.template_path", templatePath)
	config.SetForTest("dns.coredns.zones_path", filepath.Join(tempDir, "zones"))
	config.SetForTest("dns.domain", "test.local")

	// Create zones directory
	err = os.MkdirAll(filepath.Join(tempDir, "zones"), 0755)
	require.NoError(t, err)

	manager := coredns.NewManager("127.0.0.1")
	err = manager.AddZone("test-service")
	require.NoError(t, err)

	// Create handler with mock dependencies if provided
	var handler *RecordHandler
	var err2 error
	if proxyManager != nil && tailscaleClient != nil {
		handler, err2 = NewRecordHandler(manager, proxyManager, tailscaleClient)
	} else {
		handler, err2 = NewRecordHandler(manager, nil, nil)
	}
	require.NoError(t, err2)

	return handler
}

func TestAddRecordHandler(t *testing.T) {
	// Test without proxy manager (original behavior)
	t.Run("Without proxy manager", func(t *testing.T) {
		handler := setupTestHandler(t)

		tests := []struct {
			name           string
			method         string
			requestBody    interface{}
			expectedStatus int
			expectedBody   string
			expectJSON     bool
		}{
			{
				name:   "Valid request without port",
				method: http.MethodPost,
				requestBody: map[string]string{
					"service_name": "test-service",
					"name":         "test-record",
				},
				expectedStatus: http.StatusCreated,
				expectJSON:     true,
			},
			{
				name:   "Valid request with port (no proxy manager)",
				method: http.MethodPost,
				requestBody: map[string]interface{}{
					"service_name": "test-service",
					"name":         "test-record-with-port",
					"port":         3000,
				},
				expectedStatus: http.StatusCreated,
				expectJSON:     true,
			},
			{
				name:   "Invalid method",
				method: http.MethodGet,
				requestBody: map[string]string{
					"service_name": "test-service",
					"name":         "test-record",
				},
				expectedStatus: http.StatusMethodNotAllowed,
				expectedBody:   "Method not allowed\n",
			},
			{
				name:           "Invalid JSON",
				method:         http.MethodPost,
				requestBody:    "invalid json",
				expectedStatus: http.StatusBadRequest,
				expectedBody:   "Invalid request body\n",
			},
			{
				name:   "Missing service_name",
				method: http.MethodPost,
				requestBody: map[string]string{
					"name": "test-record",
				},
				expectedStatus: http.StatusBadRequest,
				expectedBody:   "Missing required fields: service_name, name\n",
			},
			{
				name:   "Invalid port",
				method: http.MethodPost,
				requestBody: map[string]interface{}{
					"service_name": "test-service",
					"name":         "test-record",
					"port":         70000, // Invalid port
				},
				expectedStatus: http.StatusBadRequest,
				expectedBody:   "Port must be between 1 and 65535\n",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				var body []byte
				if tt.requestBody != nil {
					if strBody, ok := tt.requestBody.(string); ok {
						body = []byte(strBody)
					} else {
						body, _ = json.Marshal(tt.requestBody)
					}
				}

				req := httptest.NewRequest(tt.method, "/add-record", bytes.NewBuffer(body))
				w := httptest.NewRecorder()

				handler.AddRecord(w, req)

				assert.Equal(t, tt.expectedStatus, w.Code)

				if tt.expectJSON {
					// For JSON responses, verify it's a valid Record
					var response Record
					err := json.Unmarshal(w.Body.Bytes(), &response)
					assert.NoError(t, err)
					assert.NotNil(t, response.Record)
					assert.NotEmpty(t, response.Record.Name)
					assert.Equal(t, "A", response.Record.Type)
					assert.NotEmpty(t, response.Record.IP)

					// Verify proxy_rule inference: present if port was in request, absent otherwise
					if reqBody, ok := tt.requestBody.(map[string]interface{}); ok {
						if _, hasPort := reqBody["port"]; hasPort {
							// When no proxy manager is available, proxy_rule should be nil
							assert.Nil(t, response.ProxyRule, "proxy_rule should be nil when proxy manager is not available")
						} else {
							assert.Nil(t, response.ProxyRule, "proxy_rule should be absent when no port is specified")
						}
					} else if _, ok := tt.requestBody.(map[string]string); ok {
						// For string-only requests (no port field possible)
						assert.Nil(t, response.ProxyRule, "proxy_rule should be absent when no port is specified")
					}
				} else if tt.expectedBody != "" {
					assert.Equal(t, tt.expectedBody, w.Body.String())
				}
			})
		}
	})

	// Test with mock proxy manager
	t.Run("With proxy manager", func(t *testing.T) {
		mockProxy := newMockProxyManager()
		mockTailscale := newMockTailscaleClient()
		handler := setupTestHandlerWithProxy(t, mockProxy, mockTailscale)

		tests := []struct {
			name           string
			method         string
			requestBody    interface{}
			expectedStatus int
			expectedBody   string
			expectJSON     bool
		}{
			{
				name:   "Valid request with port (with proxy manager)",
				method: http.MethodPost,
				requestBody: map[string]interface{}{
					"service_name": "test-service",
					"name":         "test-record-with-port",
					"port":         3000,
				},
				expectedStatus: http.StatusCreated,
				expectJSON:     true,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				var body []byte
				if tt.requestBody != nil {
					if strBody, ok := tt.requestBody.(string); ok {
						body = []byte(strBody)
					} else {
						body, _ = json.Marshal(tt.requestBody)
					}
				}

				req := httptest.NewRequest(tt.method, "/add-record", bytes.NewBuffer(body))
				// Set a mock source IP for the request
				req.Header.Set("X-Forwarded-For", "192.168.1.100")
				w := httptest.NewRecorder()

				handler.AddRecord(w, req)

				assert.Equal(t, tt.expectedStatus, w.Code)

				if tt.expectJSON {
					// For JSON responses, verify it's a valid Record
					var response Record
					err := json.Unmarshal(w.Body.Bytes(), &response)
					assert.NoError(t, err)
					assert.NotNil(t, response.Record)
					assert.NotEmpty(t, response.Record.Name)
					assert.Equal(t, "A", response.Record.Type)
					assert.NotEmpty(t, response.Record.IP)

					// Verify proxy_rule inference: present if port was in request
					if reqBody, ok := tt.requestBody.(map[string]interface{}); ok {
						if _, hasPort := reqBody["port"]; hasPort {
							assert.NotNil(t, response.ProxyRule, "proxy_rule should be present when port is specified")
							if response.ProxyRule != nil {
								assert.Greater(t, response.ProxyRule.TargetPort, 0, "proxy rule should have valid target port")
								assert.Equal(t, "test-record-with-port.test.local", response.ProxyRule.Hostname)
								assert.Equal(t, 3000, response.ProxyRule.TargetPort)
								assert.Equal(t, "100.64.1.1", response.ProxyRule.TargetIP)
							}
						} else {
							assert.Nil(t, response.ProxyRule, "proxy_rule should be absent when no port is specified")
						}
					}
				} else if tt.expectedBody != "" {
					assert.Equal(t, tt.expectedBody, w.Body.String())
				}
			})
		}
	})
}

func TestListRecordsHandler(t *testing.T) {
	handler := setupTestHandler(t)

	// Add some test records
	manager := coredns.NewManager("127.0.0.1")

	// Set up test configuration
	tempDir, err := os.MkdirTemp("", "coredns-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	config.SetForTest("dns.coredns.config_path", filepath.Join(tempDir, "Corefile"))
	config.SetForTest("dns.coredns.zones_path", filepath.Join(tempDir, "zones"))
	config.SetForTest("dns.domain", "test.local")

	err = os.MkdirAll(filepath.Join(tempDir, "zones"), 0755)
	require.NoError(t, err)

	err = manager.AddZone("test-service")
	require.NoError(t, err)
	err = manager.AddRecord("test-service", "device1", "100.64.1.1")
	require.NoError(t, err)
	err = manager.AddRecord("test-service", "device2", "100.64.1.2")
	require.NoError(t, err)

	// Create handler with the manager that has records
	handler, err = NewRecordHandler(manager, nil, nil)
	require.NoError(t, err)

	tests := []struct {
		name           string
		method         string
		expectedStatus int
		expectedCount  int
	}{
		{
			name:           "Valid GET request",
			method:         http.MethodGet,
			expectedStatus: http.StatusOK,
			expectedCount:  3, // 2 added records + 1 NS record
		},
		{
			name:           "Invalid method",
			method:         http.MethodPost,
			expectedStatus: http.StatusMethodNotAllowed,
			expectedCount:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, "/list-records", nil)
			w := httptest.NewRecorder()

			handler.ListRecords(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if tt.expectedStatus == http.StatusOK {
				assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

				var records []Record
				err := json.Unmarshal(w.Body.Bytes(), &records)
				require.NoError(t, err)
				assert.Len(t, records, tt.expectedCount)

				if tt.expectedCount > 0 {
					// Verify the structure of returned records
					for _, record := range records {
						assert.NotNil(t, record.Record)
						assert.NotEmpty(t, record.Record.Name)
						assert.Equal(t, "A", record.Record.Type)
						assert.NotEmpty(t, record.Record.IP)
					}

					// Verify specific records exist
					recordMap := make(map[string]string)
					for _, record := range records {
						recordMap[record.Record.Name] = record.Record.IP
					}
					assert.Equal(t, "100.64.1.1", recordMap["device1"])
					assert.Equal(t, "100.64.1.2", recordMap["device2"])
					assert.Equal(t, "127.0.0.1", recordMap["ns"]) // NS record automatically created
				}
			}
		})
	}
}

func TestListRecordsHandler_EmptyZone(t *testing.T) {
	handler := setupTestHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/list-records", nil)
	w := httptest.NewRecorder()

	handler.ListRecords(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var records []coredns.Record
	err := json.Unmarshal(w.Body.Bytes(), &records)
	require.NoError(t, err)
	assert.Len(t, records, 1) // Should have 1 NS record
	assert.Equal(t, "ns", records[0].Name)
	assert.Equal(t, "A", records[0].Type)
	assert.Equal(t, "127.0.0.1", records[0].IP)
}
