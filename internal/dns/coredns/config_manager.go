package coredns

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/jerkytreats/dns/internal/logging"
)

// TLSConfig represents TLS configuration for a domain
type TLSConfig struct {
	CertFile string
	KeyFile  string
	Port     int
}

// DomainConfig represents a domain configuration block
type DomainConfig struct {
	Domain    string
	Port      int
	TLSConfig *TLSConfig
	ZoneFile  string
	Enabled   bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

// ConfigManager manages dynamic CoreDNS configuration generation
type ConfigManager struct {
	basePath     string
	templatePath string
	domain       string
	zonesPath    string

	// Domain configurations
	domains      map[string]*DomainConfig
	domainsMutex sync.RWMutex

	// Configuration state
	lastGenerated time.Time
	configVersion int

	// Dependencies
	restartManager RestartManagerInterface
}

// NewConfigManager creates a new ConfigManager instance
func NewConfigManager(basePath, templatePath, domain, zonesPath string) *ConfigManager {
	logging.Info("Creating CoreDNS ConfigManager")

	cm := &ConfigManager{
		basePath:     basePath,
		templatePath: templatePath,
		domain:       domain,
		zonesPath:    zonesPath,
		domains:      make(map[string]*DomainConfig),
	}

	// Initialize restart manager
	cm.restartManager = NewRestartManager()

	return cm
}

// GenerateCorefile generates the complete Corefile from template and domain configurations
func (cm *ConfigManager) GenerateCorefile() error {
	cm.domainsMutex.RLock()
	defer cm.domainsMutex.RUnlock()

	logging.Info("Generating dynamic Corefile from template")

	// Read template file
	templateContent, err := os.ReadFile(cm.templatePath)
	if err != nil {
		logging.Error("Failed to read Corefile template: %v", err)
		return fmt.Errorf("failed to read template: %w", err)
	}

	// Parse template
	tmpl, err := template.New("corefile").Parse(string(templateContent))
	if err != nil {
		logging.Error("Failed to parse Corefile template: %v", err)
		return fmt.Errorf("failed to parse template: %w", err)
	}

	// Prepare template data
	templateData := struct {
		BaseDomain string
		Domains    []*DomainConfig
		ZonesPath  string
	}{
		BaseDomain: cm.domain,
		Domains:    cm.getDomainList(),
		ZonesPath:  cm.zonesPath,
	}

	// Execute template
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, templateData); err != nil {
		logging.Error("Failed to execute Corefile template: %v", err)
		return fmt.Errorf("failed to execute template: %w", err)
	}

	// Add dynamic domain configurations
	dynamicConfig := cm.generateDynamicConfigInternal()
	if dynamicConfig != "" {
		buf.WriteString("\n# Dynamic domain configurations\n")
		buf.WriteString(dynamicConfig)
	}

	// Write to temporary file first for validation
	tempPath := cm.basePath + ".tmp"
	if err := os.WriteFile(tempPath, buf.Bytes(), 0644); err != nil {
		logging.Error("Failed to write temporary Corefile: %v", err)
		return fmt.Errorf("failed to write temporary config: %w", err)
	}

	// Validate configuration syntax (basic validation)
	if err := cm.validateConfig(tempPath); err != nil {
		os.Remove(tempPath)
		logging.Error("Generated Corefile failed validation: %v", err)
		return fmt.Errorf("config validation failed: %w", err)
	}

	// Move temporary file to final location
	if err := os.Rename(tempPath, cm.basePath); err != nil {
		os.Remove(tempPath)
		logging.Error("Failed to move Corefile to final location: %v", err)
		return fmt.Errorf("failed to finalize config: %w", err)
	}

	cm.lastGenerated = time.Now()
	cm.configVersion++

	logging.Info("Successfully generated Corefile (version %d)", cm.configVersion)
	return nil
}

// AddDomain adds a new domain configuration
func (cm *ConfigManager) AddDomain(domain string, tlsConfig *TLSConfig) error {
	cm.domainsMutex.Lock()
	defer cm.domainsMutex.Unlock()

	logging.Info("Adding domain configuration: %s", domain)

	// Check if domain already exists
	if _, exists := cm.domains[domain]; exists {
		logging.Debug("Domain %s already exists, updating configuration", domain)
	}

	// Create domain configuration
	domainConfig := &DomainConfig{
		Domain:    domain,
		Port:      53,
		TLSConfig: tlsConfig,
		ZoneFile:  filepath.Join(cm.zonesPath, domain+".zone"),
		Enabled:   true,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// Set TLS port if TLS is configured
	if tlsConfig != nil {
		domainConfig.Port = tlsConfig.Port
	}

	cm.domains[domain] = domainConfig

	// Regenerate and apply configuration
	if err := cm.applyConfiguration(); err != nil {
		delete(cm.domains, domain)
		return fmt.Errorf("failed to apply configuration after adding domain: %w", err)
	}

	logging.Info("Successfully added domain: %s", domain)
	return nil
}

// RemoveDomain removes a domain configuration
func (cm *ConfigManager) RemoveDomain(domain string) error {
	cm.domainsMutex.Lock()
	defer cm.domainsMutex.Unlock()

	logging.Info("Removing domain configuration: %s", domain)

	// Check if domain exists
	if _, exists := cm.domains[domain]; !exists {
		logging.Debug("Domain %s does not exist, nothing to remove", domain)
		return nil
	}

	// Remove domain
	delete(cm.domains, domain)

	// Regenerate and apply configuration
	if err := cm.applyConfiguration(); err != nil {
		logging.Error("Failed to apply configuration after removing domain: %v", err)
		return fmt.Errorf("failed to apply configuration after removing domain: %w", err)
	}

	logging.Info("Successfully removed domain: %s", domain)
	return nil
}

// EnableTLS enables TLS for a domain
func (cm *ConfigManager) EnableTLS(domain, certPath, keyPath string) error {
	cm.domainsMutex.Lock()
	defer cm.domainsMutex.Unlock()

	logging.Info("Enabling TLS for domain: %s", domain)

	// Check if domain exists
	domainConfig, exists := cm.domains[domain]
	if !exists {
		return fmt.Errorf("domain %s not found", domain)
	}

	// Verify certificate files exist
	if _, err := os.Stat(certPath); os.IsNotExist(err) {
		return fmt.Errorf("certificate file not found: %s", certPath)
	}
	if _, err := os.Stat(keyPath); os.IsNotExist(err) {
		return fmt.Errorf("key file not found: %s", keyPath)
	}

	// Create or update TLS configuration
	tlsConfig := &TLSConfig{
		CertFile: certPath,
		KeyFile:  keyPath,
		Port:     853, // Standard DNS over TLS port
	}

	domainConfig.TLSConfig = tlsConfig
	domainConfig.UpdatedAt = time.Now()

	// Regenerate and apply configuration
	if err := cm.applyConfiguration(); err != nil {
		// Rollback TLS configuration on failure
		domainConfig.TLSConfig = nil
		return fmt.Errorf("failed to apply TLS configuration: %w", err)
	}

	logging.Info("Successfully enabled TLS for domain: %s", domain)
	return nil
}

// DisableTLS disables TLS for a domain
func (cm *ConfigManager) DisableTLS(domain string) error {
	cm.domainsMutex.Lock()
	defer cm.domainsMutex.Unlock()

	logging.Info("Disabling TLS for domain: %s", domain)

	// Check if domain exists
	domainConfig, exists := cm.domains[domain]
	if !exists {
		return fmt.Errorf("domain %s not found", domain)
	}

	// Remove TLS configuration
	domainConfig.TLSConfig = nil
	domainConfig.UpdatedAt = time.Now()

	// Regenerate and apply configuration
	if err := cm.applyConfiguration(); err != nil {
		return fmt.Errorf("failed to apply configuration after disabling TLS: %w", err)
	}

	logging.Info("Successfully disabled TLS for domain: %s", domain)
	return nil
}

// WriteConfig writes the current configuration to file
func (cm *ConfigManager) WriteConfig() error {
	return cm.GenerateCorefile()
}

// RestartCoreDNS restarts the CoreDNS service
func (cm *ConfigManager) RestartCoreDNS() error {
	logging.Info("Restarting CoreDNS service")
	return cm.restartManager.RestartCoreDNS()
}

// GetDomainConfig returns configuration for a specific domain
func (cm *ConfigManager) GetDomainConfig(domain string) (*DomainConfig, bool) {
	cm.domainsMutex.RLock()
	defer cm.domainsMutex.RUnlock()

	domainConfig, exists := cm.domains[domain]
	return domainConfig, exists
}

// GetAllDomains returns all configured domains
func (cm *ConfigManager) GetAllDomains() map[string]*DomainConfig {
	cm.domainsMutex.RLock()
	defer cm.domainsMutex.RUnlock()

	// Return a copy to prevent external modification
	domains := make(map[string]*DomainConfig)
	for k, v := range cm.domains {
		domains[k] = v
	}
	return domains
}

// GetConfigVersion returns the current configuration version
func (cm *ConfigManager) GetConfigVersion() int {
	return cm.configVersion
}

// GetLastGenerated returns the last generation timestamp
func (cm *ConfigManager) GetLastGenerated() time.Time {
	return cm.lastGenerated
}

// SetRestartManager sets the restart manager (for testing)
func (cm *ConfigManager) SetRestartManager(rm RestartManagerInterface) {
	cm.restartManager = rm
}

// applyConfiguration generates and applies the complete configuration
// This method should be called from within a locked context
func (cm *ConfigManager) applyConfiguration() error {
	// Generate configuration without acquiring locks (we're already locked)
	if err := cm.generateCorefileInternal(); err != nil {
		return fmt.Errorf("failed to generate configuration: %w", err)
	}

	// Restart CoreDNS with new configuration
	if err := cm.RestartCoreDNS(); err != nil {
		return fmt.Errorf("failed to restart CoreDNS: %w", err)
	}

	return nil
}

// generateCorefileInternal generates the Corefile without acquiring locks
// This is used internally when locks are already held
func (cm *ConfigManager) generateCorefileInternal() error {
	logging.Info("Generating dynamic Corefile from template")

	// Read template file
	templateContent, err := os.ReadFile(cm.templatePath)
	if err != nil {
		logging.Error("Failed to read Corefile template: %v", err)
		return fmt.Errorf("failed to read template: %w", err)
	}

	// Parse template
	tmpl, err := template.New("corefile").Parse(string(templateContent))
	if err != nil {
		logging.Error("Failed to parse Corefile template: %v", err)
		return fmt.Errorf("failed to parse template: %w", err)
	}

	// Prepare template data
	templateData := struct {
		BaseDomain string
		Domains    []*DomainConfig
		ZonesPath  string
	}{
		BaseDomain: cm.domain,
		Domains:    cm.getDomainListInternal(),
		ZonesPath:  cm.zonesPath,
	}

	// Execute template
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, templateData); err != nil {
		logging.Error("Failed to execute Corefile template: %v", err)
		return fmt.Errorf("failed to execute template: %w", err)
	}

	// Add dynamic domain configurations
	dynamicConfig := cm.generateDynamicConfigInternal()
	if dynamicConfig != "" {
		buf.WriteString("\n# Dynamic domain configurations\n")
		buf.WriteString(dynamicConfig)
	}

	// Write to temporary file first for validation
	tempPath := cm.basePath + ".tmp"
	if err := os.WriteFile(tempPath, buf.Bytes(), 0644); err != nil {
		logging.Error("Failed to write temporary Corefile: %v", err)
		return fmt.Errorf("failed to write temporary config: %w", err)
	}

	// Validate configuration syntax (basic validation)
	if err := cm.validateConfig(tempPath); err != nil {
		os.Remove(tempPath)
		logging.Error("Generated Corefile failed validation: %v", err)
		return fmt.Errorf("config validation failed: %w", err)
	}

	// Move temporary file to final location
	if err := os.Rename(tempPath, cm.basePath); err != nil {
		os.Remove(tempPath)
		logging.Error("Failed to move Corefile to final location: %v", err)
		return fmt.Errorf("failed to finalize config: %w", err)
	}

	cm.lastGenerated = time.Now()
	cm.configVersion++

	logging.Info("Successfully generated Corefile (version %d)", cm.configVersion)
	return nil
}

// generateDynamicConfigInternal generates dynamic domain configuration blocks without locks
func (cm *ConfigManager) generateDynamicConfigInternal() string {
	var configs []string

	for _, domainConfig := range cm.domains {
		if !domainConfig.Enabled {
			continue
		}

		// Generate standard DNS configuration block
		config := fmt.Sprintf(`
%s:%d {
    file %s %s
    errors
    log
}`, domainConfig.Domain, domainConfig.Port, domainConfig.ZoneFile, domainConfig.Domain)

		configs = append(configs, config)

		// Generate TLS configuration block if TLS is enabled
		if domainConfig.TLSConfig != nil {
			tlsConfig := fmt.Sprintf(`
%s:%d {
    tls %s %s
    file %s %s
    errors
    log
}`, domainConfig.Domain, domainConfig.TLSConfig.Port,
				domainConfig.TLSConfig.CertFile, domainConfig.TLSConfig.KeyFile,
				domainConfig.ZoneFile, domainConfig.Domain)

			configs = append(configs, tlsConfig)
		}
	}

	return strings.Join(configs, "\n")
}

// getDomainListInternal returns a list of domain configurations without locks
func (cm *ConfigManager) getDomainListInternal() []*DomainConfig {
	var domains []*DomainConfig
	for _, domainConfig := range cm.domains {
		if domainConfig.Enabled {
			domains = append(domains, domainConfig)
		}
	}
	return domains
}

// getDomainList returns a list of domain configurations for template use
func (cm *ConfigManager) getDomainList() []*DomainConfig {
	var domains []*DomainConfig
	for _, domainConfig := range cm.domains {
		if domainConfig.Enabled {
			domains = append(domains, domainConfig)
		}
	}
	return domains
}

// validateConfig performs basic validation on the generated configuration
func (cm *ConfigManager) validateConfig(configPath string) error {
	// Read the generated configuration
	content, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("failed to read config for validation: %w", err)
	}

	configStr := string(content)

	// Basic syntax validation
	if strings.Contains(configStr, "{{") || strings.Contains(configStr, "}}") {
		return fmt.Errorf("template variables not resolved")
	}

	// Check for balanced braces
	openBraces := strings.Count(configStr, "{")
	closeBraces := strings.Count(configStr, "}")
	if openBraces != closeBraces {
		return fmt.Errorf("unbalanced braces in configuration: %d open, %d close", openBraces, closeBraces)
	}

	// TODO: Add more sophisticated validation (e.g., CoreDNS syntax validation)

	logging.Debug("Configuration validation passed")
	return nil
}
