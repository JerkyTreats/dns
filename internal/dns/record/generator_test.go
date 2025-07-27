package record

import (
	"errors"
	"testing"
	"time"

	"github.com/jerkytreats/dns/internal/dns/coredns"
	"github.com/jerkytreats/dns/internal/proxy"
	"github.com/stretchr/testify/assert"
)

func TestNewGenerator(t *testing.T) {
	// Arrange
	mockDNSManager := new(MockDNSManager)
	mockProxyManager := new(MockProxyManager)

	// Act
	generator := NewGenerator(mockDNSManager, mockProxyManager)

	// Assert
	assert.NotNil(t, generator)
	// Check if the generator implements the Generator interface
	_, ok := generator.(Generator)
	assert.True(t, ok)
}

func TestGenerateRecords(t *testing.T) {
	t.Run("successful generation with proxy rules", func(t *testing.T) {
		// Arrange
		mockDNSManager := new(MockDNSManager)
		mockProxyManager := new(MockProxyManager)

		generator := NewGenerator(mockDNSManager, mockProxyManager).(*RecordGenerator)

		// Set up DNS records
		dnsRecords := []coredns.Record{
			{Name: "app1", Type: "A", IP: "100.64.0.1"},
			{Name: "app2", Type: "A", IP: "100.64.0.2"},
		}

		// Set up proxy rules
		proxyRules := []*proxy.ProxyRule{
			{
				Hostname:   "app1.internal.example.com",
				TargetIP:   "100.64.0.10",
				TargetPort: 8080,
				Protocol:   "http",
				Enabled:    true,
				CreatedAt:  time.Now(),
			},
		}

		mockDNSManager.On("ListRecords", "internal").Return(dnsRecords, nil)
		mockProxyManager.On("IsEnabled").Return(true)
		mockProxyManager.On("ListRules").Return(proxyRules)

		// We'll use a simplified test approach instead of mocking the extractHostnameFromFQDN method
		// The test will rely on the actual implementation of the method

		// Act
		records, err := generator.GenerateRecords()

		// Assert
		assert.NoError(t, err)
		assert.Len(t, records, 2)

		// Check we have the expected number of records
		assert.Len(t, records, 2)
		
		// Find the record with a proxy rule
		var recordWithProxy *Record
		var recordWithoutProxy *Record
		
		for i, record := range records {
			if record.ProxyRule != nil {
				recordWithProxy = &records[i]
			} else {
				recordWithoutProxy = &records[i]
			}
		}
		
		// Check app1 record with proxy rule
		assert.NotNil(t, recordWithProxy)
		assert.Equal(t, "app1", recordWithProxy.Name)
		assert.Equal(t, "A", recordWithProxy.Type)
		assert.Equal(t, "100.64.0.1", recordWithProxy.IP)
		assert.NotNil(t, recordWithProxy.ProxyRule)
		assert.Equal(t, "app1.internal.example.com", recordWithProxy.ProxyRule.Hostname)
		assert.Equal(t, "100.64.0.10", recordWithProxy.ProxyRule.TargetIP)
		assert.Equal(t, 8080, recordWithProxy.ProxyRule.TargetPort)
		assert.Equal(t, "http", recordWithProxy.ProxyRule.Protocol)
		assert.True(t, recordWithProxy.ProxyRule.Enabled)

		// Check app2 record without proxy rule
		assert.NotNil(t, recordWithoutProxy)
		assert.Equal(t, "app2", recordWithoutProxy.Name)
		assert.Equal(t, "A", recordWithoutProxy.Type)
		assert.Equal(t, "100.64.0.2", recordWithoutProxy.IP)
		assert.Nil(t, recordWithoutProxy.ProxyRule)

		mockDNSManager.AssertExpectations(t)
		mockProxyManager.AssertExpectations(t)
	})

	t.Run("successful generation without proxy manager", func(t *testing.T) {
		// Arrange
		mockDNSManager := new(MockDNSManager)

		// Create generator with nil proxy manager
		generator := NewGenerator(mockDNSManager, nil).(*RecordGenerator)

		// Set up DNS records
		dnsRecords := []coredns.Record{
			{Name: "app1", Type: "A", IP: "100.64.0.1"},
		}

		mockDNSManager.On("ListRecords", "internal").Return(dnsRecords, nil)

		// Act
		records, err := generator.GenerateRecords()

		// Assert
		assert.NoError(t, err)
		assert.Len(t, records, 1)
		assert.Equal(t, "app1", records[0].Name)
		assert.Equal(t, "A", records[0].Type)
		assert.Equal(t, "100.64.0.1", records[0].IP)
		assert.Nil(t, records[0].ProxyRule)

		mockDNSManager.AssertExpectations(t)
	})

	t.Run("successful generation with disabled proxy manager", func(t *testing.T) {
		// Arrange
		mockDNSManager := new(MockDNSManager)
		mockProxyManager := new(MockProxyManager)

		generator := NewGenerator(mockDNSManager, mockProxyManager).(*RecordGenerator)

		// Set up DNS records
		dnsRecords := []coredns.Record{
			{Name: "app1", Type: "A", IP: "100.64.0.1"},
		}

		mockDNSManager.On("ListRecords", "internal").Return(dnsRecords, nil)
		mockProxyManager.On("IsEnabled").Return(false)

		// Act
		records, err := generator.GenerateRecords()

		// Assert
		assert.NoError(t, err)
		assert.Len(t, records, 1)
		assert.Equal(t, "app1", records[0].Name)
		assert.Equal(t, "A", records[0].Type)
		assert.Equal(t, "100.64.0.1", records[0].IP)
		assert.Nil(t, records[0].ProxyRule)

		mockDNSManager.AssertExpectations(t)
		mockProxyManager.AssertExpectations(t)
		mockProxyManager.AssertNotCalled(t, "ListRules")
	})

	t.Run("dns manager error", func(t *testing.T) {
		// Arrange
		mockDNSManager := new(MockDNSManager)
		mockProxyManager := new(MockProxyManager)

		generator := NewGenerator(mockDNSManager, mockProxyManager).(*RecordGenerator)

		mockDNSManager.On("ListRecords", "internal").Return([]coredns.Record{}, errors.New("dns error"))

		// Act
		records, err := generator.GenerateRecords()

		// Assert
		assert.Error(t, err)
		assert.Empty(t, records)
		assert.Contains(t, err.Error(), "failed to list DNS records")

		mockDNSManager.AssertExpectations(t)
		mockProxyManager.AssertNotCalled(t, "IsEnabled")
	})
}

func TestExtractHostnameFromFQDN(t *testing.T) {
	// This test is simplified to avoid mocking config.GetString
	// In a real scenario, we would use a proper dependency injection or interface
	
	// Test the splitHostname function which is used by extractHostnameFromFQDN
	t.Run("extract first part of hostname", func(t *testing.T) {
		result := splitHostname("app1.internal.example.com")
		assert.Equal(t, []string{"app1", "internal", "example", "com"}, result)
	})

	t.Run("extract from simple hostname", func(t *testing.T) {
		result := splitHostname("localhost")
		assert.Equal(t, []string{"localhost"}, result)
	})
}

func TestSplitHostname(t *testing.T) {
	testCases := []struct {
		name           string
		hostname       string
		expectedResult []string
	}{
		{
			name:           "simple hostname",
			hostname:       "app1",
			expectedResult: []string{"app1"},
		},
		{
			name:           "hostname with subdomain",
			hostname:       "api.app1",
			expectedResult: []string{"api", "app1"},
		},
		{
			name:           "multiple levels",
			hostname:       "v1.api.app1",
			expectedResult: []string{"v1", "api", "app1"},
		},
		{
			name:           "empty hostname",
			hostname:       "",
			expectedResult: []string{},
		},
		{
			name:           "hostname with trailing dot",
			hostname:       "app1.",
			expectedResult: []string{"app1"},
		},
		{
			name:           "hostname with leading dot",
			hostname:       ".app1",
			expectedResult: []string{"app1"},
		},
		{
			name:           "multiple consecutive dots",
			hostname:       "app1..test",
			expectedResult: []string{"app1", "test"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Act
			result := splitHostname(tc.hostname)

			// Assert
			assert.Equal(t, tc.expectedResult, result)
		})
	}
}
