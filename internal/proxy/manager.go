package proxy

import (
	"bytes"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/jerkytreats/dns/internal/config"
	"github.com/jerkytreats/dns/internal/logging"
	"github.com/jerkytreats/dns/internal/tailscale"
	"github.com/jerkytreats/dns/pkg/validation"
)

// TailscaleClientInterface defines the minimal interface needed for automatic proxy setup
type TailscaleClientInterface interface {
	GetDeviceByIP(ip string) (*tailscale.Device, error)
	GetTailscaleIP(device *tailscale.Device) string
}

// Reloader interface for dependency injection
type Reloader interface {
	Reload(configPath string) error
}

// CaddyReloader implements Reloader for actual Caddy reloads
type CaddyReloader struct{}

func (c *CaddyReloader) Reload(configPath string) error {
	// Caddy admin API is disabled, so use supervisord to reload configuration
	// First reread the configuration
	rereadCmd := exec.Command("supervisorctl", "reread")
	if err := rereadCmd.Run(); err != nil {
		return fmt.Errorf("failed to reread supervisor configuration: %w", err)
	}

	// Then update to restart applications with changed configuration
	updateCmd := exec.Command("supervisorctl", "update")
	if err := updateCmd.Run(); err != nil {
		return fmt.Errorf("failed to update supervisor configuration: %w", err)
	}

	logging.Debug("Successfully reloaded Caddy configuration via supervisord")
	return nil
}

const (
	// Proxy configuration keys
	ProxyConfigPathKey   = "proxy.config_path"
	ProxyTemplatePathKey = "proxy.template_path"
	ProxyEnabledKey      = "proxy.enabled"
)

// Default values for optional configuration keys
const (
	DefaultProxyConfigPath   = "/app/configs/Caddyfile"
	DefaultProxyTemplatePath = "/etc/caddy/Caddyfile.template"
	DefaultProxyEnabled      = true
)

// ProxyRule represents a single reverse proxy rule
type ProxyRule struct {
	Hostname   string    `json:"hostname"`    // llm.internal.jerkytreats.dev
	TargetIP   string    `json:"target_ip"`   // 100.2.2.2
	TargetPort int       `json:"target_port"` // 8080
	Protocol   string    `json:"protocol"`    // http/https
	Enabled    bool      `json:"enabled"`
	CreatedAt  time.Time `json:"created_at"`
}

// Validate checks if the ProxyRule is valid
func (p *ProxyRule) Validate() error {
	if err := validation.ValidateFQDN(p.Hostname); err != nil {
		return fmt.Errorf("invalid hostname: %w", err)
	}

	if p.TargetIP == "" {
		return fmt.Errorf("target IP cannot be empty")
	}

	if p.TargetPort <= 0 || p.TargetPort > 65535 {
		return fmt.Errorf("target port must be between 1 and 65535, got %d", p.TargetPort)
	}

	if p.Protocol != "http" && p.Protocol != "https" {
		return fmt.Errorf("protocol must be 'http' or 'https', got '%s'", p.Protocol)
	}

	return nil
}

// NewProxyRule creates a new ProxyRule with validation
func NewProxyRule(hostname, targetIP string, targetPort int, protocol string) (*ProxyRule, error) {
	rule := &ProxyRule{
		Hostname:   hostname,
		TargetIP:   targetIP,
		TargetPort: targetPort,
		Protocol:   protocol,
		Enabled:    true,
		CreatedAt:  time.Now(),
	}

	if err := rule.Validate(); err != nil {
		return nil, fmt.Errorf("failed to create proxy rule: %w", err)
	}

	return rule, nil
}

// ProxyConfig represents the template data for Caddyfile generation
type ProxyConfig struct {
	ProxyRules  []*ProxyRule
	GeneratedAt string
	Version     int
	Port        string
}

// ProxyManagerInterface defines the interface for proxy management
type ProxyManagerInterface interface {
	AddRule(proxyRule *ProxyRule) error
	RemoveRule(hostname string) error
	ListRules() []*ProxyRule
	IsEnabled() bool
	GetStats() map[string]interface{}
	RestoreFromStorage() error
}

// Manager manages reverse proxy rules and configuration
type Manager struct {
	mu sync.RWMutex

	configPath   string
	templatePath string
	enabled      bool
	reloader     Reloader
	port         string

	rules         map[string]*ProxyRule
	configVersion int
	lastGenerated time.Time
}

// Ensure Manager implements ProxyManagerInterface
var _ ProxyManagerInterface = (*Manager)(nil)

// NewManager creates a new proxy manager
// If reloader is nil, a default CaddyReloader will be used
func NewManager(reloader Reloader) (*Manager, error) {
	if reloader == nil {
		reloader = &CaddyReloader{}
	}

	configPath := config.GetString("proxy.caddy.config_path")
	if configPath == "" {
		configPath = DefaultProxyConfigPath
	}

	templatePath := config.GetString("proxy.caddy.template_path")
	if templatePath == "" {
		templatePath = DefaultProxyTemplatePath
	}

	port := config.GetString("proxy.caddy.port")
	if port == "" {
		port = "80"
	}

	enabled := config.GetBool("proxy.enabled")
	if !enabled {
		logging.Info("Reverse proxy is disabled")
		return &Manager{
			configPath:   configPath,
			templatePath: templatePath,
			enabled:      false,
			reloader:     reloader,
			port:         port,
			rules:        make(map[string]*ProxyRule),
		}, nil
	}

	logging.Info("Proxy manager initialized with config path: %s, port: %s", configPath, port)

	manager := &Manager{
		configPath:   configPath,
		templatePath: templatePath,
		enabled:      true,
		reloader:     reloader,
		port:         port,
		rules:        make(map[string]*ProxyRule),
	}

	// Restore proxy rules from storage (Caddyfile) during initialization
	logging.Info("Restoring proxy rules from storage...")
	if err := manager.RestoreFromStorage(); err != nil {
		logging.Warn("Failed to restore proxy rules from storage: %v", err)
		logging.Warn("Continuing with empty proxy rules...")
	} else {
		logging.Info("Proxy rules restored successfully from storage")
	}

	return manager, nil
}

// AddRule adds or updates a reverse proxy rule from ProxyRule
func (m *Manager) AddRule(proxyRule *ProxyRule) error {
	if !m.enabled {
		logging.Debug("Proxy disabled, skipping rule addition for %s", proxyRule.Hostname)
		return nil
	}

	// Validate the entire ProxyRule struct
	if err := proxyRule.Validate(); err != nil {
		return fmt.Errorf("invalid proxy rule: %w", err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	logging.Info("Adding proxy rule: %s -> %s:%d", proxyRule.Hostname, proxyRule.TargetIP, proxyRule.TargetPort)

	m.rules[proxyRule.Hostname] = proxyRule

	if err := m.generateConfig(); err != nil {
		delete(m.rules, proxyRule.Hostname)
		return fmt.Errorf("failed to generate proxy config: %w", err)
	}

	if err := m.reloadProxy(); err != nil {
		return fmt.Errorf("failed to reload proxy: %w", err)
	}

	return nil
}

// RemoveRule removes a reverse proxy rule
func (m *Manager) RemoveRule(hostname string) error {
	if !m.enabled {
		logging.Debug("Proxy disabled, skipping rule removal for %s", hostname)
		return nil
	}

	// Validate FQDN before proceeding
	if err := validation.ValidateFQDN(hostname); err != nil {
		return fmt.Errorf("invalid proxy rule hostname: %w", err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	logging.Info("Removing proxy rule for: %s", hostname)

	if _, exists := m.rules[hostname]; !exists {
		logging.Debug("Proxy rule for %s does not exist", hostname)
		return nil
	}

	delete(m.rules, hostname)

	if err := m.generateConfig(); err != nil {
		return fmt.Errorf("failed to generate proxy config: %w", err)
	}

	if err := m.reloadProxy(); err != nil {
		return fmt.Errorf("failed to reload proxy: %w", err)
	}

	return nil
}

// ListRules returns all current proxy rules
func (m *Manager) ListRules() []*ProxyRule {
	if !m.enabled {
		return []*ProxyRule{}
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	rules := make([]*ProxyRule, 0, len(m.rules))
	for _, rule := range m.rules {
		rules = append(rules, rule)
	}

	return rules
}

// generateConfig generates the Caddyfile from template
func (m *Manager) generateConfig() error {
	if !m.enabled {
		return nil
	}

	// Read template file
	templateContent, err := os.ReadFile(m.templatePath)
	if err != nil {
		return fmt.Errorf("failed to read proxy template: %w", err)
	}

	// Parse template
	tmpl, err := template.New("caddyfile").Parse(string(templateContent))
	if err != nil {
		return fmt.Errorf("failed to parse proxy template: %w", err)
	}

	// Prepare template data
	activeRules := make([]*ProxyRule, 0)
	for _, rule := range m.rules {
		if rule.Enabled {
			activeRules = append(activeRules, rule)
		}
	}

	templateData := ProxyConfig{
		ProxyRules:  activeRules,
		GeneratedAt: time.Now().Format(time.RFC3339),
		Version:     m.configVersion + 1,
		Port:        m.port,
	}

	// Execute template
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, templateData); err != nil {
		return fmt.Errorf("failed to execute proxy template: %w", err)
	}

	// Write config file
	if err := os.WriteFile(m.configPath, buf.Bytes(), 0644); err != nil {
		return fmt.Errorf("failed to write proxy config: %w", err)
	}

	m.configVersion++
	m.lastGenerated = time.Now()

	logging.Debug("Generated proxy config with %d rules", len(activeRules))
	return nil
}

// reloadProxy signals the reloader to reload configuration
func (m *Manager) reloadProxy() error {
	if !m.enabled {
		return nil
	}

	return m.reloader.Reload(m.configPath)
}

// IsEnabled returns whether the proxy manager is enabled
func (m *Manager) IsEnabled() bool {
	return m.enabled
}

// GetStats returns basic statistics about the proxy manager
func (m *Manager) GetStats() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return map[string]interface{}{
		"enabled":        m.enabled,
		"total_rules":    len(m.rules),
		"config_version": m.configVersion,
		"last_generated": m.lastGenerated.Format(time.RFC3339),
	}
}

// GetSourceIP extracts the client IP from HTTP request headers
// Checks X-Forwarded-For, X-Real-IP, and RemoteAddr in that order
func GetSourceIP(r *http.Request) string {
	// Check X-Forwarded-For header (handles multiple proxies)
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// X-Forwarded-For can contain multiple IPs, take the first one
		ips := strings.Split(xff, ",")
		if len(ips) > 0 {
			ip := strings.TrimSpace(ips[0])
			if ip != "" {
				logging.Debug("Source IP from X-Forwarded-For: %s", ip)
				return ip
			}
		}
	}

	// Check X-Real-IP header (simpler proxy setup)
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		ip := strings.TrimSpace(xri)
		if ip != "" {
			logging.Debug("Source IP from X-Real-IP: %s", ip)
			return ip
		}
	}

	// Fall back to RemoteAddr (direct connection)
	if r.RemoteAddr != "" {
		// RemoteAddr is in format "IP:port", extract just the IP
		ip := strings.Split(r.RemoteAddr, ":")[0]
		logging.Debug("Source IP from RemoteAddr: %s", ip)
		return ip
	}

	logging.Warn("Unable to determine source IP from request")
	return ""
}

// RestoreFromStorage restores proxy rules from the persisted Caddyfile
// This method reads the existing Caddyfile and extracts proxy rules to populate the in-memory rules map
func (m *Manager) RestoreFromStorage() error {
	if !m.enabled {
		logging.Debug("Proxy disabled, skipping storage restoration")
		return nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	logging.Info("Restoring proxy rules from Caddyfile: %s", m.configPath)

	// Check if Caddyfile exists
	if _, err := os.Stat(m.configPath); os.IsNotExist(err) {
		logging.Debug("Caddyfile does not exist, no rules to restore")
		return nil
	}

	// Read existing Caddyfile
	content, err := os.ReadFile(m.configPath)
	if err != nil {
		return fmt.Errorf("failed to read Caddyfile: %w", err)
	}

	// Parse proxy rules from Caddyfile content
	rules, err := m.parseRulesFromCaddyfile(string(content))
	if err != nil {
		return fmt.Errorf("failed to parse rules from Caddyfile: %w", err)
	}

	// Clear existing rules and populate with restored rules
	m.rules = make(map[string]*ProxyRule)
	for _, rule := range rules {
		m.rules[rule.Hostname] = rule
	}

	logging.Info("Successfully restored %d proxy rules from Caddyfile", len(rules))
	return nil
}

// parseRulesFromCaddyfile extracts proxy rules from Caddyfile content
// This is a simplified parser that looks for reverse_proxy directives
func (m *Manager) parseRulesFromCaddyfile(content string) ([]*ProxyRule, error) {
	var rules []*ProxyRule

	// Split content into lines for parsing
	lines := strings.Split(content, "\n")
	currentHostname := ""

	for i, line := range lines {
		line = strings.TrimSpace(line)

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Look for hostname blocks (lines ending with {)
		if strings.HasSuffix(line, " {") {
			currentHostname = strings.TrimSuffix(line, " {")
			continue
		}

		// Look for reverse_proxy directives
		if strings.HasPrefix(line, "reverse_proxy") && currentHostname != "" {
			// Parse reverse_proxy line: "reverse_proxy http://100.64.1.10:8080"
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				target := parts[1]

				// Parse target URL to extract IP and port
				targetIP, targetPort, protocol, err := m.parseTargetURL(target)
				if err != nil {
					logging.Warn("Failed to parse target URL %s on line %d: %v", target, i+1, err)
					continue
				}

				rule := &ProxyRule{
					Hostname:   currentHostname,
					TargetIP:   targetIP,
					TargetPort: targetPort,
					Protocol:   protocol,
					Enabled:    true,
					CreatedAt:  time.Now(), // We don't have original creation time
				}

				rules = append(rules, rule)
				logging.Debug("Parsed proxy rule: %s -> %s:%d", currentHostname, targetIP, targetPort)
			}
		}

		// Reset hostname when block ends
		if line == "}" {
			currentHostname = ""
		}
	}

	return rules, nil
}

// parseTargetURL parses a target URL and extracts IP, port, and protocol
// Supports formats like: http://100.64.1.10:8080, https://100.64.1.10:443
func (m *Manager) parseTargetURL(target string) (string, int, string, error) {
	// Default values
	protocol := "http"
	port := 80

	// Remove protocol prefix
	if strings.HasPrefix(target, "https://") {
		protocol = "https"
		port = 443
		target = strings.TrimPrefix(target, "https://")
	} else if strings.HasPrefix(target, "http://") {
		protocol = "http"
		port = 80
		target = strings.TrimPrefix(target, "http://")
	}

	// Split IP and port
	parts := strings.Split(target, ":")
	if len(parts) < 1 {
		return "", 0, "", fmt.Errorf("invalid target format: %s", target)
	}

	ip := parts[0]
	if len(parts) >= 2 {
		var err error
		port, err = strconv.Atoi(parts[1])
		if err != nil {
			return "", 0, "", fmt.Errorf("invalid port in target: %s", parts[1])
		}
	}

	return ip, port, protocol, nil
}
