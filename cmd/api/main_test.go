package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jerkytreats/dns/internal/api/handler"
	"github.com/jerkytreats/dns/internal/config"
	"github.com/jerkytreats/dns/internal/dns/coredns"
	"github.com/jerkytreats/dns/internal/proxy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHealthCheckHandler(t *testing.T) {
	// Setup test configuration
	config.SetForTest("app.version", "1.0.0")
	t.Cleanup(config.ResetForTest)

	// Create test components
	dnsManager := coredns.NewManager("127.0.0.1")
	mockDNSChecker := &mockDNSChecker{}
	mockProxyManager := &mockProxyManager{}

	// Create handler registry
	handlerRegistry, err := handler.NewHandlerRegistry(dnsManager, mockDNSChecker, nil, mockProxyManager, nil, nil)
	require.NoError(t, err)

	// Create a mux and register handlers
	mux := http.NewServeMux()
	handlerRegistry.RegisterHandlers(mux)

	tests := []struct {
		name           string
		method         string
		expectedStatus int
		expectedBody   map[string]interface{}
	}{
		{
			name:           "Valid GET request",
			method:         http.MethodGet,
			expectedStatus: http.StatusOK,
			expectedBody: map[string]interface{}{
				"status":  "healthy",
				"version": "1.0.0",
				"components": map[string]interface{}{
					"api": map[string]interface{}{
						"status":  "healthy",
						"message": "API is running",
					},
					"coredns": map[string]interface{}{
						"status":  "healthy",
						"message": "CoreDNS responded in 10ms",
					},
					"caddy": map[string]interface{}{
						"status":  "healthy",
						"message": "Caddy responded in 5ms",
					},
					"sync": map[string]interface{}{
						"status":  "disabled",
						"message": "Dynamic zone sync is disabled",
					},
				},
			},
		},
		{
			name:           "Invalid method",
			method:         http.MethodPost,
			expectedStatus: http.StatusMethodNotAllowed,
			expectedBody:   nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, "/health", nil)
			w := httptest.NewRecorder()

			mux.ServeHTTP(w, req)

			// Check status code
			assert.Equal(t, tt.expectedStatus, w.Code)

			// Check response body if expected
			if tt.expectedBody != nil {
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedBody, response)
			}
		})
	}
}

// mockDNSChecker implements the healthcheck.Checker interface for testing
type mockDNSChecker struct{}

func (m *mockDNSChecker) Name() string {
	return "mock-dns-checker"
}

func (m *mockDNSChecker) CheckOnce() (bool, time.Duration, error) {
	return true, 10 * time.Millisecond, nil
}

func (m *mockDNSChecker) WaitHealthy() bool {
	return true
}

// mockProxyManager implements the proxy.ProxyManagerInterface for testing
type mockProxyManager struct{}

func (m *mockProxyManager) AddRule(proxyRule *proxy.ProxyRule) error {
	return nil
}

func (m *mockProxyManager) RemoveRule(hostname string) error {
	return nil
}

func (m *mockProxyManager) ListRules() []*proxy.ProxyRule {
	return []*proxy.ProxyRule{}
}

func (m *mockProxyManager) IsEnabled() bool {
	return true
}

func (m *mockProxyManager) GetStats() map[string]interface{} {
	return map[string]interface{}{}
}

func (m *mockProxyManager) RestoreFromStorage() error {
	return nil
}

func (m *mockProxyManager) CheckHealth() (bool, time.Duration, error) {
	return true, 5 * time.Millisecond, nil
}
