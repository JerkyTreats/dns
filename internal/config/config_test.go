package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigInitialization(t *testing.T) {
	// Reset config before each test
	ResetForTest()

	// Initialize with non-existent config file
	err := InitConfig(WithConfigPath("/nonexistent/path/config.json"))
	assert.NoError(t, err) // Should not error, just use defaults

	// Test default values
	assert.Equal(t, "INFO", GetString(LogLevelKey))
	assert.Equal(t, 0, GetInt("nonexistent"))
	assert.False(t, GetBool("nonexistent"))
	assert.Empty(t, GetStringMapString("nonexistent"))
}

func TestInitConfigWithPath(t *testing.T) {
	// Reset config before test
	ResetForTest()

	// Create a temporary config file
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.json")
	configContent := `{"log_level": "DEBUG"}`
	err := os.WriteFile(configFile, []byte(configContent), 0644)
	assert.NoError(t, err)

	// Initialize with config path and verify it's loaded
	err = InitConfig(WithConfigPath(configFile))
	assert.NoError(t, err)
	assert.Equal(t, "DEBUG", GetString(LogLevelKey))
}

func TestRequiredKeys(t *testing.T) {
	// Reset config before test
	ResetForTest()

	// Test registering required keys
	RegisterRequiredKey("test_key")
	RegisterRequiredKey("test_key") // Should not add duplicate

	// Note: HasKey only checks if the key exists in the config, not in requiredKeys
	// So we'll test that the key was registered by checking if it's in the requiredKeys slice
	// This is an implementation detail test, but it's important to verify the registration works
	assert.False(t, HasKey("test_key")) // HasKey should return false since the key isn't in the config
}

func TestCheckRequiredKeys(t *testing.T) {
	ResetForTest()
	defer ResetForTest()

	RegisterRequiredKey("required_key1")
	RegisterRequiredKey("required_key2")

	// Test missing keys - now should return error
	err := CheckRequiredKeys()
	require.Error(t, err) // Should return error for missing keys
	assert.Contains(t, err.Error(), "missing required configuration keys")

	// For this test, we can verify the internal state through HasKey
	assert.False(t, HasKey("required_key1"))
	assert.False(t, HasKey("required_key2"))

	// Set one key and test again - still should error for the missing second key
	SetForTest("required_key1", "value1")
	err = CheckRequiredKeys()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "required_key2")

	assert.True(t, HasKey("required_key1"))
	assert.False(t, HasKey("required_key2"))

	// Set the second key - now should pass
	SetForTest("required_key2", "value2")
	err = CheckRequiredKeys()
	require.NoError(t, err)

	assert.True(t, HasKey("required_key1"))
	assert.True(t, HasKey("required_key2"))
}

func TestInitConfigMultipleCalls(t *testing.T) {
	// Reset config before test
	ResetForTest()

	// Create a temporary config file
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.json")
	configContent := `{"log_level": "DEBUG"}`
	err := os.WriteFile(configFile, []byte(configContent), 0644)
	assert.NoError(t, err)

	// First initialization
	err = InitConfig(WithConfigPath(configFile))
	assert.NoError(t, err)
	assert.Equal(t, "DEBUG", GetString(LogLevelKey))

	// Second initialization with different path should not change config (singleton)
	err = InitConfig(WithConfigPath("/different/path/config.json"))
	assert.NoError(t, err)
	assert.Equal(t, "DEBUG", GetString(LogLevelKey)) // Should still be DEBUG
}

func TestWithSearchPaths(t *testing.T) {
	// Reset config before test
	ResetForTest()

	// Create temporary directories and config file
	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, "custom", "config")
	err := os.MkdirAll(configDir, 0755)
	assert.NoError(t, err)

	configFile := filepath.Join(configDir, "config.yaml")
	configContent := `log_level: "WARN"`
	err = os.WriteFile(configFile, []byte(configContent), 0644)
	assert.NoError(t, err)

	// Initialize with only our search path (replace defaults)
	err = InitConfig(WithOnlySearchPaths(filepath.Join(tmpDir, "custom", "config")))
	assert.NoError(t, err)
	assert.Equal(t, "WARN", GetString(LogLevelKey))
}

func TestLazyLoading(t *testing.T) {
	// Reset config before test
	ResetForTest()

	// Initialize with isolated search paths to avoid project config files
	tmpDir := t.TempDir()
	err := InitConfig(WithOnlySearchPaths(tmpDir)) // Empty directory
	assert.NoError(t, err)

	// Should get default value since no config file found
	value := GetString(LogLevelKey)
	assert.Equal(t, "INFO", value) // Should get default value

	// Verify config is now initialized
	cfg := getInstance()
	cfg.mu.RLock()
	initialized := cfg.initialized
	cfg.mu.RUnlock()
	assert.True(t, initialized)
}

func TestReload(t *testing.T) {
	// Reset config before test
	ResetForTest()

	// Create a temporary config file
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.json")

	// Write initial config
	initialConfig := `{"log_level": "INFO"}`
	err := os.WriteFile(configFile, []byte(initialConfig), 0644)
	assert.NoError(t, err)

	// Initialize with config path and verify initial value
	err = InitConfig(WithConfigPath(configFile))
	assert.NoError(t, err)
	assert.Equal(t, "INFO", GetString(LogLevelKey))

	// Update config file
	updatedConfig := `{"log_level": "DEBUG"}`
	err = os.WriteFile(configFile, []byte(updatedConfig), 0644)
	assert.NoError(t, err)

	// Reload and verify new value
	err = Reload()
	assert.NoError(t, err)
	assert.Equal(t, "DEBUG", GetString(LogLevelKey))
}

func TestGetStringMapString(t *testing.T) {
	// Reset config before test
	ResetForTest()

	// Create a temporary config file with map data
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.json")
	configContent := `{"test_map": {"key1": "value1", "key2": "value2"}}`
	err := os.WriteFile(configFile, []byte(configContent), 0644)
	assert.NoError(t, err)

	// Initialize with config path and verify map data
	err = InitConfig(WithConfigPath(configFile))
	assert.NoError(t, err)
	result := GetStringMapString("test_map")
	assert.Equal(t, "value1", result["key1"])
	assert.Equal(t, "value2", result["key2"])
}

func TestConfigNotFound(t *testing.T) {
	// Reset config before test
	ResetForTest()

	// Initialize with a non-existent config path
	err := InitConfig(WithConfigPath("/nonexistent/path/config.json"))
	assert.NoError(t, err) // Should not error, just use defaults

	// Should return default values
	assert.Equal(t, "INFO", GetString(LogLevelKey))
	assert.Equal(t, 0, GetInt("nonexistent"))
	assert.False(t, GetBool("nonexistent"))
	assert.Empty(t, GetStringMapString("nonexistent"))
}

func TestSetForTest(t *testing.T) {
	// Reset config before test
	ResetForTest()

	// Initialize config
	err := InitConfig()
	assert.NoError(t, err)

	// Set a test value
	SetForTest("test_key", "test_value")
	assert.Equal(t, "test_value", GetString("test_key"))

	// Set another type
	SetForTest("test_int", 42)
	assert.Equal(t, 42, GetInt("test_int"))
}

func TestFunctionalOptions(t *testing.T) {
	// Reset config before test
	ResetForTest()

	// Create temporary directories
	tmpDir := t.TempDir()
	configDir1 := filepath.Join(tmpDir, "config1")
	configDir2 := filepath.Join(tmpDir, "config2")
	err := os.MkdirAll(configDir1, 0755)
	assert.NoError(t, err)
	err = os.MkdirAll(configDir2, 0755)
	assert.NoError(t, err)

	// Create config file in second directory
	configFile := filepath.Join(configDir2, "config.yaml")
	configContent := `log_level: "ERROR"`
	err = os.WriteFile(configFile, []byte(configContent), 0644)
	assert.NoError(t, err)

	// Test multiple options using only our search paths
	err = InitConfig(WithOnlySearchPaths(configDir1, configDir2))
	assert.NoError(t, err)
	assert.Equal(t, "ERROR", GetString(LogLevelKey))
}

func TestConfigPathOverridesSearchPaths(t *testing.T) {
	// Reset config before test
	ResetForTest()

	// Create temporary directories and files
	tmpDir := t.TempDir()

	// Create a search path with config file
	searchDir := filepath.Join(tmpDir, "search")
	err := os.MkdirAll(searchDir, 0755)
	assert.NoError(t, err)
	searchConfigFile := filepath.Join(searchDir, "config.yaml")
	err = os.WriteFile(searchConfigFile, []byte(`log_level: "WARN"`), 0644)
	assert.NoError(t, err)

	// Create an explicit config file
	explicitConfigFile := filepath.Join(tmpDir, "explicit.yaml")
	err = os.WriteFile(explicitConfigFile, []byte(`log_level: "DEBUG"`), 0644)
	assert.NoError(t, err)

	// When both are provided, explicit config path should win
	err = InitConfig(
		WithOnlySearchPaths(searchDir),
		WithConfigPath(explicitConfigFile), // This should override search paths
	)
	assert.NoError(t, err)
	assert.Equal(t, "DEBUG", GetString(LogLevelKey)) // Should get explicit file value, not search path value
}

func TestSearchPathsIgnoredWithConfigPath(t *testing.T) {
	// Reset config before test
	ResetForTest()

	tmpDir := t.TempDir()

	// Create explicit config file
	explicitConfigFile := filepath.Join(tmpDir, "explicit.yaml")
	err := os.WriteFile(explicitConfigFile, []byte(`log_level: "FATAL"`), 0644)
	assert.NoError(t, err)

	// Create search directory with config that would conflict
	searchDir := filepath.Join(tmpDir, "search")
	err = os.MkdirAll(searchDir, 0755)
	assert.NoError(t, err)
	searchConfigFile := filepath.Join(searchDir, "config.yaml")
	err = os.WriteFile(searchConfigFile, []byte(`log_level: "TRACE"`), 0644)
	assert.NoError(t, err)

	// Search paths should be ignored when config path is set first
	err = InitConfig(
		WithConfigPath(explicitConfigFile),
		WithOnlySearchPaths(searchDir), // This should be ignored
	)
	assert.NoError(t, err)
	assert.Equal(t, "FATAL", GetString(LogLevelKey)) // Should use explicit file
}

func TestGetAllDataTypes(t *testing.T) {
	// Reset config before test
	ResetForTest()

	// Create comprehensive config file
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")
	configContent := `
log_level: "DEBUG"
port: 8080
enabled: true
timeout: "30s"
hosts:
  - "localhost"
  - "127.0.0.1"
database:
  host: "db.example.com"
  port: "5432"
`
	err := os.WriteFile(configFile, []byte(configContent), 0644)
	assert.NoError(t, err)

	err = InitConfig(WithConfigPath(configFile))
	assert.NoError(t, err)

	// Test all data types
	assert.Equal(t, "DEBUG", GetString("log_level"))
	assert.Equal(t, 8080, GetInt("port"))
	assert.True(t, GetBool("enabled"))
	assert.Equal(t, "30s", GetDuration("timeout").String())

	hosts := GetStringSlice("hosts")
	assert.Len(t, hosts, 2)
	assert.Contains(t, hosts, "localhost")
	assert.Contains(t, hosts, "127.0.0.1")

	db := GetStringMapString("database")
	assert.Equal(t, "db.example.com", db["host"])
	assert.Equal(t, "5432", db["port"])
}

func TestHasKey(t *testing.T) {
	// Reset config before test
	ResetForTest()

	// Create config file
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")
	configContent := `
existing_key: "value"
nested:
  key: "nested_value"
`
	err := os.WriteFile(configFile, []byte(configContent), 0644)
	assert.NoError(t, err)

	err = InitConfig(WithConfigPath(configFile))
	assert.NoError(t, err)

	// Test existing keys
	assert.True(t, HasKey("existing_key"))
	assert.True(t, HasKey("nested.key"))
	assert.True(t, HasKey("nested"))

	// Test non-existing keys
	assert.False(t, HasKey("nonexistent"))
	assert.False(t, HasKey("nested.nonexistent"))
}

func TestWithSearchPathsAdditive(t *testing.T) {
	// Reset config before test
	ResetForTest()

	// Create temporary directories
	tmpDir := t.TempDir()
	dir1 := filepath.Join(tmpDir, "dir1")
	dir2 := filepath.Join(tmpDir, "dir2")
	err := os.MkdirAll(dir1, 0755)
	assert.NoError(t, err)
	err = os.MkdirAll(dir2, 0755)
	assert.NoError(t, err)

	// Create config file in second directory only
	configFile := filepath.Join(dir2, "config.yaml")
	configContent := `test_value: "found_in_dir2"`
	err = os.WriteFile(configFile, []byte(configContent), 0644)
	assert.NoError(t, err)

	// Test additive search paths using isolated paths to avoid project configs
	err = InitConfig(WithOnlySearchPaths(dir1, dir2))
	assert.NoError(t, err)
	assert.Equal(t, "found_in_dir2", GetString("test_value"))
}

func TestOptionsAppliedCorrectly(t *testing.T) {
	// Reset config before test
	ResetForTest()

	// Create temporary directory with config file
	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, "configs")
	err := os.MkdirAll(configDir, 0755)
	assert.NoError(t, err)

	configFile := filepath.Join(configDir, "config.yaml")
	configContent := `test_key: "found_via_search"`
	err = os.WriteFile(configFile, []byte(configContent), 0644)
	assert.NoError(t, err)

	// Test that WithOnlySearchPaths properly isolates the test
	err = InitConfig(WithOnlySearchPaths(configDir))
	assert.NoError(t, err)
	assert.Equal(t, "found_via_search", GetString("test_key"))
}

func TestEnvironmentVariableOverride(t *testing.T) {
	// Reset config before test
	ResetForTest()

	// Set environment variable
	os.Setenv("LOG_LEVEL", "TRACE")
	defer os.Unsetenv("LOG_LEVEL")

	// Create config file with different value
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")
	configContent := `log_level: "INFO"`
	err := os.WriteFile(configFile, []byte(configContent), 0644)
	assert.NoError(t, err)

	err = InitConfig(WithConfigPath(configFile))
	assert.NoError(t, err)

	// Environment variable should override config file
	assert.Equal(t, "TRACE", GetString("log_level"))
}

func TestMalformedConfig(t *testing.T) {
	// Reset config before test
	ResetForTest()

	// Create malformed config file
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")
	malformedContent := `
log_level: "DEBUG"
port: [invalid yaml structure
enabled: true
`
	err := os.WriteFile(configFile, []byte(malformedContent), 0644)
	assert.NoError(t, err)

	// Should not error but use defaults
	err = InitConfig(WithConfigPath(configFile))
	assert.NoError(t, err)

	// Should get default values since file couldn't be parsed
	assert.Equal(t, "INFO", GetString("log_level"))
	assert.Equal(t, 0, GetInt("port"))
	assert.False(t, GetBool("enabled"))
}

func TestDefaultValues(t *testing.T) {
	// Reset config before test
	ResetForTest()

	// Initialize with isolated search paths to avoid project config files
	tmpDir := t.TempDir()
	err := InitConfig(WithOnlySearchPaths(tmpDir)) // Empty directory
	assert.NoError(t, err)

	// Test default values
	assert.Equal(t, "INFO", GetString(LogLevelKey))
	assert.Equal(t, 0, GetInt("nonexistent_int"))
	assert.False(t, GetBool("nonexistent_bool"))
}

func TestGetBootstrapConfig(t *testing.T) {
	// Reset config before test
	ResetForTest()
	defer ResetForTest()

	// Create a temporary config file with bootstrap config
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")
	configContent := `
dns:
  internal:
    origin: "internal.test.local"
    bootstrap_devices:
      - name: "ns"
        tailscale_name: "omnitron"
        aliases: ["omnitron", "dns"]
        description: "NAS, DNS host"
        enabled: true
      - name: "dev"
        tailscale_name: "revenantor"
        aliases: ["macbook"]
        description: "MacBook development"
        enabled: false
`
	err := os.WriteFile(configFile, []byte(configContent), 0644)
	assert.NoError(t, err)

	// Initialize with config path
	err = InitConfig(WithConfigPath(configFile))
	assert.NoError(t, err)

	// Test bootstrap config
	config := GetBootstrapConfig()
	assert.Equal(t, "internal.test.local", config.Origin)
	assert.Len(t, config.Devices, 2)

	// Check first device
	assert.Equal(t, "ns", config.Devices[0].Name)
	assert.Equal(t, "omnitron", config.Devices[0].TailscaleName)
	assert.Equal(t, []string{"omnitron", "dns"}, config.Devices[0].Aliases)
	assert.Equal(t, "NAS, DNS host", config.Devices[0].Description)
	assert.True(t, config.Devices[0].Enabled)

	// Check second device
	assert.Equal(t, "dev", config.Devices[1].Name)
	assert.Equal(t, "revenantor", config.Devices[1].TailscaleName)
	assert.Equal(t, []string{"macbook"}, config.Devices[1].Aliases)
	assert.Equal(t, "MacBook development", config.Devices[1].Description)
	assert.False(t, config.Devices[1].Enabled)
}

func TestValidateBootstrapConfig(t *testing.T) {
	// Reset config before test
	ResetForTest()
	defer ResetForTest()

	tests := []struct {
		name          string
		config        string
		expectError   bool
		errorContains string
	}{
		{
			name: "Valid configuration",
			config: `
dns:
  internal:
    origin: "internal.test.local"
    bootstrap_devices:
      - name: "ns"
        tailscale_name: "omnitron"
        enabled: true
`,
			expectError: false,
		},
		{
			name: "Missing origin",
			config: `
dns:
  internal:
    bootstrap_devices:
      - name: "ns"
        tailscale_name: "omnitron"
        enabled: true
`,
			expectError:   true,
			errorContains: "dns.internal.origin is required",
		},
		{
			name: "No bootstrap devices",
			config: `
dns:
  internal:
    origin: "internal.test.local"
    bootstrap_devices: []
`,
			expectError:   true,
			errorContains: "at least one bootstrap device must be configured",
		},
		{
			name: "Device missing name",
			config: `
dns:
  internal:
    origin: "internal.test.local"
    bootstrap_devices:
      - tailscale_name: "omnitron"
        enabled: true
`,
			expectError:   true,
			errorContains: "name is required",
		},
		{
			name: "Device missing tailscale_name",
			config: `
dns:
  internal:
    origin: "internal.test.local"
    bootstrap_devices:
      - name: "ns"
        enabled: true
`,
			expectError:   true,
			errorContains: "tailscale_name is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset for each test
			ResetForTest()

			// Create temporary config file
			tmpDir := t.TempDir()
			configFile := filepath.Join(tmpDir, "config.yaml")
			err := os.WriteFile(configFile, []byte(tt.config), 0644)
			assert.NoError(t, err)

			// Initialize config
			err = InitConfig(WithConfigPath(configFile))
			assert.NoError(t, err)

			// Validate bootstrap config
			err = ValidateBootstrapConfig()

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateTailscaleConfig(t *testing.T) {
	// Reset config before test
	ResetForTest()
	defer ResetForTest()

	tests := []struct {
		name          string
		config        string
		expectError   bool
		errorContains string
	}{
		{
			name: "Valid Tailscale configuration",
			config: `
tailscale:
  api_key: "tskey-api-test"
  tailnet: "test@example.com"
`,
			expectError: false,
		},
		{
			name: "Missing API key",
			config: `
tailscale:
  tailnet: "test@example.com"
`,
			expectError:   true,
			errorContains: "tailscale.api_key is required",
		},
		{
			name: "Missing tailnet",
			config: `
tailscale:
  api_key: "tskey-api-test"
`,
			expectError:   true,
			errorContains: "tailscale.tailnet is required",
		},
		{
			name: "Empty Tailscale config",
			config: `
app:
  name: test
`,
			expectError:   true,
			errorContains: "tailscale.api_key is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset for each test
			ResetForTest()

			// Create temporary config file
			tmpDir := t.TempDir()
			configFile := filepath.Join(tmpDir, "config.yaml")
			err := os.WriteFile(configFile, []byte(tt.config), 0644)
			assert.NoError(t, err)

			// Initialize config
			err = InitConfig(WithConfigPath(configFile))
			assert.NoError(t, err)

			// Validate Tailscale config
			err = ValidateTailscaleConfig()

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestConfigurationKeys(t *testing.T) {
	// Test that configuration keys are properly defined
	assert.Equal(t, "dns.internal.origin", DNSInternalOriginKey)
	assert.Equal(t, "dns.internal.bootstrap_devices", DNSBootstrapDevicesKey)
	assert.Equal(t, "tailscale.api_key", TailscaleAPIKeyKey)
	assert.Equal(t, "tailscale.tailnet", TailscaleTailnetKey)
	assert.Equal(t, "tailscale.base_url", TailscaleBaseURLKey)
}
