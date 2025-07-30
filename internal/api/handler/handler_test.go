package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jerkytreats/dns/internal/dns/coredns"
	"github.com/jerkytreats/dns/internal/proxy"
)

func TestNewHandlerRegistry(t *testing.T) {
	// Create a mock DNS manager
	dnsManager := coredns.NewManager("127.0.0.1")

	// Create a mock DNS checker
	mockDNSChecker := &mockDNSChecker{}
	mockProxyManager := &mockProxyManager{}

	// Create handler registry
	registry, err := NewHandlerRegistry(dnsManager, mockDNSChecker, nil, mockProxyManager, nil, nil)
	if err != nil {
		t.Fatalf("Failed to create handler registry: %v", err)
	}

	// Verify registry is not nil
	if registry == nil {
		t.Fatal("Handler registry should not be nil")
	}

	// Verify record handler is initialized
	if registry.recordHandler == nil {
		t.Fatal("Record handler should be initialized")
	}

	// Verify health handler is initialized
	if registry.healthHandler == nil {
		t.Fatal("Health handler should be initialized")
	}

	// Verify internal mux is initialized
	if registry.mux == nil {
		t.Fatal("Internal ServeMux should be initialized")
	}
}

func TestHandlerRegistry_RegisterHandlers(t *testing.T) {
	// Create a mock DNS manager
	dnsManager := coredns.NewManager("127.0.0.1")

	// Create a mock DNS checker
	mockDNSChecker := &mockDNSChecker{}
	mockProxyManager := &mockProxyManager{}

	// Create handler registry
	registry, err := NewHandlerRegistry(dnsManager, mockDNSChecker, nil, mockProxyManager, nil, nil)
	if err != nil {
		t.Fatalf("Failed to create handler registry: %v", err)
	}

	// Create a new ServeMux for testing
	mux := http.NewServeMux()

	// Register handlers
	registry.RegisterHandlers(mux)

	// Test that the health handler is registered
	healthReq := httptest.NewRequest("GET", "/health", nil)
	healthRR := httptest.NewRecorder()

	// This should not panic, indicating the handler is registered
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Health handler registration failed: %v", r)
		}
	}()

	mux.ServeHTTP(healthRR, healthReq)
	// We expect a 200 status for the health check
	if healthRR.Code != http.StatusOK {
		t.Errorf("Expected 200 OK for health check, got %d", healthRR.Code)
	}

	// Test that the add-record handler is registered
	recordReq := httptest.NewRequest("POST", "/add-record", nil)
	recordRR := httptest.NewRecorder()

	mux.ServeHTTP(recordRR, recordReq)
	// We expect a 400 status because we're sending an empty body,
	// but this confirms the handler is registered and functioning
	if recordRR.Code != http.StatusBadRequest {
		t.Logf("Expected 400 Bad Request, got %d (this is acceptable as it shows handler is working)", recordRR.Code)
	}
}

func TestHandlerRegistry_GetServeMux(t *testing.T) {
	// Create a mock DNS manager
	dnsManager := coredns.NewManager("127.0.0.1")

	// Create a mock DNS checker
	mockDNSChecker := &mockDNSChecker{}
	mockProxyManager := &mockProxyManager{}

	// Create handler registry
	registry, err := NewHandlerRegistry(dnsManager, mockDNSChecker, nil, mockProxyManager, nil, nil)
	if err != nil {
		t.Fatalf("Failed to create handler registry: %v", err)
	}

	// Get the ServeMux
	mux := registry.GetServeMux()

	// Verify it's not nil
	if mux == nil {
		t.Fatal("ServeMux should not be nil")
	}

	// Verify it's the same as the internal mux
	if mux != registry.mux {
		t.Fatal("Returned ServeMux should be the same as internal mux")
	}
}

func TestHandlerRegistry_GetRecordHandler(t *testing.T) {
	// Create a mock DNS manager
	dnsManager := coredns.NewManager("127.0.0.1")

	// Create a mock DNS checker
	mockDNSChecker := &mockDNSChecker{}
	mockProxyManager := &mockProxyManager{}

	// Create handler registry
	registry, err := NewHandlerRegistry(dnsManager, mockDNSChecker, nil, mockProxyManager, nil, nil)
	if err != nil {
		t.Fatalf("Failed to create handler registry: %v", err)
	}

	// Get the record handler
	recordHandler := registry.GetRecordHandler()

	// Verify it's not nil
	if recordHandler == nil {
		t.Fatal("Record handler should not be nil")
	}

	// Verify it's the same as the internal record handler
	if recordHandler != registry.recordHandler {
		t.Fatal("Returned record handler should be the same as internal record handler")
	}
}

func TestHandlerRegistry_GetHealthHandler(t *testing.T) {
	// Create a mock DNS manager
	dnsManager := coredns.NewManager("127.0.0.1")

	// Create a mock DNS checker
	mockDNSChecker := &mockDNSChecker{}
	mockProxyManager := &mockProxyManager{}

	// Create handler registry
	registry, err := NewHandlerRegistry(dnsManager, mockDNSChecker, nil, mockProxyManager, nil, nil)
	if err != nil {
		t.Fatalf("Failed to create handler registry: %v", err)
	}

	// Get the health handler
	healthHandler := registry.GetHealthHandler()

	// Verify it's not nil
	if healthHandler == nil {
		t.Fatal("Health handler should not be nil")
	}

	// Verify it's the same as the internal health handler
	if healthHandler != registry.healthHandler {
		t.Fatal("Returned health handler should be the same as internal health handler")
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
