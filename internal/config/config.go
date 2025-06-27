// Package config provides centralized configuration loading for DNS using spf13/viper.
// All config access must go through this package.
package config

import (
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/spf13/viper"
)

// Exported configuration keys
const (
	LogLevelKey = "log_level"

	// DNS Sync configuration keys
	DNSSyncEnabledKey         = "dns.internal.enabled"
	DNSInternalOriginKey      = "dns.internal.origin"
	DNSSyncPollingEnabledKey  = "dns.internal.polling.enabled"
	DNSSyncPollingIntervalKey = "dns.internal.polling.interval"

	// Tailscale configuration keys
	TailscaleAPIKeyKey  = "tailscale.api_key"
	TailscaleTailnetKey = "tailscale.tailnet"
	TailscaleBaseURLKey = "tailscale.base_url" // Optional, for testing

	// Device storage configuration keys
	DeviceStoragePathKey        = "device.storage.path"
	DeviceStorageBackupCountKey = "device.storage.backup_count"
)

// PollingConfig represents the polling configuration section
type PollingConfig struct {
	Enabled  bool          `yaml:"enabled" mapstructure:"enabled"`
	Interval time.Duration `yaml:"interval" mapstructure:"interval"`
}

// SyncConfig represents the sync configuration section
type SyncConfig struct {
	Enabled bool          `yaml:"enabled" mapstructure:"enabled"`
	Origin  string        `yaml:"origin" mapstructure:"origin"`
	Polling PollingConfig `yaml:"polling" mapstructure:"polling"`
}

// Config holds the configuration state and provides thread-safe access
type Config struct {
	viper       *viper.Viper
	initialized bool
	initOnce    sync.Once
	mu          sync.RWMutex
	configPath  string
	searchPaths []string
}

var (
	instance          *Config
	instanceOnce      sync.Once
	requiredKeys      []string
	requiredKeysMutex sync.Mutex
	missingKeys       []string
)

// getInstance returns the singleton config instance
func getInstance() *Config {
	instanceOnce.Do(func() {
		instance = &Config{
			searchPaths: []string{"./configs"},
		}
	})
	return instance
}

func FirstTimeInit(configFile *string) error {
	if *configFile != "" {
		if err := InitConfig(WithConfigPath(*configFile)); err != nil {
			fmt.Printf("Failed to initialize configuration: %v\n", err)
			os.Exit(1)
		}
	} else {
		if err := InitConfig(); err != nil {
			fmt.Printf("Failed to initialize configuration: %v\n", err)
			os.Exit(1)
		}
	}

	if err := CheckRequiredKeys(); err != nil {
		fmt.Printf("Configuration validation failed: %v\n", err)
		os.Exit(1)
	}

	return nil
}

// InitConfig explicitly initializes the configuration with optional parameters
func InitConfig(opts ...ConfigOption) error {
	cfg := getInstance()
	return cfg.init(opts...)
}

// ConfigOption allows for functional options pattern
type ConfigOption func(*Config)

// WithConfigPath sets a specific config file path
// This will override any search paths - use either this OR search paths, not both
func WithConfigPath(path string) ConfigOption {
	return func(c *Config) {
		c.configPath = path
		// Clear search paths since explicit path takes precedence
		c.searchPaths = nil
	}
}

// WithSearchPaths sets additional search paths for config files
// Only used if no explicit config path is set
func WithSearchPaths(paths ...string) ConfigOption {
	return func(c *Config) {
		// Only apply if no explicit config path is set
		if c.configPath == "" {
			c.searchPaths = append(c.searchPaths, paths...)
		}
	}
}

// WithOnlySearchPaths replaces the default search paths entirely
// Only used if no explicit config path is set
func WithOnlySearchPaths(paths ...string) ConfigOption {
	return func(c *Config) {
		// Only apply if no explicit config path is set
		if c.configPath == "" {
			c.searchPaths = paths
		}
	}
}

// init initializes the config instance with the provided options
func (c *Config) init(opts ...ConfigOption) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	var err error
	c.initOnce.Do(func() {
		// Apply options
		for _, opt := range opts {
			opt(c)
		}

		// Load config
		c.viper, err = c.loadConfig()
		if err == nil {
			c.initialized = true
		}
	})
	return err
}

// loadConfig initializes viper and loads config from file and env.
func (c *Config) loadConfig() (*viper.Viper, error) {
	v := viper.New()
	v.SetConfigType("yaml")
	v.SetConfigName("config")

	// Mutually exclusive: either use explicit config file OR search paths
	if c.configPath != "" {
		// Use explicit config file path
		v.SetConfigFile(c.configPath)
	} else {
		// Use search paths to find config file
		for _, path := range c.searchPaths {
			v.AddConfigPath(path)
		}
	}

	v.AutomaticEnv()
	v.SetDefault(LogLevelKey, "INFO")
	v.SetDefault(DNSSyncPollingEnabledKey, false)
	v.SetDefault(DNSSyncPollingIntervalKey, "1h")
	v.SetDefault(DeviceStoragePathKey, "data/devices.json")
	v.SetDefault(DeviceStorageBackupCountKey, 3)

	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			// File not found: return viper instance with defaults
			return v, nil
		}
		// For parse errors or other errors, log and return viper with defaults
		return v, nil
	}
	return v, nil
}

// ensureInitialized ensures config is initialized (lazy loading fallback)
func (c *Config) ensureInitialized() error {
	c.mu.RLock()
	if c.initialized {
		c.mu.RUnlock()
		return nil
	}
	c.mu.RUnlock()

	// Need to initialize
	return c.init()
}

// Reload reloads the configuration from disk (for hot reload, optional).
func Reload() error {
	cfg := getInstance()
	cfg.mu.Lock()
	defer cfg.mu.Unlock()

	newViper, err := cfg.loadConfig()
	if err != nil {
		return err
	}
	cfg.viper = newViper
	return nil
}

// GetString returns a string config value.
func GetString(key string) string {
	cfg := getInstance()
	_ = cfg.ensureInitialized()
	cfg.mu.RLock()
	defer cfg.mu.RUnlock()

	if cfg.viper == nil {
		return ""
	}
	return cfg.viper.GetString(key)
}

// GetInt returns an int config value.
func GetInt(key string) int {
	cfg := getInstance()
	_ = cfg.ensureInitialized()
	cfg.mu.RLock()
	defer cfg.mu.RUnlock()

	if cfg.viper == nil {
		return 0
	}
	return cfg.viper.GetInt(key)
}

// GetBool returns a bool config value.
func GetBool(key string) bool {
	cfg := getInstance()
	_ = cfg.ensureInitialized()
	cfg.mu.RLock()
	defer cfg.mu.RUnlock()

	if cfg.viper == nil {
		return false
	}
	return cfg.viper.GetBool(key)
}

// GetStringMapString returns a map[string]string config value.
func GetStringMapString(key string) map[string]string {
	cfg := getInstance()
	_ = cfg.ensureInitialized()
	cfg.mu.RLock()
	defer cfg.mu.RUnlock()

	if cfg.viper == nil {
		return make(map[string]string)
	}
	return cfg.viper.GetStringMapString(key)
}

// GetStringSlice returns a []string config value.
func GetStringSlice(key string) []string {
	cfg := getInstance()
	_ = cfg.ensureInitialized()
	cfg.mu.RLock()
	defer cfg.mu.RUnlock()

	if cfg.viper == nil {
		return []string{}
	}
	return cfg.viper.GetStringSlice(key)
}

// GetDuration returns a time.Duration config value.
func GetDuration(key string) time.Duration {
	cfg := getInstance()
	_ = cfg.ensureInitialized()
	cfg.mu.RLock()
	defer cfg.mu.RUnlock()

	if cfg.viper == nil {
		return 0
	}
	return cfg.viper.GetDuration(key)
}

// GetSyncConfig returns the sync configuration section
func GetSyncConfig() SyncConfig {
	cfg := getInstance()
	_ = cfg.ensureInitialized() // Ensure config is loaded

	cfg.mu.RLock()
	defer cfg.mu.RUnlock()

	var syncConf SyncConfig
	if cfg.viper == nil {
		return syncConf // Return empty struct if viper is not initialized
	}

	// Unmarshal the relevant sub-section of the config into the struct
	// This approach is cleaner than getting each value individually.
	if err := cfg.viper.UnmarshalKey("dns.internal", &syncConf); err != nil {
		// In case of an error, you might want to log it or handle it gracefully.
		// For now, returning a zero-value struct.
		return SyncConfig{}
	}

	return syncConf
}

// RegisterRequiredKey adds a key to the list of required configuration keys
func RegisterRequiredKey(key string) {
	requiredKeysMutex.Lock()
	defer requiredKeysMutex.Unlock()
	requiredKeys = append(requiredKeys, key)
}

// CheckRequiredKeys validates that all required keys have values.
func CheckRequiredKeys() error {
	requiredKeysMutex.Lock()
	defer requiredKeysMutex.Unlock()

	cfg := getInstance()
	_ = cfg.ensureInitialized()

	// Reset missing keys slice
	missingKeys = []string{}

	for _, key := range requiredKeys {
		if !cfg.hasKey(key) {
			missingKeys = append(missingKeys, key)
		}
	}

	if len(missingKeys) > 0 {
		return fmt.Errorf("missing required configuration keys: %v", missingKeys)
	}
	return nil
}

// HasKey checks if a configuration key exists.
func HasKey(key string) bool {
	cfg := getInstance()
	_ = cfg.ensureInitialized()
	cfg.mu.RLock()
	defer cfg.mu.RUnlock()

	return cfg.hasKey(key)
}

func (c *Config) hasKey(key string) bool {
	if c.viper == nil {
		return false
	}
	return c.viper.IsSet(key)
}

// SetForTest sets a configuration value for testing purposes.
func SetForTest(key string, value interface{}) {
	cfg := getInstance()
	_ = cfg.ensureInitialized()
	cfg.mu.Lock()
	defer cfg.mu.Unlock()

	if cfg.viper != nil {
		cfg.viper.Set(key, value)
	}
}

// ResetForTest resets the configuration instance for testing.
func ResetForTest() {
	instanceOnce = sync.Once{}
	instance = nil
	requiredKeys = []string{}
	missingKeys = []string{}
}

// ValidateTailscaleConfig validates the Tailscale configuration
func ValidateTailscaleConfig() error {
	apiKey := GetString(TailscaleAPIKeyKey)
	if apiKey == "" {
		return fmt.Errorf("tailscale.api_key is required")
	}

	tailnet := GetString(TailscaleTailnetKey)
	if tailnet == "" {
		return fmt.Errorf("tailscale.tailnet is required")
	}

	return nil
}
