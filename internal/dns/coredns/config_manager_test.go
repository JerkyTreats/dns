package coredns

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNewConfigManager(t *testing.T) {
	basePath := "/tmp/test-corefile"
	templatePath := "/tmp/test-template"
	domain := "test.example.com"
	zonesPath := "/tmp/zones"

	cm := NewConfigManager(basePath, templatePath, domain, zonesPath)

	if cm == nil {
		t.Fatal("NewConfigManager returned nil")
	}

	if cm.basePath != basePath {
		t.Errorf("Expected basePath %s, got %s", basePath, cm.basePath)
	}

	if cm.templatePath != templatePath {
		t.Errorf("Expected templatePath %s, got %s", templatePath, cm.templatePath)
	}

	if cm.domain != domain {
		t.Errorf("Expected domain %s, got %s", domain, cm.domain)
	}

	if cm.zonesPath != zonesPath {
		t.Errorf("Expected zonesPath %s, got %s", zonesPath, cm.zonesPath)
	}

	if cm.domains == nil {
		t.Error("Expected domains map to be initialized")
	}

	if cm.restartManager == nil {
		t.Error("Expected restartManager to be initialized")
	}
}

func TestConfigManager_AddDomain(t *testing.T) {
	// Create temporary directories
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "Corefile")
	templatePath := filepath.Join(tempDir, "Corefile.template")
	zonesPath := filepath.Join(tempDir, "zones")

	// Create a simple template
	templateContent := `# Test template
. {
    errors
    log
    forward . /etc/resolv.conf
}

{{- if .Domains}}
{{- range .Domains}}
{{- if .Enabled}}
{{.Domain}}:{{.Port}} {
    file {{.ZoneFile}} {{.Domain}}
    errors
    log
}
{{- end}}
{{- end}}
{{- end}}`

	if err := os.WriteFile(templatePath, []byte(templateContent), 0644); err != nil {
		t.Fatalf("Failed to create template file: %v", err)
	}

	// Create zones directory
	if err := os.MkdirAll(zonesPath, 0755); err != nil {
		t.Fatalf("Failed to create zones directory: %v", err)
	}

	// Create a dummy zone file
	zoneFile := filepath.Join(zonesPath, "test.example.com.zone")
	zoneContent := `$ORIGIN test.example.com.
@ IN SOA ns1.test.example.com. admin.test.example.com. (
    2024010101 3600 1800 604800 86400
)
@ IN NS ns1.test.example.com.
@ IN A 192.168.1.1`

	if err := os.WriteFile(zoneFile, []byte(zoneContent), 0644); err != nil {
		t.Fatalf("Failed to create zone file: %v", err)
	}

	cm := NewConfigManager(configPath, templatePath, "test.example.com", zonesPath)

	// Test adding a domain
	domain := "test.example.com"
	err := cm.AddDomain(domain, nil)
	if err != nil {
		t.Fatalf("AddDomain failed: %v", err)
	}

	// Verify domain was added
	domainConfig, exists := cm.GetDomainConfig(domain)
	if !exists {
		t.Fatalf("Domain %s was not added", domain)
	}

	if domainConfig.Domain != domain {
		t.Errorf("Expected domain %s, got %s", domain, domainConfig.Domain)
	}

	if domainConfig.Port != 53 {
		t.Errorf("Expected port 53, got %d", domainConfig.Port)
	}

	if !domainConfig.Enabled {
		t.Error("Expected domain to be enabled")
	}

	// Verify configuration file was generated
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Error("Configuration file was not generated")
	}

	// Read and verify configuration content
	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("Failed to read generated config: %v", err)
	}

	configStr := string(content)
	if !strings.Contains(configStr, domain) {
		t.Errorf("Generated config does not contain domain %s", domain)
	}
}

func TestConfigManager_EnableTLS(t *testing.T) {
	// Create temporary directories
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "Corefile")
	templatePath := filepath.Join(tempDir, "Corefile.template")
	zonesPath := filepath.Join(tempDir, "zones")
	certPath := filepath.Join(tempDir, "cert.pem")
	keyPath := filepath.Join(tempDir, "key.pem")

	// Create template file
	templateContent := `# Test template
. {
    errors
    log
    forward . /etc/resolv.conf
}

{{- if .Domains}}
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
{{- end}}`

	if err := os.WriteFile(templatePath, []byte(templateContent), 0644); err != nil {
		t.Fatalf("Failed to create template file: %v", err)
	}

	// Create zones directory and zone file
	if err := os.MkdirAll(zonesPath, 0755); err != nil {
		t.Fatalf("Failed to create zones directory: %v", err)
	}

	// Create dummy certificate files
	if err := os.WriteFile(certPath, []byte("dummy cert"), 0644); err != nil {
		t.Fatalf("Failed to create cert file: %v", err)
	}

	if err := os.WriteFile(keyPath, []byte("dummy key"), 0644); err != nil {
		t.Fatalf("Failed to create key file: %v", err)
	}

	// Create a dummy zone file
	zoneFile := filepath.Join(zonesPath, "test.example.com.zone")
	if err := os.WriteFile(zoneFile, []byte("dummy zone"), 0644); err != nil {
		t.Fatalf("Failed to create zone file: %v", err)
	}

	cm := NewConfigManager(configPath, templatePath, "test.example.com", zonesPath)

	// First add the domain
	domain := "test.example.com"
	if err := cm.AddDomain(domain, nil); err != nil {
		t.Fatalf("AddDomain failed: %v", err)
	}

	// Enable TLS
	if err := cm.EnableTLS(domain, certPath, keyPath); err != nil {
		t.Fatalf("EnableTLS failed: %v", err)
	}

	// Verify TLS configuration
	domainConfig, exists := cm.GetDomainConfig(domain)
	if !exists {
		t.Fatalf("Domain %s not found", domain)
	}

	if domainConfig.TLSConfig == nil {
		t.Fatal("TLS configuration not set")
	}

	if domainConfig.TLSConfig.CertFile != certPath {
		t.Errorf("Expected cert file %s, got %s", certPath, domainConfig.TLSConfig.CertFile)
	}

	if domainConfig.TLSConfig.KeyFile != keyPath {
		t.Errorf("Expected key file %s, got %s", keyPath, domainConfig.TLSConfig.KeyFile)
	}

	if domainConfig.TLSConfig.Port != 853 {
		t.Errorf("Expected TLS port 853, got %d", domainConfig.TLSConfig.Port)
	}
}

func TestConfigManager_RemoveDomain(t *testing.T) {
	// Create temporary directories
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "Corefile")
	templatePath := filepath.Join(tempDir, "Corefile.template")
	zonesPath := filepath.Join(tempDir, "zones")

	// Create simple template
	templateContent := `# Test template
. {
    errors
    log
    forward . /etc/resolv.conf
}`

	if err := os.WriteFile(templatePath, []byte(templateContent), 0644); err != nil {
		t.Fatalf("Failed to create template file: %v", err)
	}

	// Create zones directory and zone file
	if err := os.MkdirAll(zonesPath, 0755); err != nil {
		t.Fatalf("Failed to create zones directory: %v", err)
	}

	zoneFile := filepath.Join(zonesPath, "test.example.com.zone")
	if err := os.WriteFile(zoneFile, []byte("dummy zone"), 0644); err != nil {
		t.Fatalf("Failed to create zone file: %v", err)
	}

	cm := NewConfigManager(configPath, templatePath, "test.example.com", zonesPath)

	// Add a domain first
	domain := "test.example.com"
	if err := cm.AddDomain(domain, nil); err != nil {
		t.Fatalf("AddDomain failed: %v", err)
	}

	// Verify domain exists
	if _, exists := cm.GetDomainConfig(domain); !exists {
		t.Fatal("Domain was not added")
	}

	// Remove the domain
	if err := cm.RemoveDomain(domain); err != nil {
		t.Fatalf("RemoveDomain failed: %v", err)
	}

	// Verify domain was removed
	if _, exists := cm.GetDomainConfig(domain); exists {
		t.Fatal("Domain was not removed")
	}
}

func TestConfigManager_GetConfigVersion(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "Corefile")
	templatePath := filepath.Join(tempDir, "Corefile.template")
	zonesPath := filepath.Join(tempDir, "zones")

	// Create simple template
	templateContent := `# Test template
. {
    errors
    log
    forward . /etc/resolv.conf
}`

	if err := os.WriteFile(templatePath, []byte(templateContent), 0644); err != nil {
		t.Fatalf("Failed to create template file: %v", err)
	}

	if err := os.MkdirAll(zonesPath, 0755); err != nil {
		t.Fatalf("Failed to create zones directory: %v", err)
	}

	cm := NewConfigManager(configPath, templatePath, "test.example.com", zonesPath)

	// Initial version should be 0
	if version := cm.GetConfigVersion(); version != 0 {
		t.Errorf("Expected initial version 0, got %d", version)
	}

	// Generate config should increment version
	if err := cm.GenerateCorefile(); err != nil {
		t.Fatalf("GenerateCorefile failed: %v", err)
	}

	if version := cm.GetConfigVersion(); version != 1 {
		t.Errorf("Expected version 1 after generation, got %d", version)
	}
}

func TestConfigManager_GetLastGenerated(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "Corefile")
	templatePath := filepath.Join(tempDir, "Corefile.template")
	zonesPath := filepath.Join(tempDir, "zones")

	// Create simple template
	templateContent := `# Test template
. {
    errors
    log
    forward . /etc/resolv.conf
}`

	if err := os.WriteFile(templatePath, []byte(templateContent), 0644); err != nil {
		t.Fatalf("Failed to create template file: %v", err)
	}

	if err := os.MkdirAll(zonesPath, 0755); err != nil {
		t.Fatalf("Failed to create zones directory: %v", err)
	}

	cm := NewConfigManager(configPath, templatePath, "test.example.com", zonesPath)

	// Initial last generated should be zero
	if !cm.GetLastGenerated().IsZero() {
		t.Error("Expected initial last generated to be zero time")
	}

	// Generate config should update last generated
	beforeGenerate := time.Now()
	if err := cm.GenerateCorefile(); err != nil {
		t.Fatalf("GenerateCorefile failed: %v", err)
	}
	afterGenerate := time.Now()

	lastGenerated := cm.GetLastGenerated()
	if lastGenerated.Before(beforeGenerate) || lastGenerated.After(afterGenerate) {
		t.Error("Last generated time not updated correctly")
	}
}
