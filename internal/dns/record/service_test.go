package record

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jerkytreats/dns/internal/config"
	"github.com/jerkytreats/dns/internal/dns/coredns"
	"github.com/jerkytreats/dns/internal/proxy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func setupTestConfig() {
	// Mock the config package for DNS domain
	config.SetForTest(coredns.DNSDomainKey, "internal")
}

// Test for NewService
func TestNewService(t *testing.T) {
	// Arrange
	mockDNSManager := new(MockDNSManager)
	mockProxyManager := new(MockProxyManager)
	mockTailscaleClient := new(MockTailscaleClient)

	// Act
	service := NewService(mockDNSManager, mockProxyManager, mockTailscaleClient)

	// Assert
	assert.NotNil(t, service)
	assert.Same(t, mockDNSManager, service.dnsManager)
	assert.Same(t, mockProxyManager, service.proxyManager)
	assert.Same(t, mockTailscaleClient, service.tailscaleClient)
	assert.NotNil(t, service.generator)
	assert.NotNil(t, service.validator)
}

// Test for CreateRecord
func TestCreateRecord(t *testing.T) {
	// Setup test config for DNS domain
	setupTestConfig()
	t.Run("successful creation without proxy", func(t *testing.T) {
		// Arrange
		mockDNSManager := new(MockDNSManager)
		mockProxyManager := new(MockProxyManager)
		mockTailscaleClient := new(MockTailscaleClient)

		service := NewService(mockDNSManager, mockProxyManager, mockTailscaleClient)

		req := CreateRecordRequest{
			ServiceName: "internal",
			Name:        "testrecord",
		}

		mockTailscaleClient.On("GetCurrentDeviceIP").Return("100.64.0.1", nil)
		mockDNSManager.On("AddRecord", req.ServiceName, req.Name, "100.64.0.1").Return(nil)

		// Act
		record, err := service.CreateRecord(req)

		// Assert
		assert.NoError(t, err)
		assert.NotNil(t, record)
		assert.Equal(t, req.Name, record.Name)
		assert.Equal(t, "A", record.Type)
		assert.Equal(t, "100.64.0.1", record.IP)
		assert.Nil(t, record.ProxyRule)

		mockTailscaleClient.AssertExpectations(t)
		mockDNSManager.AssertExpectations(t)
		mockProxyManager.AssertNotCalled(t, "AddRule")
	})

	t.Run("successful creation with proxy", func(t *testing.T) {
		// Arrange
		mockDNSManager := new(MockDNSManager)
		mockProxyManager := new(MockProxyManager)
		mockTailscaleClient := new(MockTailscaleClient)

		service := NewService(mockDNSManager, mockProxyManager, mockTailscaleClient)

		port := 8080
		req := CreateRecordRequest{
			ServiceName: "internal",
			Name:        "testrecord",
			Port:        &port,
		}

		mockTailscaleClient.On("GetCurrentDeviceIP").Return("100.64.0.1", nil)
		mockDNSManager.On("AddRecord", req.ServiceName, req.Name, "100.64.0.1").Return(nil)
		mockProxyManager.On("IsEnabled").Return(true)
		mockProxyManager.On("AddRule", mock.MatchedBy(func(rule *proxy.ProxyRule) bool {
			return rule != nil && rule.Hostname == "testrecord.internal"
		})).Return(nil)

		// Act
		record, err := service.CreateRecord(req)

		// Assert
		assert.NoError(t, err)
		assert.NotNil(t, record)
		assert.Equal(t, req.Name, record.Name)
		assert.Equal(t, "A", record.Type)
		assert.Equal(t, "100.64.0.1", record.IP)
		assert.NotNil(t, record.ProxyRule)

		mockTailscaleClient.AssertExpectations(t)
		mockDNSManager.AssertExpectations(t)
		mockProxyManager.AssertExpectations(t)
	})

	t.Run("validation error", func(t *testing.T) {
		// Arrange
		mockDNSManager := new(MockDNSManager)
		mockProxyManager := new(MockProxyManager)
		mockTailscaleClient := new(MockTailscaleClient)

		service := NewService(mockDNSManager, mockProxyManager, mockTailscaleClient)

		req := CreateRecordRequest{
			// Missing required fields
		}

		// Act
		record, err := service.CreateRecord(req)

		// Assert
		assert.Error(t, err)
		assert.Nil(t, record)
		assert.Contains(t, err.Error(), "normalization failed")

		mockTailscaleClient.AssertNotCalled(t, "GetCurrentDeviceIP")
		mockDNSManager.AssertNotCalled(t, "AddRecord")
	})

	t.Run("tailscale error", func(t *testing.T) {
		// Arrange
		mockDNSManager := new(MockDNSManager)
		mockProxyManager := new(MockProxyManager)
		mockTailscaleClient := new(MockTailscaleClient)

		service := NewService(mockDNSManager, mockProxyManager, mockTailscaleClient)

		req := CreateRecordRequest{
			ServiceName: "internal",
			Name:        "testrecord",
		}

		mockTailscaleClient.On("GetCurrentDeviceIP").Return("", errors.New("tailscale error"))

		// Act
		record, err := service.CreateRecord(req)

		// Assert
		assert.Error(t, err)
		assert.Nil(t, record)
		assert.Contains(t, err.Error(), "failed to get DNS Manager IP")

		mockTailscaleClient.AssertExpectations(t)
		mockDNSManager.AssertNotCalled(t, "AddRecord")
	})

	t.Run("dns manager error", func(t *testing.T) {
		// Arrange
		mockDNSManager := new(MockDNSManager)
		mockProxyManager := new(MockProxyManager)
		mockTailscaleClient := new(MockTailscaleClient)

		service := NewService(mockDNSManager, mockProxyManager, mockTailscaleClient)

		req := CreateRecordRequest{
			ServiceName: "internal",
			Name:        "testrecord",
		}

		mockTailscaleClient.On("GetCurrentDeviceIP").Return("100.64.0.1", nil)
		mockDNSManager.On("AddRecord", req.ServiceName, req.Name, "100.64.0.1").Return(errors.New("dns error"))

		// Act
		record, err := service.CreateRecord(req)

		// Assert
		assert.Error(t, err)
		assert.Nil(t, record)
		assert.Contains(t, err.Error(), "failed to add DNS record")

		mockTailscaleClient.AssertExpectations(t)
		mockDNSManager.AssertExpectations(t)
	})

	t.Run("proxy manager error", func(t *testing.T) {
		// Arrange
		mockDNSManager := new(MockDNSManager)
		mockProxyManager := new(MockProxyManager)
		mockTailscaleClient := new(MockTailscaleClient)

		service := NewService(mockDNSManager, mockProxyManager, mockTailscaleClient)

		port := 8080
		req := CreateRecordRequest{
			ServiceName: "internal",
			Name:        "testrecord",
			Port:        &port,
		}

		mockTailscaleClient.On("GetCurrentDeviceIP").Return("100.64.0.1", nil)
		mockDNSManager.On("AddRecord", req.ServiceName, req.Name, "100.64.0.1").Return(nil)
		mockProxyManager.On("IsEnabled").Return(true)
		mockProxyManager.On("AddRule", mock.AnythingOfType("*proxy.ProxyRule")).Return(errors.New("proxy error"))

		// Act
		record, err := service.CreateRecord(req)

		// Assert
		assert.NoError(t, err) // Operation should still succeed even if proxy creation fails
		assert.NotNil(t, record)
		assert.Equal(t, req.Name, record.Name)
		assert.Equal(t, "A", record.Type)
		assert.Equal(t, "100.64.0.1", record.IP)
		assert.Nil(t, record.ProxyRule) // Proxy rule should not be set due to error

		mockTailscaleClient.AssertExpectations(t)
		mockDNSManager.AssertExpectations(t)
		mockProxyManager.AssertExpectations(t)
	})
}

// Test for CreateRecord with device detection
func TestCreateRecordWithDeviceDetection(t *testing.T) {
	// Setup test config for DNS domain
	setupTestConfig()
	t.Run("successful creation with source IP", func(t *testing.T) {
		// Arrange
		mockDNSManager := new(MockDNSManager)
		mockProxyManager := new(MockProxyManager)
		mockTailscaleClient := new(MockTailscaleClient)

		service := NewService(mockDNSManager, mockProxyManager, mockTailscaleClient)

		port := 8080
		req := CreateRecordRequest{
			ServiceName: "internal",
			Name:        "testrecord",
			Port:        &port,
		}

		// Create test HTTP request with source IP
		httpReq := httptest.NewRequest(http.MethodPost, "/records", nil)
		httpReq.RemoteAddr = "192.168.1.10:45678"

		mockTailscaleClient.On("GetCurrentDeviceIP").Return("100.64.0.1", nil)
		mockDNSManager.On("AddRecord", req.ServiceName, req.Name, "100.64.0.1").Return(nil)
		mockProxyManager.On("IsEnabled").Return(true)
		// With the new implementation, we use device detection instead of source IP
		mockProxyManager.On("AddRule", mock.MatchedBy(func(rule *proxy.ProxyRule) bool {
			return rule != nil && rule.Hostname == "testrecord.internal" && rule.TargetIP == "100.64.0.1"
		})).Return(nil)

		// Act
		record, err := service.CreateRecord(req, httpReq)

		// Assert
		assert.NoError(t, err)
		assert.NotNil(t, record)
		assert.Equal(t, req.Name, record.Name)
		assert.Equal(t, "A", record.Type)
		assert.Equal(t, "100.64.0.1", record.IP)
		assert.NotNil(t, record.ProxyRule)

		mockTailscaleClient.AssertExpectations(t)
		mockDNSManager.AssertExpectations(t)
		mockProxyManager.AssertExpectations(t)
	})

	t.Run("device detection success", func(t *testing.T) {
		// Arrange
		mockDNSManager := new(MockDNSManager)
		mockProxyManager := new(MockProxyManager)
		mockTailscaleClient := new(MockTailscaleClient)

		service := NewService(mockDNSManager, mockProxyManager, mockTailscaleClient)

		port := 8080
		req := CreateRecordRequest{
			ServiceName: "internal",
			Name:        "testrecord",
			Port:        &port,
		}

		// Create test HTTP request with source IP
		httpReq := httptest.NewRequest(http.MethodPost, "/records", nil)
		httpReq.RemoteAddr = "192.168.1.10:45678"

		mockTailscaleClient.On("GetCurrentDeviceIP").Return("100.64.0.1", nil)
		mockDNSManager.On("AddRecord", req.ServiceName, req.Name, "100.64.0.1").Return(nil)
		mockProxyManager.On("IsEnabled").Return(true)
		// With the new implementation, we use device detection instead of source IP
		mockProxyManager.On("AddRule", mock.MatchedBy(func(rule *proxy.ProxyRule) bool {
			return rule != nil && rule.Hostname == "testrecord.internal" && rule.TargetIP == "100.64.0.1"
		})).Return(nil)

		// Act
		record, err := service.CreateRecord(req, httpReq)

		// Assert
		assert.NoError(t, err)
		assert.NotNil(t, record)
		assert.Equal(t, req.Name, record.Name)
		assert.Equal(t, "A", record.Type)
		assert.Equal(t, "100.64.0.1", record.IP)
		assert.NotNil(t, record.ProxyRule) // Changed from assert.Nil to assert.NotNil

		mockTailscaleClient.AssertExpectations(t)
		mockDNSManager.AssertExpectations(t)
		mockProxyManager.AssertExpectations(t)
	})
}

// Test for ListRecords
func TestListRecords(t *testing.T) {
	t.Run("successful listing", func(t *testing.T) {
		// Arrange
		mockDNSManager := new(MockDNSManager)
		mockProxyManager := new(MockProxyManager)
		mockTailscaleClient := new(MockTailscaleClient)

		// Create a mock generator that will be injected into the service
		mockGenerator := new(MockGenerator)

		service := NewService(mockDNSManager, mockProxyManager, mockTailscaleClient)
		// Replace the generator with our mock
		service.generator = mockGenerator

		expectedRecords := []Record{
			{Name: "record1", Type: "A", IP: "100.64.0.1"},
			{Name: "record2", Type: "A", IP: "100.64.0.2"},
		}

		mockGenerator.On("GenerateRecords").Return(expectedRecords, nil)

		// Act
		records, err := service.ListRecords()

		// Assert
		assert.NoError(t, err)
		assert.Equal(t, expectedRecords, records)
		mockGenerator.AssertExpectations(t)
	})

	t.Run("generator error", func(t *testing.T) {
		// Arrange
		mockDNSManager := new(MockDNSManager)
		mockProxyManager := new(MockProxyManager)
		mockTailscaleClient := new(MockTailscaleClient)

		// Create a mock generator that will be injected into the service
		mockGenerator := new(MockGenerator)

		service := NewService(mockDNSManager, mockProxyManager, mockTailscaleClient)
		// Replace the generator with our mock
		service.generator = mockGenerator

		mockGenerator.On("GenerateRecords").Return([]Record{}, errors.New("generator error"))

		// Act
		records, err := service.ListRecords()

		// Assert
		assert.Error(t, err)
		assert.Empty(t, records)
		mockGenerator.AssertExpectations(t)
	})
}

// Test for RemoveRecord
func TestRemoveRecord(t *testing.T) {
	t.Run("successful removal without proxy", func(t *testing.T) {
		// Arrange
		mockDNSManager := new(MockDNSManager)
		mockProxyManager := new(MockProxyManager)
		mockTailscaleClient := new(MockTailscaleClient)

		service := NewService(mockDNSManager, mockProxyManager, mockTailscaleClient)

		req := RemoveRecordRequest{
			ServiceName: "internal",
			Name:        "testrecord",
		}

		mockDNSManager.On("RemoveRecord", req.ServiceName, req.Name).Return(nil)
		mockProxyManager.On("IsEnabled").Return(false)

		// Act
		err := service.RemoveRecord(req)

		// Assert
		assert.NoError(t, err)
		mockDNSManager.AssertExpectations(t)
		mockProxyManager.AssertExpectations(t)
	})

	t.Run("successful removal with proxy", func(t *testing.T) {
		// Arrange
		mockDNSManager := new(MockDNSManager)
		mockProxyManager := new(MockProxyManager)
		mockTailscaleClient := new(MockTailscaleClient)

		// Set up config for DNS domain
		config.SetForTest("dns.domain", "internal")
		defer config.SetForTest("dns.domain", "") // Reset after test

		service := NewService(mockDNSManager, mockProxyManager, mockTailscaleClient)

		req := RemoveRecordRequest{
			ServiceName: "internal",
			Name:        "testrecord",
		}

		mockDNSManager.On("RemoveRecord", req.ServiceName, req.Name).Return(nil)
		mockProxyManager.On("IsEnabled").Return(true)
		mockProxyManager.On("RemoveRule", "testrecord.internal").Return(nil)

		// Act
		err := service.RemoveRecord(req)

		// Assert
		assert.NoError(t, err)
		mockDNSManager.AssertExpectations(t)
		mockProxyManager.AssertExpectations(t)
	})

	t.Run("validation error", func(t *testing.T) {
		// Arrange
		mockDNSManager := new(MockDNSManager)
		mockProxyManager := new(MockProxyManager)
		mockTailscaleClient := new(MockTailscaleClient)

		service := NewService(mockDNSManager, mockProxyManager, mockTailscaleClient)

		req := RemoveRecordRequest{
			ServiceName: "", // Invalid - empty service name
			Name:        "testrecord",
		}

		// Act
		err := service.RemoveRecord(req)

		// Assert
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "validation failed")
		assert.Contains(t, err.Error(), "service_name is required")
	})

	t.Run("dns manager error", func(t *testing.T) {
		// Arrange
		mockDNSManager := new(MockDNSManager)
		mockProxyManager := new(MockProxyManager)
		mockTailscaleClient := new(MockTailscaleClient)

		service := NewService(mockDNSManager, mockProxyManager, mockTailscaleClient)

		req := RemoveRecordRequest{
			ServiceName: "internal",
			Name:        "testrecord",
		}

		mockDNSManager.On("RemoveRecord", req.ServiceName, req.Name).Return(errors.New("dns error"))

		// Act
		err := service.RemoveRecord(req)

		// Assert
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to remove DNS record")
		mockDNSManager.AssertExpectations(t)
	})

	t.Run("proxy manager error - should not fail removal", func(t *testing.T) {
		// Arrange
		mockDNSManager := new(MockDNSManager)
		mockProxyManager := new(MockProxyManager)
		mockTailscaleClient := new(MockTailscaleClient)

		// Set up config for DNS domain
		config.SetForTest("dns.domain", "internal")
		defer config.SetForTest("dns.domain", "") // Reset after test

		service := NewService(mockDNSManager, mockProxyManager, mockTailscaleClient)

		req := RemoveRecordRequest{
			ServiceName: "internal",
			Name:        "testrecord",
		}

		// DNS removal succeeds, but proxy removal fails
		mockDNSManager.On("RemoveRecord", req.ServiceName, req.Name).Return(nil)
		mockProxyManager.On("IsEnabled").Return(true)
		mockProxyManager.On("RemoveRule", "testrecord.internal").Return(errors.New("proxy error"))

		// Act - should still succeed despite proxy error (logged as warning)
		err := service.RemoveRecord(req)

		// Assert
		assert.NoError(t, err) // Should not fail despite proxy error
		mockDNSManager.AssertExpectations(t)
		mockProxyManager.AssertExpectations(t)
	})

	t.Run("nil proxy manager", func(t *testing.T) {
		// Arrange
		mockDNSManager := new(MockDNSManager)
		mockTailscaleClient := new(MockTailscaleClient)

		service := NewService(mockDNSManager, nil, mockTailscaleClient)

		req := RemoveRecordRequest{
			ServiceName: "internal",
			Name:        "testrecord",
		}

		mockDNSManager.On("RemoveRecord", req.ServiceName, req.Name).Return(nil)

		// Act
		err := service.RemoveRecord(req)

		// Assert
		assert.NoError(t, err)
		mockDNSManager.AssertExpectations(t)
	})

	t.Run("empty dns domain - proxy removal skipped", func(t *testing.T) {
		// Arrange
		mockDNSManager := new(MockDNSManager)
		mockProxyManager := new(MockProxyManager)
		mockTailscaleClient := new(MockTailscaleClient)

		service := NewService(mockDNSManager, mockProxyManager, mockTailscaleClient)

		req := RemoveRecordRequest{
			ServiceName: "internal",
			Name:        "testrecord",
		}

		// Set empty DNS domain in config (this would need config mocking in real scenario)
		mockDNSManager.On("RemoveRecord", req.ServiceName, req.Name).Return(nil)
		mockProxyManager.On("IsEnabled").Return(true)
		// No RemoveRule call expected because domain is empty

		// Act
		err := service.RemoveRecord(req)

		// Assert
		assert.NoError(t, err)
		mockDNSManager.AssertExpectations(t)
		mockProxyManager.AssertExpectations(t)
	})
}

// Test for CreateRecord with explicit target_device
func TestCreateRecordWithTargetDevice(t *testing.T) {
	setupTestConfig()

	t.Run("successful creation with explicit target_device", func(t *testing.T) {
		// Arrange
		mockDNSManager := new(MockDNSManager)
		mockProxyManager := new(MockProxyManager)
		mockTailscaleClient := new(MockTailscaleClient)

		service := NewService(mockDNSManager, mockProxyManager, mockTailscaleClient)

		port := 8000
		targetDevice := "leviathan"
		req := CreateRecordRequest{
			ServiceName:  "internal",
			Name:         "chat",
			Port:         &port,
			TargetDevice: &targetDevice,
		}

		// Create test HTTP request to trigger device detection path
		httpReq := httptest.NewRequest(http.MethodPost, "/add-record", nil)

		// DNS Manager IP lookup for DNS record (points to Anton)
		mockTailscaleClient.On("GetCurrentDeviceIP").Return("100.72.130.7", nil)
		mockDNSManager.On("AddRecord", "internal", "chat", "100.72.130.7").Return(nil)
		mockProxyManager.On("IsEnabled").Return(true)

		// Target device IP lookup for proxy rule (should resolve leviathan's IP)
		mockTailscaleClient.On("GetCurrentDeviceIPByName", "leviathan").Return("100.70.110.111", nil)

		// Proxy rule should use leviathan's IP, not Anton's
		mockProxyManager.On("AddRule", mock.MatchedBy(func(rule *proxy.ProxyRule) bool {
			return rule != nil &&
				rule.Hostname == "chat.internal" &&
				rule.TargetIP == "100.70.110.111" && // leviathan's IP
				rule.TargetPort == 8000
		})).Return(nil)

		// Act
		record, err := service.CreateRecord(req, httpReq)

		// Assert
		assert.NoError(t, err)
		assert.NotNil(t, record)
		assert.Equal(t, "chat", record.Name)
		assert.Equal(t, "A", record.Type)
		assert.Equal(t, "100.72.130.7", record.IP) // DNS record points to Anton (DNS Manager)
		assert.NotNil(t, record.ProxyRule)
		assert.Equal(t, "100.70.110.111", record.ProxyRule.TargetIP) // Proxy targets leviathan
		assert.Equal(t, 8000, record.ProxyRule.TargetPort)

		mockTailscaleClient.AssertExpectations(t)
		mockDNSManager.AssertExpectations(t)
		mockProxyManager.AssertExpectations(t)
	})

	t.Run("target_device takes priority over config device_name", func(t *testing.T) {
		// Arrange
		mockDNSManager := new(MockDNSManager)
		mockProxyManager := new(MockProxyManager)
		mockTailscaleClient := new(MockTailscaleClient)

		// Set a configured device name (e.g., "anton")
		config.SetForTest("tailscale.device_name", "anton")
		defer config.SetForTest("tailscale.device_name", "")

		service := NewService(mockDNSManager, mockProxyManager, mockTailscaleClient)

		port := 8000
		targetDevice := "leviathan" // This should override "anton" from config
		req := CreateRecordRequest{
			ServiceName:  "internal",
			Name:         "chat",
			Port:         &port,
			TargetDevice: &targetDevice,
		}

		httpReq := httptest.NewRequest(http.MethodPost, "/add-record", nil)

		// For DNS Manager IP, it uses configured device name "anton"
		mockTailscaleClient.On("GetCurrentDeviceIPByName", "anton").Return("100.72.130.7", nil)
		mockDNSManager.On("AddRecord", "internal", "chat", "100.72.130.7").Return(nil)
		mockProxyManager.On("IsEnabled").Return(true)

		// For proxy target, target_device "leviathan" should take priority over config
		mockTailscaleClient.On("GetCurrentDeviceIPByName", "leviathan").Return("100.70.110.111", nil)

		mockProxyManager.On("AddRule", mock.MatchedBy(func(rule *proxy.ProxyRule) bool {
			return rule != nil && rule.TargetIP == "100.70.110.111" // leviathan's IP, not anton's
		})).Return(nil)

		// Act
		record, err := service.CreateRecord(req, httpReq)

		// Assert
		assert.NoError(t, err)
		assert.NotNil(t, record)
		assert.NotNil(t, record.ProxyRule)
		assert.Equal(t, "100.70.110.111", record.ProxyRule.TargetIP)

		mockTailscaleClient.AssertExpectations(t)
		mockDNSManager.AssertExpectations(t)
		mockProxyManager.AssertExpectations(t)
	})
}
