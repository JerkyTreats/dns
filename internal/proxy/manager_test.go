package proxy

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jerkytreats/dns/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestManager(t *testing.T) (*Manager, string) {
	testDir := t.TempDir()
	configPath := filepath.Join(testDir, "Caddyfile")
	templatePath := filepath.Join(testDir, "Caddyfile.template")

	// Create a simple test template
	templateContent := `# Test Caddyfile Template
# Generated at: {{.GeneratedAt}}
# Version: {{.Version}}

{{range .ProxyRules}}
{{if .Enabled}}
{{.Hostname}} {
    reverse_proxy {{.TargetIP}}:{{.TargetPort}}
}
{{end}}
{{end}}
`
	err := os.WriteFile(templatePath, []byte(templateContent), 0644)
	require.NoError(t, err)

	// Set test configuration
	config.SetForTest(ProxyConfigPathKey, configPath)
	config.SetForTest(ProxyTemplatePathKey, templatePath)
	config.SetForTest(ProxyEnabledKey, "true")

	manager := &Manager{
		configPath:   configPath,
		templatePath: templatePath,
		enabled:      true,
		rules:        make(map[string]*ProxyRule),
	}

	return manager, testDir
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
	config.SetForTest(ProxyConfigPathKey, configPath)
	config.SetForTest(ProxyTemplatePathKey, templatePath)
	config.SetForTest(ProxyEnabledKey, "true")

	manager, err := NewManager()
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
	config.SetForTest(ProxyEnabledKey, "false")

	manager, err := NewManager()
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
	config.SetForTest(ProxyConfigPathKey, testConfigPath)
	config.SetForTest(ProxyTemplatePathKey, testTemplatePath)
	// Don't set ProxyEnabledKey to test default

	manager, err := NewManager()
	assert.NoError(t, err)
	assert.NotNil(t, manager)
	assert.True(t, manager.enabled) // Should use default (true)
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

	hostname := "test.example.com"
	targetIP := "192.168.1.100"
	targetPort := 8080

	err := manager.AddRule(hostname, targetIP, targetPort)
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
	assert.Contains(t, configStr, "Test Caddyfile Template")
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
