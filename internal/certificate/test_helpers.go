package certificate

import (
	"os"
	"path/filepath"
	"testing"
)

// CleanupCertificateDataDir removes all files in the certificate data directory
// This function should be called in test setup or teardown to prevent test data accumulation
func CleanupCertificateDataDir(t *testing.T) {
	t.Helper()

	// Get the absolute path to the certificate data directory
	// This assumes the current working directory is the project root
	dataDir := filepath.Join("internal", "certificate", "data")
	absPath, err := filepath.Abs(dataDir)
	if err != nil {
		t.Logf("Warning: Failed to get absolute path for certificate data directory: %v", err)
		return
	}

	// Ensure the directory exists
	if _, err := os.Stat(absPath); os.IsNotExist(err) {
		t.Logf("Certificate data directory does not exist: %s", absPath)
		return
	}

	// Read all entries in the directory
	entries, err := os.ReadDir(absPath)
	if err != nil {
		t.Logf("Warning: Failed to read certificate data directory: %v", err)
		return
	}

	// Count of files removed
	removed := 0

	// Remove all files except .gitkeep
	for _, entry := range entries {
		if entry.Name() == ".gitkeep" {
			continue
		}

		filePath := filepath.Join(absPath, entry.Name())
		if err := os.Remove(filePath); err != nil {
			t.Logf("Warning: Failed to remove file %s: %v", filePath, err)
		} else {
			removed++
		}
	}

	// Create .gitkeep file if it doesn't exist
	gitkeepPath := filepath.Join(absPath, ".gitkeep")
	if _, err := os.Stat(gitkeepPath); os.IsNotExist(err) {
		file, err := os.Create(gitkeepPath)
		if err != nil {
			t.Logf("Warning: Failed to create .gitkeep file: %v", err)
		} else {
			file.Close()
		}
	}

	t.Logf("Cleaned up %d files from certificate data directory", removed)
}
