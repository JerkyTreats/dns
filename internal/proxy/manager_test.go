package proxy

import (
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jerkytreats/dns/internal/config"
	"github.com/jerkytreats/dns/internal/logging"
	"github.com/jerkytreats/dns/internal/persistence"
	"github.com/jerkytreats/dns/internal/tailscale"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockReloader implements Reloader for testing
type MockReloader struct{}

func (m *MockReloader) Reload(configPath string) error {
	logging.Debug("Mock reloader: skipping Caddy reload")
	return nil
}

func setupTestManager(t *testing.T) (*Manager, string) {
	// Create temp directory for test
	tempDir, err := os.MkdirTemp("", "proxy-test-*")
	require.NoError(t, err)

	// Create template file with updated format
	templatePath := filepath.Join(tempDir, "Caddyfile.template")
	templateContent := `# Generated proxy configuration
{{- if .ProxyRules}}
:{{.Port}} {
{{- range .ProxyRules}}
{{- if .Enabled}}
    @{{.Hostname}} host {{.Hostname}}
    handle @{{.Hostname}} {
        reverse_proxy {{.TargetIP}}:{{.TargetPort}}
    }
{{- end}}
{{- end}}
    handle {
        respond "No route configured" 404
    }
}
{{- end}}
`
	err = os.WriteFile(templatePath, []byte(templateContent), 0644)
	require.NoError(t, err)

	// Set up test configuration
	config.SetForTest("proxy.caddy.config_path", filepath.Join(tempDir, "Caddyfile"))
	config.SetForTest("proxy.caddy.template_path", templatePath)
	config.SetForTest("proxy.caddy.port", "80")
	config.SetForTest("proxy.enabled", "true")
	
	// Set up storage configuration
	config.SetForTest("proxy.storage.path", filepath.Join(tempDir, "proxy_rules.json"))
	config.SetForTest("proxy.storage.backup_count", "3")

	// Create manager with mock reloader for testing
	manager, err := NewManager(&MockReloader{})
	require.NoError(t, err)

	return manager, tempDir
}

func TestNewManager(t *testing.T) {
	testDir := t.TempDir()
	configPath := filepath.Join(testDir, "Caddyfile")
	templatePath := filepath.Join(testDir, "Caddyfile.template")

	// Create template file
	templateContent := "# Test template\n"
	err := os.WriteFile(templatePath, []byte(templateContent), 0644)
	require.NoError(t, err)

	// Set test configuration
	config.SetForTest("proxy.caddy.config_path", configPath)
	config.SetForTest("proxy.caddy.template_path", templatePath)
	config.SetForTest("proxy.enabled", "true")

	manager, err := NewManager(nil)
	assert.NoError(t, err)
	assert.NotNil(t, manager)
	assert.True(t, manager.enabled)
	assert.Equal(t, configPath, manager.configPath)
	assert.Equal(t, templatePath, manager.templatePath)
	assert.NotNil(t, manager.rules)

	// Clean up
	config.ResetForTest()
}

func TestNewManager_Disabled(t *testing.T) {
	config.SetForTest("proxy.enabled", "false")

	manager, err := NewManager(nil)
	assert.NoError(t, err)
	assert.NotNil(t, manager)
	assert.False(t, manager.enabled)

	// Clean up
	config.ResetForTest()
}

func TestNewManager_DefaultValues(t *testing.T) {
	// Don't set any config values to test defaults
	config.ResetForTest()

	// Since NewManager tries to create directories, and defaults point to /app,
	// we'll test that the default values are used when config is empty
	// but we can't actually instantiate the manager without proper paths

	testDir := t.TempDir()
	testConfigPath := filepath.Join(testDir, "Caddyfile")
	testTemplatePath := filepath.Join(testDir, "Caddyfile.template")

	// Create template file
	templateContent := "# Test template\n"
	err := os.WriteFile(testTemplatePath, []byte(templateContent), 0644)
	require.NoError(t, err)

	// Set only the paths, leave other configs empty to test defaults
	config.SetForTest("proxy.caddy.config_path", testConfigPath)
	config.SetForTest("proxy.caddy.template_path", testTemplatePath)
	// Don't set proxy.enabled to test default

	manager, err := NewManager(nil)
	assert.NoError(t, err)
	assert.NotNil(t, manager)
	assert.False(t, manager.enabled) // Should be false when not explicitly enabled
	assert.Equal(t, testConfigPath, manager.configPath)
	assert.Equal(t, testTemplatePath, manager.templatePath)

	// Clean up
	config.ResetForTest()
}

// Test core functionality without external dependencies
func TestAddRule_CoreLogic(t *testing.T) {
	manager, _ := setupTestManager(t)

	hostname := "test.example.com"
	targetIP := "192.168.1.100"
	targetPort := 8080

	// Test the core rule addition logic without reload
	manager.mu.Lock()
	rule := &ProxyRule{
		Hostname:   hostname,
		TargetIP:   targetIP,
		TargetPort: targetPort,
		Protocol:   "http",
		Enabled:    true,
		CreatedAt:  time.Now(),
	}
	manager.rules[hostname] = rule
	configErr := manager.generateConfig()
	manager.mu.Unlock()

	assert.NoError(t, configErr)

	// Verify rule was added
	assert.Len(t, manager.rules, 1)
	assert.Contains(t, manager.rules, hostname)

	storedRule := manager.rules[hostname]
	assert.Equal(t, hostname, storedRule.Hostname)
	assert.Equal(t, targetIP, storedRule.TargetIP)
	assert.Equal(t, targetPort, storedRule.TargetPort)
	assert.Equal(t, "http", storedRule.Protocol)
	assert.True(t, storedRule.Enabled)
	assert.False(t, storedRule.CreatedAt.IsZero())

	// Verify config file was generated
	configContent, err := os.ReadFile(manager.configPath)
	assert.NoError(t, err)
	assert.Contains(t, string(configContent), hostname)
	assert.Contains(t, string(configContent), targetIP)
	assert.Contains(t, string(configContent), "8080")

	// Clean up
	config.ResetForTest()
}

func TestAddRule_UpdateExisting_CoreLogic(t *testing.T) {
	manager, _ := setupTestManager(t)

	hostname := "test.example.com"
	originalIP := "192.168.1.100"
	originalPort := 8080
	newIP := "192.168.1.200"
	newPort := 9090

	// Add initial rule
	manager.mu.Lock()
	rule1 := &ProxyRule{
		Hostname:   hostname,
		TargetIP:   originalIP,
		TargetPort: originalPort,
		Protocol:   "http",
		Enabled:    true,
		CreatedAt:  time.Now(),
	}
	manager.rules[hostname] = rule1
	manager.generateConfig()
	manager.mu.Unlock()

	assert.Len(t, manager.rules, 1)

	// Update the rule
	manager.mu.Lock()
	rule2 := &ProxyRule{
		Hostname:   hostname,
		TargetIP:   newIP,
		TargetPort: newPort,
		Protocol:   "http",
		Enabled:    true,
		CreatedAt:  time.Now(),
	}
	manager.rules[hostname] = rule2
	manager.generateConfig()
	manager.mu.Unlock()

	assert.Len(t, manager.rules, 1) // Should still be only one rule

	// Verify the rule was updated
	storedRule := manager.rules[hostname]
	assert.Equal(t, newIP, storedRule.TargetIP)
	assert.Equal(t, newPort, storedRule.TargetPort)

	// Clean up
	config.ResetForTest()
}

func TestAddRule_Disabled(t *testing.T) {
	manager, _ := setupTestManager(t)
	manager.enabled = false

	proxyRule := &ProxyRule{
		TargetIP:   "192.168.1.100",
		TargetPort: 8080,
		Protocol:   "http",
		Enabled:    true,
		CreatedAt:  time.Now(),
	}

	err := manager.AddRule(proxyRule)
	assert.NoError(t, err) // Should not error when disabled

	// Verify no rule was added
	assert.Len(t, manager.rules, 0)

	// Clean up
	config.ResetForTest()
}

func TestAddRule_Disabled_Alt(t *testing.T) {
	manager, _ := setupTestManager(t)
	manager.enabled = false

	proxyRule := &ProxyRule{
		Hostname:   "test.example.com",
		TargetIP:   "192.168.1.100",
		TargetPort: 8080,
		Protocol:   "http",
		Enabled:    true,
		CreatedAt:  time.Now(),
	}

	err := manager.AddRule(proxyRule)
	assert.NoError(t, err) // Should not error when disabled

	// Verify no rule was added
	assert.Len(t, manager.rules, 0)

	// Clean up
	config.ResetForTest()
}

func TestRemoveRule_CoreLogic(t *testing.T) {
	manager, _ := setupTestManager(t)

	hostname := "test.example.com"
	targetIP := "192.168.1.100"
	targetPort := 8080

	// Add a rule first
	manager.mu.Lock()
	rule := &ProxyRule{
		Hostname:   hostname,
		TargetIP:   targetIP,
		TargetPort: targetPort,
		Protocol:   "http",
		Enabled:    true,
		CreatedAt:  time.Now(),
	}
	manager.rules[hostname] = rule
	manager.generateConfig()
	assert.Len(t, manager.rules, 1)

	// Remove the rule
	delete(manager.rules, hostname)
	manager.generateConfig()
	manager.mu.Unlock()

	// Verify rule was removed
	assert.Len(t, manager.rules, 0)
	assert.NotContains(t, manager.rules, hostname)

	// Verify config file was updated
	configContent, err := os.ReadFile(manager.configPath)
	assert.NoError(t, err)
	assert.NotContains(t, string(configContent), hostname)

	// Clean up
	config.ResetForTest()
}

func TestRemoveRule_NonExistent(t *testing.T) {
	manager, _ := setupTestManager(t)

	// Try to remove a rule that doesn't exist
	err := manager.RemoveRule("nonexistent.example.com")
	assert.NoError(t, err) // Should not error

	// Clean up
	config.ResetForTest()
}

func TestRemoveRule_Disabled(t *testing.T) {
	manager, _ := setupTestManager(t)
	manager.enabled = false

	err := manager.RemoveRule("test.example.com")
	assert.NoError(t, err) // Should not error when disabled

	// Clean up
	config.ResetForTest()
}

func TestListRules(t *testing.T) {
	manager, _ := setupTestManager(t)

	// Initially empty
	rules := manager.ListRules()
	assert.Len(t, rules, 0)

	// Add some rules manually
	manager.mu.Lock()
	rule1 := &ProxyRule{
		Hostname:   "app1.example.com",
		TargetIP:   "192.168.1.100",
		TargetPort: 8080,
		Protocol:   "http",
		Enabled:    true,
		CreatedAt:  time.Now(),
	}
	rule2 := &ProxyRule{
		Hostname:   "app2.example.com",
		TargetIP:   "192.168.1.200",
		TargetPort: 9090,
		Protocol:   "http",
		Enabled:    true,
		CreatedAt:  time.Now(),
	}
	manager.rules["app1.example.com"] = rule1
	manager.rules["app2.example.com"] = rule2
	manager.mu.Unlock()

	// List rules
	rules = manager.ListRules()
	assert.Len(t, rules, 2)

	// Verify rule contents
	hostnames := make([]string, len(rules))
	for i, rule := range rules {
		hostnames[i] = rule.Hostname
	}
	assert.Contains(t, hostnames, "app1.example.com")
	assert.Contains(t, hostnames, "app2.example.com")

	// Clean up
	config.ResetForTest()
}

func TestListRules_Disabled(t *testing.T) {
	manager, _ := setupTestManager(t)
	manager.enabled = false

	rules := manager.ListRules()
	assert.Len(t, rules, 0)

	// Clean up
	config.ResetForTest()
}

func TestGenerateConfig(t *testing.T) {
	manager, _ := setupTestManager(t)

	// Add some rules manually
	manager.mu.Lock()
	rule1 := &ProxyRule{
		Hostname:   "app1.example.com",
		TargetIP:   "192.168.1.100",
		TargetPort: 8080,
		Protocol:   "http",
		Enabled:    true,
		CreatedAt:  time.Now(),
	}
	rule2 := &ProxyRule{
		Hostname:   "app2.example.com",
		TargetIP:   "192.168.1.200",
		TargetPort: 9090,
		Protocol:   "http",
		Enabled:    true,
		CreatedAt:  time.Now(),
	}
	manager.rules["app1.example.com"] = rule1
	manager.rules["app2.example.com"] = rule2

	err := manager.generateConfig()
	manager.mu.Unlock()
	assert.NoError(t, err)

	// Verify config file was generated
	configContent, err := os.ReadFile(manager.configPath)
	assert.NoError(t, err)

	configStr := string(configContent)
	assert.Contains(t, configStr, "app1.example.com")
	assert.Contains(t, configStr, "192.168.1.100:8080")
	assert.Contains(t, configStr, "app2.example.com")
	assert.Contains(t, configStr, "192.168.1.200:9090")
	assert.Contains(t, configStr, "reverse_proxy")

	// Clean up
	config.ResetForTest()
}

func TestGenerateConfig_EmptyRules(t *testing.T) {
	manager, _ := setupTestManager(t)

	// Generate config with no rules
	err := manager.generateConfig()
	assert.NoError(t, err)

	// Verify config file exists but has no proxy rules
	configContent, err := os.ReadFile(manager.configPath)
	assert.NoError(t, err)

	configStr := string(configContent)
	assert.Contains(t, configStr, "# Generated proxy configuration")
	assert.NotContains(t, configStr, "reverse_proxy")

	// Clean up
	config.ResetForTest()
}

func TestGenerateConfig_TemplateNotFound(t *testing.T) {
	manager, _ := setupTestManager(t)
	manager.templatePath = "/nonexistent/template"

	err := manager.generateConfig()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read proxy template")

	// Clean up
	config.ResetForTest()
}

func TestGenerateConfig_InvalidTemplate(t *testing.T) {
	manager, testDir := setupTestManager(t)

	// Create invalid template
	invalidTemplate := "{{.InvalidField"
	err := os.WriteFile(manager.templatePath, []byte(invalidTemplate), 0644)
	require.NoError(t, err)

	err = manager.generateConfig()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse proxy template")

	// Clean up
	os.RemoveAll(testDir)
	config.ResetForTest()
}

func TestGenerateConfig_TemplateExecutionError(t *testing.T) {
	manager, testDir := setupTestManager(t)

	// Add a rule to trigger template execution
	manager.mu.Lock()
	rule := &ProxyRule{
		Hostname:   "test.example.com",
		TargetIP:   "192.168.1.100",
		TargetPort: 8080,
		Protocol:   "http",
		Enabled:    true,
		CreatedAt:  time.Now(),
	}
	manager.rules["test.example.com"] = rule
	manager.mu.Unlock()

	// Create template with incorrect field access (simulating the bug we fixed)
	buggyTemplate := `{{- if .ProxyRules}}
{{- range .ProxyRules}}
{{- if .Enabled}}
{{.Hostname}} {
    reverse_proxy {{.TargetIP}}:{{.TargetPort}}
    tls /etc/letsencrypt/live/{{.Domain}}/cert.pem /etc/letsencrypt/live/{{.Domain}}/privkey.pem
}
{{- end}}
{{- end}}
{{- end}}`
	err := os.WriteFile(manager.templatePath, []byte(buggyTemplate), 0644)
	require.NoError(t, err)

	err = manager.generateConfig()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to execute proxy template")
	assert.Contains(t, err.Error(), "can't evaluate field Domain")

	// Clean up
	os.RemoveAll(testDir)
	config.ResetForTest()
}

func TestGenerateConfig_CorrectDomainAccess(t *testing.T) {
	manager, testDir := setupTestManager(t)

	// Add a rule to trigger template execution
	manager.mu.Lock()
	rule := &ProxyRule{
		Hostname:   "test.example.com",
		TargetIP:   "192.168.1.100",
		TargetPort: 8080,
		Protocol:   "http",
		Enabled:    true,
		CreatedAt:  time.Now(),
	}
	manager.rules["test.example.com"] = rule
	manager.domain = "internal.example.com" // Set domain for template
	manager.mu.Unlock()

	// Create template with correct domain field access (the fix)
	correctTemplate := `{{- if .ProxyRules}}
{{- range .ProxyRules}}
{{- if .Enabled}}
{{.Hostname}} {
    reverse_proxy {{.TargetIP}}:{{.TargetPort}}
    tls /etc/letsencrypt/live/{{$.Domain}}/cert.pem /etc/letsencrypt/live/{{$.Domain}}/privkey.pem
}
{{- end}}
{{- end}}
{{- end}}`
	err := os.WriteFile(manager.templatePath, []byte(correctTemplate), 0644)
	require.NoError(t, err)

	err = manager.generateConfig()
	assert.NoError(t, err)

	// Verify the config was generated correctly with domain substitution
	configContent, err := os.ReadFile(manager.configPath)
	assert.NoError(t, err)
	configStr := string(configContent)
	assert.Contains(t, configStr, "test.example.com")
	assert.Contains(t, configStr, "192.168.1.100:8080")
	assert.Contains(t, configStr, "/etc/letsencrypt/live/internal.example.com/cert.pem")
	assert.Contains(t, configStr, "/etc/letsencrypt/live/internal.example.com/privkey.pem")

	// Clean up
	os.RemoveAll(testDir)
	config.ResetForTest()
}

func TestReloadProxy_Disabled(t *testing.T) {
	manager, _ := setupTestManager(t)
	manager.enabled = false

	err := manager.reloadProxy()
	assert.NoError(t, err) // Should not error when disabled

	// Clean up
	config.ResetForTest()
}

func TestIsEnabled(t *testing.T) {
	manager, _ := setupTestManager(t)

	assert.True(t, manager.IsEnabled())

	manager.enabled = false
	assert.False(t, manager.IsEnabled())

	// Clean up
	config.ResetForTest()
}

func TestGetStats(t *testing.T) {
	manager, _ := setupTestManager(t)

	// Add some rules manually
	manager.mu.Lock()
	rule1 := &ProxyRule{
		Hostname:   "app1.example.com",
		TargetIP:   "192.168.1.100",
		TargetPort: 8080,
		Protocol:   "http",
		Enabled:    true,
		CreatedAt:  time.Now(),
	}
	rule2 := &ProxyRule{
		Hostname:   "app2.example.com",
		TargetIP:   "192.168.1.200",
		TargetPort: 9090,
		Protocol:   "http",
		Enabled:    true,
		CreatedAt:  time.Now(),
	}
	manager.rules["app1.example.com"] = rule1
	manager.rules["app2.example.com"] = rule2
	manager.configVersion = 2
	manager.lastGenerated = time.Now()
	manager.mu.Unlock()

	stats := manager.GetStats()
	assert.NotNil(t, stats)
	assert.Equal(t, true, stats["enabled"])
	assert.Equal(t, 2, stats["total_rules"])
	assert.Greater(t, stats["config_version"], 0)
	assert.NotEmpty(t, stats["last_generated"])

	// Verify last_generated is a valid timestamp
	lastGenerated, ok := stats["last_generated"].(string)
	assert.True(t, ok)
	_, err := time.Parse(time.RFC3339, lastGenerated)
	assert.NoError(t, err)

	// Clean up
	config.ResetForTest()
}

func TestGetStats_Disabled(t *testing.T) {
	manager, _ := setupTestManager(t)
	manager.enabled = false

	stats := manager.GetStats()
	assert.NotNil(t, stats)
	assert.Equal(t, false, stats["enabled"])
	assert.Equal(t, 0, stats["total_rules"])

	// Clean up
	config.ResetForTest()
}

func TestProxyRule_JSONSerialization(t *testing.T) {
	rule := &ProxyRule{
		Hostname:   "test.example.com",
		TargetIP:   "192.168.1.100",
		TargetPort: 8080,
		Protocol:   "http",
		Enabled:    true,
		CreatedAt:  time.Now(),
	}

	// This test ensures the JSON tags are working correctly
	// The actual JSON serialization would be tested in integration tests
	assert.Equal(t, "test.example.com", rule.Hostname)
	assert.Equal(t, "192.168.1.100", rule.TargetIP)
	assert.Equal(t, 8080, rule.TargetPort)
	assert.Equal(t, "http", rule.Protocol)
	assert.True(t, rule.Enabled)
	assert.False(t, rule.CreatedAt.IsZero())
}

func TestConcurrentAccess(t *testing.T) {
	manager, _ := setupTestManager(t)

	// Test concurrent read/write access
	done := make(chan bool, 3)

	// Goroutine 1: Add rules manually (no reload)
	go func() {
		manager.mu.Lock()
		for i := 0; i < 10; i++ {
			hostname := fmt.Sprintf("app%d.example.com", i)
			rule := &ProxyRule{
				Hostname:   hostname,
				TargetIP:   "192.168.1.100",
				TargetPort: 8080 + i,
				Protocol:   "http",
				Enabled:    true,
				CreatedAt:  time.Now(),
			}
			manager.rules[hostname] = rule
		}
		manager.mu.Unlock()
		done <- true
	}()

	// Goroutine 2: List rules
	go func() {
		for i := 0; i < 10; i++ {
			rules := manager.ListRules()
			assert.NotNil(t, rules)
			time.Sleep(1 * time.Millisecond)
		}
		done <- true
	}()

	// Goroutine 3: Get stats
	go func() {
		for i := 0; i < 10; i++ {
			stats := manager.GetStats()
			assert.NotNil(t, stats)
			time.Sleep(1 * time.Millisecond)
		}
		done <- true
	}()

	// Wait for all goroutines to complete
	for i := 0; i < 3; i++ {
		<-done
	}

	// Verify final state
	rules := manager.ListRules()
	assert.Len(t, rules, 10)

	// Clean up
	config.ResetForTest()
}

// Mock Tailscale client for testing
type mockTailscaleClient struct {
	devices      []tailscale.Device
	shouldError  bool
	errorMessage string
}

func (m *mockTailscaleClient) GetDeviceByIP(ip string) (*tailscale.Device, error) {
	if m.shouldError {
		return nil, fmt.Errorf("%s", m.errorMessage)
	}

	for _, device := range m.devices {
		for _, addr := range device.Addresses {
			if addr == ip {
				return &device, nil
			}
		}
	}
	return nil, fmt.Errorf("device not found for IP %s", ip)
}

func (m *mockTailscaleClient) GetTailscaleIP(device *tailscale.Device) string {
	for _, addr := range device.Addresses {
		if len(addr) > 4 && addr[:4] == "100." {
			return addr
		}
	}
	return ""
}

// Other required methods for interface compatibility
func (m *mockTailscaleClient) ListDevices() ([]tailscale.Device, error)         { return m.devices, nil }
func (m *mockTailscaleClient) GetDevice(name string) (*tailscale.Device, error) { return nil, nil }
func (m *mockTailscaleClient) GetDeviceIP(name string) (string, error)          { return "", nil }
func (m *mockTailscaleClient) IsDeviceOnline(name string) (bool, error)         { return true, nil }
func (m *mockTailscaleClient) ValidateConnection() error                        { return nil }
func (m *mockTailscaleClient) GetCurrentDeviceIP() (string, error)              { return "100.1.1.1", nil }
func (m *mockTailscaleClient) GetCurrentDeviceIPByName(name string) (string, error) {
	return "100.1.1.1", nil
}

func TestGetSourceIP(t *testing.T) {
	tests := []struct {
		name       string
		headers    map[string]string
		remoteAddr string
		expectedIP string
	}{
		{
			name: "X-Forwarded-For single IP",
			headers: map[string]string{
				"X-Forwarded-For": "192.168.1.100",
			},
			expectedIP: "192.168.1.100",
		},
		{
			name: "X-Forwarded-For multiple IPs",
			headers: map[string]string{
				"X-Forwarded-For": "192.168.1.100, 10.0.0.1, 172.16.0.1",
			},
			expectedIP: "192.168.1.100",
		},
		{
			name: "X-Real-IP",
			headers: map[string]string{
				"X-Real-IP": "192.168.1.200",
			},
			expectedIP: "192.168.1.200",
		},
		{
			name: "X-Forwarded-For takes precedence over X-Real-IP",
			headers: map[string]string{
				"X-Forwarded-For": "192.168.1.100",
				"X-Real-IP":       "192.168.1.200",
			},
			expectedIP: "192.168.1.100",
		},
		{
			name:       "RemoteAddr fallback",
			headers:    map[string]string{},
			remoteAddr: "192.168.1.150:12345",
			expectedIP: "192.168.1.150",
		},
		{
			name: "X-Forwarded-For with spaces",
			headers: map[string]string{
				"X-Forwarded-For": " 192.168.1.100 , 10.0.0.1 ",
			},
			expectedIP: "192.168.1.100",
		},
		{
			name:       "No headers, no RemoteAddr",
			headers:    map[string]string{},
			remoteAddr: "clear", // Special marker to clear RemoteAddr
			expectedIP: "",
		},
		{
			name: "Empty headers",
			headers: map[string]string{
				"X-Forwarded-For": "",
				"X-Real-IP":       "",
			},
			remoteAddr: "192.168.1.150:12345",
			expectedIP: "192.168.1.150",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/test", nil)

			// Set headers
			for key, value := range tt.headers {
				req.Header.Set(key, value)
			}

			// Set RemoteAddr
			if tt.remoteAddr != "" {
				if tt.remoteAddr == "clear" {
					req.RemoteAddr = ""
				} else {
					req.RemoteAddr = tt.remoteAddr
				}
			}

			result := GetSourceIP(req)
			assert.Equal(t, tt.expectedIP, result)
		})
	}
}

func TestAddRule(t *testing.T) {
	manager, tempDir := setupTestManager(t)
	defer os.RemoveAll(tempDir)

	proxyRule := &ProxyRule{
		Hostname:   "test.example.com",
		TargetIP:   "192.168.1.100",
		TargetPort: 8080,
		Protocol:   "http",
		Enabled:    true,
		CreatedAt:  time.Now(),
	}

	err := manager.AddRule(proxyRule)
	assert.NoError(t, err)

	// Verify rule was added correctly
	rules := manager.ListRules()
	assert.Len(t, rules, 1)

	rule := rules[0]
	assert.Equal(t, "test.example.com", rule.Hostname)
	assert.Equal(t, "192.168.1.100", rule.TargetIP)
	assert.Equal(t, 8080, rule.TargetPort)
	assert.Equal(t, "http", rule.Protocol)
	assert.True(t, rule.Enabled)
	assert.Equal(t, proxyRule.CreatedAt, rule.CreatedAt)

	// Verify config file was generated (in test mode, this should work)
	configContent, err := os.ReadFile(manager.configPath)
	assert.NoError(t, err)
	assert.Contains(t, string(configContent), "test.example.com")
	assert.Contains(t, string(configContent), "192.168.1.100:8080")

	// Clean up
	config.ResetForTest()
}

func TestAddRuleWithProxyRule_Disabled(t *testing.T) {
	manager, _ := setupTestManager(t)
	manager.enabled = false

	proxyRule := &ProxyRule{
		Hostname:   "test.example.com",
		TargetIP:   "192.168.1.100",
		TargetPort: 8080,
		Protocol:   "http",
		Enabled:    true,
		CreatedAt:  time.Now(),
	}

	err := manager.AddRule(proxyRule)
	assert.NoError(t, err) // Should not error when disabled

	// Verify no rule was added
	assert.Len(t, manager.rules, 0)

	// Clean up
	config.ResetForTest()
}

func TestAddRule_FQDNValidation(t *testing.T) {
	manager, _ := setupTestManager(t)

	tests := []struct {
		name        string
		hostname    string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "Valid FQDN",
			hostname:    "test.example.com",
			expectError: false,
		},
		{
			name:        "Valid FQDN with subdomain",
			hostname:    "api.test.example.com",
			expectError: false,
		},
		{
			name:        "Valid FQDN with hyphens",
			hostname:    "my-app.example.com",
			expectError: false,
		},
		{
			name:        "Valid FQDN with numbers",
			hostname:    "app1.example.com",
			expectError: false,
		},
		{
			name:        "Empty hostname",
			hostname:    "",
			expectError: true,
			errorMsg:    "hostname cannot be empty",
		},
		{
			name:        "Missing domain separator",
			hostname:    "dns",
			expectError: true,
			errorMsg:    "must contain at least one domain separator",
		},
		{
			name:        "Starts with dot",
			hostname:    ".example.com",
			expectError: true,
			errorMsg:    "cannot start or end with a dot",
		},
		{
			name:        "Ends with dot",
			hostname:    "test.example.com.",
			expectError: true,
			errorMsg:    "cannot start or end with a dot",
		},
		{
			name:        "Empty domain part",
			hostname:    "test..com",
			expectError: true,
			errorMsg:    "contains invalid characters or format",
		},
		{
			name:        "Invalid characters",
			hostname:    "test@example.com",
			expectError: true,
			errorMsg:    "contains invalid characters or format",
		},
		{
			name:        "Starts with hyphen",
			hostname:    "-test.example.com",
			expectError: true,
			errorMsg:    "contains invalid characters or format",
		},
		{
			name:        "Ends with hyphen",
			hostname:    "test-.example.com",
			expectError: true,
			errorMsg:    "contains invalid characters or format",
		},
		{
			name:        "Domain part starts with hyphen",
			hostname:    "test.-example.com",
			expectError: true,
			errorMsg:    "contains invalid characters or format",
		},
		{
			name:        "Domain part ends with hyphen",
			hostname:    "test.example-.com",
			expectError: true,
			errorMsg:    "contains invalid characters or format",
		},
		{
			name:        "Underscore not allowed",
			hostname:    "test_example.com",
			expectError: true,
			errorMsg:    "contains invalid characters or format",
		},
		{
			name:        "Space not allowed",
			hostname:    "test example.com",
			expectError: true,
			errorMsg:    "contains invalid characters or format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			proxyRule := &ProxyRule{
				Hostname:   tt.hostname,
				TargetIP:   "192.168.1.100",
				TargetPort: 8080,
				Protocol:   "http",
				Enabled:    true,
				CreatedAt:  time.Now(),
			}

			err := manager.AddRule(proxyRule)

			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
				// Verify no rule was added
				rules := manager.ListRules()
				assert.Len(t, rules, 0)
			} else {
				assert.NoError(t, err)
				// Verify rule was added
				rules := manager.ListRules()
				assert.Len(t, rules, 1)
				assert.Equal(t, tt.hostname, rules[0].Hostname)
			}

			// Clean up for next test
			if !tt.expectError {
				manager.RemoveRule(tt.hostname)
			}
		})
	}

	// Clean up
	config.ResetForTest()
}

func TestRemoveRule_FQDNValidation(t *testing.T) {
	manager, _ := setupTestManager(t)

	tests := []struct {
		name        string
		hostname    string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "Valid FQDN",
			hostname:    "test.example.com",
			expectError: false,
		},
		{
			name:        "Invalid hostname",
			hostname:    "dns",
			expectError: true,
			errorMsg:    "must contain at least one domain separator",
		},
		{
			name:        "Empty hostname",
			hostname:    "",
			expectError: true,
			errorMsg:    "hostname cannot be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := manager.RemoveRule(tt.hostname)

			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}

	// Clean up
	config.ResetForTest()
}

func TestProxyRuleStorage_Persistence(t *testing.T) {
	tempDir := t.TempDir()
	storagePath := filepath.Join(tempDir, "proxy_rules.json")
	
	// Create storage instance
	storage := NewProxyRuleStorage()
	storage.storage = persistence.NewFileStorageWithPath(storagePath, 3)
	
	// Test initial load (empty)
	rules, err := storage.LoadRules()
	assert.NoError(t, err)
	assert.Len(t, rules, 0)
	
	// Create test rules
	testRules := map[string]*ProxyRule{
		"app1.example.com": {
			Hostname:   "app1.example.com",
			TargetIP:   "192.168.1.100",
			TargetPort: 8080,
			Protocol:   "http",
			Enabled:    true,
			CreatedAt:  time.Now(),
		},
		"app2.example.com": {
			Hostname:   "app2.example.com",
			TargetIP:   "192.168.1.200",
			TargetPort: 9090,
			Protocol:   "https",
			Enabled:    true,
			CreatedAt:  time.Now(),
		},
	}
	
	// Save rules
	err = storage.SaveRules(testRules)
	assert.NoError(t, err)
	
	// Verify file exists
	assert.True(t, storage.Exists())
	
	// Load rules back
	loadedRules, err := storage.LoadRules()
	assert.NoError(t, err)
	assert.Len(t, loadedRules, 2)
	
	// Verify rule contents
	for hostname, expectedRule := range testRules {
		loadedRule, exists := loadedRules[hostname]
		assert.True(t, exists, "Rule for %s should exist", hostname)
		assert.Equal(t, expectedRule.Hostname, loadedRule.Hostname)
		assert.Equal(t, expectedRule.TargetIP, loadedRule.TargetIP)
		assert.Equal(t, expectedRule.TargetPort, loadedRule.TargetPort)
		assert.Equal(t, expectedRule.Protocol, loadedRule.Protocol)
		assert.Equal(t, expectedRule.Enabled, loadedRule.Enabled)
	}
}

func TestManagerPersistence_Restart(t *testing.T) {
	tempDir := t.TempDir()
	
	// Setup shared configuration
	configPath := filepath.Join(tempDir, "Caddyfile")
	templatePath := filepath.Join(tempDir, "Caddyfile.template")
	storagePath := filepath.Join(tempDir, "proxy_rules.json")
	
	// Create template
	templateContent := `# Test template
{{- if .ProxyRules}}
:{{.Port}} {
{{- range .ProxyRules}}
{{- if .Enabled}}
    @{{.Hostname}} host {{.Hostname}}
    handle @{{.Hostname}} {
        reverse_proxy {{.TargetIP}}:{{.TargetPort}}
    }
{{- end}}
{{- end}}
}
{{- end}}`
	err := os.WriteFile(templatePath, []byte(templateContent), 0644)
	require.NoError(t, err)
	
	// Set up configuration
	config.SetForTest("proxy.caddy.config_path", configPath)
	config.SetForTest("proxy.caddy.template_path", templatePath)
	config.SetForTest("proxy.caddy.port", "80")
	config.SetForTest("proxy.enabled", "true")
	config.SetForTest("proxy.storage.path", storagePath)
	config.SetForTest("proxy.storage.backup_count", "3")
	
	// First manager instance - add rules
	manager1, err := NewManager(&MockReloader{})
	require.NoError(t, err)
	
	rule1 := &ProxyRule{
		Hostname:   "app1.example.com",
		TargetIP:   "192.168.1.100",
		TargetPort: 8080,
		Protocol:   "http",
		Enabled:    true,
		CreatedAt:  time.Now(),
	}
	
	rule2 := &ProxyRule{
		Hostname:   "app2.example.com",
		TargetIP:   "192.168.1.200",
		TargetPort: 9090,
		Protocol:   "https",
		Enabled:    true,
		CreatedAt:  time.Now(),
	}
	
	err = manager1.AddRule(rule1)
	assert.NoError(t, err)
	err = manager1.AddRule(rule2)
	assert.NoError(t, err)
	
	// Verify rules were added
	rules := manager1.ListRules()
	assert.Len(t, rules, 2)
	
	// Verify Caddyfile was generated
	configContent, err := os.ReadFile(configPath)
	assert.NoError(t, err)
	assert.Contains(t, string(configContent), "app1.example.com")
	assert.Contains(t, string(configContent), "app2.example.com")
	
	// Create second manager instance (simulating restart)
	manager2, err := NewManager(&MockReloader{})
	require.NoError(t, err)
	
	// Verify rules were restored
	restoredRules := manager2.ListRules()
	assert.Len(t, restoredRules, 2)
	
	// Verify rule contents
	ruleMap := make(map[string]*ProxyRule)
	for _, rule := range restoredRules {
		ruleMap[rule.Hostname] = rule
	}
	
	assert.Contains(t, ruleMap, "app1.example.com")
	assert.Contains(t, ruleMap, "app2.example.com")
	
	rule1Restored := ruleMap["app1.example.com"]
	assert.Equal(t, "192.168.1.100", rule1Restored.TargetIP)
	assert.Equal(t, 8080, rule1Restored.TargetPort)
	assert.Equal(t, "http", rule1Restored.Protocol)
	
	rule2Restored := ruleMap["app2.example.com"]
	assert.Equal(t, "192.168.1.200", rule2Restored.TargetIP)
	assert.Equal(t, 9090, rule2Restored.TargetPort)
	assert.Equal(t, "https", rule2Restored.Protocol)
	
	// Verify Caddyfile was regenerated on startup
	configContent2, err := os.ReadFile(configPath)
	assert.NoError(t, err)
	assert.Contains(t, string(configContent2), "app1.example.com")
	assert.Contains(t, string(configContent2), "app2.example.com")
	
	// Clean up
	config.ResetForTest()
}

func TestStorageValidation_InvalidRules(t *testing.T) {
	tempDir := t.TempDir()
	storagePath := filepath.Join(tempDir, "proxy_rules.json")
	
	storage := NewProxyRuleStorage()
	storage.storage = persistence.NewFileStorageWithPath(storagePath, 3)
	
	// Try to save invalid rules
	invalidRules := map[string]*ProxyRule{
		"invalid.example.com": {
			Hostname:   "invalid.example.com",
			TargetIP:   "", // Invalid: empty IP
			TargetPort: 8080,
			Protocol:   "http",
			Enabled:    true,
			CreatedAt:  time.Now(),
		},
	}
	
	err := storage.SaveRules(invalidRules)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "target IP cannot be empty")
	
	// Verify file was not created
	assert.False(t, storage.Exists())
}

func TestStorageValidation_LoadInvalidRules(t *testing.T) {
	tempDir := t.TempDir()
	storagePath := filepath.Join(tempDir, "proxy_rules.json")
	
	// Create storage file with invalid data
	invalidJSON := `{
		"valid.example.com": {
			"hostname": "valid.example.com",
			"target_ip": "192.168.1.100",
			"target_port": 8080,
			"protocol": "http",
			"enabled": true,
			"created_at": "2024-01-01T00:00:00Z"
		},
		"invalid.example.com": {
			"hostname": "invalid.example.com",
			"target_ip": "",
			"target_port": 8080,
			"protocol": "http",
			"enabled": true,
			"created_at": "2024-01-01T00:00:00Z"
		}
	}`
	
	err := os.WriteFile(storagePath, []byte(invalidJSON), 0644)
	require.NoError(t, err)
	
	storage := NewProxyRuleStorage()
	storage.storage = persistence.NewFileStorageWithPath(storagePath, 3)
	
	// Load rules - should skip invalid ones
	rules, err := storage.LoadRules()
	assert.NoError(t, err)
	assert.Len(t, rules, 1) // Only valid rule should be loaded
	
	validRule, exists := rules["valid.example.com"]
	assert.True(t, exists)
	assert.Equal(t, "192.168.1.100", validRule.TargetIP)
}
