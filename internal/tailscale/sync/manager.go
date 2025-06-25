// Package sync provides dynamic zone synchronization using Tailscale device discovery.
package sync

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/jerkytreats/dns/internal/config"
	"github.com/jerkytreats/dns/internal/logging"
	"github.com/jerkytreats/dns/internal/tailscale"
)

// CorednsManager defines the interface for managing CoreDNS.
// This allows for mocking in tests.
type CorednsManager interface {
	AddZone(serviceName string) error
	AddRecord(serviceName, name, ip string) error
	DropRecord(serviceName, name, ip string) error
	Reload() error
}

// TailscaleClient defines the interface for interacting with the Tailscale API.
// This allows for mocking in tests.
type TailscaleClient interface {
	ListDevices() ([]tailscale.Device, error)
}

const (
	// Default cache TTL for resolved IPs
	defaultCacheTTL = 5 * time.Minute

	// Maximum retry attempts for device resolution
	maxRetryAttempts = 3

	// Retry delay between attempts
	retryDelay = 2 * time.Second
)

// Manager handles dynamic zone synchronization with Tailscale integration
type Manager struct {
	corednsManager  CorednsManager
	tailscaleClient TailscaleClient
	config          config.SyncConfig

	// IP cache to map device HOSTNAME to IP
	ipCache    map[string]string
	cacheMutex sync.RWMutex

	// Sync state
	synced bool
	mu     sync.Mutex
}

// SyncResult represents the overall sync operation result
type SyncResult struct {
	Success         bool
	TotalDevices    int
	ResolvedDevices int
	SkippedDevices  int
	FailedDevices   int
	Error           error
}

// NewManager creates a new sync manager
func NewManager(corednsManager CorednsManager, tailscaleClient TailscaleClient) (*Manager, error) {
	syncConfig := config.GetSyncConfig()

	m := &Manager{
		corednsManager:  corednsManager,
		tailscaleClient: tailscaleClient,
		config:          syncConfig,
		ipCache:         make(map[string]string),
	}

	return m, nil
}

// EnsureInternalZone ensures the internal zone exists and syncs devices
func (m *Manager) EnsureInternalZone() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	logging.Info("Ensuring internal zone exists and is synced")

	// Check if zone already exists by attempting to add a test record
	zoneName := extractZoneName(m.config.Origin)
	if zoneName == "" {
		return fmt.Errorf("invalid origin format: %s", m.config.Origin)
	}

	// Try to create the zone (will fail gracefully if it already exists)
	if err := m.createInternalZoneIfNeeded(zoneName); err != nil {
		return fmt.Errorf("failed to create internal zone: %w", err)
	}

	// Sync devices
	result, err := m.syncDevices(zoneName)
	if err != nil {
		return fmt.Errorf("failed to sync devices: %w", err)
	}

	m.logSyncResult(result)
	m.synced = true

	if result.FailedDevices > 0 {
		logging.Warn("Sync completed with %d failed devices", result.FailedDevices)
	} else {
		logging.Info("Sync completed successfully")
	}

	return nil
}

// createInternalZoneIfNeeded creates the internal zone if it doesn't exist
func (m *Manager) createInternalZoneIfNeeded(zoneName string) error {
	logging.Debug("Creating internal zone if needed: %s", zoneName)

	// If zoneName is empty, the origin equals the base domain, which is
	// already registered elsewhere. Nothing to create.
	if zoneName == "" {
		logging.Info("Origin matches base domain; no additional internal zone needed")
		return nil
	}

	// Attempt to create the zone
	// The CoreDNS manager should handle the case where zone already exists
	if err := m.corednsManager.AddZone(zoneName); err != nil {
		return fmt.Errorf("failed to create internal zone: %w", err)
	}

	logging.Info("Created internal zone: %s", zoneName)
	return nil
}

// syncDevices fetches all devices from the Tailscale API and manages their DNS records.
func (m *Manager) syncDevices(zoneName string) (*SyncResult, error) {
	logging.Info("Starting dynamic sync for zone: %s", zoneName)

	// 1. Fetch all devices from Tailscale API
	tsDevices, err := m.tailscaleClient.ListDevices()
	if err != nil {
		return &SyncResult{Success: false, Error: err}, fmt.Errorf("failed to list tailscale devices: %w", err)
	}

	result := &SyncResult{
		TotalDevices: len(tsDevices),
	}

	// 2. Process each device
	for _, tsDevice := range tsDevices {
		hostname := tsDevice.Hostname
		newIP := getTailscaleIP(tsDevice.Addresses)

		// We only care about devices that have an IP address
		if newIP == "" {
			logging.Debug("Skipping device %s with no IP address", hostname)
			result.SkippedDevices++
			continue
		}

		// 3. Check cache for existing IP
		oldIP, isKnownDevice := m.getCachedIP(hostname)

		if isKnownDevice && oldIP != newIP {
			// IP has changed, update the record
			logging.Info("IP for device %s has changed from %s to %s. Updating record.", hostname, oldIP, newIP)
			// Drop the old record first
			if err := m.corednsManager.DropRecord(zoneName, hostname, oldIP); err != nil {
				logging.Error("Failed to drop old record for %s: %v", hostname, err)
				// Log the error but continue to try and add the new one
				result.FailedDevices++
			}
		}

		// 4. Add the new/updated record if it's a new device or the IP has changed
		if !isKnownDevice || oldIP != newIP {
			if err := m.corednsManager.AddRecord(zoneName, hostname, newIP); err != nil {
				logging.Error("Failed to add record for %s: %v", hostname, err)
				result.FailedDevices++
				continue
			}
			logging.Info("Added/Updated DNS record for %s -> %s", hostname, newIP)
		}

		// 5. Update the cache with the new IP
		m.cacheIP(hostname, newIP)
		result.ResolvedDevices++
	}

	// Reload CoreDNS after all changes are made
	if err := m.corednsManager.Reload(); err != nil {
		logging.Error("Failed to reload CoreDNS after sync: %v", err)
		result.Error = err
	}

	result.Success = result.FailedDevices == 0 && result.Error == nil
	logging.Info("Dynamic sync complete for zone %s. Total: %d, Synced: %d, Skipped: %d, Failed: %d",
		zoneName, result.TotalDevices, result.ResolvedDevices, result.SkippedDevices, result.FailedDevices)

	return result, result.Error
}

// getTailscaleIP extracts the Tailscale IP (100.x.y.z) from a list of addresses.
func getTailscaleIP(addresses []string) string {
	for _, addr := range addresses {
		if strings.HasPrefix(addr, "100.") {
			return addr
		}
	}
	return ""
}

func (m *Manager) getCachedIP(hostname string) (string, bool) {
	m.cacheMutex.RLock()
	defer m.cacheMutex.RUnlock()
	ip, exists := m.ipCache[hostname]
	return ip, exists
}

func (m *Manager) cacheIP(hostname, ip string) {
	m.cacheMutex.Lock()
	defer m.cacheMutex.Unlock()
	m.ipCache[hostname] = ip
}

// StartPolling starts the background polling to refresh device IPs.
func (m *Manager) StartPolling(interval time.Duration) {
	logging.Info("Starting background IP sync polling with interval: %s", interval)
	ticker := time.NewTicker(interval)

	go func() {
		for range ticker.C {
			logging.Info("Running periodic device sync...")
			if err := m.RefreshDeviceIPs(); err != nil {
				logging.Error("Error during periodic device sync: %v", err)
			}
		}
	}()
}

// RefreshDeviceIPs re-runs the sync process to update all records.
func (m *Manager) RefreshDeviceIPs() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	logging.Info("Refreshing all device IPs from Tailscale")

	zoneName := extractZoneName(m.config.Origin)
	if zoneName == "" {
		return fmt.Errorf("invalid origin format for refresh: %s", m.config.Origin)
	}

	// Re-run the full sync logic
	result, err := m.syncDevices(zoneName)
	if err != nil {
		return err
	}

	m.logSyncResult(result)
	return nil
}

// IsZoneSynced returns true if the initial sync has completed
func (m *Manager) IsZoneSynced() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.synced
}

func (m *Manager) logSyncResult(result *SyncResult) {
	if result.Error != nil {
		logging.Error("Sync process finished with an error: %v", result.Error)
	}
	logging.Info("Sync Result -> Success: %v, Total: %d, Resolved: %d, Skipped: %d, Failed: %d",
		result.Success, result.TotalDevices, result.ResolvedDevices, result.SkippedDevices, result.FailedDevices)
}

// extractZoneName extracts the service part of the origin.
// e.g., "internal.jerkytreats.dev" -> "internal"
func extractZoneName(origin string) string {
	// Trim trailing dot if present for consistent parsing
	origin = strings.TrimSuffix(origin, ".")

	// Split the FQDN into its labels
	parts := strings.Split(origin, ".")

	// A valid bootstrap origin must be a subdomain of a base domain,
	// e.g., "internal.example.com" has 3 parts. An origin that is just
	// "example.com" (2 parts) is not a valid internal zone origin.
	if len(parts) < 3 {
		return ""
	}

	// The service name is the first label of the FQDN.
	return parts[0]
}
