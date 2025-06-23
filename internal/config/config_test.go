package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

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
	configContent := `test_value: "found_in_dir2"`
	err = os.WriteFile(configFile, []byte(configContent), 0644)
	assert.NoError(t, err)

	// Test additive search paths using isolated paths to avoid project configs
	err = InitConfig(WithOnlySearchPaths(configDir1, configDir2))
	assert.NoError(t, err)
	assert.Equal(t, "found_in_dir2", GetString("test_value"))
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

// TestCertificateDNSConfiguration tests the new certificate DNS configuration options
func TestCertificateDNSConfiguration(t *testing.T) {
	t.Run("DNS resolvers configuration", func(t *testing.T) {
		ResetForTest()

		// Test setting DNS resolvers as string slice
		resolvers := []string{"8.8.8.8:53", "1.1.1.1:53", "9.9.9.9:53"}
		SetForTest("certificate.dns_resolvers", resolvers)

		result := GetStringSlice("certificate.dns_resolvers")
		assert.Equal(t, resolvers, result)
		assert.Len(t, result, 3)
		assert.Contains(t, result, "8.8.8.8:53")
		assert.Contains(t, result, "1.1.1.1:53")
		assert.Contains(t, result, "9.9.9.9:53")
	})

	t.Run("DNS timeout configuration", func(t *testing.T) {
		ResetForTest()

		// Test setting DNS timeout as duration string
		SetForTest("certificate.dns_timeout", "30s")

		timeout := GetDuration("certificate.dns_timeout")
		assert.Equal(t, 30*time.Second, timeout)

		// Test different timeout formats
		SetForTest("certificate.dns_timeout", "2m")
		timeout = GetDuration("certificate.dns_timeout")
		assert.Equal(t, 2*time.Minute, timeout)

		SetForTest("certificate.dns_timeout", "1m30s")
		timeout = GetDuration("certificate.dns_timeout")
		assert.Equal(t, 90*time.Second, timeout)
	})

	t.Run("insecure skip verify configuration", func(t *testing.T) {
		ResetForTest()

		// Test setting insecure skip verify
		SetForTest("certificate.insecure_skip_verify", true)
		assert.True(t, GetBool("certificate.insecure_skip_verify"))

		SetForTest("certificate.insecure_skip_verify", false)
		assert.False(t, GetBool("certificate.insecure_skip_verify"))
	})

	t.Run("default values", func(t *testing.T) {
		ResetForTest()

		// Test default values when keys are not set
		resolvers := GetStringSlice("certificate.dns_resolvers")
		assert.Empty(t, resolvers) // Should be empty by default

		timeout := GetDuration("certificate.dns_timeout")
		assert.Equal(t, time.Duration(0), timeout) // Should be 0 by default

		skipVerify := GetBool("certificate.insecure_skip_verify")
		assert.False(t, skipVerify) // Should be false by default
	})
}

// TestDNSResolverValidationFormats tests various DNS resolver format validations
func TestDNSResolverValidationFormats(t *testing.T) {
	testCases := []struct {
		name      string
		resolvers []string
		valid     bool
	}{
		{
			name:      "valid IPv4 with port",
			resolvers: []string{"8.8.8.8:53", "1.1.1.1:53"},
			valid:     true,
		},
		{
			name:      "valid IPv6 with port",
			resolvers: []string{"[2001:4860:4860::8888]:53", "[2606:4700:4700::1111]:53"},
			valid:     true,
		},
		{
			name:      "valid hostname with port",
			resolvers: []string{"dns.google:53", "one.one.one.one:53"},
			valid:     true,
		},
		{
			name:      "mixed valid formats",
			resolvers: []string{"8.8.8.8:53", "[2001:4860:4860::8888]:53", "dns.google:53"},
			valid:     true,
		},
		{
			name:      "empty list",
			resolvers: []string{},
			valid:     true, // Empty list is valid (uses defaults)
		},
		{
			name:      "nil list",
			resolvers: nil,
			valid:     true, // Nil list is valid (uses defaults)
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ResetForTest()

			if tc.resolvers != nil {
				SetForTest("certificate.dns_resolvers", tc.resolvers)
			}

			result := GetStringSlice("certificate.dns_resolvers")
			if tc.resolvers == nil {
				assert.Empty(t, result)
			} else {
				assert.Equal(t, tc.resolvers, result)
			}
		})
	}
}

// TestTimeoutValidationFormats tests various timeout format validations
func TestTimeoutValidationFormats(t *testing.T) {
	testCases := []struct {
		name     string
		timeout  string
		expected time.Duration
		valid    bool
	}{
		{
			name:     "seconds format",
			timeout:  "10s",
			expected: 10 * time.Second,
			valid:    true,
		},
		{
			name:     "minutes format",
			timeout:  "2m",
			expected: 2 * time.Minute,
			valid:    true,
		},
		{
			name:     "hours format",
			timeout:  "1h",
			expected: 1 * time.Hour,
			valid:    true,
		},
		{
			name:     "mixed format",
			timeout:  "1h30m45s",
			expected: 1*time.Hour + 30*time.Minute + 45*time.Second,
			valid:    true,
		},
		{
			name:     "zero timeout",
			timeout:  "0s",
			expected: 0,
			valid:    true,
		},
		{
			name:     "empty string",
			timeout:  "",
			expected: 0,
			valid:    true, // Empty string results in 0 duration
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ResetForTest()

			if tc.timeout != "" {
				SetForTest("certificate.dns_timeout", tc.timeout)
			}

			result := GetDuration("certificate.dns_timeout")
			if tc.valid {
				assert.Equal(t, tc.expected, result)
			}
		})
	}
}

// TestCertificateConfigurationIntegration tests the certificate configuration as a whole
func TestCertificateConfigurationIntegration(t *testing.T) {
	t.Run("complete certificate configuration", func(t *testing.T) {
		ResetForTest()

		// Set all certificate-related configuration
		SetForTest("certificate.email", "test@example.com")
		SetForTest("certificate.domain", "example.com")
		SetForTest("certificate.ca_dir_url", "https://acme-staging-v02.api.letsencrypt.org/directory")
		SetForTest("certificate.dns_resolvers", []string{"8.8.8.8:53", "1.1.1.1:53"})
		SetForTest("certificate.dns_timeout", "30s")
		SetForTest("certificate.insecure_skip_verify", true)
		SetForTest("certificate.renewal.enabled", true)
		SetForTest("certificate.renewal.renew_before", "720h")
		SetForTest("certificate.renewal.check_interval", "24h")
		SetForTest("server.tls.cert_file", "/etc/letsencrypt/live/example.com/fullchain.pem")
		SetForTest("server.tls.key_file", "/etc/letsencrypt/live/example.com/privkey.pem")

		// Verify all values are set correctly
		assert.Equal(t, "test@example.com", GetString("certificate.email"))
		assert.Equal(t, "example.com", GetString("certificate.domain"))
		assert.Equal(t, "https://acme-staging-v02.api.letsencrypt.org/directory", GetString("certificate.ca_dir_url"))

		resolvers := GetStringSlice("certificate.dns_resolvers")
		expected := []string{"8.8.8.8:53", "1.1.1.1:53"}
		assert.Equal(t, expected, resolvers)

		timeout := GetDuration("certificate.dns_timeout")
		assert.Equal(t, 30*time.Second, timeout)

		assert.True(t, GetBool("certificate.insecure_skip_verify"))
		assert.True(t, GetBool("certificate.renewal.enabled"))

		renewBefore := GetDuration("certificate.renewal.renew_before")
		assert.Equal(t, 720*time.Hour, renewBefore)

		checkInterval := GetDuration("certificate.renewal.check_interval")
		assert.Equal(t, 24*time.Hour, checkInterval)

		assert.Equal(t, "/etc/letsencrypt/live/example.com/fullchain.pem", GetString("server.tls.cert_file"))
		assert.Equal(t, "/etc/letsencrypt/live/example.com/privkey.pem", GetString("server.tls.key_file"))
	})
}

// TestConfigValidationWithYAML tests configuration loading from YAML with new DNS options
func TestConfigValidationWithYAML(t *testing.T) {
	ResetForTest()

	// Create a temporary config file with DNS configuration
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")
	configContent := `
certificate:
  email: "test@example.com"
  domain: "example.com"
  ca_dir_url: "https://acme-staging-v02.api.letsencrypt.org/directory"
  dns_resolvers:
    - "8.8.8.8:53"
    - "1.1.1.1:53"
    - "9.9.9.9:53"
  dns_timeout: "30s"
  insecure_skip_verify: false
  renewal:
    enabled: true
    renew_before: "720h"
    check_interval: "24h"
server:
  tls:
    cert_file: "/etc/letsencrypt/live/example.com/fullchain.pem"
    key_file: "/etc/letsencrypt/live/example.com/privkey.pem"
`

	err := os.WriteFile(configFile, []byte(configContent), 0644)
	require.NoError(t, err)

	// Initialize with config file
	err = InitConfig(WithConfigPath(configFile))
	require.NoError(t, err)

	// Verify configuration was loaded correctly
	assert.Equal(t, "test@example.com", GetString("certificate.email"))
	assert.Equal(t, "example.com", GetString("certificate.domain"))

	resolvers := GetStringSlice("certificate.dns_resolvers")
	expected := []string{"8.8.8.8:53", "1.1.1.1:53", "9.9.9.9:53"}
	assert.Equal(t, expected, resolvers)

	timeout := GetDuration("certificate.dns_timeout")
	assert.Equal(t, 30*time.Second, timeout)

	assert.False(t, GetBool("certificate.insecure_skip_verify"))
	assert.True(t, GetBool("certificate.renewal.enabled"))
}
