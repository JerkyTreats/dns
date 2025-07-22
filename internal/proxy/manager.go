package proxy

import (
	"bytes"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
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

const (
	// Proxy configuration keys
	ProxyConfigPathKey   = "proxy.config_path"
	ProxyTemplatePathKey = "proxy.template_path"
	ProxyEnabledKey      = "proxy.enabled"
)

// Default values for optional configuration keys
const (
	DefaultProxyConfigPath   = "/app/configs/Caddyfile"
	DefaultProxyTemplatePath = "/app/templates/Caddyfile.template"
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

// ProxyConfig represents the template data for Caddyfile generation
type ProxyConfig struct {
	ProxyRules  []*ProxyRule
	GeneratedAt string
	Version     int
}

// Manager manages Caddy reverse proxy configuration and lifecycle
type Manager struct {
	mu sync.RWMutex

	configPath   string
	templatePath string
	enabled      bool

	rules         map[string]*ProxyRule
	configVersion int
	lastGenerated time.Time
}

// NewManager creates a new proxy manager
func NewManager() (*Manager, error) {
	configPath := config.GetString(ProxyConfigPathKey)
	if configPath == "" {
		configPath = DefaultProxyConfigPath
	}

	templatePath := config.GetString(ProxyTemplatePathKey)
	if templatePath == "" {
		templatePath = DefaultProxyTemplatePath
	}

	enabled := DefaultProxyEnabled
	if config.GetString(ProxyEnabledKey) != "" {
		enabled = config.GetBool(ProxyEnabledKey)
	}

	if !enabled {
		logging.Info("Reverse proxy is disabled")
		return &Manager{enabled: false}, nil
	}

	// Ensure config directory exists
	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		return nil, fmt.Errorf("failed to create proxy config directory: %w", err)
	}

	manager := &Manager{
		configPath:   configPath,
		templatePath: templatePath,
		enabled:      enabled,
		rules:        make(map[string]*ProxyRule),
	}

	logging.Info("Proxy manager initialized with config path: %s", configPath)
	return manager, nil
}

// AddRule adds or updates a reverse proxy rule
func (m *Manager) AddRule(hostname, targetIP string, targetPort int) error {
	if !m.enabled {
		logging.Debug("Proxy disabled, skipping rule addition for %s", hostname)
		return nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	logging.Info("Adding proxy rule: %s -> %s:%d", hostname, targetIP, targetPort)

	rule := &ProxyRule{
		Hostname:   hostname,
		TargetIP:   targetIP,
		TargetPort: targetPort,
		Protocol:   "http", // Default to HTTP
		Enabled:    true,
		CreatedAt:  time.Now(),
	}

	m.rules[hostname] = rule

	if err := m.generateConfig(); err != nil {
		delete(m.rules, hostname)
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

// reloadProxy signals Caddy to reload its configuration
func (m *Manager) reloadProxy() error {
	if !m.enabled {
		return nil
	}

	// Use Caddy's reload API endpoint
	cmd := exec.Command("/usr/local/bin/caddy", "reload", "--config", m.configPath)
	if err := cmd.Run(); err != nil {
		logging.Warn("Failed to reload Caddy via API, will restart via supervisor")
		// If reload fails, we could potentially signal supervisord to restart Caddy
		// For now, we'll just log the error and continue
		return fmt.Errorf("failed to reload caddy: %w", err)
	}

	logging.Debug("Successfully reloaded Caddy configuration")
	return nil
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

// SetupAutomaticProxy handles automatic proxy rule creation based on request source
func (m *Manager) SetupAutomaticProxy(r *http.Request, dnsName, dnsManagerIP string, port int, tailscaleClient TailscaleClientInterface) {
	// Extract source IP from request
	sourceIP := getSourceIP(r)
	if sourceIP == "" {
		logging.Warn("Unable to determine source IP - skipping proxy setup")
		return
	}

	if tailscaleClient == nil {
		logging.Warn("Tailscale client not available - skipping automatic proxy setup")
		return
	}

	// Find corresponding Tailscale device
	device, err := tailscaleClient.GetDeviceByIP(sourceIP)
	var targetIP string

	if err != nil {
		logging.Warn("Failed to find device for IP %s: %v - using DNS Manager as fallback", sourceIP, err)
		// Fallback: use DNS Manager's own IP
		targetIP = dnsManagerIP
	} else {
		// Use device's Tailscale IP
		targetIP = tailscaleClient.GetTailscaleIP(device)
		if targetIP == "" {
			logging.Warn("No Tailscale IP found for device %s - using DNS Manager as fallback", device.Name)
			targetIP = dnsManagerIP
		} else {
			logging.Info("Found source device: %s (%s) -> %s", device.Name, device.Hostname, targetIP)
		}
	}

	// Create proxy rule
	if err := m.AddRule(dnsName, targetIP, port); err != nil {
		logging.Error("Failed to add proxy rule: %v", err)
		// Don't fail the entire request, but log the error
		logging.Warn("DNS record created but proxy rule failed - service accessible via DNS only")
	} else {
		logging.Info("Successfully added proxy rule: %s -> %s:%d", dnsName, targetIP, port)
	}
}

// getSourceIP extracts the client IP from HTTP request headers
// Checks X-Forwarded-For, X-Real-IP, and RemoteAddr in that order
func getSourceIP(r *http.Request) string {
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
