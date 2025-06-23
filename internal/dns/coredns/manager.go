package coredns

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
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

	return &Manager{
		reloadCommand:  reloadCommand,
		configPath:     configPath,
		templatePath:   templatePath,
		domain:         domain,
		zonesPath:      zonesPath,
		domains:        make(map[string]*DomainConfig),
		restartManager: NewRestartManager(),
	}
}

// ------------------- Domain Management -------------------- //

// AddDomain registers a new domain in memory and regenerates the Corefile.
func (m *Manager) AddDomain(domain string, tlsConfig *TLSConfig) error {
	m.domainsMutex.Lock()
	defer m.domainsMutex.Unlock()

	logging.Info("Adding domain configuration: %s", domain)

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

	zoneDomain := fmt.Sprintf("%s.%s.", serviceName, m.domain)
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

	zoneFile := filepath.Join(m.zonesPath, fmt.Sprintf("%s.%s.zone", serviceName, m.domain))
	if err := os.WriteFile(zoneFile, []byte(zoneContent), 0644); err != nil {
		return fmt.Errorf("failed to write zone file: %w", err)
	}

	// Register domain so Corefile gets regenerated
	return m.AddDomain(fmt.Sprintf("%s.%s", serviceName, m.domain), nil)
}

// RemoveZone removes the zone file for the given service and updates the Corefile.
func (m *Manager) RemoveZone(serviceName string) error {
	logging.Info("Removing zone for service: %s", serviceName)

	zoneFile := filepath.Join(m.zonesPath, fmt.Sprintf("%s.%s.zone", serviceName, m.domain))
	if err := os.Remove(zoneFile); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove zone file: %w", err)
	}

	return m.RemoveDomain(fmt.Sprintf("%s.%s", serviceName, m.domain))
}

// ------------------- Record helpers -------------------- //

// AddRecord appends an A record to the zone file and triggers a CoreDNS reload (not full restart).
func (m *Manager) AddRecord(serviceName, name, ip string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	logging.Info("Attempting to add record: service=%s, name=%s, ip=%s", serviceName, name, ip)

	zoneFile := filepath.Join(m.zonesPath, fmt.Sprintf("%s.%s.zone", serviceName, m.domain))
	if _, err := os.Stat(zoneFile); os.IsNotExist(err) {
		return fmt.Errorf("zone file for service '%s' does not exist", serviceName)
	}

	recordContent := fmt.Sprintf("%s\tIN\tA\t%s\n", name, ip)

	f, err := os.OpenFile(zoneFile, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open zone file: %w", err)
	}
	defer f.Close()

	if _, err := f.WriteString(recordContent); err != nil {
		return fmt.Errorf("failed to write record to zone file: %w", err)
	}

	logging.Info("Record for %s.%s.%s -> %s added to zone file.", name, serviceName, m.domain, ip)

	return m.Reload()
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
