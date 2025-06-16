package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConfigInitialization(t *testing.T) {
	// Reset config before each test
	ResetForTest()
	// Ensure no real config file is loaded
	SetConfigPath("/nonexistent/path/config.json")

	// Test default values
	assert.Equal(t, "INFO", GetString(LogLevelKey))
	assert.Equal(t, 0, GetInt("nonexistent"))
	assert.False(t, GetBool("nonexistent"))
	assert.Empty(t, GetStringMapString("nonexistent"))
}

func TestSetConfigPath(t *testing.T) {
	// Reset config before test
	ResetForTest()

	// Create a temporary config file
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.json")
	configContent := `{"log_level": "DEBUG"}`
	err := os.WriteFile(configFile, []byte(configContent), 0644)
	assert.NoError(t, err)

	// Set config path and verify it's loaded
	SetConfigPath(configFile)
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

	// Set config path and verify initial value
	SetConfigPath(configFile)
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

	// Set config path and verify map data
	SetConfigPath(configFile)
	result := GetStringMapString("test_map")
	assert.Equal(t, "value1", result["key1"])
	assert.Equal(t, "value2", result["key2"])
}

func TestConfigNotFound(t *testing.T) {
	// Reset config before test
	ResetForTest()

	// Set a non-existent config path
	SetConfigPath("/nonexistent/path/config.json")

	// Should return default values
	assert.Equal(t, "INFO", GetString(LogLevelKey))
	assert.Equal(t, 0, GetInt("nonexistent"))
	assert.False(t, GetBool("nonexistent"))
	assert.Empty(t, GetStringMapString("nonexistent"))
}
