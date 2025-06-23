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

	// Prepare Corefile template required by ConfigManager
	templatePath := filepath.Join(tempDir, "Corefile.template")
	templateContent := `. {
    errors
    log
}
`
	_ = os.WriteFile(templatePath, []byte(templateContent), 0644)

	// Mock reload script path (still used for Reload tests)
	reloadScriptPath := filepath.Join(tempDir, "reload.sh")
	reloadScriptContent := "#!/bin/sh\necho 'reloaded'"
	_ = os.WriteFile(reloadScriptPath, []byte(reloadScriptContent), 0755)

	manager := NewManager(configPath, templatePath, zonesPath, []string{reloadScriptPath}, "test.local")

	t.Run("AddRecord", func(t *testing.T) {
		// Before adding a record, we need a zone. Let's create a dummy zone file.
		zoneFileName := filepath.Join(zonesPath, "test-service.test.local.zone")
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
		managerNoReload := NewManager(configPath, templatePath, zonesPath, []string{}, "test.local")
		err := managerNoReload.Reload()
		assert.NoError(t, err, "Reload should not error when no command is configured")
	})

	t.Run("Reload with failing command", func(t *testing.T) {
		managerFailingReload := NewManager(configPath, templatePath, zonesPath, []string{"/bin/false"}, "test.local")
		err := managerFailingReload.Reload()
		assert.Error(t, err)
		assert.True(t, strings.Contains(err.Error(), "reloading CoreDNS failed"), "Error message should indicate failure")
	})
}

func TestZoneValidation(t *testing.T) {
	// Setup test environment
	tempDir, err := os.MkdirTemp("", "coredns-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	configPath := filepath.Join(tempDir, "Corefile")
	zonesPath := filepath.Join(tempDir, "zones")

	// Create initial Corefile with existing zone
	initialConfig := `test.local:53 {
    errors
    log
    file /zones/test.local.zone
}

.:53 {
    forward . 8.8.8.8
}`
	err = os.WriteFile(configPath, []byte(initialConfig), 0644)
	require.NoError(t, err)

	// Create template and config manager for validation tests
	templatePath2 := filepath.Join(tempDir, "Corefile.template2")
	_ = os.WriteFile(templatePath2, []byte(`. {
    errors
}

{{range .Domains}}
{{if .Enabled}}

# Configuration for {{.Domain}}
{{.Domain}}:{{.Port}} {
    file {{.ZoneFile}} {{.Domain}}
    errors
    log
}

{{end}}
{{end}}
`), 0644)
	manager := NewManager(configPath, templatePath2, zonesPath, []string{}, "test.local")

	t.Run("AddZone creates new zone", func(t *testing.T) {
		err := manager.AddZone("new-service")
		require.NoError(t, err)

		// Verify zone file was created
		zoneFile := filepath.Join(zonesPath, "new-service.test.local.zone")
		_, err = os.Stat(zoneFile)
		require.NoError(t, err)

		// Verify Corefile was updated
		config, err := os.ReadFile(configPath)
		require.NoError(t, err)
		assert.Contains(t, string(config), "new-service.test.local:53")
	})

	t.Run("AddZone does not duplicate existing zone", func(t *testing.T) {
		// Add the same zone twice
		err := manager.AddZone("duplicate-service")
		require.NoError(t, err)

		err = manager.AddZone("duplicate-service")
		require.NoError(t, err) // Should not error

		// Verify only one occurrence in Corefile
		config, err := os.ReadFile(configPath)
		require.NoError(t, err)
		configStr := string(config)

		count := strings.Count(configStr, "duplicate-service.test.local:53")
		assert.Equal(t, 1, count, "Zone should only appear once in Corefile")
	})

	t.Run("zoneExistsInConfig detects existing zones", func(t *testing.T) {
		config := `test.local:53 {
    errors
    log
}

existing-zone.test.local:53 {
    file /zones/existing-zone.zone
    errors
    log
}`

		// Test existing zone detection
		exists := manager.zoneExistsInConfig(config, "existing-zone.test.local:53")
		assert.True(t, exists, "Should detect existing zone")

		// Test non-existing zone
		exists = manager.zoneExistsInConfig(config, "non-existing.test.local:53")
		assert.False(t, exists, "Should not detect non-existing zone")

		// Test partial matches don't trigger false positives
		exists = manager.zoneExistsInConfig(config, "test.local:53")
		assert.True(t, exists, "Should detect zone at start of config")
	})

	t.Run("RemoveZone cleans up properly", func(t *testing.T) {
		// Add a zone
		err := manager.AddZone("removable-service")
		require.NoError(t, err)

		// Verify it exists
		config, err := os.ReadFile(configPath)
		require.NoError(t, err)
		assert.Contains(t, string(config), "removable-service.test.local:53")

		// Remove the zone
		err = manager.RemoveZone("removable-service")
		require.NoError(t, err)

		// Verify it's gone from config
		config, err = os.ReadFile(configPath)
		require.NoError(t, err)
		assert.NotContains(t, string(config), "removable-service.test.local:53")

		// Verify zone file is removed
		zoneFile := filepath.Join(zonesPath, "removable-service.test.local.zone")
		_, err = os.Stat(zoneFile)
		assert.True(t, os.IsNotExist(err), "Zone file should be removed")
	})
}
