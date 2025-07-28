package proxy

import (
	"encoding/json"
	"fmt"

	"github.com/jerkytreats/dns/internal/config"
	"github.com/jerkytreats/dns/internal/logging"
	"github.com/jerkytreats/dns/internal/persistence"
)

// Configuration keys for proxy rule storage
const (
	ProxyRuleStoragePathKey        = "proxy.storage.path"
	ProxyRuleStorageBackupCountKey = "proxy.storage.backup_count"
)

// Default values for proxy rule storage
const (
	DefaultProxyRuleStoragePath        = "data/proxy_rules.json"
	DefaultProxyRuleStorageBackupCount = 3
)

// ProxyRuleStorage handles persistence of proxy rules to local file storage
type ProxyRuleStorage struct {
	storage *persistence.FileStorage
}

// NewProxyRuleStorage creates a new proxy rule storage instance
func NewProxyRuleStorage() *ProxyRuleStorage {
	storagePath := config.GetString(ProxyRuleStoragePathKey)
	if storagePath == "" {
		storagePath = DefaultProxyRuleStoragePath
	}

	backupCount := config.GetInt(ProxyRuleStorageBackupCountKey)
	if backupCount <= 0 {
		backupCount = DefaultProxyRuleStorageBackupCount
	}

	return &ProxyRuleStorage{
		storage: persistence.NewFileStorageWithPath(storagePath, backupCount),
	}
}

// LoadRules loads all proxy rules from storage
func (prs *ProxyRuleStorage) LoadRules() (map[string]*ProxyRule, error) {
	logging.Debug("Loading proxy rules from storage: %s", prs.storage.GetPath())

	data, err := prs.storage.Read()
	if err != nil {
		return nil, fmt.Errorf("failed to read proxy rules from storage: %w", err)
	}

	// Return empty map if no data (first run)
	if data == nil {
		logging.Debug("No proxy rules found in storage, returning empty set")
		return make(map[string]*ProxyRule), nil
	}

	var rules map[string]*ProxyRule
	if err := json.Unmarshal(data, &rules); err != nil {
		return nil, fmt.Errorf("failed to unmarshal proxy rules: %w", err)
	}

	// Validate all loaded rules
	validRules := make(map[string]*ProxyRule)
	for hostname, rule := range rules {
		if err := rule.Validate(); err != nil {
			logging.Warn("Skipping invalid proxy rule for %s: %v", hostname, err)
			continue
		}
		validRules[hostname] = rule
	}

	logging.Info("Loaded %d valid proxy rules from storage", len(validRules))
	return validRules, nil
}

// SaveRules saves all proxy rules to storage
func (prs *ProxyRuleStorage) SaveRules(rules map[string]*ProxyRule) error {
	logging.Debug("Saving %d proxy rules to storage: %s", len(rules), prs.storage.GetPath())

	// Validate all rules before saving
	for hostname, rule := range rules {
		if err := rule.Validate(); err != nil {
			return fmt.Errorf("invalid proxy rule for %s: %w", hostname, err)
		}
	}

	data, err := json.MarshalIndent(rules, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal proxy rules: %w", err)
	}

	if err := prs.storage.Write(data); err != nil {
		return fmt.Errorf("failed to write proxy rules to storage: %w", err)
	}

	logging.Debug("Successfully saved proxy rules to storage")
	return nil
}

// Exists checks if the proxy rules storage file exists
func (prs *ProxyRuleStorage) Exists() bool {
	return prs.storage.Exists()
}

// GetStorageInfo returns information about the proxy rules storage
func (prs *ProxyRuleStorage) GetStorageInfo() map[string]interface{} {
	info := prs.storage.GetStorageInfo()
	info["type"] = "proxy_rules"
	return info
}