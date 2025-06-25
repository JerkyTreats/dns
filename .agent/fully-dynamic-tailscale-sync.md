# Fully Dynamic Tailscale Sync Design Brief

## 1. Problem Statement

The current Tailscale device synchronization relies on a static list of devices defined in the `config.yaml` file under `dns.internal.sync_devices`. This approach has two main drawbacks:

*   **Manual Configuration**: Whenever a new device is added to the Tailscale network, an administrator must manually update the configuration file to include it in the DNS sync. This is inefficient and error-prone.
*   **Incomplete Picture**: The DNS manager only knows about the devices explicitly listed in its configuration, preventing it from having a complete, real-time view of the entire Tailscale network.

The goal is to move to a fully dynamic system where the DNS service automatically discovers and manages records for all devices in the tailnet without manual intervention.

## 2. Proposed Solution

The solution is to eliminate the static `sync_devices` configuration and instead treat the Tailscale API as the single source of truth for all network devices.

The new process will be as follows:

1.  **Remove Static Configuration**: The `dns.internal.sync_devices` configuration key will be removed entirely.
2.  **Dynamic Discovery**: On startup and during periodic syncs, the service will fetch the complete list of devices directly from the Tailscale API.
3.  **Automatic Record Management**: For each device returned by the API, the system will create a DNS A record mapping the device's hostname to its Tailscale IP address (e.g., `my-laptop.internal.jerkytreats.dev` -> `100.x.y.z`).
4.  **Intelligent IP Updates**: The sync process will be idempotent. If a device's IP address has changed since the last sync, the system will seamlessly update the corresponding DNS record. This will involve locating the old record, removing it, and creating a new one with the correct IP.
5.  **New `DropRecord` Functionality**: To support IP updates, a new internal `DropRecord` function will be added to the CoreDNS manager. This function will be responsible for removing a specific DNS record from a zone file.

This change will make the DNS service self-managing, ensuring that the DNS records are always an accurate reflection of the current state of the Tailscale network.

## 3. Architecture Design

### A. Configuration Changes

The `dns.internal.sync_devices` key will be removed from the configuration schema. The `internal/config/config.go` file will be updated to reflect this change.

**Removed from `config.yaml.template`**:
```yaml
# dns.internal:
#   sync_devices: [] # THIS ENTIRE SECTION WILL BE REMOVED
```

**`internal/config/config.go` changes**:
The `SyncDevice` struct and its usage within the `SyncConfig` struct will be removed. The `GetSyncConfig` function will no longer load this data.

### B. CoreDNS Manager Enhancement

A new function, `DropRecord`, will be added to the CoreDNS manager.

**`internal/dns/coredns/manager.go`**:
```go
// Manager manages CoreDNS configuration and zone files.
type Manager struct {
    // ... existing fields
}

// AddRecord adds a new A record to a zone file.
func (m *Manager) AddRecord(zone, name, ip string) error {
    // ... existing implementation
}

// NEW: DropRecord removes an A record from a zone file.
func (m *Manager) DropRecord(zone, name, ip string) error {
    // Implementation details:
    // 1. Construct the full record line to be removed (e.g., "name IN A ip").
    // 2. Read the zone file line by line.
    // 3. Write to a new temporary file, excluding the line that matches the record to be dropped.
    // 4. Replace the original zone file with the temporary file.
    // 5. Trigger a CoreDNS reload.
}
```

### C. Sync Manager Logic Overhaul

The `internal/tailscale/sync/manager.go` will be significantly refactored. It will no longer iterate over a configured device list but will instead fetch all devices from Tailscale.

**`internal/tailscale/sync/manager.go`**:
```go
// Manager handles dynamic zone synchronization with Tailscale integration
type Manager struct {
	corednsManager  *coredns.Manager
	tailscaleClient *tailscale.Client
	config          config.SyncConfig // Will no longer contain devices

	// IP cache to map device HOSTNAME to IP
	ipCache    map[string]string
	cacheMutex sync.RWMutex

	// ... existing fields
}

// syncDevices will be the core of the new logic.
func (m *Manager) syncDevices(zoneName string) (*SyncResult, error) {
    // 1. Fetch all devices from Tailscale API
    tsDevices, err := m.tailscaleClient.ListDevices()
    if err != nil {
        return nil, err
    }

    // 2. Process each device
    for _, tsDevice := range tsDevices {
        hostname := tsDevice.Hostname // e.g., "omnitron"
        newIP := tsDevice.IPv4 // The 100.x.y.z address

        // 3. Check cache for existing IP
        oldIP, isKnownDevice := m.ipCache[hostname]

        if isKnownDevice && oldIP != newIP {
            // IP has changed, update the record
            logging.Info("IP for device %s has changed from %s to %s", hostname, oldIP, newIP)
            // Drop the old record first
            if err := m.corednsManager.DropRecord(zoneName, hostname, oldIP); err != nil {
                logging.Error("Failed to drop old record for %s: %v", hostname, err)
                // Continue to try and add the new one
            }
        }

        // 4. Add the new/updated record
        if err := m.corednsManager.AddRecord(zoneName, hostname, newIP); err != nil {
            logging.Error("Failed to add record for %s: %v", hostname, err)
            continue
        }

        // 5. Update the cache with the new IP
        m.cacheIP(hostname, newIP)
    }

    // Reload CoreDNS after all changes are made
    m.corednsManager.Reload()

    return &SyncResult{Success: true, ResolvedDevices: len(tsDevices)}, nil
}
```

### D. Data Flow

The updated synchronization flow will be:

```
Sync Process Triggered (Startup or Polling)
    |
    V
1. `sync.Manager` calls `tailscaleClient.ListDevices()`
    |
    V
2. Tailscale API returns a list of all devices in the tailnet.
    |
    V
3. For each device:
    a. Extract hostname and IP.
    b. Check internal cache for a previous IP for that hostname.
    c. If IP has changed: call `corednsManager.DropRecord()` with the old IP.
    d. Call `corednsManager.AddRecord()` with the new IP.
    e. Update internal cache with the new IP.
    |
    V
4. `corednsManager.Reload()` is called to apply all changes.
```

## 4. Implementation Plan

| Phase | Task | Files to Modify | Status |
| :--- | :--- | :--- | :--- |
| 1. CoreDNS | Implement `DropRecord` function | `internal/dns/coredns/manager.go` | To Do |
| 1. CoreDNS | Create unit test for `DropRecord` | `internal/dns/coredns/manager_test.go` | To Do |
| 2. Config | Remove `sync_devices` from template | `configs/config.yaml.template` | To Do |
| 2. Config | Remove `SyncDevice` and update `SyncConfig` | `internal/config/config.go` | To Do |
| 2. Config | Update `GetSyncConfig` function | `internal/config/config.go` | To Do |
| 3. Sync Manager | Overhaul `syncDevices` method for dynamic discovery | `internal/tailscale/sync/manager.go` | To Do |
| 3. Sync Manager | Update IP caching logic | `internal/tailscale/sync/manager.go` | To Do |
| 3. Sync Manager | Remove device validation from `NewManager` | `internal/tailscale/sync/manager.go` | To Do |
| 4. Testing | Create end-to-end integration test | `tests/integration_test.go` | To Do |
| 4. Testing | Manual verification | N/A | To Do |
