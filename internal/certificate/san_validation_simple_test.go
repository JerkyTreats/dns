package certificate

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jerkytreats/dns/internal/config"
	"github.com/jerkytreats/dns/internal/persistence"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManager_ValidateAndUpdateSANDomains_LogicOnly(t *testing.T) {
	// Create temporary directory for test
	tempDir, err := os.MkdirTemp("", "san_validation_simple_test_")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Setup test config
	config.SetForTest(CertDomainKey, "internal.example.com")
	defer config.ResetForTest()

	// Setup storage with initial domains
	storage := &CertificateDomainStorage{
		storage: persistence.NewFileStorageWithPath(filepath.Join(tempDir, "simple_test.json"), 3),
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
		{Name: "", Type: "A"},       // Should be skipped
	}
	mockProvider.On("ListRecords").Return(dnsRecords, nil)

	// Use mock manager that doesn't trigger actual certificate operations
	manager := &MockManagerForSANValidation{
		domainStorage:     storage,
		dnsRecordProvider: mockProvider,
		addedDomains:      make([]string, 0),
	}

	// Run validation
	err = manager.ValidateAndUpdateSANDomains()
	require.NoError(t, err)

	// Verify the correct domain was identified for addition
	expectedAddedDomains := []string{"dns.internal.example.com"}
	assert.ElementsMatch(t, expectedAddedDomains, manager.addedDomains)

	mockProvider.AssertExpectations(t)
}

func TestManager_ValidateAndUpdateSANDomains_NoProvider(t *testing.T) {
	manager := &Manager{
		dnsRecordProvider: nil,
		domainStorage:     &CertificateDomainStorage{},
	}

	err := manager.ValidateAndUpdateSANDomains()
	assert.NoError(t, err) // Should return nil when provider is not set
}

func TestManager_ValidateAndUpdateSANDomains_NoStorage(t *testing.T) {
	mockProvider := &MockDNSRecordProvider{}
	manager := &Manager{
		dnsRecordProvider: mockProvider,
		domainStorage:     nil,
	}

	err := manager.ValidateAndUpdateSANDomains()
	assert.NoError(t, err) // Should return nil when storage is not set (logs warning)
}