// Package bootstrap provides dynamic zone bootstrapping using Tailscale device discovery.
package bootstrap

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/jerkytreats/dns/internal/config"
	"github.com/jerkytreats/dns/internal/dns/coredns"
	"github.com/jerkytreats/dns/internal/logging"
	"github.com/jerkytreats/dns/internal/tailscale"
)

const (
	// Default cache TTL for resolved IPs
	defaultCacheTTL = 5 * time.Minute

	// Maximum retry attempts for device resolution
	maxRetryAttempts = 3

	// Retry delay between attempts
	retryDelay = 2 * time.Second
)

// Manager handles dynamic zone bootstrapping with Tailscale integration
type Manager struct {
	corednsManager  *coredns.Manager
	tailscaleClient *tailscale.Client
	config          config.BootstrapConfig

	// IP cache to reduce Tailscale API calls
	ipCache    map[string]*cachedIP
	cacheMutex sync.RWMutex

	// Bootstrap state
	bootstrapped bool
	mu           sync.Mutex
}

// cachedIP represents a cached IP address with TTL
type cachedIP struct {
	ip        string
	timestamp time.Time
	ttl       time.Duration
}

// DeviceResolution represents the result of resolving a device
type DeviceResolution struct {
	Device  config.BootstrapDevice
	IP      string
	Online  bool
	Error   error
	Skipped bool
	Reason  string
}

// BootstrapResult represents the overall bootstrap operation result
type BootstrapResult struct {
	Success         bool
	TotalDevices    int
	ResolvedDevices int
	SkippedDevices  int
	FailedDevices   int
	Resolutions     []DeviceResolution
	Error           error
}

// NewManager creates a new bootstrap manager
func NewManager(corednsManager *coredns.Manager, tailscaleClient *tailscale.Client, bootstrapConfig config.BootstrapConfig) *Manager {
	return &Manager{
		corednsManager:  corednsManager,
		tailscaleClient: tailscaleClient,
		config:          bootstrapConfig,
		ipCache:         make(map[string]*cachedIP),
	}
}

// EnsureInternalZone ensures the internal zone exists and bootstraps devices
func (m *Manager) EnsureInternalZone() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	logging.Info("Ensuring internal zone exists and is bootstrapped")

	// Check if zone already exists by attempting to add a test record
	zoneName := extractZoneName(m.config.Origin)
	if zoneName == "" {
		return fmt.Errorf("invalid origin format: %s", m.config.Origin)
	}

	// Try to create the zone (will fail gracefully if it already exists)
	if err := m.createInternalZoneIfNeeded(zoneName); err != nil {
		return fmt.Errorf("failed to create internal zone: %w", err)
	}

	// Bootstrap devices
	result, err := m.bootstrapDevices(zoneName)
	if err != nil {
		return fmt.Errorf("failed to bootstrap devices: %w", err)
	}

	m.logBootstrapResult(result)
	m.bootstrapped = true

	if result.FailedDevices > 0 {
		logging.Warn("Bootstrap completed with %d failed devices", result.FailedDevices)
	} else {
		logging.Info("Bootstrap completed successfully")
	}

	return nil
}

// createInternalZoneIfNeeded creates the internal zone if it doesn't exist
func (m *Manager) createInternalZoneIfNeeded(zoneName string) error {
	logging.Debug("Creating internal zone if needed: %s", zoneName)

	// Attempt to create the zone
	// The CoreDNS manager should handle the case where zone already exists
	if err := m.corednsManager.AddZone(zoneName); err != nil {
		// Check if this is an "already exists" error by trying to add a test record
		testErr := m.corednsManager.AddRecord(zoneName, "_bootstrap_test", "127.0.0.1")
		if testErr == nil {
			// Zone exists and we can add records, so original error was likely "zone already exists"
			logging.Debug("Internal zone already exists: %s", zoneName)
			return nil
		}
		return err
	}

	logging.Info("Created internal zone: %s", zoneName)
	return nil
}

// bootstrapDevices resolves device IPs and creates DNS records
func (m *Manager) bootstrapDevices(zoneName string) (*BootstrapResult, error) {
	result := &BootstrapResult{
		TotalDevices: len(m.config.Devices),
		Resolutions:  make([]DeviceResolution, 0, len(m.config.Devices)),
	}

	logging.Info("Bootstrapping %d devices for zone %s", len(m.config.Devices), zoneName)

	for _, device := range m.config.Devices {
		resolution := m.resolveDevice(device, zoneName)
		result.Resolutions = append(result.Resolutions, resolution)

		if resolution.Skipped {
			result.SkippedDevices++
			continue
		}

		if resolution.Error != nil {
			result.FailedDevices++
			continue
		}

		result.ResolvedDevices++
	}

	result.Success = result.FailedDevices == 0
	return result, nil
}

// resolveDevice resolves a single device and creates its DNS records
func (m *Manager) resolveDevice(device config.BootstrapDevice, zoneName string) DeviceResolution {
	resolution := DeviceResolution{Device: device}

	// Check if device is enabled
	if !device.Enabled {
		resolution.Skipped = true
		resolution.Reason = "device disabled in configuration"
		logging.Debug("Skipping disabled device: %s", device.Name)
		return resolution
	}

	// Resolve IP with retry logic
	ip, err := m.resolveDeviceIPWithRetry(device.TailscaleName)
	if err != nil {
		resolution.Error = err
		logging.Error("Failed to resolve IP for device %s (%s): %v", device.Name, device.TailscaleName, err)
		return resolution
	}

	resolution.IP = ip
	resolution.Online = true

	// Create DNS records
	if err := m.createDeviceRecords(zoneName, device, ip); err != nil {
		resolution.Error = fmt.Errorf("failed to create DNS records: %w", err)
		logging.Error("Failed to create DNS records for device %s: %v", device.Name, err)
		return resolution
	}

	logging.Info("Successfully bootstrapped device %s (%s) -> %s", device.Name, device.TailscaleName, ip)
	return resolution
}

// resolveDeviceIPWithRetry resolves device IP with caching and retry logic
func (m *Manager) resolveDeviceIPWithRetry(tailscaleName string) (string, error) {
	// Check cache first
	if cachedIP := m.getCachedIP(tailscaleName); cachedIP != "" {
		logging.Debug("Using cached IP for device %s: %s", tailscaleName, cachedIP)
		return cachedIP, nil
	}

	var lastError error
	for attempt := 1; attempt <= maxRetryAttempts; attempt++ {
		ip, err := m.tailscaleClient.GetDeviceIP(tailscaleName)
		if err == nil {
			// Cache the resolved IP
			m.cacheIP(tailscaleName, ip, defaultCacheTTL)
			return ip, nil
		}

		lastError = err
		logging.Debug("Attempt %d/%d failed to resolve IP for device %s: %v", attempt, maxRetryAttempts, tailscaleName, err)

		if attempt < maxRetryAttempts {
			time.Sleep(retryDelay)
		}
	}

	return "", fmt.Errorf("failed to resolve IP after %d attempts: %w", maxRetryAttempts, lastError)
}

// createDeviceRecords creates DNS records for a device and its aliases
func (m *Manager) createDeviceRecords(zoneName string, device config.BootstrapDevice, ip string) error {
	// Create primary record
	if err := m.corednsManager.AddRecord(zoneName, device.Name, ip); err != nil {
		return fmt.Errorf("failed to add primary record for %s: %w", device.Name, err)
	}

	// Create alias records
	for _, alias := range device.Aliases {
		if err := m.corednsManager.AddRecord(zoneName, alias, ip); err != nil {
			logging.Error("Failed to add alias record %s for device %s: %v", alias, device.Name, err)
			// Continue with other aliases even if one fails
		} else {
			logging.Debug("Added alias record: %s -> %s", alias, ip)
		}
	}

	return nil
}

// getCachedIP retrieves a cached IP if it's still valid
func (m *Manager) getCachedIP(deviceName string) string {
	m.cacheMutex.RLock()
	defer m.cacheMutex.RUnlock()

	cached, exists := m.ipCache[deviceName]
	if !exists {
		return ""
	}

	if time.Since(cached.timestamp) > cached.ttl {
		// Cache expired
		delete(m.ipCache, deviceName)
		return ""
	}

	return cached.ip
}

// cacheIP caches an IP address with TTL
func (m *Manager) cacheIP(deviceName, ip string, ttl time.Duration) {
	m.cacheMutex.Lock()
	defer m.cacheMutex.Unlock()

	m.ipCache[deviceName] = &cachedIP{
		ip:        ip,
		timestamp: time.Now(),
		ttl:       ttl,
	}
}

// RefreshDeviceIPs refreshes IP addresses for all devices from Tailscale
func (m *Manager) RefreshDeviceIPs() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.bootstrapped {
		return fmt.Errorf("zone not bootstrapped yet")
	}

	logging.Info("Refreshing device IPs from Tailscale")

	// Clear cache to force refresh
	m.cacheMutex.Lock()
	m.ipCache = make(map[string]*cachedIP)
	m.cacheMutex.Unlock()

	zoneName := extractZoneName(m.config.Origin)
	if zoneName == "" {
		return fmt.Errorf("invalid origin format: %s", m.config.Origin)
	}

	result, err := m.bootstrapDevices(zoneName)
	if err != nil {
		return fmt.Errorf("failed to refresh device IPs: %w", err)
	}

	m.logBootstrapResult(result)

	if result.FailedDevices > 0 {
		return fmt.Errorf("refresh completed with %d failed devices", result.FailedDevices)
	}

	logging.Info("Device IP refresh completed successfully")
	return nil
}

// IsZoneBootstrapped returns whether the zone has been bootstrapped
func (m *Manager) IsZoneBootstrapped() bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	// If corednsManager is nil (e.g., in tests), return the internal state
	if m.corednsManager == nil {
		return m.bootstrapped
	}

	// Check if zone file exists and has content
	zoneName := extractZoneName(m.config.Origin)
	if zoneName == "" {
		return false
	}

	// Try to add a test record to see if zone exists and is accessible
	testErr := m.corednsManager.AddRecord(zoneName, "_bootstrap_test", "127.0.0.1")
	if testErr != nil {
		// Zone doesn't exist or isn't accessible
		return false
	}

	// If we can add records, the zone is bootstrapped
	// Update our internal state to match reality
	m.bootstrapped = true
	return true
}

// ValidateConfiguration validates the Tailscale connection and bootstrap config
func (m *Manager) ValidateConfiguration() error {
	logging.Info("Validating bootstrap configuration")

	// Validate Tailscale connection
	if err := m.tailscaleClient.ValidateConnection(); err != nil {
		return fmt.Errorf("Tailscale connection validation failed: %w", err)
	}

	// Validate bootstrap configuration - use manager's config
	if err := m.validateLocalBootstrapConfig(); err != nil {
		return fmt.Errorf("bootstrap configuration validation failed: %w", err)
	}

	logging.Info("Bootstrap configuration validation successful")
	return nil
}

// validateLocalBootstrapConfig validates the manager's bootstrap configuration
func (m *Manager) validateLocalBootstrapConfig() error {
	if m.config.Origin == "" {
		return fmt.Errorf("dns.internal.origin is required")
	}

	if len(m.config.Devices) == 0 {
		return fmt.Errorf("at least one bootstrap device must be configured")
	}

	for i, device := range m.config.Devices {
		if device.Name == "" {
			return fmt.Errorf("device %d: name is required", i)
		}
		if device.TailscaleName == "" {
			return fmt.Errorf("device %d (%s): tailscale_name is required", i, device.Name)
		}
	}

	return nil
}

// logBootstrapResult logs the results of a bootstrap operation
func (m *Manager) logBootstrapResult(result *BootstrapResult) {
	logging.Info("Bootstrap result: %d total, %d resolved, %d skipped, %d failed",
		result.TotalDevices, result.ResolvedDevices, result.SkippedDevices, result.FailedDevices)

	for _, resolution := range result.Resolutions {
		if resolution.Skipped {
			logging.Debug("Device %s: skipped (%s)", resolution.Device.Name, resolution.Reason)
		} else if resolution.Error != nil {
			logging.Error("Device %s: failed - %v", resolution.Device.Name, resolution.Error)
		} else {
			logging.Debug("Device %s: resolved to %s", resolution.Device.Name, resolution.IP)
		}
	}
}

// extractZoneName extracts the zone name from an origin (removes trailing dot)
func extractZoneName(origin string) string {
	zoneName := strings.TrimSuffix(origin, ".")
	if zoneName == "" {
		return ""
	}

	// For internal zones, we typically want just the subdomain part
	// e.g., "internal.jerkytreats.dev" -> "internal"
	parts := strings.Split(zoneName, ".")
	if len(parts) > 0 {
		return parts[0]
	}

	return zoneName
}
