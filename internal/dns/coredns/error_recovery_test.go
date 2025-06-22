package coredns

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jerkytreats/dns/internal/config"
)

func TestErrorRecovery_ConfigGenerationFailure(t *testing.T) {
	// Setup test environment
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "Corefile")
	templatePath := filepath.Join(tempDir, "nonexistent.template")
	zonesPath := filepath.Join(tempDir, "zones")
	domain := "test.example.com"

	// Create ConfigManager with invalid template path
	cm := NewConfigManager(configPath, templatePath, domain, zonesPath)

	// Test: GenerateCorefile should fail gracefully
	err := cm.GenerateCorefile()
	if err == nil {
		t.Error("Expected error when template file doesn't exist")
	}

	// Verify no partial files were created
	if _, err := os.Stat(configPath); err == nil {
		t.Error("Config file should not exist after failed generation")
	}
}

func TestErrorRecovery_ConfigRollback(t *testing.T) {
	// Setup test environment
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "Corefile")
	templatePath := filepath.Join(tempDir, "Corefile.template")
	backupPath := filepath.Join(tempDir, "Corefile.backup")
	zonesPath := filepath.Join(tempDir, "zones")
	domain := "test.example.com"

	// Create a valid template
	templateContent := `# Test Template
. {
    errors
    log
    forward . 8.8.8.8
}
`
	if err := os.WriteFile(templatePath, []byte(templateContent), 0644); err != nil {
		t.Fatalf("Failed to create template: %v", err)
	}

	// Create initial config and backup
	validConfig := `# Valid config
. {
    errors
    log
    forward . 8.8.8.8
}
`
	if err := os.WriteFile(configPath, []byte(validConfig), 0644); err != nil {
		t.Fatalf("Failed to create initial config: %v", err)
	}
	if err := os.WriteFile(backupPath, []byte(validConfig), 0644); err != nil {
		t.Fatalf("Failed to create backup: %v", err)
	}

	cm := NewConfigManager(configPath, templatePath, domain, zonesPath)

	// Test: Add domain should succeed
	if err := cm.AddDomain("valid.example.com", nil); err != nil {
		t.Fatalf("Failed to add valid domain: %v", err)
	}

	// Simulate failure by making config path unwritable
	if err := os.Chmod(filepath.Dir(configPath), 0444); err != nil {
		t.Fatalf("Failed to make directory read-only: %v", err)
	}

	// Restore permissions for cleanup
	defer func() {
		os.Chmod(filepath.Dir(configPath), 0755)
	}()

	// Test: Adding another domain should fail but not corrupt state
	err := cm.AddDomain("invalid.example.com", nil)
	if err == nil {
		t.Error("Expected error when config path is unwritable")
	}

	// Verify ConfigManager state is consistent
	if version := cm.GetConfigVersion(); version == 0 {
		t.Error("Config version should not be reset after failure")
	}

	// Verify domain wasn't added to internal state on failure
	domains := cm.GetAllDomains()
	if _, exists := domains["invalid.example.com"]; exists {
		t.Error("Failed domain should not be in ConfigManager state")
	}

	// Verify original domain is still there
	if _, exists := domains["valid.example.com"]; !exists {
		t.Error("Original domain should still exist after failure")
	}
}

func TestErrorRecovery_RestartFailureRecovery(t *testing.T) {
	// Setup test environment
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "Corefile")
	templatePath := filepath.Join(tempDir, "Corefile.template")
	zonesPath := filepath.Join(tempDir, "zones")
	domain := "test.example.com"

	// Create a valid template
	templateContent := `# Test Template
. {
    errors
    log
    forward . 8.8.8.8
}

{{- range .Domains}}
{{- if .Enabled}}
{{.Domain}}:{{.Port}} {
    file {{.ZoneFile}} {{.Domain}}
    errors
    log
}
{{- end}}
{{- end}}
`
	if err := os.WriteFile(templatePath, []byte(templateContent), 0644); err != nil {
		t.Fatalf("Failed to create template: %v", err)
	}

	cm := NewConfigManager(configPath, templatePath, domain, zonesPath)

	// Mock restart manager for controlled failure
	originalRM := cm.restartManager
	mockRM := &MockRestartManager{
		shouldFail: true,
		failCount:  0,
		maxFails:   2, // Fail twice, then succeed
	}
	// Replace the restart manager using SetRestartManager method
	cm.SetRestartManager(mockRM)

	// Test: Add domain should handle restart failures gracefully
	err := cm.AddDomain("test.example.com", nil)
	if err == nil {
		t.Error("Expected error due to restart failure")
	}

	// Verify config was generated but restart failed
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Error("Config file should exist even if restart failed")
	}

	// Test: Retry with restart working should succeed
	mockRM.shouldFail = false
	err = cm.AddDomain("retry.example.com", nil)
	if err != nil {
		t.Errorf("Retry should succeed when restart works: %v", err)
	}

	// Restore original restart manager
	cm.SetRestartManager(originalRM)
}

func TestErrorRecovery_CertificateIntegrationFailure(t *testing.T) {
	// Setup test environment
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "Corefile")
	templatePath := filepath.Join(tempDir, "Corefile.template")
	zonesPath := filepath.Join(tempDir, "zones")
	domain := "test.example.com"

	// Create a valid template
	templateContent := `# Test Template
. {
    errors
    log
    forward . 8.8.8.8
}

{{- range .Domains}}
{{- if .Enabled}}
{{.Domain}}:{{.Port}} {
    file {{.ZoneFile}} {{.Domain}}
    errors
    log
}

{{- if .TLSConfig}}
{{.Domain}}:{{.TLSConfig.Port}} {
    tls {{.TLSConfig.CertFile}} {{.TLSConfig.KeyFile}}
    file {{.ZoneFile}} {{.Domain}}
    errors
    log
}
{{- end}}
{{- end}}
{{- end}}
`
	if err := os.WriteFile(templatePath, []byte(templateContent), 0644); err != nil {
		t.Fatalf("Failed to create template: %v", err)
	}

	cm := NewConfigManager(configPath, templatePath, domain, zonesPath)

	// Test: Add domain without TLS
	if err := cm.AddDomain("test.example.com", nil); err != nil {
		t.Fatalf("Failed to add domain: %v", err)
	}

	// Test: Enable TLS with invalid certificate paths
	err := cm.EnableTLS("test.example.com", "/invalid/cert.pem", "/invalid/key.pem")

	// Should fail with invalid cert paths but not corrupt state
	if err == nil {
		t.Error("EnableTLS should fail with invalid cert paths")
	}

	// Verify domain configuration was not corrupted
	domains := cm.GetAllDomains()
	if domain, exists := domains["test.example.com"]; exists {
		if domain.TLSConfig != nil {
			t.Error("TLS config should not be set when cert files don't exist")
		}
	} else {
		t.Error("Domain should still exist after failed TLS enablement")
	}
}

func TestErrorRecovery_ConcurrentOperations(t *testing.T) {
	// Setup test environment
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "Corefile")
	templatePath := filepath.Join(tempDir, "Corefile.template")
	zonesPath := filepath.Join(tempDir, "zones")
	domain := "test.example.com"

	// Create a valid template
	templateContent := `# Test Template
. {
    errors
    log
    forward . 8.8.8.8
}

{{- range .Domains}}
{{- if .Enabled}}
{{.Domain}}:{{.Port}} {
    file {{.ZoneFile}} {{.Domain}}
    errors
    log
}
{{- end}}
{{- end}}
`
	if err := os.WriteFile(templatePath, []byte(templateContent), 0644); err != nil {
		t.Fatalf("Failed to create template: %v", err)
	}

	cm := NewConfigManager(configPath, templatePath, domain, zonesPath)

	// Test: Concurrent domain additions
	concurrency := 5
	results := make(chan error, concurrency)

	for i := 0; i < concurrency; i++ {
		go func(index int) {
			domainName := fmt.Sprintf("domain%d.example.com", index)
			err := cm.AddDomain(domainName, nil)
			results <- err
		}(i)
	}

	// Collect results
	var errors []error
	for i := 0; i < concurrency; i++ {
		if err := <-results; err != nil {
			errors = append(errors, err)
		}
	}

	// Some operations might fail due to concurrency, but state should be consistent
	domains := cm.GetAllDomains()
	if len(domains) == 0 {
		t.Error("At least some domains should have been added")
	}

	// Verify config file was created successfully
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Error("Config file should exist after concurrent operations")
	}

	t.Logf("Added %d domains with %d errors from %d concurrent operations",
		len(domains), len(errors), concurrency)
}

// MockRestartManager for testing restart failures
type MockRestartManager struct {
	shouldFail bool
	failCount  int
	maxFails   int
}

func (m *MockRestartManager) RestartCoreDNS() error {
	if m.shouldFail && m.failCount < m.maxFails {
		m.failCount++
		return fmt.Errorf("mock restart failure %d", m.failCount)
	}
	return nil
}

func (m *MockRestartManager) RestartCoreDNSWithRollback(backupPath string) error {
	return m.RestartCoreDNS()
}

func (m *MockRestartManager) IsHealthy() bool {
	return !m.shouldFail
}

func (m *MockRestartManager) GetHealthStatus() *HealthStatus {
	return &HealthStatus{
		Healthy:      !m.shouldFail,
		ResponseTime: 50 * time.Millisecond,
		LastCheck:    time.Now(),
		RestartCount: m.failCount,
	}
}

// Setup test configuration
func init() {
	// Initialize test configuration
	config.SetForTest("dns.coredns.restart_timeout", "5s")
	config.SetForTest("dns.coredns.health_check_retries", 3)
	config.SetForTest("dns.coredns.reload_command", []string{})
}
