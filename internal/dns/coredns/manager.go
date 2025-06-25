package coredns

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/jerkytreats/dns/internal/config"
	"github.com/jerkytreats/dns/internal/logging"
)

const (
	// DNS
	DNSConfigPathKey         = "dns.coredns.config_path"
	DNSTemplatePathKey       = "dns.coredns.template_path"
	DNSZonesPathKey          = "dns.coredns.zones_path"
	DNSReloadCommandKey      = "dns.coredns.reload_command"
	DNSRestartTimeoutKey     = "dns.coredns.restart_timeout"
	DNSHealthCheckRetriesKey = "dns.coredns.health_check_retries"
	DNSDomainKey             = "dns.domain"
)

func init() {
	config.RegisterRequiredKey(DNSConfigPathKey)
	config.RegisterRequiredKey(DNSZonesPathKey)
	config.RegisterRequiredKey(DNSReloadCommandKey)
	config.RegisterRequiredKey(DNSDomainKey)
	// Template path is optional, will use default if not provided
}

var serviceNameRegex = regexp.MustCompile(`^[a-z0-9-]+$`)

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

// Manager manages CoreDNS configuration, zones and lifecycle.
type Manager struct {
	// Operational / reload settings
	reloadCommand []string
	mu            sync.Mutex

	// CoreDNS configuration paths & state
	configPath   string
	templatePath string
	domain       string
	zonesPath    string

	// Domain configurations (was ConfigManager state)
	domains      map[string]*DomainConfig
	domainsMutex sync.RWMutex

	lastGenerated time.Time
	configVersion int

	// Dependencies
	restartManager RestartManagerInterface
}

// NewManager creates a new CoreDNS manager.
// NOTE: this replaces both the old Manager and ConfigManager constructors.
func NewManager(configPath, templatePath, zonesPath string, reloadCommand []string, domain string) *Manager {
	if templatePath == "" {
		templatePath = "configs/coredns/Corefile.template"
	}

	manager := &Manager{
		reloadCommand:  reloadCommand,
		configPath:     configPath,
		templatePath:   templatePath,
		domain:         domain,
		zonesPath:      zonesPath,
		domains:        make(map[string]*DomainConfig),
		restartManager: NewRestartManager(),
	}

	// Load existing domains from Corefile if it exists
	manager.loadExistingDomains()

	return manager
}

// ------------------- Domain Management -------------------- //

// AddDomain registers a new domain in memory and regenerates the Corefile.
func (m *Manager) AddDomain(domain string, tlsConfig *TLSConfig) error {
	m.domainsMutex.Lock()
	defer m.domainsMutex.Unlock()

	logging.Info("Adding domain configuration: %s", domain)

	// Check if domain already exists with same configuration
	if existing, exists := m.domains[domain]; exists {
		// If TLS config is the same, no need to regenerate
		if tlsConfig == nil && existing.TLSConfig == nil {
			logging.Debug("Domain %s already exists with same configuration, skipping", domain)
			return nil
		}
		// If TLS configs are equivalent, no need to regenerate
		if tlsConfig != nil && existing.TLSConfig != nil &&
			tlsConfig.CertFile == existing.TLSConfig.CertFile &&
			tlsConfig.KeyFile == existing.TLSConfig.KeyFile &&
			tlsConfig.Port == existing.TLSConfig.Port {
			logging.Debug("Domain %s already exists with same TLS configuration, skipping", domain)
			return nil
		}
	}

	domainCfg := &DomainConfig{
		Domain:    domain,
		Port:      53,
		TLSConfig: tlsConfig,
		ZoneFile:  filepath.Join(m.zonesPath, domain+".zone"),
		Enabled:   true,
	}

	if tlsConfig != nil {
		domainCfg.Port = tlsConfig.Port
	}

	m.domains[domain] = domainCfg

	if err := m.applyConfiguration(); err != nil {
		delete(m.domains, domain)
		return fmt.Errorf("failed to apply configuration after adding domain: %w", err)
	}

	return nil
}

// RemoveDomain removes an existing domain and regenerates the Corefile.
func (m *Manager) RemoveDomain(domain string) error {
	m.domainsMutex.Lock()
	defer m.domainsMutex.Unlock()

	logging.Info("Removing domain configuration: %s", domain)

	if _, exists := m.domains[domain]; !exists {
		logging.Debug("Domain %s does not exist, nothing to remove", domain)
		return nil
	}

	delete(m.domains, domain)

	if err := m.applyConfiguration(); err != nil {
		return fmt.Errorf("failed to apply configuration after removing domain: %w", err)
	}

	return nil
}

// EnableTLS enables DNS-over-TLS for a domain.
func (m *Manager) EnableTLS(domain, certPath, keyPath string) error {
	m.domainsMutex.Lock()
	defer m.domainsMutex.Unlock()

	logging.Info("Enabling TLS for domain: %s", domain)

	d, ok := m.domains[domain]
	if !ok {
		return fmt.Errorf("domain %s not found", domain)
	}

	if _, err := os.Stat(certPath); os.IsNotExist(err) {
		return fmt.Errorf("certificate file not found: %s", certPath)
	}
	if _, err := os.Stat(keyPath); os.IsNotExist(err) {
		return fmt.Errorf("key file not found: %s", keyPath)
	}

	d.TLSConfig = &TLSConfig{CertFile: certPath, KeyFile: keyPath, Port: 853}

	if err := m.applyConfiguration(); err != nil {
		d.TLSConfig = nil // rollback
		return fmt.Errorf("failed to apply TLS configuration: %w", err)
	}
	return nil
}

// DisableTLS disables DNS-over-TLS for a domain.
func (m *Manager) DisableTLS(domain string) error {
	m.domainsMutex.Lock()
	defer m.domainsMutex.Unlock()

	logging.Info("Disabling TLS for domain: %s", domain)

	d, ok := m.domains[domain]
	if !ok {
		return fmt.Errorf("domain %s not found", domain)
	}

	d.TLSConfig = nil

	if err := m.applyConfiguration(); err != nil {
		return fmt.Errorf("failed to apply configuration after disabling TLS: %w", err)
	}
	return nil
}

// ------------------- Zone helpers (public) -------------------- //

func (m *Manager) AddZone(serviceName string) error {
	logging.Info("Adding zone for service: %s", serviceName)

	if !serviceNameRegex.MatchString(serviceName) {
		return fmt.Errorf("invalid service name format")
	}

	zoneDomain := fmt.Sprintf("%s.", m.domain)
	ns := fmt.Sprintf("ns1.%s.", m.domain)
	admin := fmt.Sprintf("admin.%s.", m.domain)

	zoneContent := fmt.Sprintf(`$ORIGIN %s
@\t3600 IN\tSOA %s %s (
	2024061601 ; serial
	7200       ; refresh
	3600       ; retry
	1209600    ; expire
	3600       ; minimum
)
@\t3600 IN\tNS %s
@\t3600 IN\tA 100.64.0.1
`, zoneDomain, ns, admin, ns)

	if err := os.MkdirAll(m.zonesPath, 0755); err != nil {
		return fmt.Errorf("failed to create zones directory: %w", err)
	}

	zoneFile := filepath.Join(m.zonesPath, fmt.Sprintf("%s.zone", m.domain))
	if err := os.WriteFile(zoneFile, []byte(zoneContent), 0644); err != nil {
		return fmt.Errorf("failed to write zone file: %w", err)
	}

	// Register domain so Corefile gets regenerated
	return m.AddDomain(m.domain, nil)
}

// RemoveZone removes the zone file for the given service and updates the Corefile.
func (m *Manager) RemoveZone(serviceName string) error {
	logging.Info("Removing zone for service: %s", serviceName)

	zoneFile := filepath.Join(m.zonesPath, fmt.Sprintf("%s.zone", m.domain))
	if err := os.Remove(zoneFile); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove zone file: %w", err)
	}

	return m.RemoveDomain(m.domain)
}

// ------------------- Record helpers -------------------- //

// AddRecord upserts an A record in the zone file.
// If a record for the name exists, its IP is updated. Otherwise, a new record is added.
func (m *Manager) AddRecord(serviceName, name, ip string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	logging.Debug("Adding record %s -> %s for service %s", name, ip, serviceName)

	// Note: serviceName helps identify the logical zone, but m.domain defines the file
	zoneFile := filepath.Join(m.zonesPath, fmt.Sprintf("%s.zone", m.domain))

	// Read the existing zone file
	content, err := os.ReadFile(zoneFile)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("zone file for service %s (domain %s) does not exist", serviceName, m.domain)
		}
		return fmt.Errorf("failed to read zone file: %w", err)
	}

	lines := strings.Split(string(content), "\n")
	var newLines []string
	recordExists := false
	recordUpdated := false

	// Use a regex that is more robust to variations in whitespace
	// It looks for a line starting with the name, followed by "IN A", and then an IP.
	// The ^ and $ anchors ensure we match the whole line.
	recordRegex := regexp.MustCompile(fmt.Sprintf(`^%s\s+IN\s+A\s+.*`, regexp.QuoteMeta(name)))

	for _, line := range lines {
		if recordRegex.MatchString(line) {
			recordExists = true
			// If IP is different, update the line
			if !strings.HasSuffix(line, ip) {
				newRecord := fmt.Sprintf("%s\tIN A\t%s", name, ip)
				newLines = append(newLines, newRecord)
				recordUpdated = true
				logging.Debug("Updating existing record for %s", name)
			} else {
				// IP is the same, keep the line as is
				newLines = append(newLines, line)
			}
		} else {
			newLines = append(newLines, line)
		}
	}

	// If the record does not exist, add it to the end.
	if !recordExists {
		newRecord := fmt.Sprintf("%s\tIN A\t%s", name, ip)
		newLines = append(newLines, newRecord)
		recordUpdated = true
		logging.Debug("Adding new record for %s", name)
	}

	// If no changes were made, don't write the file.
	if !recordUpdated {
		logging.Debug("Record for %s already exists and is up to date.", name)
		return nil
	}

	// Write the updated content back to the zone file
	output := strings.Join(newLines, "\n")
	if err := os.WriteFile(zoneFile, []byte(output), 0644); err != nil {
		return fmt.Errorf("failed to write updated zone file: %w", err)
	}

	return nil
}

// DropRecord removes an A record with a specific IP from the zone file.
// This is useful when a device's IP changes.
func (m *Manager) DropRecord(serviceName, name, ip string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	logging.Debug("Dropping record %s -> %s for service %s", name, ip, serviceName)

	zoneFile := filepath.Join(m.zonesPath, fmt.Sprintf("%s.zone", m.domain))

	content, err := os.ReadFile(zoneFile)
	if err != nil {
		if os.IsNotExist(err) {
			logging.Warn("Zone file for service %s not found, nothing to drop.", serviceName)
			return nil
		}
		return fmt.Errorf("failed to read zone file for drop: %w", err)
	}

	lines := strings.Split(string(content), "\n")
	var newLines []string
	dropped := false

	// This regex is specific: it matches the name, "IN A", and the exact IP.
	// It's designed to only remove the record if it matches the expected old IP.
	recordToRemove := fmt.Sprintf("%s\tIN A\t%s", name, ip)

	for _, line := range lines {
		// Trim whitespace for a more reliable comparison
		if strings.TrimSpace(line) != strings.TrimSpace(recordToRemove) {
			newLines = append(newLines, line)
		} else {
			dropped = true
		}
	}

	if !dropped {
		logging.Warn("Record %s -> %s not found in zone, nothing to drop.", name, ip)
		return nil
	}

	output := strings.Join(newLines, "\n")
	if err := os.WriteFile(zoneFile, []byte(output), 0644); err != nil {
		return fmt.Errorf("failed to write updated zone file after dropping record: %w", err)
	}

	return nil
}

// RemoveRecord removes all A records for a given name from the zone file.
func (m *Manager) RemoveRecord(serviceName, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	logging.Debug("Removing all records for name %s in service %s", name, serviceName)

	zoneFile := filepath.Join(m.zonesPath, fmt.Sprintf("%s.zone", m.domain))
	content, err := os.ReadFile(zoneFile)
	if err != nil {
		if os.IsNotExist(err) {
			logging.Warn("Zone file for service %s not found, nothing to remove.", serviceName)
			return nil
		}
		return fmt.Errorf("failed to read zone file for remove: %w", err)
	}

	lines := strings.Split(string(content), "\n")
	var newLines []string
	removed := false

	// This regex is broader: it matches any "IN A" record for the given name.
	recordRegex := regexp.MustCompile(fmt.Sprintf(`^%s\s+IN\s+A\s+.*`, regexp.QuoteMeta(name)))

	for _, line := range lines {
		if !recordRegex.MatchString(line) {
			newLines = append(newLines, line)
		} else {
			removed = true
		}
	}

	if !removed {
		logging.Warn("Record for name %s not found in zone, nothing to remove.", name)
		return nil
	}

	output := strings.Join(newLines, "\n")
	if err := os.WriteFile(zoneFile, []byte(output), 0644); err != nil {
		return fmt.Errorf("failed to write updated zone file after removing record: %w", err)
	}

	return nil
}

// ------------------- CoreDNS reload helpers -------------------- //

// Reload sends a lightweight reload (e.g., SIGHUP or docker-compose restart) when only zone data changes.
func (m *Manager) Reload() error {
	if len(m.reloadCommand) == 0 {
		logging.Warn("No reload command configured for CoreDNS.")
		return nil
	}

	if m.isTestEnvironment() {
		return m.reloadForTest()
	}

	cmd := exec.Command(m.reloadCommand[0], m.reloadCommand[1:]...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	if err := cmd.Run(); err != nil {
		logging.Error("CoreDNS reload failed: %s", out.String())
		return fmt.Errorf("reloading CoreDNS failed: %w: %s", err, out.String())
	}

	logging.Info("CoreDNS reloaded successfully: %s", out.String())
	return nil
}

func (m *Manager) isTestEnvironment() bool {
	if len(m.reloadCommand) == 0 {
		return true
	}
	return len(m.reloadCommand) >= 2 && m.reloadCommand[0] == "docker-compose" && m.reloadCommand[1] == "restart"
}

func (m *Manager) reloadForTest() error {
	if len(m.reloadCommand) == 0 {
		logging.Info("No reload command configured - relying on CoreDNS file monitoring")
		return nil
	}

	cmd := exec.Command("docker-compose", "-f", "docker-compose.test.yml", "restart", "coredns-test")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	if err := cmd.Run(); err != nil {
		logging.Error("CoreDNS restart failed: %s", out.String())
		return fmt.Errorf("restarting CoreDNS container failed: %w: %s", err, out.String())
	}
	logging.Info("CoreDNS container restarted successfully: %s", out.String())
	time.Sleep(5 * time.Second)
	return nil
}

// RestartCoreDNS triggers a full restart via RestartManager (used after Corefile regeneration).
func (m *Manager) RestartCoreDNS() error {
	logging.Info("Restarting CoreDNS service")
	return m.restartManager.RestartCoreDNS()
}

// ------------------- Corefile generation -------------------- //

func (m *Manager) applyConfiguration() error {
	// Check if we need to regenerate the Corefile
	if !m.needsRegeneration() {
		logging.Debug("Corefile is up to date, skipping regeneration")
		return nil
	}

	rendered, err := m.renderCorefile()
	if err != nil {
		return err
	}

	if err := m.validateRendered(rendered); err != nil {
		return err
	}

	if err := m.writeConfigBytes(rendered); err != nil {
		return err
	}

	m.lastGenerated = time.Now()
	m.configVersion++

	return m.RestartCoreDNS()
}

// needsRegeneration checks if the Corefile needs to be regenerated
func (m *Manager) needsRegeneration() bool {
	// If Corefile doesn't exist, we need to generate it
	if _, err := os.Stat(m.configPath); os.IsNotExist(err) {
		logging.Debug("Corefile does not exist, regeneration needed")
		return true
	}

	// Check if template exists
	if _, err := os.Stat(m.templatePath); os.IsNotExist(err) {
		logging.Warn("Corefile template does not exist: %v", err)
		return false // Don't regenerate if template is missing
	}

	// Generate what the new content would be
	newContent, err := m.renderCorefile()
	if err != nil {
		logging.Warn("Failed to render Corefile for comparison: %v", err)
		return true // Regenerate on error to be safe
	}

	// Read existing content
	existingContent, err := os.ReadFile(m.configPath)
	if err != nil {
		logging.Warn("Failed to read existing Corefile for comparison: %v", err)
		return true // Regenerate on error to be safe
	}

	// Compare content
	if bytes.Equal(existingContent, newContent) {
		logging.Debug("Corefile content is identical, no regeneration needed")
		return false
	}

	logging.Debug("Corefile content differs, regeneration needed")
	return true
}

func (m *Manager) renderCorefile() ([]byte, error) {
	templateContent, err := os.ReadFile(m.templatePath)
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
		BaseDomain:  m.domain,
		Domains:     m.getDomainListInternal(),
		ZonesPath:   m.zonesPath,
		GeneratedAt: time.Now().Format("2006-01-02 15:04:05 MST"),
		Version:     m.configVersion + 1,
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("failed to execute template: %w", err)
	}

	return buf.Bytes(), nil
}

func (m *Manager) writeConfigBytes(b []byte) error {
	tempPath := m.configPath + ".tmp"
	if err := os.WriteFile(tempPath, b, 0644); err != nil {
		return fmt.Errorf("failed to write temporary config: %w", err)
	}

	if err := os.Rename(tempPath, m.configPath); err != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("failed to finalize config: %w", err)
	}
	return nil
}

func (m *Manager) validateRendered(content []byte) error {
	s := string(content)
	if strings.Contains(s, "{{") || strings.Contains(s, "}}") {
		return fmt.Errorf("template variables not resolved")
	}
	if strings.Count(s, "{") != strings.Count(s, "}") {
		return fmt.Errorf("unbalanced braces in configuration")
	}
	return nil
}

func (m *Manager) generateDynamicConfigInternal() string {
	var cfgs []string
	for _, d := range m.domains {
		if !d.Enabled {
			continue
		}
		cfg := fmt.Sprintf(`
%s:%d {
	file %s %s
	errors
	log
}`, d.Domain, d.Port, d.ZoneFile, d.Domain)
		cfgs = append(cfgs, cfg)
		if d.TLSConfig != nil {
			tls := fmt.Sprintf(`
%s:%d {
	tls %s %s
	file %s %s
	errors
	log
}`, d.Domain, d.TLSConfig.Port, d.TLSConfig.CertFile, d.TLSConfig.KeyFile, d.ZoneFile, d.Domain)
			cfgs = append(cfgs, tls)
		}
	}
	return strings.Join(cfgs, "\n")
}

func (m *Manager) getDomainListInternal() []*DomainConfig {
	var list []*DomainConfig
	for _, d := range m.domains {
		if d.Enabled {
			list = append(list, d)
		}
	}
	return list
}

// ------------------- Misc helpers -------------------- //

// zoneExistsInConfig is retained for test compatibility.
func (m *Manager) zoneExistsInConfig(config, zoneName string) bool {
	pattern := regexp.MustCompile(fmt.Sprintf(`(?m)^%s\s*\{`, regexp.QuoteMeta(zoneName)))
	return pattern.MatchString(config)
}

func (m *Manager) loadExistingDomains() {
	// Check if Corefile exists
	if _, err := os.Stat(m.configPath); os.IsNotExist(err) {
		logging.Debug("Corefile does not exist, no existing domains to load")
		return
	}

	// Read existing Corefile
	content, err := os.ReadFile(m.configPath)
	if err != nil {
		logging.Warn("Failed to read existing Corefile for domain loading: %v", err)
		return
	}

	// Parse domains from Corefile content
	domains := m.parseDomainsFromCorefile(string(content))

	m.domainsMutex.Lock()
	defer m.domainsMutex.Unlock()

	for domain, config := range domains {
		m.domains[domain] = config
		logging.Debug("Loaded existing domain: %s", domain)
	}

	if len(domains) > 0 {
		logging.Info("Loaded %d existing domains from Corefile", len(domains))
	}
}

// parseDomainsFromCorefile extracts domain configurations from Corefile content
func (m *Manager) parseDomainsFromCorefile(content string) map[string]*DomainConfig {
	domains := make(map[string]*DomainConfig)

	// Simple regex to match domain blocks
	// This looks for patterns like "domain:port {" or "domain {"
	domainRegex := regexp.MustCompile(`^([^:]+)(?::(\d+))?\s*\{`)
	tlsRegex := regexp.MustCompile(`tls\s+([^\s]+)\s+([^\s]+)`)

	lines := strings.Split(content, "\n")
	for i, line := range lines {
		line = strings.TrimSpace(line)

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Check for domain block start
		matches := domainRegex.FindStringSubmatch(line)
		if len(matches) >= 2 {
			domain := strings.TrimSpace(matches[1])
			port := 53 // default port

			if len(matches) >= 3 && matches[2] != "" {
				if p, err := strconv.Atoi(matches[2]); err == nil {
					port = p
				}
			}

			// Look ahead for TLS configuration
			var tlsConfig *TLSConfig
			for j := i + 1; j < len(lines) && !strings.Contains(lines[j], "}"); j++ {
				tlsMatches := tlsRegex.FindStringSubmatch(lines[j])
				if len(tlsMatches) >= 3 {
					tlsConfig = &TLSConfig{
						CertFile: tlsMatches[1],
						KeyFile:  tlsMatches[2],
						Port:     port,
					}
					break
				}
			}

			domains[domain] = &DomainConfig{
				Domain:    domain,
				Port:      port,
				TLSConfig: tlsConfig,
				ZoneFile:  filepath.Join(m.zonesPath, domain+".zone"),
				Enabled:   true,
			}
		}
	}

	return domains
}
