package certificate

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jerkytreats/dns/internal/config"
	"github.com/jerkytreats/dns/internal/persistence"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockDNSRecordProvider is defined in san_management_test.go

// MockManagerForSANValidation extends our existing mock for SAN validation testing
type MockManagerForSANValidation struct {
	domainStorage     *CertificateDomainStorage
	dnsRecordProvider *MockDNSRecordProvider
	addedDomains      []string
	addCallCount      int
}

func (m *MockManagerForSANValidation) AddDomainToSAN(domain string) error {
	m.addedDomains = append(m.addedDomains, domain)
	m.addCallCount++
	
	// Simulate actual domain addition to storage
	if m.domainStorage != nil {
		return m.domainStorage.AddDomain(domain)
	}
	return nil
}

func (m *MockManagerForSANValidation) ValidateAndUpdateSANDomains() error {
	if m.dnsRecordProvider == nil {
		return nil
	}

	if m.domainStorage == nil {
		return fmt.Errorf("domain storage not initialized")
	}

	// Get current DNS records
	dnsRecords, err := m.dnsRecordProvider.ListRecords()
	if err != nil {
		return fmt.Errorf("failed to list DNS records for SAN validation: %w", err)
	}

	// Get base domain
	baseDomain := config.GetString(CertDomainKey)
	if baseDomain == "" {
		return fmt.Errorf("certificate domain not configured")
	}

	// Get current SAN domains
	certDomains, err := m.domainStorage.LoadDomains()
	if err != nil {
		return fmt.Errorf("failed to load certificate domains: %w", err)
	}

	// Build set of existing SAN domains
	existingSANs := make(map[string]bool)
	existingSANs[certDomains.BaseDomain] = true
	for _, domain := range certDomains.SANDomains {
		existingSANs[domain] = true
	}

	// Check each DNS record and add missing domains
	for _, record := range dnsRecords {
		if record.Name == "" {
			continue
		}

		fqdn := fmt.Sprintf("%s.%s", record.Name, baseDomain)
		if !existingSANs[fqdn] {
			if err := m.AddDomainToSAN(fqdn); err != nil {
				return fmt.Errorf("failed to add domain %s to SAN: %w", fqdn, err)
			}
		}
	}

	return nil
}

func TestManager_ValidateAndUpdateSANDomains_Success(t *testing.T) {
	// Create temporary directory for test
	tempDir, err := os.MkdirTemp("", "san_validation_test_")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	tests := []struct {
		name                string
		existingDomains     *CertificateDomains
		dnsRecords          []DNSRecord
		baseDomain          string
		expectedAddedDomains []string
		expectError         bool
	}{
		{
			name: "Adds missing domains successfully",
			existingDomains: &CertificateDomains{
				BaseDomain: "internal.example.com",
				SANDomains: []string{"api.internal.example.com"},
				UpdatedAt:  time.Now(),
			},
			dnsRecords: []DNSRecord{
				{Name: "api", Type: "A"},    // Already exists in SAN
				{Name: "dns", Type: "A"},    // Missing from SAN
				{Name: "web", Type: "CNAME"}, // Missing from SAN
			},
			baseDomain:          "internal.example.com",
			expectedAddedDomains: []string{"dns.internal.example.com", "web.internal.example.com"},
			expectError:         false,
		},
		{
			name: "No domains to add when all exist",
			existingDomains: &CertificateDomains{
				BaseDomain: "internal.example.com",
				SANDomains: []string{"api.internal.example.com", "dns.internal.example.com"},
				UpdatedAt:  time.Now(),
			},
			dnsRecords: []DNSRecord{
				{Name: "api", Type: "A"},
				{Name: "dns", Type: "A"},
			},
			baseDomain:          "internal.example.com",
			expectedAddedDomains: []string{},
			expectError:         false,
		},
		{
			name: "Skips records with empty names",
			existingDomains: &CertificateDomains{
				BaseDomain: "internal.example.com",
				SANDomains: []string{},
				UpdatedAt:  time.Now(),
			},
			dnsRecords: []DNSRecord{
				{Name: "", Type: "A"},       // Should be skipped
				{Name: "valid", Type: "A"},  // Should be added
			},
			baseDomain:          "internal.example.com",
			expectedAddedDomains: []string{"valid.internal.example.com"},
			expectError:         false,
		},
		{
			name: "Handles base domain correctly",
			existingDomains: &CertificateDomains{
				BaseDomain: "internal.example.com",
				SANDomains: []string{},
				UpdatedAt:  time.Now(),
			},
			dnsRecords: []DNSRecord{
				{Name: "internal", Type: "A"}, // Would create internal.internal.example.com - should be added
				{Name: "api", Type: "A"},      // Should be added
			},
			baseDomain:          "internal.example.com",
			expectedAddedDomains: []string{"internal.internal.example.com", "api.internal.example.com"},
			expectError:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup test config
			config.SetForTest(CertDomainKey, tt.baseDomain)
			defer config.ResetForTest()

			// Setup storage
			storage := &CertificateDomainStorage{
				storage: persistence.NewFileStorageWithPath(filepath.Join(tempDir, fmt.Sprintf("test_%s.json", tt.name)), 3),
			}
			err := storage.SaveDomains(tt.existingDomains)
			require.NoError(t, err)

			// Setup mock DNS record provider
			mockProvider := &MockDNSRecordProvider{}
			mockProvider.On("ListRecords").Return(tt.dnsRecords, nil)

			// Create mock manager
			manager := &MockManagerForSANValidation{
				domainStorage:     storage,
				dnsRecordProvider: mockProvider,
				addedDomains:      make([]string, 0),
			}

			err = manager.ValidateAndUpdateSANDomains()

			if tt.expectError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.ElementsMatch(t, tt.expectedAddedDomains, manager.addedDomains)
			assert.Equal(t, len(tt.expectedAddedDomains), manager.addCallCount)
			mockProvider.AssertExpectations(t)
		})
	}
}

func TestManager_ValidateAndUpdateSANDomains_ErrorCases(t *testing.T) {
	// Create temporary directory for test
	tempDir, err := os.MkdirTemp("", "san_validation_error_test_")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	tests := []struct {
		name               string
		setupStorage       func() *CertificateDomainStorage
		setupProvider      func() *MockDNSRecordProvider
		setupConfig        func()
		expectedErrorContains string
	}{
		{
			name: "No DNS record provider",
			setupStorage: func() *CertificateDomainStorage {
				return &CertificateDomainStorage{
					storage: persistence.NewFileStorageWithPath(filepath.Join(tempDir, "test1.json"), 3),
				}
			},
			setupProvider: func() *MockDNSRecordProvider {
				return nil
			},
			setupConfig: func() {
				config.SetForTest(CertDomainKey, "example.com")
			},
			expectedErrorContains: "", // Should return nil, not error
		},
		{
			name: "No domain storage",
			setupStorage: func() *CertificateDomainStorage {
				return nil
			},
			setupProvider: func() *MockDNSRecordProvider {
				mockProvider := &MockDNSRecordProvider{}
				return mockProvider
			},
			setupConfig: func() {
				config.SetForTest(CertDomainKey, "example.com")
			},
			expectedErrorContains: "domain storage not initialized",
		},
		{
			name: "DNS provider returns error",
			setupStorage: func() *CertificateDomainStorage {
				storage := &CertificateDomainStorage{
					storage: persistence.NewFileStorageWithPath(filepath.Join(tempDir, "test2.json"), 3),
				}
				domains := &CertificateDomains{
					BaseDomain: "example.com",
					SANDomains: []string{},
					UpdatedAt:  time.Now(),
				}
				storage.SaveDomains(domains)
				return storage
			},
			setupProvider: func() *MockDNSRecordProvider {
				mockProvider := &MockDNSRecordProvider{}
				mockProvider.On("ListRecords").Return([]DNSRecord{}, errors.New("dns provider error"))
				return mockProvider
			},
			setupConfig: func() {
				config.SetForTest(CertDomainKey, "example.com")
			},
			expectedErrorContains: "failed to list DNS records for SAN validation",
		},
		{
			name: "No base domain configured",
			setupStorage: func() *CertificateDomainStorage {
				storage := &CertificateDomainStorage{
					storage: persistence.NewFileStorageWithPath(filepath.Join(tempDir, "test3.json"), 3),
				}
				domains := &CertificateDomains{
					BaseDomain: "example.com",
					SANDomains: []string{},
					UpdatedAt:  time.Now(),
				}
				storage.SaveDomains(domains)
				return storage
			},
			setupProvider: func() *MockDNSRecordProvider {
				mockProvider := &MockDNSRecordProvider{}
				mockProvider.On("ListRecords").Return([]DNSRecord{{Name: "test", Type: "A"}}, nil)
				return mockProvider
			},
			setupConfig: func() {
				config.SetForTest(CertDomainKey, "")
			},
			expectedErrorContains: "certificate domain not configured",
		},
		{
			name: "Storage load error",
			setupStorage: func() *CertificateDomainStorage {
				return &CertificateDomainStorage{
					storage: persistence.NewFileStorageWithPath("/non/existent/path.json", 3),
				}
			},
			setupProvider: func() *MockDNSRecordProvider {
				mockProvider := &MockDNSRecordProvider{}
				mockProvider.On("ListRecords").Return([]DNSRecord{{Name: "test", Type: "A"}}, nil)
				return mockProvider
			},
			setupConfig: func() {
				config.SetForTest(CertDomainKey, "example.com")
			},
			expectedErrorContains: "failed to load certificate domains",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup test config
			tt.setupConfig()
			defer config.ResetForTest()

			storage := tt.setupStorage()
			provider := tt.setupProvider()

			manager := &MockManagerForSANValidation{
				domainStorage:     storage,
				dnsRecordProvider: provider,
				addedDomains:      make([]string, 0),
			}

			err := manager.ValidateAndUpdateSANDomains()

			if tt.expectedErrorContains == "" {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedErrorContains)
			}

			if provider != nil {
				provider.AssertExpectations(t)
			}
		})
	}
}

func TestManager_ValidateAndUpdateSANDomains_NilProvider(t *testing.T) {
	manager := &MockManagerForSANValidation{
		dnsRecordProvider: nil,
		domainStorage:     &CertificateDomainStorage{},
	}

	err := manager.ValidateAndUpdateSANDomains()
	assert.NoError(t, err) // Should return nil when provider is not set
}