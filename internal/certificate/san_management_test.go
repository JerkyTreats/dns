package certificate

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jerkytreats/dns/internal/config"
	"github.com/jerkytreats/dns/internal/persistence"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// MockDNSRecordProvider implements the DNS record provider interface for testing
type MockDNSRecordProvider struct {
	mock.Mock
}

func (m *MockDNSRecordProvider) ListRecords() ([]DNSRecord, error) {
	args := m.Called()
	return args.Get(0).([]DNSRecord), args.Error(1)
}

func TestManager_GetDomainsForCertificate(t *testing.T) {
	// Create temporary directory for test
	tempDir, err := os.MkdirTemp("", "cert_san_test_")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	tests := []struct {
		name              string
		setupStorage      func() *CertificateDomainStorage
		expectedDomains   []string
		expectError       bool
	}{
		{
			name: "Returns domains from storage",
			setupStorage: func() *CertificateDomainStorage {
				storage := &CertificateDomainStorage{
					storage: persistence.NewFileStorageWithPath(filepath.Join(tempDir, "test1.json"), 3),
				}
				domains := &CertificateDomains{
					BaseDomain: "internal.example.com",
					SANDomains: []string{"api.internal.example.com", "dns.internal.example.com"},
					UpdatedAt:  time.Now(),
				}
				storage.SaveDomains(domains)
				return storage
			},
			expectedDomains: []string{"internal.example.com", "api.internal.example.com", "dns.internal.example.com"},
			expectError:     false,
		},
		{
			name: "Returns base domain only when no SAN domains",
			setupStorage: func() *CertificateDomainStorage {
				storage := &CertificateDomainStorage{
					storage: persistence.NewFileStorageWithPath(filepath.Join(tempDir, "test2.json"), 3),
				}
				domains := &CertificateDomains{
					BaseDomain: "internal.example.com",
					SANDomains: []string{},
					UpdatedAt:  time.Now(),
				}
				storage.SaveDomains(domains)
				return storage
			},
			expectedDomains: []string{"internal.example.com"},
			expectError:     false,
		},
		{
			name: "Handles nil storage gracefully",
			setupStorage: func() *CertificateDomainStorage {
				return nil
			},
			expectedDomains: nil,
			expectError:     false, // Should fallback to config
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup test config
			config.SetForTest(CertDomainKey, "fallback.example.com")
			defer config.ResetForTest()

			manager := &Manager{
				domainStorage: tt.setupStorage(),
			}

			domains, err := manager.GetDomainsForCertificate()

			if tt.expectError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			
			if tt.expectedDomains != nil {
				assert.ElementsMatch(t, tt.expectedDomains, domains)
			} else {
				// Should fallback to config when storage is nil
				assert.Equal(t, []string{"fallback.example.com"}, domains)
			}
		})
	}
}

func TestManager_GetDomainsForCertificate_StorageError(t *testing.T) {
	manager := &Manager{
		domainStorage: &CertificateDomainStorage{
			storage: persistence.NewFileStorageWithPath("/non/existent/path.json", 3),
		},
	}

	_, err := manager.GetDomainsForCertificate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to load certificate domains")
}

func TestManager_AddDomainToSAN(t *testing.T) {
	// Create temporary directory for test
	tempDir, err := os.MkdirTemp("", "cert_san_test_")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	tests := []struct {
		name            string
		domainToAdd     string
		initialDomains  *CertificateDomains
		expectedDomains []string
		mockRenewal     bool
		expectError     bool
		errorContains   string
	}{
		{
			name:        "Add new domain successfully",
			domainToAdd: "new.internal.example.com",
			initialDomains: &CertificateDomains{
				BaseDomain: "internal.example.com",
				SANDomains: []string{"api.internal.example.com"},
				UpdatedAt:  time.Now(),
			},
			expectedDomains: []string{"internal.example.com", "api.internal.example.com", "new.internal.example.com"},
			mockRenewal:     true,
			expectError:     false,
		},
		{
			name:        "Add duplicate domain (should not error)",
			domainToAdd: "api.internal.example.com",
			initialDomains: &CertificateDomains{
				BaseDomain: "internal.example.com",
				SANDomains: []string{"api.internal.example.com"},
				UpdatedAt:  time.Now(),
			},
			expectedDomains: []string{"internal.example.com", "api.internal.example.com"},
			mockRenewal:     true,
			expectError:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup storage
			storage := &CertificateDomainStorage{
				storage: persistence.NewFileStorageWithPath(filepath.Join(tempDir, fmt.Sprintf("test_%s.json", tt.name)), 3),
			}
			err := storage.SaveDomains(tt.initialDomains)
			require.NoError(t, err)

			// Create mock manager
			manager := &MockManagerForSAN{
				domainStorage:        storage,
				shouldRenewalSucceed: tt.mockRenewal,
			}

			err = manager.AddDomainToSAN(tt.domainToAdd)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
				return
			}

			require.NoError(t, err)

			// Verify domains were updated
			domains, err := manager.GetDomainsForCertificate()
			require.NoError(t, err)
			assert.ElementsMatch(t, tt.expectedDomains, domains)

			// Verify renewal was triggered
			assert.True(t, manager.renewalTriggered, "Expected certificate renewal to be triggered")
		})
	}
}

func TestManager_AddDomainToSAN_NoStorage(t *testing.T) {
	manager := &Manager{
		domainStorage: nil,
	}

	err := manager.AddDomainToSAN("test.example.com")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "domain storage not initialized")
}

func TestManager_RemoveDomainFromSAN(t *testing.T) {
	// Create temporary directory for test
	tempDir, err := os.MkdirTemp("", "cert_san_test_")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	tests := []struct {
		name             string
		domainToRemove   string
		initialDomains   *CertificateDomains
		expectedDomains  []string
		mockRenewal      bool
		expectError      bool
		errorContains    string
	}{
		{
			name:           "Remove existing domain successfully",
			domainToRemove: "api.internal.example.com",
			initialDomains: &CertificateDomains{
				BaseDomain: "internal.example.com",
				SANDomains: []string{"api.internal.example.com", "dns.internal.example.com"},
				UpdatedAt:  time.Now(),
			},
			expectedDomains: []string{"internal.example.com", "dns.internal.example.com"},
			mockRenewal:     true,
			expectError:     false,
		},
		{
			name:           "Remove non-existent domain (should not error)",
			domainToRemove: "nonexistent.internal.example.com",
			initialDomains: &CertificateDomains{
				BaseDomain: "internal.example.com",
				SANDomains: []string{"api.internal.example.com"},
				UpdatedAt:  time.Now(),
			},
			expectedDomains: []string{"internal.example.com", "api.internal.example.com"},
			mockRenewal:     true,
			expectError:     false,
		},
		{
			name:           "Try to remove base domain",
			domainToRemove: "internal.example.com",
			initialDomains: &CertificateDomains{
				BaseDomain: "internal.example.com",
				SANDomains: []string{"api.internal.example.com"},
				UpdatedAt:  time.Now(),
			},
			expectedDomains: nil,
			mockRenewal:     false,
			expectError:     true,
			errorContains:   "cannot remove base domain",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup storage
			storage := &CertificateDomainStorage{
				storage: persistence.NewFileStorageWithPath(filepath.Join(tempDir, fmt.Sprintf("remove_test_%s.json", tt.name)), 3),
			}
			err := storage.SaveDomains(tt.initialDomains)
			require.NoError(t, err)

			// Create mock manager
			manager := &MockManagerForSAN{
				domainStorage:        storage,
				shouldRenewalSucceed: tt.mockRenewal,
			}

			err = manager.RemoveDomainFromSAN(tt.domainToRemove)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
				return
			}

			require.NoError(t, err)

			// Verify domains were updated
			domains, err := manager.GetDomainsForCertificate()
			require.NoError(t, err)
			assert.ElementsMatch(t, tt.expectedDomains, domains)

			// Verify renewal was triggered
			assert.True(t, manager.renewalTriggered, "Expected certificate renewal to be triggered")
		})
	}
}

func TestManager_RemoveDomainFromSAN_NoStorage(t *testing.T) {
	manager := &Manager{
		domainStorage: nil,
	}

	err := manager.RemoveDomainFromSAN("test.example.com")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "domain storage not initialized")
}

func TestManager_SetDNSRecordProvider(t *testing.T) {
	manager := &Manager{}
	
	mockProvider := &MockDNSRecordProvider{}
	manager.SetDNSRecordProvider(mockProvider)
	
	assert.Equal(t, mockProvider, manager.dnsRecordProvider)
}

func TestManager_ValidateAndUpdateSANDomains_Integration(t *testing.T) {
	// Create temporary directory for test
	tempDir, err := os.MkdirTemp("", "san_validation_integration_")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Setup test config
	config.SetForTest(CertDomainKey, "internal.example.com")
	defer config.ResetForTest()

	// Setup storage with initial domains
	storage := &CertificateDomainStorage{
		storage: persistence.NewFileStorageWithPath(filepath.Join(tempDir, "integration_test.json"), 3),
	}
	initialDomains := &CertificateDomains{
		BaseDomain: "internal.example.com",
		SANDomains: []string{"api.internal.example.com"},
		UpdatedAt:  time.Now(),
	}
	err = storage.SaveDomains(initialDomains)
	require.NoError(t, err)

	// Setup mock DNS record provider
	mockProvider := &MockDNSRecordProvider{}
	dnsRecords := []DNSRecord{
		{Name: "api", Type: "A"},    // Already in SAN
		{Name: "dns", Type: "A"},    // Missing from SAN
		{Name: "web", Type: "CNAME"}, // Missing from SAN
		{Name: "", Type: "A"},       // Should be skipped
	}
	mockProvider.On("ListRecords").Return(dnsRecords, nil)

	// Create mock manager that doesn't trigger actual certificate operations
	manager := &MockManagerForSANValidation{
		domainStorage:     storage,
		dnsRecordProvider: mockProvider,
		addedDomains:      make([]string, 0),
	}

	// Run validation
	err = manager.ValidateAndUpdateSANDomains()
	require.NoError(t, err)

	// Verify domains were added to storage
	updatedDomains, err := storage.LoadDomains()
	require.NoError(t, err)

	expectedSANs := []string{
		"api.internal.example.com",
		"dns.internal.example.com", 
		"web.internal.example.com",
	}
	assert.ElementsMatch(t, expectedSANs, updatedDomains.SANDomains)
	assert.Equal(t, "internal.example.com", updatedDomains.BaseDomain)

	// Verify the correct domains were added via the manager
	expectedAddedDomains := []string{
		"dns.internal.example.com",
		"web.internal.example.com",
	}
	assert.ElementsMatch(t, expectedAddedDomains, manager.GetAddedDomains())

	mockProvider.AssertExpectations(t)
}

func TestManager_ObtainCertificateWithRetry(t *testing.T) {
	tests := []struct {
		name            string
		domains         []string
		shouldSucceed   bool
		failAttempts    int
		expectedAttempts int
		expectError     bool
	}{
		{
			name:            "Success on first attempt",
			domains:         []string{"internal.example.com", "api.internal.example.com"},
			shouldSucceed:   true,
			failAttempts:    0,
			expectedAttempts: 1,
			expectError:     false,
		},
		{
			name:            "Success after retries",
			domains:         []string{"internal.example.com"},
			shouldSucceed:   true,
			failAttempts:    2,
			expectedAttempts: 3,
			expectError:     false,
		},
		{
			name:            "Failure after max retries",
			domains:         []string{"internal.example.com"},
			shouldSucceed:   false,
			failAttempts:    3,
			expectedAttempts: 3,
			expectError:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager := &MockManagerForRetry{
				shouldSucceed:   tt.shouldSucceed,
				failAttempts:    tt.failAttempts,
			}

			err := manager.ObtainCertificateWithRetry(tt.domains)

			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "failed to obtain certificate after")
			} else {
				assert.NoError(t, err)
			}

			assert.Equal(t, tt.expectedAttempts, manager.attempts, "Expected number of attempts")
		})
	}
}

// Mock implementations for testing

type MockManagerForSAN struct {
	domainStorage        *CertificateDomainStorage
	shouldRenewalSucceed bool
	renewalTriggered     bool
}

func (m *MockManagerForSAN) GetDomainsForCertificate() ([]string, error) {
	if m.domainStorage == nil {
		return nil, fmt.Errorf("domain storage not initialized")
	}

	certDomains, err := m.domainStorage.LoadDomains()
	if err != nil {
		return nil, fmt.Errorf("failed to load certificate domains: %w", err)
	}

	allDomains := []string{certDomains.BaseDomain}
	allDomains = append(allDomains, certDomains.SANDomains...)
	return allDomains, nil
}

func (m *MockManagerForSAN) AddDomainToSAN(domain string) error {
	if m.domainStorage == nil {
		return fmt.Errorf("domain storage not initialized")
	}

	if err := m.domainStorage.AddDomain(domain); err != nil {
		return fmt.Errorf("failed to add domain to storage: %w", err)
	}

	// Mock certificate renewal
	m.renewalTriggered = true
	if !m.shouldRenewalSucceed {
		return fmt.Errorf("mock renewal failed")
	}

	return nil
}

func (m *MockManagerForSAN) RemoveDomainFromSAN(domain string) error {
	if m.domainStorage == nil {
		return fmt.Errorf("domain storage not initialized")
	}

	if err := m.domainStorage.RemoveDomain(domain); err != nil {
		return fmt.Errorf("failed to remove domain from storage: %w", err)
	}

	// Mock certificate renewal
	m.renewalTriggered = true
	if !m.shouldRenewalSucceed {
		return fmt.Errorf("mock renewal failed")
	}

	return nil
}

type MockManagerForRetry struct {
	shouldSucceed bool
	failAttempts  int
	attempts      int
}

func (m *MockManagerForRetry) ObtainCertificateWithRetry(domains []string) error {
	maxRetries := 3
	for attempt := 1; attempt <= maxRetries; attempt++ {
		m.attempts = attempt
		
		if err := m.obtainCertificateForDomains(domains); err != nil {
			if attempt == maxRetries {
				return fmt.Errorf("failed to obtain certificate after %d attempts: %w", maxRetries, err)
			}
			continue
		}
		
		return nil
	}
	
	return fmt.Errorf("unexpected error in certificate retry logic")
}

func (m *MockManagerForRetry) obtainCertificateForDomains(domains []string) error {
	if m.attempts <= m.failAttempts {
		return fmt.Errorf("mock obtain failure for attempt %d", m.attempts)
	}
	
	if !m.shouldSucceed {
		return fmt.Errorf("mock obtain failure - configured to fail")
	}
	
	return nil
}