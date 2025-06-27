// Package persistence provides file-based storage implementations with atomic operations and backup support
package persistence

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/jerkytreats/dns/internal/config"
	"github.com/jerkytreats/dns/internal/logging"
)

// FileStorage provides thread-safe file-based storage with atomic operations
type FileStorage struct {
	filePath    string
	backupCount int
	mutex       sync.RWMutex
}

// NewFileStorage creates a new file storage instance
func NewFileStorage() *FileStorage {
	filePath := config.GetString(config.DeviceStoragePathKey)
	backupCount := config.GetInt(config.DeviceStorageBackupCountKey)

	return &FileStorage{
		filePath:    filePath,
		backupCount: backupCount,
	}
}

// NewFileStorageWithPath creates a new file storage instance with custom path
func NewFileStorageWithPath(filePath string, backupCount int) *FileStorage {
	return &FileStorage{
		filePath:    filePath,
		backupCount: backupCount,
	}
}

// Read reads data from the storage file
func (fs *FileStorage) Read() ([]byte, error) {
	fs.mutex.RLock()
	defer fs.mutex.RUnlock()

	logging.Debug("Reading device data from: %s", fs.filePath)

	data, err := os.ReadFile(fs.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			logging.Debug("Device storage file does not exist, will create on first write")
			return nil, nil
		}

		logging.Warn("Failed to read device storage file, attempting recovery from backup: %v", err)
		return fs.recoverFromBackup()
	}

	logging.Debug("Successfully read %d bytes from device storage", len(data))
	return data, nil
}

// Write atomically writes data to the storage file with backup
func (fs *FileStorage) Write(data []byte) error {
	fs.mutex.Lock()
	defer fs.mutex.Unlock()

	logging.Debug("Writing %d bytes to device storage: %s", len(data), fs.filePath)

	if err := fs.ensureDirectory(); err != nil {
		return fmt.Errorf("failed to ensure storage directory: %w", err)
	}

	// Create backup if file exists
	if err := fs.createBackup(); err != nil {
		logging.Warn("Failed to create backup before write: %v", err)
		// Continue with write even if backup fails
	}

	// Write to temporary file first (atomic operation)
	tempFile := fs.filePath + ".tmp"
	if err := os.WriteFile(tempFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write temporary file: %w", err)
	}

	// Atomically move temporary file to target location
	if err := os.Rename(tempFile, fs.filePath); err != nil {
		// Clean up temporary file on failure
		os.Remove(tempFile)
		return fmt.Errorf("failed to move temporary file to target: %w", err)
	}

	// Clean up old backups
	if err := fs.cleanupOldBackups(); err != nil {
		logging.Warn("Failed to cleanup old backups: %v", err)
		// Continue - this is not a critical error
	}

	logging.Debug("Successfully wrote device data to storage")
	return nil
}

// Exists checks if the storage file exists
func (fs *FileStorage) Exists() bool {
	fs.mutex.RLock()
	defer fs.mutex.RUnlock()

	_, err := os.Stat(fs.filePath)
	return err == nil
}

// GetPath returns the storage file path
func (fs *FileStorage) GetPath() string {
	return fs.filePath
}

// ensureDirectory creates the directory for the storage file if it doesn't exist
func (fs *FileStorage) ensureDirectory() error {
	dir := filepath.Dir(fs.filePath)
	return os.MkdirAll(dir, 0755)
}

// createBackup creates a backup of the current file
func (fs *FileStorage) createBackup() error {
	if !fs.fileExists(fs.filePath) {
		return nil // No file to backup
	}

	timestamp := time.Now().Format("20060102-150405")
	backupPath := fmt.Sprintf("%s.backup.%s", fs.filePath, timestamp)

	data, err := os.ReadFile(fs.filePath)
	if err != nil {
		return fmt.Errorf("failed to read current file for backup: %w", err)
	}

	if err := os.WriteFile(backupPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write backup file: %w", err)
	}

	logging.Debug("Created backup: %s", backupPath)
	return nil
}

// cleanupOldBackups removes old backup files beyond the configured limit
func (fs *FileStorage) cleanupOldBackups() error {
	if fs.backupCount <= 0 {
		return nil // No cleanup needed
	}

	dir := filepath.Dir(fs.filePath)
	baseName := filepath.Base(fs.filePath)
	backupPattern := baseName + ".backup.*"

	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("failed to read directory for backup cleanup: %w", err)
	}

	var backupFiles []os.DirEntry
	for _, entry := range entries {
		if matched, _ := filepath.Match(backupPattern, entry.Name()); matched {
			backupFiles = append(backupFiles, entry)
		}
	}

	if len(backupFiles) <= fs.backupCount {
		return nil // No cleanup needed
	}

	// Sort by modification time (newest first)
	sort.Slice(backupFiles, func(i, j int) bool {
		info1, _ := backupFiles[i].Info()
		info2, _ := backupFiles[j].Info()
		return info1.ModTime().After(info2.ModTime())
	})

	// Remove excess backup files
	for i := fs.backupCount; i < len(backupFiles); i++ {
		backupPath := filepath.Join(dir, backupFiles[i].Name())
		if err := os.Remove(backupPath); err != nil {
			logging.Warn("Failed to remove old backup %s: %v", backupPath, err)
		} else {
			logging.Debug("Removed old backup: %s", backupPath)
		}
	}

	return nil
}

// recoverFromBackup attempts to recover data from the most recent backup
func (fs *FileStorage) recoverFromBackup() ([]byte, error) {
	dir := filepath.Dir(fs.filePath)
	baseName := filepath.Base(fs.filePath)
	backupPattern := baseName + ".backup.*"

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory for backup recovery: %w", err)
	}

	var backupFiles []os.DirEntry
	for _, entry := range entries {
		if matched, _ := filepath.Match(backupPattern, entry.Name()); matched {
			backupFiles = append(backupFiles, entry)
		}
	}

	if len(backupFiles) == 0 {
		return nil, fmt.Errorf("no backup files found for recovery")
	}

	// Sort by modification time (newest first)
	sort.Slice(backupFiles, func(i, j int) bool {
		info1, _ := backupFiles[i].Info()
		info2, _ := backupFiles[j].Info()
		return info1.ModTime().After(info2.ModTime())
	})

	// Try to recover from the most recent backup
	mostRecentBackup := filepath.Join(dir, backupFiles[0].Name())
	data, err := os.ReadFile(mostRecentBackup)
	if err != nil {
		return nil, fmt.Errorf("failed to read most recent backup %s: %w", mostRecentBackup, err)
	}

	logging.Info("Successfully recovered device data from backup: %s", mostRecentBackup)
	return data, nil
}

// fileExists checks if a file exists
func (fs *FileStorage) fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// ListBackups returns a list of available backup files sorted by timestamp (newest first)
func (fs *FileStorage) ListBackups() ([]string, error) {
	fs.mutex.RLock()
	defer fs.mutex.RUnlock()

	dir := filepath.Dir(fs.filePath)
	baseName := filepath.Base(fs.filePath)
	backupPattern := baseName + ".backup.*"

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory: %w", err)
	}

	var backupFiles []string
	for _, entry := range entries {
		if matched, _ := filepath.Match(backupPattern, entry.Name()); matched {
			backupFiles = append(backupFiles, entry.Name())
		}
	}

	// Sort by timestamp in filename (newest first)
	sort.Slice(backupFiles, func(i, j int) bool {
		// Extract timestamp from filename
		ts1 := fs.extractTimestamp(backupFiles[i])
		ts2 := fs.extractTimestamp(backupFiles[j])
		return ts1 > ts2
	})

	return backupFiles, nil
}

// extractTimestamp extracts timestamp from backup filename
func (fs *FileStorage) extractTimestamp(filename string) string {
	parts := strings.Split(filename, ".backup.")
	if len(parts) < 2 {
		return ""
	}
	return parts[len(parts)-1]
}

// GetStorageInfo returns information about the storage file and backups
func (fs *FileStorage) GetStorageInfo() map[string]interface{} {
	fs.mutex.RLock()
	defer fs.mutex.RUnlock()

	info := map[string]interface{}{
		"file_path":    fs.filePath,
		"backup_count": fs.backupCount,
		"exists":       fs.fileExists(fs.filePath),
	}

	if stat, err := os.Stat(fs.filePath); err == nil {
		info["size"] = stat.Size()
		info["modified"] = stat.ModTime()
	}

	backups, err := fs.ListBackups()
	if err == nil {
		info["available_backups"] = len(backups)
		info["backup_files"] = backups
	}

	return info
}
