package proxy

import (
	"bytes"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/jerkytreats/dns/internal/config"
	"github.com/jerkytreats/dns/internal/logging"
	"github.com/jerkytreats/dns/internal/tailscale"
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
	if err := validateFQDN(p.Hostname); err != nil {
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
func NewManager() (*Manager, error) {
	return NewManagerWithReloader(&CaddyReloader{})
}

// NewManagerWithReloader creates a new proxy manager with a custom reloader
func NewManagerWithReloader(reloader Reloader) (*Manager, error) {
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

	return &Manager{
		configPath:   configPath,
		templatePath: templatePath,
		enabled:      true,
		reloader:     reloader,
		port:         port,
		rules:        make(map[string]*ProxyRule),
	}, nil
}

// AddRule adds or updates a reverse proxy rule from ProxyRule
// validateFQDN checks if a hostname is a valid FQDN
func validateFQDN(hostname string) error {
	if hostname == "" {
		return fmt.Errorf("hostname cannot be empty")
	}

	// Check if it contains at least one dot (domain separator)
	if !strings.Contains(hostname, ".") {
		return fmt.Errorf("hostname '%s' is not a valid FQDN - must contain at least one domain separator (.)", hostname)
	}

	// Check if it starts or ends with a dot
	if strings.HasPrefix(hostname, ".") || strings.HasSuffix(hostname, ".") {
		return fmt.Errorf("hostname '%s' is not a valid FQDN - cannot start or end with a dot", hostname)
	}

	// Check for valid characters (letters, digits, hyphens, dots)
	// Must start and end with alphanumeric
	if !isValidFQDN(hostname) {
		return fmt.Errorf("hostname '%s' is not a valid FQDN - contains invalid characters or format", hostname)
	}

	return nil
}

// isValidFQDN checks if a string is a valid FQDN format
func isValidFQDN(hostname string) bool {
	// Must not be empty
	if hostname == "" {
		return false
	}

	// Must contain at least one dot
	if !strings.Contains(hostname, ".") {
		return false
	}

	// Must not start or end with dot
	if strings.HasPrefix(hostname, ".") || strings.HasSuffix(hostname, ".") {
		return false
	}

	// Split by dots and validate each part
	parts := strings.Split(hostname, ".")
	for _, part := range parts {
		// Each part must not be empty
		if part == "" {
			return false
		}

		// Each part must start and end with alphanumeric
		if len(part) == 0 || !isAlphanumeric(part[0]) || !isAlphanumeric(part[len(part)-1]) {
			return false
		}

		// Each part must only contain alphanumeric and hyphens
		for _, char := range part {
			if !isAlphanumericOrHyphen(char) {
				return false
			}
		}
	}

	return true
}

// isAlphanumeric checks if a byte is alphanumeric
func isAlphanumeric(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9')
}

// isAlphanumericOrHyphen checks if a rune is alphanumeric or hyphen
func isAlphanumericOrHyphen(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-'
}

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
	if err := validateFQDN(hostname); err != nil {
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
