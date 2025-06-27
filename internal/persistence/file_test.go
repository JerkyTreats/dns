package persistence

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// setupTestDir creates a temporary directory for testing
func setupTestDir(t *testing.T) string {
	dir, err := os.MkdirTemp("", "file_storage_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	return dir
}

// cleanupTestDir removes the test directory
func cleanupTestDir(t *testing.T, dir string) {
	if err := os.RemoveAll(dir); err != nil {
		t.Errorf("Failed to cleanup test dir: %v", err)
	}
}

func TestNewFileStorage(t *testing.T) {
	storage := NewFileStorageWithPath("test.json", 3)

	if storage == nil {
		t.Fatal("NewFileStorageWithPath returned nil")
	}

	if storage.filePath != "test.json" {
		t.Errorf("Expected filePath 'test.json', got '%s'", storage.filePath)
	}

	if storage.backupCount != 3 {
		t.Errorf("Expected backupCount 3, got %d", storage.backupCount)
	}
}

func TestFileStorage_WriteAndRead(t *testing.T) {
	testDir := setupTestDir(t)
	defer cleanupTestDir(t, testDir)

	filePath := filepath.Join(testDir, "test.json")
	storage := NewFileStorageWithPath(filePath, 3)

	testData := []byte(`{"test": "data"}`)

	// Test writing data
	err := storage.Write(testData)
	if err != nil {
		t.Errorf("Write failed: %v", err)
	}

	// Test reading data
	readData, err := storage.Read()
	if err != nil {
		t.Errorf("Read failed: %v", err)
	}

	if string(readData) != string(testData) {
		t.Errorf("Expected '%s', got '%s'", string(testData), string(readData))
	}
}

func TestFileStorage_ReadNonExistentFile(t *testing.T) {
	testDir := setupTestDir(t)
	defer cleanupTestDir(t, testDir)

	filePath := filepath.Join(testDir, "nonexistent.json")
	storage := NewFileStorageWithPath(filePath, 3)

	data, err := storage.Read()
	if err != nil {
		t.Errorf("Read should not fail for non-existent file, got: %v", err)
	}

	if data != nil {
		t.Errorf("Expected nil data for non-existent file, got: %v", data)
	}
}

func TestFileStorage_Exists(t *testing.T) {
	testDir := setupTestDir(t)
	defer cleanupTestDir(t, testDir)

	filePath := filepath.Join(testDir, "test.json")
	storage := NewFileStorageWithPath(filePath, 3)

	// File should not exist initially
	if storage.Exists() {
		t.Error("File should not exist initially")
	}

	// Write data and check existence
	err := storage.Write([]byte("test"))
	if err != nil {
		t.Errorf("Write failed: %v", err)
	}

	if !storage.Exists() {
		t.Error("File should exist after writing")
	}
}

func TestFileStorage_GetPath(t *testing.T) {
	filePath := "/path/to/test.json"
	storage := NewFileStorageWithPath(filePath, 3)

	if storage.GetPath() != filePath {
		t.Errorf("Expected path '%s', got '%s'", filePath, storage.GetPath())
	}
}

func TestFileStorage_BackupCreation(t *testing.T) {
	testDir := setupTestDir(t)
	defer cleanupTestDir(t, testDir)

	filePath := filepath.Join(testDir, "test.json")
	storage := NewFileStorageWithPath(filePath, 3)

	// Write initial data
	initialData := []byte(`{"version": 1}`)
	err := storage.Write(initialData)
	if err != nil {
		t.Errorf("Initial write failed: %v", err)
	}

	// Write new data (should create backup)
	newData := []byte(`{"version": 2}`)
	err = storage.Write(newData)
	if err != nil {
		t.Errorf("Second write failed: %v", err)
	}

	// Check that backup was created
	entries, err := os.ReadDir(testDir)
	if err != nil {
		t.Errorf("Failed to read test dir: %v", err)
	}

	backupFound := false
	for _, entry := range entries {
		if strings.Contains(entry.Name(), "test.json.backup.") {
			backupFound = true
			break
		}
	}

	if !backupFound {
		t.Error("Backup file should have been created")
	}

	// Verify main file has new data
	readData, err := storage.Read()
	if err != nil {
		t.Errorf("Read failed: %v", err)
	}

	if string(readData) != string(newData) {
		t.Errorf("Expected main file to have new data")
	}
}

func TestFileStorage_BackupCleanup(t *testing.T) {
	testDir := setupTestDir(t)
	defer cleanupTestDir(t, testDir)

	filePath := filepath.Join(testDir, "test.json")
	storage := NewFileStorageWithPath(filePath, 2) // Keep only 2 backups

	// Write multiple times to create more backups than the limit
	for i := 0; i < 5; i++ {
		data := []byte(`{"version": ` + string(rune('0'+i)) + `}`)
		err := storage.Write(data)
		if err != nil {
			t.Errorf("Write %d failed: %v", i, err)
		}
		// Small delay to ensure different timestamps
		time.Sleep(time.Millisecond * 10)
	}

	// Count backup files
	entries, err := os.ReadDir(testDir)
	if err != nil {
		t.Errorf("Failed to read test dir: %v", err)
	}

	backupCount := 0
	for _, entry := range entries {
		if strings.Contains(entry.Name(), "test.json.backup.") {
			backupCount++
		}
	}

	if backupCount > 2 {
		t.Errorf("Expected at most 2 backup files, found %d", backupCount)
	}
}

func TestFileStorage_ListBackups(t *testing.T) {
	testDir := setupTestDir(t)
	defer cleanupTestDir(t, testDir)

	filePath := filepath.Join(testDir, "test.json")
	storage := NewFileStorageWithPath(filePath, 5)

	// Create some backups by writing multiple times
	for i := 0; i < 3; i++ {
		data := []byte(`{"version": ` + string(rune('0'+i)) + `}`)
		err := storage.Write(data)
		if err != nil {
			t.Errorf("Write %d failed: %v", i, err)
		}
		// Sleep for 1+ seconds to ensure different timestamps (format is YYYYMMDD-HHMMSS)
		time.Sleep(time.Second + time.Millisecond*100)
	}

	backups, err := storage.ListBackups()
	if err != nil {
		t.Errorf("ListBackups failed: %v", err)
	}

	// Should have 2 backups (first write doesn't create backup)
	if len(backups) != 2 {
		t.Errorf("Expected 2 backups, got %d", len(backups))
	}

	// Verify backup filenames contain the expected pattern
	for _, backup := range backups {
		if !strings.Contains(backup, "test.json.backup.") {
			t.Errorf("Backup filename should contain pattern, got: %s", backup)
		}
	}
}

func TestFileStorage_RecoveryFromBackup(t *testing.T) {
	testDir := setupTestDir(t)
	defer cleanupTestDir(t, testDir)

	filePath := filepath.Join(testDir, "test.json")
	storage := NewFileStorageWithPath(filePath, 3)

	// Write some data to create a backup
	originalData := []byte(`{"original": "data"}`)
	err := storage.Write(originalData)
	if err != nil {
		t.Errorf("Initial write failed: %v", err)
	}

	// Sleep to ensure different backup timestamp
	time.Sleep(time.Second + time.Millisecond*100)

	// Write again to create backup of the original data
	newData := []byte(`{"new": "data"}`)
	err = storage.Write(newData)
	if err != nil {
		t.Errorf("Second write failed: %v", err)
	}

	// Make the main file unreadable to trigger recovery (recovery is triggered on read errors, not file absence)
	err = os.Chmod(filePath, 0000) // Remove all permissions
	if err != nil {
		t.Errorf("Failed to change file permissions: %v", err)
	}

	// Restore permissions for cleanup
	defer func() {
		os.Chmod(filePath, 0644)
	}()

	// Verify backup exists before attempting recovery
	backups, err := storage.ListBackups()
	if err != nil {
		t.Errorf("Failed to list backups: %v", err)
	}
	if len(backups) == 0 {
		t.Error("No backups found for recovery test")
	}

	// Try to read - should recover from backup
	recoveredData, err := storage.Read()
	if err != nil {
		t.Errorf("Read with recovery failed: %v", err)
	}

	// Should have recovered the original data (from the most recent backup)
	if string(recoveredData) != string(originalData) {
		t.Errorf("Expected recovered data '%s', got '%s'", string(originalData), string(recoveredData))
	}
}

func TestFileStorage_GetStorageInfo(t *testing.T) {
	testDir := setupTestDir(t)
	defer cleanupTestDir(t, testDir)

	filePath := filepath.Join(testDir, "test.json")
	storage := NewFileStorageWithPath(filePath, 3)

	// Get info for non-existent file
	info := storage.GetStorageInfo()

	expectedPath := filePath
	if info["file_path"] != expectedPath {
		t.Errorf("Expected file_path '%s', got '%v'", expectedPath, info["file_path"])
	}

	if info["backup_count"] != 3 {
		t.Errorf("Expected backup_count 3, got %v", info["backup_count"])
	}

	if info["exists"] != false {
		t.Errorf("Expected exists false, got %v", info["exists"])
	}

	// Write data and check info again
	testData := []byte("test data")
	err := storage.Write(testData)
	if err != nil {
		t.Errorf("Write failed: %v", err)
	}

	info = storage.GetStorageInfo()
	if info["exists"] != true {
		t.Errorf("Expected exists true after write, got %v", info["exists"])
	}

	if info["size"] != int64(len(testData)) {
		t.Errorf("Expected size %d, got %v", len(testData), info["size"])
	}
}

func TestFileStorage_AtomicWrite(t *testing.T) {
	testDir := setupTestDir(t)
	defer cleanupTestDir(t, testDir)

	filePath := filepath.Join(testDir, "test.json")
	storage := NewFileStorageWithPath(filePath, 3)

	// Write initial data
	initialData := []byte(`{"initial": "data"}`)
	err := storage.Write(initialData)
	if err != nil {
		t.Errorf("Initial write failed: %v", err)
	}

	// Verify no temporary file exists
	tempFile := filePath + ".tmp"
	if _, err := os.Stat(tempFile); !os.IsNotExist(err) {
		t.Error("Temporary file should not exist after successful write")
	}

	// Verify main file exists and has correct content
	if !storage.Exists() {
		t.Error("Main file should exist after write")
	}

	readData, err := storage.Read()
	if err != nil {
		t.Errorf("Read failed: %v", err)
	}

	if string(readData) != string(initialData) {
		t.Error("File content should match written data")
	}
}

func TestFileStorage_DirectoryCreation(t *testing.T) {
	testDir := setupTestDir(t)
	defer cleanupTestDir(t, testDir)

	// Use a nested path that doesn't exist
	nestedPath := filepath.Join(testDir, "nested", "deep", "test.json")
	storage := NewFileStorageWithPath(nestedPath, 3)

	testData := []byte("test data")
	err := storage.Write(testData)
	if err != nil {
		t.Errorf("Write to nested path failed: %v", err)
	}

	// Verify directory was created
	if !storage.Exists() {
		t.Error("File should exist after write to nested path")
	}

	// Verify content
	readData, err := storage.Read()
	if err != nil {
		t.Errorf("Read from nested path failed: %v", err)
	}

	if string(readData) != string(testData) {
		t.Error("Content should match for nested path write")
	}
}

func TestFileStorage_ConcurrentAccess(t *testing.T) {
	testDir := setupTestDir(t)
	defer cleanupTestDir(t, testDir)

	filePath := filepath.Join(testDir, "test.json")
	storage := NewFileStorageWithPath(filePath, 3)

	// Test concurrent writes and reads
	done := make(chan bool, 4)

	// Concurrent writers
	for i := 0; i < 2; i++ {
		go func(id int) {
			data := []byte(`{"writer": ` + string(rune('0'+id)) + `}`)
			err := storage.Write(data)
			if err != nil {
				t.Errorf("Concurrent write %d failed: %v", id, err)
			}
			done <- true
		}(i)
	}

	// Concurrent readers
	for i := 0; i < 2; i++ {
		go func(id int) {
			_, err := storage.Read()
			// Read might fail initially if file doesn't exist yet, that's ok
			if err != nil && !os.IsNotExist(err) {
				t.Errorf("Concurrent read %d failed with unexpected error: %v", id, err)
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 4; i++ {
		<-done
	}

	// Verify final state is valid
	if storage.Exists() {
		data, err := storage.Read()
		if err != nil {
			t.Errorf("Final read failed: %v", err)
		}
		if len(data) == 0 {
			t.Error("Final data should not be empty")
		}
	}
}

func TestFileStorage_ExtractTimestamp(t *testing.T) {
	storage := NewFileStorageWithPath("test.json", 3)

	tests := []struct {
		filename string
		expected string
	}{
		{"test.json.backup.20240101-120000", "20240101-120000"},
		{"test.json.backup.20240102-140000", "20240102-140000"},
		{"invalid.filename", ""},
		{"test.json.backup.", ""},
	}

	for _, tt := range tests {
		result := storage.extractTimestamp(tt.filename)
		if result != tt.expected {
			t.Errorf("extractTimestamp('%s') = '%s', expected '%s'", tt.filename, result, tt.expected)
		}
	}
}

func TestFileStorage_ErrorHandling(t *testing.T) {
	// Test write to read-only directory (if we can create one)
	testDir := setupTestDir(t)
	defer cleanupTestDir(t, testDir)

	// Make directory read-only
	err := os.Chmod(testDir, 0444)
	if err != nil {
		t.Skipf("Cannot make directory read-only: %v", err)
	}

	// Restore permissions for cleanup
	defer func() {
		os.Chmod(testDir, 0755)
	}()

	filePath := filepath.Join(testDir, "test.json")
	storage := NewFileStorageWithPath(filePath, 3)

	// This should fail due to permissions
	err = storage.Write([]byte("test"))
	if err == nil {
		t.Error("Expected write to fail due to permissions")
	}
}
