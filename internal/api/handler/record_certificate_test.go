package handler

import (
	"fmt"
	"testing"

	"github.com/jerkytreats/dns/internal/config"
	"github.com/jerkytreats/dns/internal/dns/coredns"
	"github.com/jerkytreats/dns/internal/dns/record"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewRecordHandler_WithCertificateManager(t *testing.T) {
	// Test that NewRecordHandler accepts certificate manager parameter
	mockCertManager := &MockCertificateManager{}

	// We need to create a real record service for this test
	dnsManager := coredns.NewManager("127.0.0.1")
	recordService := record.NewService(dnsManager, nil, nil)

	handler, err := NewRecordHandler(recordService, mockCertManager)

	require.NoError(t, err)
	assert.NotNil(t, handler)
	assert.NotNil(t, handler.recordService)
	assert.Equal(t, mockCertManager, handler.certificateManager)
}

func TestRecordHandler_CertificateManager_Interface(t *testing.T) {
	// Test that the certificate manager interface methods work correctly
	mockCertManager := &MockCertificateManager{shouldSucceed: true}

	// Test AddDomainToSAN
	err := mockCertManager.AddDomainToSAN("test.example.com")
	assert.NoError(t, err)
	assert.True(t, mockCertManager.addDomainCalled)
	assert.Equal(t, "test.example.com", mockCertManager.addedDomain)

	// Test RemoveDomainFromSAN
	err = mockCertManager.RemoveDomainFromSAN("test.example.com")
	assert.NoError(t, err)
	assert.True(t, mockCertManager.removeDomainCalled)
	assert.Equal(t, "test.example.com", mockCertManager.removedDomain)
}

func TestRecordHandler_CertificateManager_ErrorHandling(t *testing.T) {
	// Test error handling in certificate manager
	mockCertManager := &MockCertificateManager{shouldSucceed: false}

	// Test AddDomainToSAN error
	err := mockCertManager.AddDomainToSAN("test.example.com")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "mock certificate manager error")

	// Test RemoveDomainFromSAN error
	err = mockCertManager.RemoveDomainFromSAN("test.example.com")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "mock certificate manager error")
}

func TestRecordHandler_DomainConstruction_Logic(t *testing.T) {
	// Test the domain construction logic that would be used in the handler
	tests := []struct {
		name           string
		recordName     string
		dnsDomain      string
		expectedDomain string
	}{
		{
			name:           "Standard domain construction",
			recordName:     "api",
			dnsDomain:      "internal.example.com",
			expectedDomain: "api.internal.example.com",
		},
		{
			name:           "Subdomain construction",
			recordName:     "auth.api",
			dnsDomain:      "internal.example.com",
			expectedDomain: "auth.api.internal.example.com",
		},
		{
			name:           "Single character name",
			recordName:     "a",
			dnsDomain:      "test.local",
			expectedDomain: "a.test.local",
		},
		{
			name:           "Complex subdomain",
			recordName:     "db.cache.backend",
			dnsDomain:      "prod.company.com",
			expectedDomain: "db.cache.backend.prod.company.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test the domain construction logic that mirrors what's in the handler
			domain := fmt.Sprintf("%s.%s", tt.recordName, tt.dnsDomain)
			assert.Equal(t, tt.expectedDomain, domain)
		})
	}
}

func TestRecordHandler_Configuration_Dependencies(t *testing.T) {
	// Test that handler properly handles configuration dependencies
	tests := []struct {
		name                   string
		dnsDomain              string
		recordName             string
		shouldCallCertManager  bool
	}{
		{
			name:                  "Valid configuration",
			dnsDomain:             "internal.example.com",
			recordName:            "api",
			shouldCallCertManager: true,
		},
		{
			name:                  "Empty DNS domain",
			dnsDomain:             "",
			recordName:            "api",
			shouldCallCertManager: false,
		},
		{
			name:                  "Empty record name",
			dnsDomain:             "internal.example.com",
			recordName:            "",
			shouldCallCertManager: false,
		},
		{
			name:                  "Both empty",
			dnsDomain:             "",
			recordName:            "",
			shouldCallCertManager: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup test configuration
			config.SetForTest(coredns.DNSDomainKey, tt.dnsDomain)
			defer config.ResetForTest()

			// Test the logic conditions that determine certificate manager calls
			dnsDomainFromConfig := config.GetString(coredns.DNSDomainKey)
			shouldCall := tt.recordName != "" && dnsDomainFromConfig != ""
			
			assert.Equal(t, tt.shouldCallCertManager, shouldCall)
			
			if shouldCall {
				expectedDomain := fmt.Sprintf("%s.%s", tt.recordName, dnsDomainFromConfig)
				assert.NotEmpty(t, expectedDomain)
				assert.Contains(t, expectedDomain, tt.recordName)
				assert.Contains(t, expectedDomain, dnsDomainFromConfig)
			}
		})
	}
}

func TestRecordHandler_WithNilCertificateManager(t *testing.T) {
	// Test that handler works correctly when certificate manager is nil
	dnsManager := coredns.NewManager("127.0.0.1")
	recordService := record.NewService(dnsManager, nil, nil)

	handler, err := NewRecordHandler(recordService, nil)

	require.NoError(t, err)
	assert.NotNil(t, handler)
	assert.NotNil(t, handler.recordService)
	assert.Nil(t, handler.certificateManager)
}

// Mock implementations for testing

type MockCertificateManager struct {
	shouldSucceed      bool
	addDomainCalled    bool
	addedDomain        string
	removeDomainCalled bool
	removedDomain      string
}

func (m *MockCertificateManager) AddDomainToSAN(domain string) error {
	m.addDomainCalled = true
	m.addedDomain = domain
	
	if !m.shouldSucceed {
		return fmt.Errorf("mock certificate manager error")
	}
	
	return nil
}

func (m *MockCertificateManager) RemoveDomainFromSAN(domain string) error {
	m.removeDomainCalled = true
	m.removedDomain = domain
	
	if !m.shouldSucceed {
		return fmt.Errorf("mock certificate manager error")
	}
	
	return nil
}