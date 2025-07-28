package certificate

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jerkytreats/dns/internal/persistence"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCertificateDomainStorage(t *testing.T) {
	storage := NewCertificateDomainStorage()
	
	assert.NotNil(t, storage)
	assert.NotNil(t, storage.storage)
}

func TestCertificateDomainStorage_SaveAndLoadDomains(t *testing.T) {
	// Create temporary directory for test
	tempDir, err := os.MkdirTemp("", "cert_domain_test_")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)
	
	// Create storage with custom path
	storage := &CertificateDomainStorage{
		storage: persistence.NewFileStorageWithPath(filepath.Join(tempDir, "domains.json"), 3),
	}
	
	// Test data
	originalDomains := &CertificateDomains{
		BaseDomain: "internal.example.com",
		SANDomains: []string{"api.internal.example.com", "dns.internal.example.com"},
		UpdatedAt:  time.Now().Truncate(time.Second), // Truncate for comparison
	}
	
	// Save domains
	err = storage.SaveDomains(originalDomains)
	require.NoError(t, err)
	
	// Load domains
	loadedDomains, err := storage.LoadDomains()
	require.NoError(t, err)
	
	// Verify data
	assert.Equal(t, originalDomains.BaseDomain, loadedDomains.BaseDomain)
	assert.Equal(t, originalDomains.SANDomains, loadedDomains.SANDomains)
	// UpdatedAt will be updated during save, so check it's recent
	assert.WithinDuration(t, time.Now(), loadedDomains.UpdatedAt, 5*time.Second)
}

func TestCertificateDomainStorage_LoadDomains_FileNotExists(t *testing.T) {
	// Create storage with non-existent path
	storage := &CertificateDomainStorage{
		storage: persistence.NewFileStorageWithPath("/non/existent/path.json", 3),
	}
	
	// Should return error when file doesn't exist
	_, err := storage.LoadDomains()
	assert.Error(t, err)
	// The error could be either read failure or JSON unmarshal failure depending on persistence implementation
	assert.True(t, 
		err.Error() == "failed to read certificate domains: open /non/existent/path.json: no such file or directory" ||
		err.Error() == "failed to unmarshal certificate domains: unexpected end of JSON input",
		"Expected either read error or unmarshal error, got: %s", err.Error())
}

func TestCertificateDomainStorage_LoadDomains_InvalidJSON(t *testing.T) {
	// Create temporary directory for test
	tempDir, err := os.MkdirTemp("", "cert_domain_test_")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)
	
	// Create file with invalid JSON
	filePath := filepath.Join(tempDir, "invalid.json")
	err = os.WriteFile(filePath, []byte("invalid json"), 0644)
	require.NoError(t, err)
	
	storage := &CertificateDomainStorage{
		storage: persistence.NewFileStorageWithPath(filePath, 3),
	}
	
	// Should return error for invalid JSON
	_, err = storage.LoadDomains()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to unmarshal certificate domains")
}

func TestCertificateDomainStorage_AddDomain(t *testing.T) {
	tests := []struct {
		name            string
		initialDomains  []string
		domainToAdd     string
		expectedDomains []string
		expectError     bool
	}{
		{
			name:            "Add new domain",
			initialDomains:  []string{"api.internal.example.com"},
			domainToAdd:     "dns.internal.example.com",
			expectedDomains: []string{"api.internal.example.com", "dns.internal.example.com"},
			expectError:     false,
		},
		{
			name:            "Add duplicate domain",
			initialDomains:  []string{"api.internal.example.com"},
			domainToAdd:     "api.internal.example.com",
			expectedDomains: []string{"api.internal.example.com"},
			expectError:     false,
		},
		{
			name:            "Add base domain",
			initialDomains:  []string{"api.internal.example.com"},
			domainToAdd:     "internal.example.com",
			expectedDomains: []string{"api.internal.example.com"},
			expectError:     false,
		},
		{
			name:            "Add another new domain",
			initialDomains:  []string{"api.internal.example.com"},
			domainToAdd:     "web.internal.example.com",
			expectedDomains: []string{"api.internal.example.com", "web.internal.example.com"},
			expectError:     false,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create separate temp dir for each test
			tempDir, err := os.MkdirTemp("", "cert_domain_addtest_")
			require.NoError(t, err)
			defer os.RemoveAll(tempDir)
			
			storage := &CertificateDomainStorage{
				storage: persistence.NewFileStorageWithPath(filepath.Join(tempDir, "domains.json"), 3),
			}
			
			// Initialize with base domains
			initialDomains := &CertificateDomains{
				BaseDomain: "internal.example.com",
				SANDomains: tt.initialDomains,
				UpdatedAt:  time.Now(),
			}
			err = storage.SaveDomains(initialDomains)
			require.NoError(t, err)
			
			err = storage.AddDomain(tt.domainToAdd)
			
			if tt.expectError {
				assert.Error(t, err)
				return
			}
			
			require.NoError(t, err)
			
			// Load and verify
			domains, err := storage.LoadDomains()
			require.NoError(t, err)
			
			assert.Equal(t, "internal.example.com", domains.BaseDomain)
			assert.ElementsMatch(t, tt.expectedDomains, domains.SANDomains)
		})
	}
}

func TestCertificateDomainStorage_RemoveDomain(t *testing.T) {
	// Create temporary directory for test
	tempDir, err := os.MkdirTemp("", "cert_domain_test_")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)
	
	storage := &CertificateDomainStorage{
		storage: persistence.NewFileStorageWithPath(filepath.Join(tempDir, "domains.json"), 3),
	}
	
	// Initialize with domains
	initialDomains := &CertificateDomains{
		BaseDomain: "internal.example.com",
		SANDomains: []string{"api.internal.example.com", "dns.internal.example.com", "web.internal.example.com"},
		UpdatedAt:  time.Now(),
	}
	err = storage.SaveDomains(initialDomains)
	require.NoError(t, err)
	
	tests := []struct {
		name            string
		domainToRemove  string
		expectedDomains []string
		expectError     bool
		errorContains   string
	}{
		{
			name:            "Remove existing domain",
			domainToRemove:  "dns.internal.example.com",
			expectedDomains: []string{"api.internal.example.com", "web.internal.example.com"},
			expectError:     false,
		},
		{
			name:            "Remove non-existent domain",
			domainToRemove:  "nonexistent.internal.example.com",
			expectedDomains: []string{"api.internal.example.com", "web.internal.example.com"},
			expectError:     false,
		},
		{
			name:            "Try to remove base domain",
			domainToRemove:  "internal.example.com",
			expectedDomains: []string{"api.internal.example.com", "web.internal.example.com"},
			expectError:     true,
			errorContains:   "cannot remove base domain",
		},
		{
			name:            "Remove another domain",
			domainToRemove:  "api.internal.example.com",
			expectedDomains: []string{"web.internal.example.com"},
			expectError:     false,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := storage.RemoveDomain(tt.domainToRemove)
			
			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
				return
			}
			
			require.NoError(t, err)
			
			// Load and verify
			domains, err := storage.LoadDomains()
			require.NoError(t, err)
			
			assert.Equal(t, "internal.example.com", domains.BaseDomain)
			assert.ElementsMatch(t, tt.expectedDomains, domains.SANDomains)
		})
	}
}

func TestCertificateDomainStorage_Exists(t *testing.T) {
	// Create temporary directory for test
	tempDir, err := os.MkdirTemp("", "cert_domain_test_")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)
	
	filePath := filepath.Join(tempDir, "domains.json")
	storage := &CertificateDomainStorage{
		storage: persistence.NewFileStorageWithPath(filePath, 3),
	}
	
	// Should not exist initially
	assert.False(t, storage.Exists())
	
	// Create file
	domains := &CertificateDomains{
		BaseDomain: "internal.example.com",
		SANDomains: []string{},
		UpdatedAt:  time.Now(),
	}
	err = storage.SaveDomains(domains)
	require.NoError(t, err)
	
	// Should exist now
	assert.True(t, storage.Exists())
}

func TestCertificateDomainStorage_SaveDomains_UpdatesTimestamp(t *testing.T) {
	// Create temporary directory for test
	tempDir, err := os.MkdirTemp("", "cert_domain_test_")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)
	
	storage := &CertificateDomainStorage{
		storage: persistence.NewFileStorageWithPath(filepath.Join(tempDir, "domains.json"), 3),
	}
	
	oldTime := time.Now().Add(-1 * time.Hour)
	domains := &CertificateDomains{
		BaseDomain: "internal.example.com",
		SANDomains: []string{"api.internal.example.com"},
		UpdatedAt:  oldTime,
	}
	
	// Save domains
	err = storage.SaveDomains(domains)
	require.NoError(t, err)
	
	// Load and verify timestamp was updated
	loadedDomains, err := storage.LoadDomains()
	require.NoError(t, err)
	
	assert.True(t, loadedDomains.UpdatedAt.After(oldTime))
	assert.WithinDuration(t, time.Now(), loadedDomains.UpdatedAt, 5*time.Second)
}

func TestCertificateDomainStorage_JSONFormatting(t *testing.T) {
	// Create temporary directory for test
	tempDir, err := os.MkdirTemp("", "cert_domain_test_")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)
	
	filePath := filepath.Join(tempDir, "domains.json")
	storage := &CertificateDomainStorage{
		storage: persistence.NewFileStorageWithPath(filePath, 3),
	}
	
	domains := &CertificateDomains{
		BaseDomain: "internal.example.com",
		SANDomains: []string{"api.internal.example.com", "dns.internal.example.com"},
		UpdatedAt:  time.Now(),
	}
	
	// Save domains
	err = storage.SaveDomains(domains)
	require.NoError(t, err)
	
	// Read raw file content
	content, err := os.ReadFile(filePath)
	require.NoError(t, err)
	
	// Verify it's properly formatted JSON
	var parsed CertificateDomains
	err = json.Unmarshal(content, &parsed)
	require.NoError(t, err)
	
	assert.Equal(t, domains.BaseDomain, parsed.BaseDomain)
	assert.ElementsMatch(t, domains.SANDomains, parsed.SANDomains)
	
	// Verify it's indented (pretty printed)
	contentStr := string(content)
	assert.Contains(t, contentStr, "  \"base_domain\"")
	assert.Contains(t, contentStr, "  \"san_domains\"")
}

