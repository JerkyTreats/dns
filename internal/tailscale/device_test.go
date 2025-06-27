package tailscale

import (
	"testing"
	"time"
)

func TestPersistedDevice(t *testing.T) {
	device := PersistedDevice{
		Name:        "test-device",
		TailscaleIP: "100.64.0.1",
		Description: "Test device description",
	}

	if device.Name != "test-device" {
		t.Errorf("Expected name 'test-device', got '%s'", device.Name)
	}
	if device.TailscaleIP != "100.64.0.1" {
		t.Errorf("Expected IP '100.64.0.1', got '%s'", device.TailscaleIP)
	}
	if device.Description != "Test device description" {
		t.Errorf("Expected description 'Test device description', got '%s'", device.Description)
	}
}

func TestValidateAnnotationRequest(t *testing.T) {
	tests := []struct {
		name        string
		request     AnnotationRequest
		expectError bool
		errorType   string
	}{
		{
			name: "valid description update",
			request: AnnotationRequest{
				Name:     "test-device",
				Property: "description",
				Value:    "New description",
			},
			expectError: false,
		},
		{
			name: "empty device name",
			request: AnnotationRequest{
				Name:     "",
				Property: "description",
				Value:    "New description",
			},
			expectError: true,
			errorType:   "required",
		},
		{
			name: "immutable name property",
			request: AnnotationRequest{
				Name:     "test-device",
				Property: "name",
				Value:    "new-name",
			},
			expectError: true,
			errorType:   "immutable",
		},
		{
			name: "immutable tailscale_ip property",
			request: AnnotationRequest{
				Name:     "test-device",
				Property: "tailscale_ip",
				Value:    "100.64.0.2",
			},
			expectError: true,
			errorType:   "immutable",
		},
		{
			name: "unknown property",
			request: AnnotationRequest{
				Name:     "test-device",
				Property: "unknown_prop",
				Value:    "some value",
			},
			expectError: true,
			errorType:   "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateAnnotationRequest(&tt.request)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
					return
				}

				switch tt.errorType {
				case "required":
					if err.Error() != "device name is required" {
						t.Errorf("Expected 'device name is required' error, got '%s'", err.Error())
					}
				case "immutable":
					if err.Error() != "property '"+tt.request.Property+"' is immutable and cannot be modified" {
						t.Errorf("Expected immutable property error, got '%s'", err.Error())
					}
				case "unknown":
					if err.Error() != "unknown property '"+tt.request.Property+"'. Valid properties: description" {
						t.Errorf("Expected unknown property error, got '%s'", err.Error())
					}
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error but got: %v", err)
				}
			}
		})
	}
}

func TestNewDeviceStorage(t *testing.T) {
	storage := NewDeviceStorage()

	if storage == nil {
		t.Fatal("NewDeviceStorage returned nil")
	}

	if len(storage.Devices) != 0 {
		t.Errorf("Expected empty devices slice, got %d devices", len(storage.Devices))
	}

	if storage.LastUpdated.IsZero() {
		t.Error("Expected LastUpdated to be set")
	}
}

func TestDeviceStorage_FindDevice(t *testing.T) {
	storage := NewDeviceStorage()

	// Add test devices
	device1 := PersistedDevice{Name: "device1", TailscaleIP: "100.64.0.1", Description: "First device"}
	device2 := PersistedDevice{Name: "device2", TailscaleIP: "100.64.0.2", Description: "Second device"}

	storage.Devices = []PersistedDevice{device1, device2}

	// Test finding existing device
	found, index := storage.FindDevice("device1")
	if found == nil {
		t.Error("Expected to find device1, got nil")
	} else {
		if found.Name != "device1" {
			t.Errorf("Expected device1, got %s", found.Name)
		}
		if index != 0 {
			t.Errorf("Expected index 0, got %d", index)
		}
	}

	// Test finding non-existent device
	found, index = storage.FindDevice("nonexistent")
	if found != nil {
		t.Error("Expected nil for non-existent device, got device")
	}
	if index != -1 {
		t.Errorf("Expected index -1 for non-existent device, got %d", index)
	}
}

func TestDeviceStorage_AddOrUpdateDevice(t *testing.T) {
	storage := NewDeviceStorage()

	// Test adding new device
	device1 := PersistedDevice{Name: "device1", TailscaleIP: "100.64.0.1", Description: "First device"}
	storage.AddOrUpdateDevice(device1)

	if len(storage.Devices) != 1 {
		t.Errorf("Expected 1 device, got %d", len(storage.Devices))
	}

	if storage.Devices[0].Name != "device1" {
		t.Errorf("Expected device1, got %s", storage.Devices[0].Name)
	}

	// Test updating existing device (preserving description)
	device1Updated := PersistedDevice{Name: "device1", TailscaleIP: "100.64.0.10", Description: ""}
	storage.AddOrUpdateDevice(device1Updated)

	if len(storage.Devices) != 1 {
		t.Errorf("Expected 1 device after update, got %d", len(storage.Devices))
	}

	if storage.Devices[0].TailscaleIP != "100.64.0.10" {
		t.Errorf("Expected IP to be updated to 100.64.0.10, got %s", storage.Devices[0].TailscaleIP)
	}

	if storage.Devices[0].Description != "First device" {
		t.Errorf("Expected description to be preserved, got '%s'", storage.Devices[0].Description)
	}

	// Test updating with new description
	device1WithNewDesc := PersistedDevice{Name: "device1", TailscaleIP: "100.64.0.11", Description: "Updated description"}
	storage.AddOrUpdateDevice(device1WithNewDesc)

	if storage.Devices[0].Description != "Updated description" {
		t.Errorf("Expected description to be updated, got '%s'", storage.Devices[0].Description)
	}
}

func TestDeviceStorage_UpdateDeviceProperty(t *testing.T) {
	storage := NewDeviceStorage()
	device := PersistedDevice{Name: "device1", TailscaleIP: "100.64.0.1", Description: "Original description"}
	storage.AddOrUpdateDevice(device)

	// Test valid property update
	err := storage.UpdateDeviceProperty("device1", "description", "New description")
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}

	if storage.Devices[0].Description != "New description" {
		t.Errorf("Expected description to be updated, got '%s'", storage.Devices[0].Description)
	}

	// Test updating non-existent device
	err = storage.UpdateDeviceProperty("nonexistent", "description", "Some description")
	if err == nil {
		t.Error("Expected error for non-existent device")
	}
	if err.Error() != "device 'nonexistent' not found" {
		t.Errorf("Expected 'device not found' error, got '%s'", err.Error())
	}

	// Test updating invalid property
	err = storage.UpdateDeviceProperty("device1", "invalid_prop", "Some value")
	if err == nil {
		t.Error("Expected error for invalid property")
	}
	if err.Error() != "cannot update property 'invalid_prop'" {
		t.Errorf("Expected 'cannot update property' error, got '%s'", err.Error())
	}
}

func TestDeviceStorage_JSONSerialization(t *testing.T) {
	storage := NewDeviceStorage()
	device1 := PersistedDevice{Name: "device1", TailscaleIP: "100.64.0.1", Description: "First device"}
	device2 := PersistedDevice{Name: "device2", TailscaleIP: "100.64.0.2", Description: "Second device"}

	storage.AddOrUpdateDevice(device1)
	storage.AddOrUpdateDevice(device2)

	// Test serialization to JSON
	jsonData, err := storage.ToJSON()
	if err != nil {
		t.Errorf("ToJSON failed: %v", err)
	}

	// Test deserialization from JSON
	newStorage := NewDeviceStorage()
	err = newStorage.FromJSON(jsonData)
	if err != nil {
		t.Errorf("FromJSON failed: %v", err)
	}

	// Verify data was preserved
	if len(newStorage.Devices) != 2 {
		t.Errorf("Expected 2 devices after deserialization, got %d", len(newStorage.Devices))
	}

	// Find devices by name since order might differ
	found1, _ := newStorage.FindDevice("device1")
	found2, _ := newStorage.FindDevice("device2")

	if found1 == nil || found1.TailscaleIP != "100.64.0.1" {
		t.Error("Device1 not properly deserialized")
	}

	if found2 == nil || found2.TailscaleIP != "100.64.0.2" {
		t.Error("Device2 not properly deserialized")
	}
}

func TestDeviceStorage_GetDevices(t *testing.T) {
	storage := NewDeviceStorage()
	device1 := PersistedDevice{Name: "device1", TailscaleIP: "100.64.0.1", Description: "First device"}
	storage.AddOrUpdateDevice(device1)

	devices := storage.GetDevices()

	if len(devices) != 1 {
		t.Errorf("Expected 1 device, got %d", len(devices))
	}

	// Verify it's a copy (modifying returned slice shouldn't affect storage)
	devices[0].Description = "Modified"

	if storage.Devices[0].Description == "Modified" {
		t.Error("GetDevices should return a copy, not original slice")
	}
}

func TestDeviceStorage_SyncWithTailscaleDevices(t *testing.T) {
	storage := NewDeviceStorage()

	// Add existing device with description
	existingDevice := PersistedDevice{Name: "existing-device", TailscaleIP: "100.64.0.1", Description: "Existing description"}
	storage.AddOrUpdateDevice(existingDevice)

	// Create mock Tailscale devices
	tsDevices := []Device{
		{
			Hostname:  "existing-device",
			Addresses: []string{"192.168.1.1", "100.64.0.2"}, // IP changed
		},
		{
			Hostname:  "new-device",
			Addresses: []string{"100.64.0.3", "192.168.1.2"},
		},
		{
			Hostname:  "device-without-tailscale-ip",
			Addresses: []string{"192.168.1.3"}, // No Tailscale IP
		},
	}

	storage.SyncWithTailscaleDevices(tsDevices)

	// Should have 2 devices (existing updated, new added, one skipped)
	if len(storage.Devices) != 2 {
		t.Errorf("Expected 2 devices after sync, got %d", len(storage.Devices))
	}

	// Check existing device was updated but description preserved
	existingFound, _ := storage.FindDevice("existing-device")
	if existingFound == nil {
		t.Fatal("Existing device not found after sync")
	}
	if existingFound.TailscaleIP != "100.64.0.2" {
		t.Errorf("Expected IP to be updated to 100.64.0.2, got %s", existingFound.TailscaleIP)
	}
	if existingFound.Description != "Existing description" {
		t.Errorf("Expected description to be preserved, got '%s'", existingFound.Description)
	}

	// Check new device was added
	newFound, _ := storage.FindDevice("new-device")
	if newFound == nil {
		t.Fatal("New device not found after sync")
	}
	if newFound.TailscaleIP != "100.64.0.3" {
		t.Errorf("Expected new device IP 100.64.0.3, got %s", newFound.TailscaleIP)
	}
	if newFound.Description != "" {
		t.Errorf("Expected empty description for new device, got '%s'", newFound.Description)
	}
}

func TestGetTailscaleIP(t *testing.T) {
	tests := []struct {
		name      string
		addresses []string
		expected  string
	}{
		{
			name:      "single tailscale IP",
			addresses: []string{"100.64.0.1"},
			expected:  "100.64.0.1",
		},
		{
			name:      "mixed addresses with tailscale IP",
			addresses: []string{"192.168.1.1", "100.64.0.1", "10.0.0.1"},
			expected:  "100.64.0.1",
		},
		{
			name:      "no tailscale IP",
			addresses: []string{"192.168.1.1", "10.0.0.1"},
			expected:  "",
		},
		{
			name:      "empty addresses",
			addresses: []string{},
			expected:  "",
		},
		{
			name:      "multiple tailscale IPs (first one wins)",
			addresses: []string{"100.64.0.1", "100.64.0.2"},
			expected:  "100.64.0.1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getTailscaleIP(tt.addresses)
			if result != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}

func TestDeviceStorage_LastUpdatedTimestamp(t *testing.T) {
	storage := NewDeviceStorage()
	initialTime := storage.LastUpdated

	// Sleep a small amount to ensure timestamp difference
	time.Sleep(time.Millisecond)

	// Add a device and check timestamp was updated
	device := PersistedDevice{Name: "test", TailscaleIP: "100.64.0.1", Description: "Test"}
	storage.AddOrUpdateDevice(device)

	if !storage.LastUpdated.After(initialTime) {
		t.Error("LastUpdated should be updated when adding device")
	}

	// Update property and check timestamp was updated again
	updateTime := storage.LastUpdated
	time.Sleep(time.Millisecond)

	err := storage.UpdateDeviceProperty("test", "description", "Updated")
	if err != nil {
		t.Errorf("UpdateDeviceProperty failed: %v", err)
	}

	if !storage.LastUpdated.After(updateTime) {
		t.Error("LastUpdated should be updated when updating device property")
	}
}
