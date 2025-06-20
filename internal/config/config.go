// Package config provides centralized, extensible configuration loading for PHITE using spf13/viper.
// All config access must go through this package.
package config

import (
	"os" // Added for ToUpper
	"sync"
	"time"

	"github.com/spf13/viper"
)

// Exported configuration keys
const (
	LogLevelKey = "log_level"
)

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
	// Replace global variable with a slice to track missing required keys
	MissingKeys []string
)

// getInstance returns the singleton config instance
func getInstance() *Config {
	instanceOnce.Do(func() {
		instance = &Config{
			searchPaths: []string{"./configs", os.ExpandEnv("$HOME/.phite")},
		}
	})
	return instance
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
		return nil
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

// RegisterRequiredKey adds a key to the list of required configuration items.
// This should be called during the init() phase of packages that require specific configurations.
// It only registers the key without loading config - use CheckRequiredKeys() to validate.
func RegisterRequiredKey(key string) {
	requiredKeysMutex.Lock()
	defer requiredKeysMutex.Unlock()
	// Avoid duplicates
	for _, k := range requiredKeys {
		if k == key {
			return
		}
	}
	requiredKeys = append(requiredKeys, key)
}

// CheckRequiredKeys validates that all registered required keys are present in the configuration.
// This should be called after setting the config path but before using config values.
func CheckRequiredKeys() error {
	requiredKeysMutex.Lock()
	defer requiredKeysMutex.Unlock()

	// Reset missing keys slice
	MissingKeys = nil

	// Check each required key
	for _, key := range requiredKeys {
		if !HasKey(key) {
			MissingKeys = append(MissingKeys, key)
		}
	}

	// Return error if any keys are missing (optional - for now just track them)
	return nil
}

// HasKey returns true if the config has the key.
func HasKey(key string) bool {
	cfg := getInstance()
	_ = cfg.ensureInitialized()
	cfg.mu.RLock()
	defer cfg.mu.RUnlock()

	if cfg.viper == nil {
		return false
	}
	return cfg.viper.IsSet(key)
}

// SetForTest sets a configuration value for testing purposes only.
func SetForTest(key string, value interface{}) {
	cfg := getInstance()
	_ = cfg.ensureInitialized()
	cfg.mu.Lock()
	defer cfg.mu.Unlock()

	if cfg.viper != nil {
		cfg.viper.Set(key, value)
	}
}

// ResetForTest resets the config singleton for test use only.
func ResetForTest() {
	instanceOnce = sync.Once{}
	instance = nil
	requiredKeysMutex.Lock()
	requiredKeys = nil
	MissingKeys = nil
	requiredKeysMutex.Unlock()
}
