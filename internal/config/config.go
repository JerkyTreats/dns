// Package config provides centralized, extensible configuration loading for DNS using spf13/viper.
// All config access must go through this package.
package config

import (
	"fmt"
	"os" // Added for ToUpper
	"sync"
	"time"

	"github.com/spf13/viper"
)

// Exported configuration keys
const (
	LogLevelKey = "log_level"

	// DNS Bootstrap configuration keys
	DNSInternalOriginKey   = "dns.internal.origin"
	DNSBootstrapDevicesKey = "dns.internal.bootstrap_devices"

	// Tailscale configuration keys
	TailscaleAPIKeyKey  = "tailscale.api_key"
	TailscaleTailnetKey = "tailscale.tailnet"
	TailscaleBaseURLKey = "tailscale.base_url" // Optional, for testing
)

// BootstrapDevice represents a device configuration for bootstrap
type BootstrapDevice struct {
	Name          string   `yaml:"name" mapstructure:"name"`
	TailscaleName string   `yaml:"tailscale_name" mapstructure:"tailscale_name"`
	Aliases       []string `yaml:"aliases,omitempty" mapstructure:"aliases"`
	Description   string   `yaml:"description,omitempty" mapstructure:"description"`
	Enabled       bool     `yaml:"enabled" mapstructure:"enabled"`
}

// BootstrapConfig represents the bootstrap configuration section
type BootstrapConfig struct {
	Origin  string            `yaml:"origin" mapstructure:"origin"`
	Devices []BootstrapDevice `yaml:"bootstrap_devices" mapstructure:"bootstrap_devices"`
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

// GetBootstrapConfig returns the bootstrap configuration
func GetBootstrapConfig() BootstrapConfig {
	cfg := getInstance()
	_ = cfg.ensureInitialized()
	cfg.mu.RLock()
	defer cfg.mu.RUnlock()

	var bootstrapConfig BootstrapConfig
	if cfg.viper != nil {
		cfg.viper.UnmarshalKey("dns.internal", &bootstrapConfig)
	}
	return bootstrapConfig
}

// RegisterRequiredKey adds a key to the list of required configuration keys.
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

// ValidateBootstrapConfig validates the bootstrap configuration
func ValidateBootstrapConfig() error {
	bootstrapConfig := GetBootstrapConfig()

	if bootstrapConfig.Origin == "" {
		return fmt.Errorf("dns.internal.origin is required")
	}

	if len(bootstrapConfig.Devices) == 0 {
		return fmt.Errorf("at least one bootstrap device must be configured")
	}

	for i, device := range bootstrapConfig.Devices {
		if device.Name == "" {
			return fmt.Errorf("device %d: name is required", i)
		}
		if device.TailscaleName == "" {
			return fmt.Errorf("device %d (%s): tailscale_name is required", i, device.Name)
		}
	}

	return nil
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
