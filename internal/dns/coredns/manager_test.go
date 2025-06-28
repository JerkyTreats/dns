package coredns

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jerkytreats/dns/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManager(t *testing.T) {
	// Setup test environment
	tempDir, err := os.MkdirTemp("", "coredns-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	configPath := filepath.Join(tempDir, "Corefile")

	templatePath := filepath.Join(tempDir, "Corefile.template")
	zonesPath := filepath.Join(tempDir, "zones")
	domain := "test.local"

	config.SetForTest(DNSConfigPathKey, configPath)
	config.SetForTest(DNSTemplatePathKey, templatePath)
	config.SetForTest(DNSZonesPathKey, zonesPath)
	config.SetForTest(DNSDomainKey, domain)

	// Prepare Corefile template required by ConfigManager
	templateContent := `. {
    errors
    log
}
`
	_ = os.WriteFile(templatePath, []byte(templateContent), 0644)

	manager := NewManager("")

	t.Run("AddRecord", func(t *testing.T) {
		// Before adding a record, a zone must exist.
		err := manager.AddZone("test-service")
		require.NoError(t, err)

		err = manager.AddRecord("test-service", "test-record", "127.0.0.1")
		require.NoError(t, err)

		// Verify the content of the zone file
		zoneFileName := filepath.Join(zonesPath, "test.local.zone")
		content, err := os.ReadFile(zoneFileName)
		require.NoError(t, err)
		expectedRecord := "test-record\tIN A\t127.0.0.1"
		assert.Contains(t, string(content), expectedRecord)
	})

	t.Run("DropRecord", func(t *testing.T) {
		// Setup: Create a zone file with a couple of records via the manager
		err := manager.AddZone("test-service-drop")
		require.NoError(t, err)
		err = manager.AddRecord("test-service-drop", "record-to-keep", "192.168.1.1")
		require.NoError(t, err)
		err = manager.AddRecord("test-service-drop", "record-to-drop", "192.168.1.2")
		require.NoError(t, err)

		// Action: Drop one of the records
		err = manager.DropRecord("test-service-drop", "record-to-drop", "192.168.1.2")
		require.NoError(t, err)

		// Verification
		zoneFileName := filepath.Join(zonesPath, "test.local.zone")
		content, err := os.ReadFile(zoneFileName)
		require.NoError(t, err)
		contentStr := string(content)

		assert.NotContains(t, contentStr, "record-to-drop	IN A	192.168.1.2")
		assert.Contains(t, contentStr, "record-to-keep	IN A	192.168.1.1")

		// Test dropping a non-existent record
		err = manager.DropRecord("test-service-drop", "non-existent-record", "1.2.3.4")
		require.NoError(t, err)

		contentAfterBogusDrop, err := os.ReadFile(zoneFileName)
		require.NoError(t, err)
		assert.Equal(t, string(content), string(contentAfterBogusDrop), "Dropping a non-existent record should not change the file")
	})

	t.Run("ListRecords", func(t *testing.T) {
		// Create separate test environment for this test
		tempDirList, err := os.MkdirTemp("", "coredns-list-test-*")
		require.NoError(t, err)
		defer os.RemoveAll(tempDirList)

		configPathList := filepath.Join(tempDirList, "Corefile")
		templatePathList := filepath.Join(tempDirList, "Corefile.template")
		zonesPathList := filepath.Join(tempDirList, "zones")
		domainList := "test.local"

		config.SetForTest(DNSConfigPathKey, configPathList)
		config.SetForTest(DNSTemplatePathKey, templatePathList)
		config.SetForTest(DNSZonesPathKey, zonesPathList)
		config.SetForTest(DNSDomainKey, domainList)

		// Prepare Corefile template
		templateContent := `. {
    errors
    log
}
`
		_ = os.WriteFile(templatePathList, []byte(templateContent), 0644)

		managerList := NewManager("")

		// Setup: Create a zone with multiple records
		err = managerList.AddZone("test-service-list")
		require.NoError(t, err)
		err = managerList.AddRecord("test-service-list", "device1", "100.64.1.1")
		require.NoError(t, err)
		err = managerList.AddRecord("test-service-list", "device2", "100.64.1.2")
		require.NoError(t, err)
		err = managerList.AddRecord("test-service-list", "server", "192.168.1.100")
		require.NoError(t, err)

		// Test listing records
		records, err := managerList.ListRecords("test-service-list")
		require.NoError(t, err)

		// Verify correct number of records (3 added + 1 NS record)
		assert.Len(t, records, 4)

		// Verify record content
		recordMap := make(map[string]string)
		for _, record := range records {
			assert.Equal(t, "A", record.Type)
			recordMap[record.Name] = record.IP
		}

		assert.Equal(t, "100.64.1.1", recordMap["device1"])
		assert.Equal(t, "100.64.1.2", recordMap["device2"])
		assert.Equal(t, "192.168.1.100", recordMap["server"])
		assert.Equal(t, "127.0.0.1", recordMap["ns"]) // NS record automatically created
	})

	t.Run("ListRecords_EmptyZone", func(t *testing.T) {
		// Create separate test environment for this test
		tempDirEmpty, err := os.MkdirTemp("", "coredns-empty-test-*")
		require.NoError(t, err)
		defer os.RemoveAll(tempDirEmpty)

		configPathEmpty := filepath.Join(tempDirEmpty, "Corefile")
		templatePathEmpty := filepath.Join(tempDirEmpty, "Corefile.template")
		zonesPathEmpty := filepath.Join(tempDirEmpty, "zones")
		domainEmpty := "test.local"

		config.SetForTest(DNSConfigPathKey, configPathEmpty)
		config.SetForTest(DNSTemplatePathKey, templatePathEmpty)
		config.SetForTest(DNSZonesPathKey, zonesPathEmpty)
		config.SetForTest(DNSDomainKey, domainEmpty)

		// Prepare Corefile template
		templateContent := `. {
    errors
    log
}
`
		_ = os.WriteFile(templatePathEmpty, []byte(templateContent), 0644)

		managerEmpty := NewManager("")

		// Setup: Create an empty zone
		err = managerEmpty.AddZone("empty-service")
		require.NoError(t, err)

		// Test listing records from empty zone (should have 1 NS record)
		records, err := managerEmpty.ListRecords("empty-service")
		require.NoError(t, err)
		assert.Len(t, records, 1)
		assert.Equal(t, "ns", records[0].Name)
		assert.Equal(t, "A", records[0].Type)
		assert.Equal(t, "127.0.0.1", records[0].IP)
	})

	t.Run("ListRecords_NonExistentZone", func(t *testing.T) {
		// Create separate test environment for this test
		tempDirNonExistent, err := os.MkdirTemp("", "coredns-nonexistent-test-*")
		require.NoError(t, err)
		defer os.RemoveAll(tempDirNonExistent)

		configPathNonExistent := filepath.Join(tempDirNonExistent, "Corefile")
		templatePathNonExistent := filepath.Join(tempDirNonExistent, "Corefile.template")
		zonesPathNonExistent := filepath.Join(tempDirNonExistent, "zones")
		domainNonExistent := "test.local"

		config.SetForTest(DNSConfigPathKey, configPathNonExistent)
		config.SetForTest(DNSTemplatePathKey, templatePathNonExistent)
		config.SetForTest(DNSZonesPathKey, zonesPathNonExistent)
		config.SetForTest(DNSDomainKey, domainNonExistent)

		managerNonExistent := NewManager("")

		// Test listing records from non-existent zone
		records, err := managerNonExistent.ListRecords("non-existent-service")
		require.NoError(t, err)
		assert.Len(t, records, 0)
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
	config.SetForTest(DNSTemplatePathKey, templatePath2)
	manager := NewManager("")

	t.Run("AddZone creates new zone", func(t *testing.T) {
		err := manager.AddZone("new-service")
		require.NoError(t, err)

		// Verify zone file was created
		zoneFile := filepath.Join(zonesPath, "test.local.zone")
		_, err = os.Stat(zoneFile)
		require.NoError(t, err)

		// Verify Corefile was updated
		config, err := os.ReadFile(configPath)
		require.NoError(t, err)
		assert.Contains(t, string(config), "test.local:53")
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

		count := strings.Count(configStr, "test.local:53")
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
		assert.Contains(t, string(config), "test.local:53")

		// Remove the zone
		err = manager.RemoveZone("removable-service")
		require.NoError(t, err)

		// Verify it's gone from config
		config, err = os.ReadFile(configPath)
		require.NoError(t, err)
		assert.NotContains(t, string(config), "test.local:53")

		// Verify zone file is removed
		zoneFile := filepath.Join(zonesPath, "test.local.zone")
		_, err = os.Stat(zoneFile)
		assert.True(t, os.IsNotExist(err), "Zone file should be removed")
	})
}

func TestManager_AddDomain_NoUnnecessaryRegeneration(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "Corefile")
	templatePath := filepath.Join(tempDir, "Corefile.template")

	// Create a simple template
	templateContent := `{{range .Domains}}
{{.Domain}}:{{.Port}} {
	file {{.ZoneFile}} {{.Domain}}
	errors
	log
}{{end}}
`
	if err := os.WriteFile(templatePath, []byte(templateContent), 0644); err != nil {
		t.Fatalf("Failed to write template: %v", err)
	}

	manager := NewManager("")

	// Check that Corefile doesn't exist initially
	if _, err := os.Stat(configPath); err == nil {
		t.Fatal("Corefile should not exist initially")
	}

	// Add domain for the first time
	if err := manager.AddDomain("test.local", nil); err != nil {
		t.Fatalf("Failed to add domain: %v", err)
	}

	// Check that Corefile was created
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Fatal("Corefile should exist after adding domain")
	}

	firstStat, err := os.Stat(configPath)
	if err != nil {
		t.Fatalf("Failed to stat Corefile: %v", err)
	}

	// Wait a bit to ensure time difference
	time.Sleep(10 * time.Millisecond)

	// Add the same domain again
	if err := manager.AddDomain("test.local", nil); err != nil {
		t.Fatalf("Failed to add domain again: %v", err)
	}

	secondStat, err := os.Stat(configPath)
	if err != nil {
		t.Fatalf("Failed to stat Corefile after second add: %v", err)
	}

	// The modification time should be the same (no regeneration)
	if !firstStat.ModTime().Equal(secondStat.ModTime()) {
		t.Errorf("Corefile was regenerated unnecessarily. First mod time: %v, Second mod time: %v",
			firstStat.ModTime(), secondStat.ModTime())
	}

	// Add domain with different TLS config should regenerate
	time.Sleep(10 * time.Millisecond)
	if err := manager.AddDomain("test.local", &TLSConfig{CertFile: "/cert.pem", KeyFile: "/key.pem", Port: 853}); err != nil {
		t.Fatalf("Failed to add domain with TLS: %v", err)
	}

	thirdStat, err := os.Stat(configPath)
	if err != nil {
		t.Fatalf("Failed to stat Corefile after TLS add: %v", err)
	}

	// The modification time should be different (regeneration occurred)
	if firstStat.ModTime().Equal(thirdStat.ModTime()) {
		t.Errorf("Corefile should have been regenerated for TLS config change. Mod time: %v", thirdStat.ModTime())
	}
}

func TestParseRecordsFromZone(t *testing.T) {
	manager := NewManager("127.0.0.1")

	tests := []struct {
		name     string
		content  string
		expected []Record
	}{
		{
			name: "Multiple A records",
			content: `$ORIGIN test.local.
$TTL 300
@       IN SOA  ns.test.local. admin.test.local. (
                2023010101 ; serial
                3600       ; refresh
                1800       ; retry
                604800     ; expire
                300        ; minimum
                )
@       IN NS   ns.test.local.
device1	IN A	100.64.1.1
device2	IN A	100.64.1.2
server	IN A	192.168.1.100`,
			expected: []Record{
				{Name: "device1", Type: "A", IP: "100.64.1.1"},
				{Name: "device2", Type: "A", IP: "100.64.1.2"},
				{Name: "server", Type: "A", IP: "192.168.1.100"},
			},
		},
		{
			name:     "Empty zone",
			content:  `$ORIGIN test.local.`,
			expected: []Record{},
		},
		{
			name: "Mixed record types - only A records returned",
			content: `device1	IN A	100.64.1.1
device2	IN CNAME	device1
device3	IN A	100.64.1.3
@	IN MX	10 mail.test.local.`,
			expected: []Record{
				{Name: "device1", Type: "A", IP: "100.64.1.1"},
				{Name: "device3", Type: "A", IP: "100.64.1.3"},
			},
		},
		{
			name: "Records with comments and empty lines",
			content: `; This is a comment
device1	IN A	100.64.1.1

; Another comment
device2	IN A	100.64.1.2
# This is also a comment
device3	IN A	100.64.1.3`,
			expected: []Record{
				{Name: "device1", Type: "A", IP: "100.64.1.1"},
				{Name: "device2", Type: "A", IP: "100.64.1.2"},
				{Name: "device3", Type: "A", IP: "100.64.1.3"},
			},
		},
		{
			name: "Records with varying whitespace",
			content: `device1    IN   A    100.64.1.1
device2		IN	A	100.64.1.2
device3 IN A 100.64.1.3`,
			expected: []Record{
				{Name: "device1", Type: "A", IP: "100.64.1.1"},
				{Name: "device2", Type: "A", IP: "100.64.1.2"},
				{Name: "device3", Type: "A", IP: "100.64.1.3"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			records := manager.parseRecordsFromZone(tt.content)
			assert.Equal(t, tt.expected, records)
		})
	}
}

func TestManager_AddZone_OverwritesExistingZone(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "Corefile")
	templatePath := filepath.Join(tempDir, "Corefile.template")
	zonesPath := tempDir

	config.SetForTest(DNSConfigPathKey, configPath)
	config.SetForTest(DNSTemplatePathKey, templatePath)
	config.SetForTest(DNSZonesPathKey, zonesPath)
	config.SetForTest(DNSDomainKey, "test.local")

	// Create a simple template
	templateContent := `{{range .Domains}}
{{.Domain}}:{{.Port}} {
	file {{.ZoneFile}} {{.Domain}}
	errors
	log
}{{end}}
`
	if err := os.WriteFile(templatePath, []byte(templateContent), 0644); err != nil {
		t.Fatalf("Failed to write template: %v", err)
	}

	manager := NewManager("")

	// Add zone for the first time
	if err := manager.AddZone("test-service"); err != nil {
		t.Fatalf("Failed to add zone: %v", err)
	}

	zoneFile := filepath.Join(zonesPath, "test.local.zone")
	if _, err := os.Stat(zoneFile); os.IsNotExist(err) {
		t.Fatal("Zone file should exist after adding zone")
	}

	// Read the initial content
	initialContent, err := os.ReadFile(zoneFile)
	if err != nil {
		t.Fatalf("Failed to read zone file: %v", err)
	}

	// Add some custom records to the zone file
	customContent := string(initialContent) + "\ncustom-record\tIN A\t192.168.1.100"
	if err := os.WriteFile(zoneFile, []byte(customContent), 0644); err != nil {
		t.Fatalf("Failed to write custom content: %v", err)
	}

	// Wait a bit to ensure time difference
	time.Sleep(10 * time.Millisecond)

	// Add the same zone again
	if err := manager.AddZone("test-service"); err != nil {
		t.Fatalf("Failed to add zone again: %v", err)
	}

	// Read the content after second add
	finalContent, err := os.ReadFile(zoneFile)
	if err != nil {
		t.Fatalf("Failed to read zone file after second add: %v", err)
	}

	// The content should be our custom content, not overwritten
	if string(finalContent) != customContent {
		t.Error("Zone file was overwritten - custom content was lost")
	}

	// Check that our custom record was preserved
	if !strings.Contains(string(finalContent), "custom-record") {
		t.Error("Custom record was overwritten - AddZone should have preserved existing content")
	}
}
