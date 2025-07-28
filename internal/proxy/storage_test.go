package proxy

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jerkytreats/dns/internal/config"
	"github.com/jerkytreats/dns/internal/persistence"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewProxyRuleStorage(t *testing.T) {
	// Test with default values
	config.ResetForTest()
	storage := NewProxyRuleStorage()
	assert.NotNil(t, storage)
	assert.NotNil(t, storage.storage)

	// Test with custom configuration
	tempDir := t.TempDir()
	customPath := filepath.Join(tempDir, "custom_rules.json")
	
	config.SetForTest("proxy.storage.path", customPath)
	config.SetForTest("proxy.storage.backup_count", "5")
	
	storage2 := NewProxyRuleStorage()
	assert.NotNil(t, storage2)
	assert.Equal(t, customPath, storage2.storage.GetPath())
	
	// Clean up
	config.ResetForTest()
}

func TestProxyRuleStorage_LoadRules_Empty(t *testing.T) {
	tempDir := t.TempDir()
	storagePath := filepath.Join(tempDir, "test_rules.json")
	
	storage := &ProxyRuleStorage{
		storage: persistence.NewFileStorageWithPath(storagePath, 3),
	}
	
	// Load from non-existent file
	rules, err := storage.LoadRules()
	assert.NoError(t, err)
	assert.NotNil(t, rules)
	assert.Len(t, rules, 0)
}

func TestProxyRuleStorage_SaveAndLoadRules(t *testing.T) {
	tempDir := t.TempDir()
	storagePath := filepath.Join(tempDir, "test_rules.json")
	
	storage := &ProxyRuleStorage{
		storage: persistence.NewFileStorageWithPath(storagePath, 3),
	}
	
	// Create test rules
	originalRules := map[string]*ProxyRule{
		"app1.example.com": {
			Hostname:   "app1.example.com",
			TargetIP:   "192.168.1.100",
			TargetPort: 8080,
			Protocol:   "http",
			Enabled:    true,
			CreatedAt:  time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
		},
		"app2.example.com": {
			Hostname:   "app2.example.com",
			TargetIP:   "192.168.1.200",
			TargetPort: 9090,
			Protocol:   "https",
			Enabled:    false,
			CreatedAt:  time.Date(2024, 1, 2, 12, 0, 0, 0, time.UTC),
		},
	}
	
	// Save rules
	err := storage.SaveRules(originalRules)
	assert.NoError(t, err)
	
	// Verify file exists
	assert.True(t, storage.Exists())
	
	// Load rules
	loadedRules, err := storage.LoadRules()
	assert.NoError(t, err)
	assert.Len(t, loadedRules, 2)
	
	// Verify rule 1
	rule1, exists := loadedRules["app1.example.com"]
	assert.True(t, exists)
	assert.Equal(t, "app1.example.com", rule1.Hostname)
	assert.Equal(t, "192.168.1.100", rule1.TargetIP)
	assert.Equal(t, 8080, rule1.TargetPort)
	assert.Equal(t, "http", rule1.Protocol)
	assert.True(t, rule1.Enabled)
	assert.Equal(t, time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC), rule1.CreatedAt)
	
	// Verify rule 2
	rule2, exists := loadedRules["app2.example.com"]
	assert.True(t, exists)
	assert.Equal(t, "app2.example.com", rule2.Hostname)
	assert.Equal(t, "192.168.1.200", rule2.TargetIP)
	assert.Equal(t, 9090, rule2.TargetPort)
	assert.Equal(t, "https", rule2.Protocol)
	assert.False(t, rule2.Enabled)
	assert.Equal(t, time.Date(2024, 1, 2, 12, 0, 0, 0, time.UTC), rule2.CreatedAt)
}

func TestProxyRuleStorage_SaveRules_Validation(t *testing.T) {
	tempDir := t.TempDir()
	storagePath := filepath.Join(tempDir, "test_rules.json")
	
	storage := &ProxyRuleStorage{
		storage: persistence.NewFileStorageWithPath(storagePath, 3),
	}
	
	tests := []struct {
		name        string
		rules       map[string]*ProxyRule
		expectError bool
		errorMsg    string
	}{
		{
			name: "Valid rules",
			rules: map[string]*ProxyRule{
				"valid.example.com": {
					Hostname:   "valid.example.com",
					TargetIP:   "192.168.1.100",
					TargetPort: 8080,
					Protocol:   "http",
					Enabled:    true,
					CreatedAt:  time.Now(),
				},
			},
			expectError: false,
		},
		{
			name: "Empty target IP",
			rules: map[string]*ProxyRule{
				"invalid.example.com": {
					Hostname:   "invalid.example.com",
					TargetIP:   "",
					TargetPort: 8080,
					Protocol:   "http",
					Enabled:    true,
					CreatedAt:  time.Now(),
				},
			},
			expectError: true,
			errorMsg:    "target IP cannot be empty",
		},
		{
			name: "Invalid port (too low)",
			rules: map[string]*ProxyRule{
				"invalid.example.com": {
					Hostname:   "invalid.example.com",
					TargetIP:   "192.168.1.100",
					TargetPort: 0,
					Protocol:   "http",
					Enabled:    true,
					CreatedAt:  time.Now(),
				},
			},
			expectError: true,
			errorMsg:    "target port must be between 1 and 65535",
		},
		{
			name: "Invalid port (too high)",
			rules: map[string]*ProxyRule{
				"invalid.example.com": {
					Hostname:   "invalid.example.com",
					TargetIP:   "192.168.1.100",
					TargetPort: 70000,
					Protocol:   "http",
					Enabled:    true,
					CreatedAt:  time.Now(),
				},
			},
			expectError: true,
			errorMsg:    "target port must be between 1 and 65535",
		},
		{
			name: "Invalid protocol",
			rules: map[string]*ProxyRule{
				"invalid.example.com": {
					Hostname:   "invalid.example.com",
					TargetIP:   "192.168.1.100",
					TargetPort: 8080,
					Protocol:   "ftp",
					Enabled:    true,
					CreatedAt:  time.Now(),
				},
			},
			expectError: true,
			errorMsg:    "protocol must be 'http' or 'https'",
		},
		{
			name: "Invalid hostname",
			rules: map[string]*ProxyRule{
				"invalid": {
					Hostname:   "invalid",
					TargetIP:   "192.168.1.100",
					TargetPort: 8080,
					Protocol:   "http",
					Enabled:    true,
					CreatedAt:  time.Now(),
				},
			},
			expectError: true,
			errorMsg:    "invalid hostname",
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := storage.SaveRules(tt.rules)
			
			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestProxyRuleStorage_LoadRules_InvalidJSON(t *testing.T) {
	tempDir := t.TempDir()
	storagePath := filepath.Join(tempDir, "test_rules.json")
	
	// Write invalid JSON
	invalidJSON := `{"incomplete": json`
	err := os.WriteFile(storagePath, []byte(invalidJSON), 0644)
	require.NoError(t, err)
	
	storage := &ProxyRuleStorage{
		storage: persistence.NewFileStorageWithPath(storagePath, 3),
	}
	
	// Should return error for invalid JSON
	rules, err := storage.LoadRules()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to unmarshal proxy rules")
	assert.Nil(t, rules)
}

func TestProxyRuleStorage_LoadRules_ValidationFiltering(t *testing.T) {
	tempDir := t.TempDir()
	storagePath := filepath.Join(tempDir, "test_rules.json")
	
	// Create mixed valid/invalid rules in JSON
	mixedRules := map[string]*ProxyRule{
		"valid1.example.com": {
			Hostname:   "valid1.example.com",
			TargetIP:   "192.168.1.100",
			TargetPort: 8080,
			Protocol:   "http",
			Enabled:    true,
			CreatedAt:  time.Now(),
		},
		"invalid.example.com": {
			Hostname:   "invalid.example.com",
			TargetIP:   "", // Invalid
			TargetPort: 8080,
			Protocol:   "http",
			Enabled:    true,
			CreatedAt:  time.Now(),
		},
		"valid2.example.com": {
			Hostname:   "valid2.example.com",
			TargetIP:   "192.168.1.200",
			TargetPort: 9090,
			Protocol:   "https",
			Enabled:    false,
			CreatedAt:  time.Now(),
		},
	}
	
	// Marshal and write to file
	data, err := json.MarshalIndent(mixedRules, "", "  ")
	require.NoError(t, err)
	err = os.WriteFile(storagePath, data, 0644)
	require.NoError(t, err)
	
	storage := &ProxyRuleStorage{
		storage: persistence.NewFileStorageWithPath(storagePath, 3),
	}
	
	// Load rules - should filter out invalid ones
	rules, err := storage.LoadRules()
	assert.NoError(t, err)
	assert.Len(t, rules, 2) // Only valid rules
	
	assert.Contains(t, rules, "valid1.example.com")
	assert.Contains(t, rules, "valid2.example.com")
	assert.NotContains(t, rules, "invalid.example.com")
}

func TestProxyRuleStorage_Exists(t *testing.T) {
	tempDir := t.TempDir()
	storagePath := filepath.Join(tempDir, "test_rules.json")
	
	storage := &ProxyRuleStorage{
		storage: persistence.NewFileStorageWithPath(storagePath, 3),
	}
	
	// Initially should not exist
	assert.False(t, storage.Exists())
	
	// Create file
	rules := map[string]*ProxyRule{
		"test.example.com": {
			Hostname:   "test.example.com",
			TargetIP:   "192.168.1.100",
			TargetPort: 8080,
			Protocol:   "http",
			Enabled:    true,
			CreatedAt:  time.Now(),
		},
	}
	
	err := storage.SaveRules(rules)
	assert.NoError(t, err)
	
	// Should now exist
	assert.True(t, storage.Exists())
}

func TestProxyRuleStorage_GetStorageInfo(t *testing.T) {
	tempDir := t.TempDir()
	storagePath := filepath.Join(tempDir, "test_rules.json")
	
	storage := &ProxyRuleStorage{
		storage: persistence.NewFileStorageWithPath(storagePath, 3),
	}
	
	// Get info before file creation
	info := storage.GetStorageInfo()
	assert.NotNil(t, info)
	assert.Equal(t, "proxy_rules", info["type"])
	assert.Contains(t, info, "file_path")
	assert.Contains(t, info, "backup_count")
	assert.Contains(t, info, "exists")
	
	// Create file and get info again
	rules := map[string]*ProxyRule{
		"test.example.com": {
			Hostname:   "test.example.com",
			TargetIP:   "192.168.1.100",
			TargetPort: 8080,
			Protocol:   "http",
			Enabled:    true,
			CreatedAt:  time.Now(),
		},
	}
	
	err := storage.SaveRules(rules)
	assert.NoError(t, err)
	
	info2 := storage.GetStorageInfo()
	assert.Equal(t, "proxy_rules", info2["type"])
	assert.Equal(t, true, info2["exists"])
	assert.Contains(t, info2, "size")
	assert.Contains(t, info2, "modified")
}

func TestProxyRuleStorage_BackupFunctionality(t *testing.T) {
	tempDir := t.TempDir()
	storagePath := filepath.Join(tempDir, "test_rules.json")
	
	storage := &ProxyRuleStorage{
		storage: persistence.NewFileStorageWithPath(storagePath, 2), // Keep 2 backups
	}
	
	// Create initial rules
	rules1 := map[string]*ProxyRule{
		"test1.example.com": {
			Hostname:   "test1.example.com",
			TargetIP:   "192.168.1.100",
			TargetPort: 8080,
			Protocol:   "http",
			Enabled:    true,
			CreatedAt:  time.Now(),
		},
	}
	
	err := storage.SaveRules(rules1)
	assert.NoError(t, err)
	
	// Sleep to ensure different backup timestamps
	time.Sleep(time.Second + time.Millisecond*100)
	
	// Update rules (should create backup)
	rules2 := map[string]*ProxyRule{
		"test2.example.com": {
			Hostname:   "test2.example.com",
			TargetIP:   "192.168.1.200",
			TargetPort: 9090,
			Protocol:   "https",
			Enabled:    true,
			CreatedAt:  time.Now(),
		},
	}
	
	err = storage.SaveRules(rules2)
	assert.NoError(t, err)
	
	// Check that backup was created
	entries, err := os.ReadDir(tempDir)
	assert.NoError(t, err)
	
	backupCount := 0
	for _, entry := range entries {
		if entry.Name() != "test_rules.json" && entry.Name() != ".DS_Store" {
			backupCount++
		}
	}
	
	assert.GreaterOrEqual(t, backupCount, 1, "At least one backup should exist")
	
	// Verify current content is the latest
	currentRules, err := storage.LoadRules()
	assert.NoError(t, err)
	assert.Len(t, currentRules, 1)
	assert.Contains(t, currentRules, "test2.example.com")
}

func TestProxyRuleStorage_EmptyRulesMap(t *testing.T) {
	tempDir := t.TempDir()
	storagePath := filepath.Join(tempDir, "test_rules.json")
	
	storage := &ProxyRuleStorage{
		storage: persistence.NewFileStorageWithPath(storagePath, 3),
	}
	
	// Save empty rules map
	emptyRules := make(map[string]*ProxyRule)
	err := storage.SaveRules(emptyRules)
	assert.NoError(t, err)
	
	// Load back
	loadedRules, err := storage.LoadRules()
	assert.NoError(t, err)
	assert.NotNil(t, loadedRules)
	assert.Len(t, loadedRules, 0)
}