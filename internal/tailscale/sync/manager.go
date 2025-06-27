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
}

// TailscaleClient defines the interface for interacting with the Tailscale API.
// This allows for mocking in tests.
type TailscaleClient interface {
	ListDevices() ([]tailscale.Device, error)
}

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

	zoneName := extractZoneName(m.config.Origin)
	if zoneName == "" {
		return fmt.Errorf("invalid origin format: %s", m.config.Origin)
	}

	if err := m.createInternalZoneIfNeeded(zoneName); err != nil {
		return fmt.Errorf("failed to create internal zone: %w", err)
	}

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

	if zoneName == "" {
		logging.Info("Origin matches base domain; no additional internal zone needed")
		return nil
	}

	if err := m.corednsManager.AddZone(zoneName); err != nil {
		return fmt.Errorf("failed to create internal zone: %w", err)
	}

	logging.Info("Created internal zone: %s", zoneName)
	return nil
}

// syncDevices fetches all devices from the Tailscale API and manages their DNS records.
func (m *Manager) syncDevices(zoneName string) (*SyncResult, error) {
	logging.Info("Starting dynamic sync for zone: %s", zoneName)

	tsDevices, err := m.tailscaleClient.ListDevices()
	if err != nil {
		return &SyncResult{Success: false, Error: err}, fmt.Errorf("failed to list tailscale devices: %w", err)
	}

	result := &SyncResult{
		TotalDevices: len(tsDevices),
	}

	for _, tsDevice := range tsDevices {
		hostname := tsDevice.Hostname
		newIP := getTailscaleIP(tsDevice.Addresses)

		if newIP == "" {
			logging.Debug("Skipping device %s with no IP address", hostname)
			result.SkippedDevices++
			continue
		}

		oldIP, isKnownDevice := m.getCachedIP(hostname)

		if isKnownDevice && oldIP != newIP {
			logging.Info("IP for device %s has changed from %s to %s. Updating record.", hostname, oldIP, newIP)
			if err := m.corednsManager.DropRecord(zoneName, hostname, oldIP); err != nil {
				logging.Error("Failed to drop old record for %s: %v", hostname, err)
				result.FailedDevices++
			}
		}

		if !isKnownDevice || oldIP != newIP {
			if err := m.corednsManager.AddRecord(zoneName, hostname, newIP); err != nil {
				logging.Error("Failed to add record for %s: %v", hostname, err)
				result.FailedDevices++
				continue
			}
			logging.Info("Added/Updated DNS record for %s -> %s", hostname, newIP)
		}

		m.cacheIP(hostname, newIP)
		result.ResolvedDevices++
	}

	// CoreDNS auto-reloads via 'reload' plugin

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
	origin = strings.TrimSuffix(origin, ".")

	parts := strings.Split(origin, ".")

	if len(parts) < 3 {
		return ""
	}

	return parts[0]
}
