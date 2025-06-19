package coredns

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManager(t *testing.T) {
	// Setup test environment
	tempDir, err := os.MkdirTemp("", "coredns-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	configPath := filepath.Join(tempDir, "Corefile")
	zonesPath := filepath.Join(tempDir, "zones")
	reloadScriptPath := filepath.Join(tempDir, "reload.sh")

	// Create a mock reload script
	reloadScriptContent := "#!/bin/sh\necho 'reloaded'"
	err = os.WriteFile(reloadScriptPath, []byte(reloadScriptContent), 0755)
	require.NoError(t, err)

	// Create test manager
	manager := NewManager(configPath, zonesPath, []string{reloadScriptPath}, "test.local")

	t.Run("AddRecord", func(t *testing.T) {
		// Before adding a record, we need a zone. Let's create a dummy zone file.
		zoneFileName := filepath.Join(zonesPath, "test-service.zone")
		err := os.MkdirAll(zonesPath, 0755)
		require.NoError(t, err)
		err = os.WriteFile(zoneFileName, []byte("$ORIGIN test-service.test.local.\n"), 0644)
		require.NoError(t, err)

		err = manager.AddRecord("test-service", "test-record", "127.0.0.1")
		require.NoError(t, err)

		// Verify the content of the zone file
		content, err := os.ReadFile(zoneFileName)
		require.NoError(t, err)
		expectedRecord := "test-record\tIN\tA\t127.0.0.1"
		assert.Contains(t, string(content), expectedRecord)
	})

	t.Run("Reload", func(t *testing.T) {
		err := manager.Reload()
		require.NoError(t, err)
	})

	t.Run("Reload with no command", func(t *testing.T) {
		managerNoReload := NewManager(configPath, zonesPath, []string{}, "test.local")
		err := managerNoReload.Reload()
		assert.NoError(t, err, "Reload should not error when no command is configured")
	})

	t.Run("Reload with failing command", func(t *testing.T) {
		managerFailingReload := NewManager(configPath, zonesPath, []string{"/bin/false"}, "test.local")
		err := managerFailingReload.Reload()
		assert.Error(t, err)
		assert.True(t, strings.Contains(err.Error(), "reloading CoreDNS failed"), "Error message should indicate failure")
	})
}
