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

// GenerateCorefile regenerates the Corefile and writes it to disk.
// It is safe for concurrent use; a read lock is taken while rendering.
func (cm *ConfigManager) GenerateCorefile() error {
	cm.domainsMutex.RLock()
	defer cm.domainsMutex.RUnlock()

	rendered, err := cm.renderCorefile()
	if err != nil {
		return err
	}

	if err := cm.validateRendered(rendered); err != nil {
		logging.Error("Rendered Corefile failed validation: %v", err)
		return err
	}

	if err := cm.writeConfigBytes(rendered); err != nil {
		return err
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

// applyConfiguration must be called with the domainsMutex already LOCKED.
// It renders, validates, writes the Corefile and restarts CoreDNS.
func (cm *ConfigManager) applyConfiguration() error {
	rendered, err := cm.renderCorefile()
	if err != nil {
		return err
	}

	if err := cm.validateRendered(rendered); err != nil {
		return err
	}

	if err := cm.writeConfigBytes(rendered); err != nil {
		return err
	}

	cm.lastGenerated = time.Now()
	cm.configVersion++

	// Restart CoreDNS with new configuration
	if err := cm.RestartCoreDNS(); err != nil {
		return fmt.Errorf("failed to restart CoreDNS: %w", err)
	}

	return nil
}

// renderCorefile renders the Corefile using the current state. Callers must
// ensure they hold at least a read lock if concurrent access is possible.
func (cm *ConfigManager) renderCorefile() ([]byte, error) {
	// Read template file
	templateContent, err := os.ReadFile(cm.templatePath)
	if err != nil {
		logging.Error("Failed to read Corefile template: %v", err)
		return nil, fmt.Errorf("failed to read template: %w", err)
	}

	tmpl, err := template.New("corefile").Parse(string(templateContent))
	if err != nil {
		logging.Error("Failed to parse Corefile template: %v", err)
		return nil, fmt.Errorf("failed to parse template: %w", err)
	}

	data := struct {
		BaseDomain  string
		Domains     []*DomainConfig
		ZonesPath   string
		GeneratedAt string
		Version     int
	}{
		BaseDomain:  cm.domain,
		Domains:     cm.getDomainListInternal(),
		ZonesPath:   cm.zonesPath,
		GeneratedAt: time.Now().Format("2006-01-02 15:04:05 MST"),
		Version:     cm.configVersion + 1,
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("failed to execute template: %w", err)
	}

	dyn := cm.generateDynamicConfigInternal()
	if dyn != "" {
		buf.WriteString("\n# Dynamic domain configurations\n")
		buf.WriteString(dyn)
	}

	return buf.Bytes(), nil
}

// writeConfigBytes writes the rendered config to a tmp file then atomically
// moves it to the final location.
func (cm *ConfigManager) writeConfigBytes(b []byte) error {
	tempPath := cm.basePath + ".tmp"
	if err := os.WriteFile(tempPath, b, 0644); err != nil {
		return fmt.Errorf("failed to write temporary config: %w", err)
	}

	if err := os.Rename(tempPath, cm.basePath); err != nil {
		os.Remove(tempPath)
		return fmt.Errorf("failed to finalize config: %w", err)
	}
	return nil
}

// validateRendered performs quick sanity checks on the rendered config.
func (cm *ConfigManager) validateRendered(content []byte) error {
	s := string(content)

	if strings.Contains(s, "{{") || strings.Contains(s, "}}") {
		return fmt.Errorf("template variables not resolved")
	}

	if strings.Count(s, "{") != strings.Count(s, "}") {
		return fmt.Errorf("unbalanced braces in configuration")
	}

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
