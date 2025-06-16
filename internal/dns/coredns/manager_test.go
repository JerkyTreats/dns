package coredns

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestManager(t *testing.T) {
	// Setup test environment
	tempDir, err := os.MkdirTemp("", "coredns-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	configPath := filepath.Join(tempDir, "Corefile")
	zonesPath := filepath.Join(tempDir, "zones")

	// Create initial Corefile
	initialConfig := `. {
    errors
    log
}`
	err = os.WriteFile(configPath, []byte(initialConfig), 0644)
	require.NoError(t, err)

	// Create test manager
	logger, _ := zap.NewDevelopment()
	manager := NewManager(logger, configPath, zonesPath, []string{"echo", "reload"})

	t.Run("AddZone", func(t *testing.T) {
		// Test adding a zone
		err := manager.AddZone("test-service")
		require.NoError(t, err)

		// Verify zone file was created
		zoneFile := filepath.Join(zonesPath, "test-service.zone")
		content, err := os.ReadFile(zoneFile)
		require.NoError(t, err)
		assert.Contains(t, string(content), "test-service.internal.jerkytreats.dev")

		// Verify Corefile was updated
		config, err := os.ReadFile(configPath)
		require.NoError(t, err)
		assert.Contains(t, string(config), "test-service.internal.jerkytreats.dev:53")
	})

	t.Run("RemoveZone", func(t *testing.T) {
		// Test removing a zone
		err := manager.RemoveZone("test-service")
		require.NoError(t, err)

		// Verify zone file was removed
		zoneFile := filepath.Join(zonesPath, "test-service.zone")
		_, err = os.Stat(zoneFile)
		assert.True(t, os.IsNotExist(err))

		// Verify Corefile was updated
		config, err := os.ReadFile(configPath)
		require.NoError(t, err)
		assert.NotContains(t, string(config), "test-service.internal.jerkytreats.dev:53")
	})

	t.Run("AddZone with invalid service name", func(t *testing.T) {
		err := manager.AddZone("invalid@service")
		assert.Error(t, err)
	})

	t.Run("RemoveZone with non-existent service", func(t *testing.T) {
		err := manager.RemoveZone("non-existent")
		assert.NoError(t, err) // Should not error when removing non-existent zone
	})
}

func TestManagerErrors(t *testing.T) {
	// Setup test environment with invalid paths
	tempDir, err := os.MkdirTemp("", "coredns-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	configPath := filepath.Join(tempDir, "nonexistent", "Corefile")
	zonesPath := filepath.Join(tempDir, "nonexistent", "zones")

	logger, _ := zap.NewDevelopment()
	manager := NewManager(logger, configPath, zonesPath, []string{"echo", "reload"})

	t.Run("AddZone with invalid paths", func(t *testing.T) {
		err := manager.AddZone("test-service")
		assert.Error(t, err)
	})

	t.Run("RemoveZone with invalid paths", func(t *testing.T) {
		err := manager.RemoveZone("test-service")
		assert.Error(t, err)
	})
}
