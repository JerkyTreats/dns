package tailscale

import (
	"encoding/json"
	"fmt"
	"time"
)

// PersistedDevice represents a device with both Tailscale data and user annotations
type PersistedDevice struct {
	Name        string `json:"name"`         // Immutable - Device hostname from Tailscale API
	TailscaleIP string `json:"tailscale_ip"` // Immutable - Tailscale IP address from API
	Description string `json:"description"`  // Mutable - User-provided annotation
}

// DeviceStorage represents the JSON storage format for device data
type DeviceStorage struct {
	Devices     []PersistedDevice `json:"devices"`
	LastUpdated time.Time         `json:"last_updated"`
}

// AnnotationRequest represents a request to update device annotations
type AnnotationRequest struct {
	Name     string `json:"name"`
	Property string `json:"property"`
	Value    string `json:"value"`
}

// DeviceProperty represents the allowed properties for annotation
type DeviceProperty string

const (
	PropertyDescription DeviceProperty = "description"
)

// ValidateAnnotationRequest validates an annotation request
func ValidateAnnotationRequest(req *AnnotationRequest) error {
	if req.Name == "" {
		return fmt.Errorf("device name is required")
	}

	switch req.Property {
	case string(PropertyDescription):
		// Description is valid and mutable
		return nil
	case "name", "tailscale_ip":
		return fmt.Errorf("property '%s' is immutable and cannot be modified", req.Property)
	default:
		return fmt.Errorf("unknown property '%s'. Valid properties: description", req.Property)
	}
}

// NewDeviceStorage creates a new device storage with initial values
func NewDeviceStorage() *DeviceStorage {
	return &DeviceStorage{
		Devices:     make([]PersistedDevice, 0),
		LastUpdated: time.Now(),
	}
}

// FindDevice finds a device by name in the storage
func (ds *DeviceStorage) FindDevice(name string) (*PersistedDevice, int) {
	for i, device := range ds.Devices {
		if device.Name == name {
			return &device, i
		}
	}
	return nil, -1
}

// AddOrUpdateDevice adds a new device or updates an existing one
func (ds *DeviceStorage) AddOrUpdateDevice(device PersistedDevice) {
	existingDevice, index := ds.FindDevice(device.Name)
	if existingDevice != nil {
		// Update existing device, preserve description if not provided
		if device.Description == "" {
			device.Description = existingDevice.Description
		}
		ds.Devices[index] = device
	} else {
		// Add new device
		ds.Devices = append(ds.Devices, device)
	}
	ds.LastUpdated = time.Now()
}

// UpdateDeviceProperty updates a specific property of a device
func (ds *DeviceStorage) UpdateDeviceProperty(name, property, value string) error {
	device, index := ds.FindDevice(name)
	if device == nil {
		return fmt.Errorf("device '%s' not found", name)
	}

	switch property {
	case string(PropertyDescription):
		ds.Devices[index].Description = value
		ds.LastUpdated = time.Now()
		return nil
	default:
		return fmt.Errorf("cannot update property '%s'", property)
	}
}

// ToJSON converts the device storage to JSON bytes
func (ds *DeviceStorage) ToJSON() ([]byte, error) {
	return json.MarshalIndent(ds, "", "  ")
}

// FromJSON populates the device storage from JSON bytes
func (ds *DeviceStorage) FromJSON(data []byte) error {
	return json.Unmarshal(data, ds)
}

// GetDevices returns a copy of all devices
func (ds *DeviceStorage) GetDevices() []PersistedDevice {
	devices := make([]PersistedDevice, len(ds.Devices))
	copy(devices, ds.Devices)
	return devices
}

// SyncWithTailscaleDevices updates the storage with devices from Tailscale API
// This preserves user annotations while updating IP addresses
func (ds *DeviceStorage) SyncWithTailscaleDevices(tailscaleDevices []Device) {
	for _, tsDevice := range tailscaleDevices {
		tailscaleIP := getTailscaleIP(tsDevice.Addresses)
		if tailscaleIP == "" {
			continue // Skip devices without Tailscale IP
		}

		persistedDevice := PersistedDevice{
			Name:        tsDevice.Hostname,
			TailscaleIP: tailscaleIP,
			Description: "", // Will be preserved if device exists
		}

		ds.AddOrUpdateDevice(persistedDevice)
	}
}

// getTailscaleIP extracts the Tailscale IP (100.x.y.z) from a list of addresses
func getTailscaleIP(addresses []string) string {
	for _, addr := range addresses {
		if len(addr) > 4 && addr[:4] == "100." {
			return addr
		}
	}
	return ""
}
