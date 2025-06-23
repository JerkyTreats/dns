package coredns

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sync"
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

// Manager manages CoreDNS configuration and lifecycle.
type Manager struct {
	configPath    string
	zonesPath     string
	reloadCommand []string
	domain        string
	mu            sync.Mutex

	// Dynamic configuration manager
	configManager *ConfigManager
}

// NewManager creates a new CoreDNS manager.
func NewManager(configPath, zonesPath string, reloadCommand []string, domain string) *Manager {
	logging.Info("Creating CoreDNS manager")
	return &Manager{
		configPath:    configPath,
		zonesPath:     zonesPath,
		reloadCommand: reloadCommand,
		domain:        domain,
	}
}

// SetConfigManager integrates the manager with a ConfigManager for dynamic configuration
func (m *Manager) SetConfigManager(cm *ConfigManager) {
	logging.Info("Integrating CoreDNS manager with ConfigManager")
	m.configManager = cm
}

// AddZone adds a new zone for a service
func (m *Manager) AddZone(serviceName string) error {
	logging.Info("Adding zone for service: %s", serviceName)

	if !serviceNameRegex.MatchString(serviceName) {
		return fmt.Errorf("invalid service name format")
	}

	zoneDomain := fmt.Sprintf("%s.%s.", serviceName, m.domain)
	ns := fmt.Sprintf("ns1.%s.", m.domain)
	admin := fmt.Sprintf("admin.%s.", m.domain)

	zoneContent := fmt.Sprintf(`$ORIGIN %s
@	3600 IN	SOA %s %s (
	2024061601 ; serial
	7200       ; refresh
	3600       ; retry
	1209600    ; expire
	3600       ; minimum
)
@	3600 IN	NS %s
@	3600 IN	A 100.64.0.1
`, zoneDomain, ns, admin, ns)

	if err := os.MkdirAll(m.zonesPath, 0755); err != nil {
		return fmt.Errorf("failed to create zones directory: %w", err)
	}

	zoneFile := filepath.Join(m.zonesPath, fmt.Sprintf("%s.zone", serviceName))
	if err := os.WriteFile(zoneFile, []byte(zoneContent), 0644); err != nil {
		return fmt.Errorf("failed to write zone file: %w", err)
	}

	// Require dynamic ConfigManager to be present; legacy path removed.
	if m.configManager == nil {
		os.Remove(zoneFile)
		return fmt.Errorf("config manager not set; legacy direct CoreDNS edits are no longer supported")
	}

	logging.Info("Using ConfigManager to add domain: %s.%s", serviceName, m.domain)
	if err := m.configManager.AddDomain(fmt.Sprintf("%s.%s", serviceName, m.domain), nil); err != nil {
		os.Remove(zoneFile)
		return fmt.Errorf("failed to add domain via ConfigManager: %w", err)
	}

	return nil
}

// AddRecord adds a new DNS record and reloads CoreDNS.
func (m *Manager) AddRecord(serviceName, name, ip string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	logging.Info("Attempting to add record: service=%s, name=%s, ip=%s", serviceName, name, ip)

	zoneFile := filepath.Join(m.zonesPath, fmt.Sprintf("%s.zone", serviceName))

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

// Reload reloads the CoreDNS configuration.
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

// isTestEnvironment checks if we're running in test mode
func (m *Manager) isTestEnvironment() bool {
	// Check if reload command is empty (test configuration) or explicitly set to docker-compose restart
	if len(m.reloadCommand) == 0 {
		return true // Test configuration disables reload commands
	}
	return len(m.reloadCommand) >= 2 && m.reloadCommand[0] == "docker-compose" && m.reloadCommand[1] == "restart"
}

// reloadForTest handles CoreDNS reload in test environment
func (m *Manager) reloadForTest() error {
	// If reload command is empty, rely on CoreDNS file watching (test configuration)
	if len(m.reloadCommand) == 0 {
		logging.Info("No reload command configured - relying on CoreDNS file monitoring")
		return nil
	}

	// If explicit docker-compose restart is configured
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

// RemoveZone removes a zone for a service
func (m *Manager) RemoveZone(serviceName string) error {
	logging.Info("Removing zone for service: %s", serviceName)

	zoneFile := filepath.Join(m.zonesPath, fmt.Sprintf("%s.zone", serviceName))
	if err := os.Remove(zoneFile); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove zone file: %w", err)
	}

	if err := m.removeFromConfig(serviceName); err != nil {
		return fmt.Errorf("failed to update CoreDNS config: %w", err)
	}

	if err := m.Reload(); err != nil {
		return fmt.Errorf("failed to reload CoreDNS: %w", err)
	}

	return nil
}

// removeFromConfig removes a zone from the CoreDNS configuration
func (m *Manager) removeFromConfig(serviceName string) error {
	config, err := os.ReadFile(m.configPath)
	if err != nil {
		logging.Error("Failed to read CoreDNS config for removal: %v", err)
		return fmt.Errorf("failed to read CoreDNS config: %w", err)
	}

	zoneBlockPattern := regexp.MustCompile(fmt.Sprintf(`(?ms)\n%s\.%s:53 \{.*?\}\n`, regexp.QuoteMeta(serviceName), regexp.QuoteMeta(m.domain)))
	newConfig := zoneBlockPattern.ReplaceAllString(string(config), "\n")

	if err := os.WriteFile(m.configPath, []byte(newConfig), 0644); err != nil {
		logging.Error("Failed to write CoreDNS config during removal: %v", err)
		return fmt.Errorf("failed to write CoreDNS config: %w", err)
	}

	return nil
}

// Record represents a DNS record for the template.
type Record struct {
	Name string
}

// zoneExistsInConfig checks if a zone already exists in the CoreDNS configuration
func (m *Manager) zoneExistsInConfig(config, zoneName string) bool {
	pattern := regexp.MustCompile(fmt.Sprintf(`(?m)^%s\s*\{`, regexp.QuoteMeta(zoneName)))
	return pattern.MatchString(config)
}
