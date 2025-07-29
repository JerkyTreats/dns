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
	"github.com/stretchr/testify/require"
)

// MockManagerForSANValidation is defined in san_validation_test.go but we need it here too
// Since Go doesn't allow sharing test types across files, we'll reference the one in san_management_test.go

func TestManager_ValidateAndUpdateSANDomains_EdgeCases(t *testing.T) {
	// Create temporary directory for test
	tempDir, err := os.MkdirTemp("", "san_validation_edge_test_")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	tests := []struct {
		name                string
		existingDomains     *CertificateDomains
		dnsRecords          []DNSRecord
		baseDomain          string
		expectedFinalSANs   []string
		expectError         bool
		description         string
	}{
		{
			name: "Large number of domains",
			existingDomains: &CertificateDomains{
				BaseDomain: "internal.example.com",
				SANDomains: []string{},
				UpdatedAt:  time.Now(),
			},
			dnsRecords: func() []DNSRecord {
				records := make([]DNSRecord, 50)
				for i := 0; i < 50; i++ {
					records[i] = DNSRecord{
						Name: fmt.Sprintf("service%d", i),
						Type: "A",
					}
				}
				return records
			}(),
			baseDomain: "internal.example.com",
			expectedFinalSANs: func() []string {
				domains := make([]string, 50)
				for i := 0; i < 50; i++ {
					domains[i] = fmt.Sprintf("service%d.internal.example.com", i)
				}
				return domains
			}(),
			expectError: false,
			description: "Should handle many domains efficiently",
		},
		{
			name: "Duplicate DNS records",
			existingDomains: &CertificateDomains{
				BaseDomain: "internal.example.com",
				SANDomains: []string{},
				UpdatedAt:  time.Now(),
			},
			dnsRecords: []DNSRecord{
				{Name: "api", Type: "A"},
				{Name: "api", Type: "A"},      // Duplicate
				{Name: "api", Type: "CNAME"},  // Same name, different type
				{Name: "dns", Type: "A"},
				{Name: "dns", Type: "A"},      // Another duplicate
			},
			baseDomain: "internal.example.com",
			expectedFinalSANs: []string{
				"api.internal.example.com",
				"dns.internal.example.com",
			},
			expectError: false,
			description: "Should deduplicate domains automatically",
		},
		{
			name: "Special characters in domain names",
			existingDomains: &CertificateDomains{
				BaseDomain: "internal.example.com",
				SANDomains: []string{},
				UpdatedAt:  time.Now(),
			},
			dnsRecords: []DNSRecord{
				{Name: "api-v1", Type: "A"},
				{Name: "web_server", Type: "A"},
				{Name: "test123", Type: "A"},
				{Name: "a", Type: "A"},          // Single character
				{Name: "very-long-domain-name-with-many-hyphens", Type: "A"},
			},
			baseDomain: "internal.example.com",
			expectedFinalSANs: []string{
				"api-v1.internal.example.com",
				"web_server.internal.example.com",
				"test123.internal.example.com",
				"a.internal.example.com",
				"very-long-domain-name-with-many-hyphens.internal.example.com",
			},
			expectError: false,
			description: "Should handle special characters in domain names",
		},
		{
			name: "Mixed case domain names",
			existingDomains: &CertificateDomains{
				BaseDomain: "internal.example.com",
				SANDomains: []string{},
				UpdatedAt:  time.Now(),
			},
			dnsRecords: []DNSRecord{
				{Name: "API", Type: "A"},
				{Name: "Web", Type: "A"},
				{Name: "DNS", Type: "A"},
			},
			baseDomain: "internal.example.com",
			expectedFinalSANs: []string{
				"API.internal.example.com",
				"Web.internal.example.com", 
				"DNS.internal.example.com",
			},
			expectError: false,
			description: "Should preserve case in domain names",
		},
		{
			name: "Base domain appears as subdomain",
			existingDomains: &CertificateDomains{
				BaseDomain: "internal.example.com",
				SANDomains: []string{},
				UpdatedAt:  time.Now(),
			},
			dnsRecords: []DNSRecord{
				{Name: "internal", Type: "A"}, // Would create internal.internal.example.com
				{Name: "example", Type: "A"},  // Would create example.internal.example.com
			},
			baseDomain: "internal.example.com",
			expectedFinalSANs: []string{
				"internal.internal.example.com",
				"example.internal.example.com",
			},
			expectError: false,
			description: "Should handle base domain components as subdomains",
		},
		{
			name: "Empty SAN list initially",
			existingDomains: &CertificateDomains{
				BaseDomain: "internal.example.com",
				SANDomains: nil, // nil vs empty slice
				UpdatedAt:  time.Now(),
			},
			dnsRecords: []DNSRecord{
				{Name: "api", Type: "A"},
			},
			baseDomain: "internal.example.com",
			expectedFinalSANs: []string{
				"api.internal.example.com",
			},
			expectError: false,
			description: "Should handle nil SAN list",
		},
		{
			name: "No DNS records to validate",
			existingDomains: &CertificateDomains{
				BaseDomain: "internal.example.com",
				SANDomains: []string{"existing.internal.example.com"},
				UpdatedAt:  time.Now(),
			},
			dnsRecords:        []DNSRecord{}, // Empty list
			baseDomain:        "internal.example.com",
			expectedFinalSANs: []string{"existing.internal.example.com"},
			expectError:       false,
			description:       "Should handle empty DNS record list gracefully",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup test config
			config.SetForTest(CertDomainKey, tt.baseDomain)
			defer config.ResetForTest()

			// Setup storage
			storage := &CertificateDomainStorage{
				storage: persistence.NewFileStorageWithPath(filepath.Join(tempDir, fmt.Sprintf("edge_test_%s.json", tt.name)), 3),
			}
			err := storage.SaveDomains(tt.existingDomains)
			require.NoError(t, err)

			// Setup mock DNS record provider
			mockProvider := &MockDNSRecordProvider{}
			mockProvider.On("ListRecords").Return(tt.dnsRecords, nil)

			// Create mock manager that doesn't trigger actual certificate operations
			manager := &MockManagerForSANValidation{
				domainStorage:     storage,
				dnsRecordProvider: mockProvider,
				addedDomains:      make([]string, 0),
			}

			// Run validation
			err = manager.ValidateAndUpdateSANDomains()

			if tt.expectError {
				assert.Error(t, err, tt.description)
				return
			}

			require.NoError(t, err, tt.description)

			// Verify final state
			finalDomains, err := storage.LoadDomains()
			require.NoError(t, err)

			assert.ElementsMatch(t, tt.expectedFinalSANs, finalDomains.SANDomains, tt.description)
			assert.Equal(t, tt.baseDomain, finalDomains.BaseDomain)

			mockProvider.AssertExpectations(t)
		})
	}
}

func TestManager_ValidateAndUpdateSANDomains_PerformanceConsiderations(t *testing.T) {
	// Create temporary directory for test
	tempDir, err := os.MkdirTemp("", "san_validation_perf_test_")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Setup test config
	config.SetForTest(CertDomainKey, "internal.example.com")
	defer config.ResetForTest()

	// Create large number of existing SAN domains
	existingSANs := make([]string, 100)
	for i := 0; i < 100; i++ {
		existingSANs[i] = fmt.Sprintf("existing%d.internal.example.com", i)
	}

	existingDomains := &CertificateDomains{
		BaseDomain: "internal.example.com",
		SANDomains: existingSANs,
		UpdatedAt:  time.Now(),
	}

	// Create many DNS records, some matching existing SANs
	dnsRecords := make([]DNSRecord, 200)
	for i := 0; i < 200; i++ {
		if i < 100 {
			// First 100 match existing SANs
			dnsRecords[i] = DNSRecord{Name: fmt.Sprintf("existing%d", i), Type: "A"}
		} else {
			// Next 100 are new
			dnsRecords[i] = DNSRecord{Name: fmt.Sprintf("new%d", i-100), Type: "A"}
		}
	}

	// Setup storage
	storage := &CertificateDomainStorage{
		storage: persistence.NewFileStorageWithPath(filepath.Join(tempDir, "perf_test.json"), 3),
	}
	err = storage.SaveDomains(existingDomains)
	require.NoError(t, err)

	// Setup mock DNS record provider
	mockProvider := &MockDNSRecordProvider{}
	mockProvider.On("ListRecords").Return(dnsRecords, nil)

	// Create mock manager
	manager := &MockManagerForSANValidation{
		domainStorage:     storage,
		dnsRecordProvider: mockProvider,
		addedDomains:      make([]string, 0),
	}

	// Measure execution time
	start := time.Now()
	err = manager.ValidateAndUpdateSANDomains()
	duration := time.Since(start)

	require.NoError(t, err)

	// Should complete in reasonable time (less than 1 second for 200 records)
	assert.Less(t, duration, time.Second, "SAN validation should complete quickly even with many records")

	// Verify results
	finalDomains, err := storage.LoadDomains()
	require.NoError(t, err)

	// Should have 200 SAN domains total (100 existing + 100 new)
	assert.Len(t, finalDomains.SANDomains, 200)

	mockProvider.AssertExpectations(t)
}

func TestManager_ValidateAndUpdateSANDomains_ConcurrentSafety(t *testing.T) {
	// This test ensures the method is safe to call concurrently
	// (though in practice it shouldn't be called concurrently)
	
	tempDir, err := os.MkdirTemp("", "san_validation_concurrent_test_")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	config.SetForTest(CertDomainKey, "internal.example.com")
	defer config.ResetForTest()

	existingDomains := &CertificateDomains{
		BaseDomain: "internal.example.com",
		SANDomains: []string{},
		UpdatedAt:  time.Now(),
	}

	storage := &CertificateDomainStorage{
		storage: persistence.NewFileStorageWithPath(filepath.Join(tempDir, "concurrent_test.json"), 3),
	}
	err = storage.SaveDomains(existingDomains)
	require.NoError(t, err)

	dnsRecords := []DNSRecord{
		{Name: "api", Type: "A"},
		{Name: "dns", Type: "A"},
	}

	mockProvider := &MockDNSRecordProvider{}
	// Allow multiple calls since we'll call it concurrently
	mockProvider.On("ListRecords").Return(dnsRecords, nil).Maybe()

	manager := &MockManagerForSANValidation{
		domainStorage:     storage,
		dnsRecordProvider: mockProvider,
		addedDomains:      make([]string, 0),
	}

	// Run multiple validations concurrently
	done := make(chan error, 3)
	for i := 0; i < 3; i++ {
		go func() {
			done <- manager.ValidateAndUpdateSANDomains()
		}()
	}

	// Wait for all to complete
	for i := 0; i < 3; i++ {
		err := <-done
		assert.NoError(t, err)
	}

	// Verify final state is consistent
	finalDomains, err := storage.LoadDomains()
	require.NoError(t, err)

	expectedSANs := []string{
		"api.internal.example.com",
		"dns.internal.example.com",
	}
	assert.ElementsMatch(t, expectedSANs, finalDomains.SANDomains)
}