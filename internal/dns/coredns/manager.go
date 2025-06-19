package coredns

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sync"

	"github.com/jerkytreats/dns/internal/config"
	"github.com/jerkytreats/dns/internal/logging"
)

const (
	// DNS
	DNSConfigPathKey    = "dns.coredns.config_path"
	DNSZonesPathKey     = "dns.coredns.zones_path"
	DNSReloadCommandKey = "dns.coredns.reload_command"
)

func init() {
	config.RegisterRequiredKey(DNSConfigPathKey)
	config.RegisterRequiredKey(DNSZonesPathKey)
	config.RegisterRequiredKey(DNSReloadCommandKey)
}

var serviceNameRegex = regexp.MustCompile(`^[a-z0-9-]+$`)

// Manager manages CoreDNS configuration and lifecycle.
type Manager struct {
	configPath    string
	zonesPath     string
	reloadCommand []string
	mu            sync.Mutex
}

// NewManager creates a new CoreDNS manager.
func NewManager(configPath, zonesPath string, reloadCommand []string) *Manager {
	return &Manager{
		configPath:    configPath,
		zonesPath:     zonesPath,
		reloadCommand: reloadCommand,
	}
}

// AddZone adds a new zone for a service
func (m *Manager) AddZone(serviceName string) error {
	// Validate service name
	if !serviceNameRegex.MatchString(serviceName) {
		return fmt.Errorf("invalid service name format")
	}

	// Create zone file content
	zoneContent := fmt.Sprintf(`$ORIGIN %s.internal.jerkytreats.dev.
@	3600 IN	SOA ns1.internal.jerkytreats.dev. admin.jerkytreats.dev. (
	2024061601 ; serial
	7200       ; refresh
	3600       ; retry
	1209600    ; expire
	3600       ; minimum
)
@	3600 IN	NS ns1.internal.jerkytreats.dev.
@	3600 IN	A 100.64.0.1
`, serviceName)

	// Ensure zones directory exists
	if err := os.MkdirAll(m.zonesPath, 0755); err != nil {
		return fmt.Errorf("failed to create zones directory: %w", err)
	}

	// Write zone file
	zoneFile := filepath.Join(m.zonesPath, fmt.Sprintf("%s.zone", serviceName))
	if err := os.WriteFile(zoneFile, []byte(zoneContent), 0644); err != nil {
		return fmt.Errorf("failed to write zone file: %w", err)
	}

	// Update CoreDNS configuration
	if err := m.updateConfig(serviceName); err != nil {
		// Rollback: remove zone file if config update fails
		os.Remove(zoneFile)
		return fmt.Errorf("failed to update CoreDNS config: %w", err)
	}

	// Reload CoreDNS
	if err := m.Reload(); err != nil {
		return fmt.Errorf("failed to reload CoreDNS: %w", err)
	}

	return nil
}

// updateConfig updates the CoreDNS configuration to include the new zone
func (m *Manager) updateConfig(serviceName string) error {
	// Read current config
	config, err := os.ReadFile(m.configPath)
	if err != nil {
		return fmt.Errorf("failed to read CoreDNS config: %w", err)
	}

	// Add zone configuration
	zoneConfig := fmt.Sprintf(`
%s.internal.jerkytreats.dev:53 {
    file %s/%s.zone
    errors
    log
}
`, serviceName, m.zonesPath, serviceName)

	// Append new zone config
	newConfig := string(config) + zoneConfig

	// Write updated config
	if err := os.WriteFile(m.configPath, []byte(newConfig), 0644); err != nil {
		return fmt.Errorf("failed to write CoreDNS config: %w", err)
	}

	return nil
}

// AddRecord adds a new DNS record and reloads CoreDNS.
func (m *Manager) AddRecord(name, ip string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	logging.Info("Attempting to add record: name=%s, ip=%s", name, ip)

	// For now, we will just log that the record would be added.
	// Implementation of file writing will come later.
	logging.Info("Record for %s -> %s would be written to zone file.", name, ip)

	return m.Reload()
}

// Reload reloads the CoreDNS configuration.
func (m *Manager) Reload() error {
	if len(m.reloadCommand) == 0 {
		logging.Warn("No reload command configured for CoreDNS.")
		return nil
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

// RemoveZone removes a zone for a service
func (m *Manager) RemoveZone(serviceName string) error {
	// Remove zone file
	zoneFile := filepath.Join(m.zonesPath, fmt.Sprintf("%s.zone", serviceName))
	if err := os.Remove(zoneFile); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove zone file: %w", err)
	}

	// Update CoreDNS configuration
	if err := m.removeFromConfig(serviceName); err != nil {
		return fmt.Errorf("failed to update CoreDNS config: %w", err)
	}

	// Reload CoreDNS
	if err := m.Reload(); err != nil {
		return fmt.Errorf("failed to reload CoreDNS: %w", err)
	}

	return nil
}

// removeFromConfig removes a zone from the CoreDNS configuration
func (m *Manager) removeFromConfig(serviceName string) error {
	// Read current config
	config, err := os.ReadFile(m.configPath)
	if err != nil {
		return fmt.Errorf("failed to read CoreDNS config: %w", err)
	}

	// Remove zone configuration block using regex
	zoneBlockPattern := regexp.MustCompile(fmt.Sprintf(`(?ms)\n%s\.internal\.jerkytreats\.dev:53 \{.*?\}\n`, regexp.QuoteMeta(serviceName)))
	newConfig := zoneBlockPattern.ReplaceAllString(string(config), "\n")

	// Write updated config
	if err := os.WriteFile(m.configPath, []byte(newConfig), 0644); err != nil {
		return fmt.Errorf("failed to write CoreDNS config: %w", err)
	}

	return nil
}

// Record represents a DNS record for the template.
type Record struct {
	Name string
}
