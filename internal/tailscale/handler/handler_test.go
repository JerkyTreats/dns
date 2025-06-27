// Package handler provides HTTP handlers for Tailscale device management API endpoints
package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/jerkytreats/dns/internal/persistence"
	"github.com/jerkytreats/dns/internal/tailscale"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupTestHandler creates a test handler with a temporary storage
func setupTestHandler(t *testing.T) (*DeviceHandler, string) {
	testDir := t.TempDir()
	filePath := filepath.Join(testDir, "test_devices.json")
	storage := persistence.NewFileStorageWithPath(filePath, 3)

	handler := NewDeviceHandler(storage)
	return handler, filePath
}

// setupTestDeviceData creates test device data in storage
func setupTestDeviceData(t *testing.T, handler *DeviceHandler) {
	deviceStorage := tailscale.NewDeviceStorage()

	// Add test devices
	device1 := tailscale.PersistedDevice{
		Name:        "test-device-1",
		TailscaleIP: "100.64.0.1",
		Description: "Test device 1",
	}
	device2 := tailscale.PersistedDevice{
		Name:        "test-device-2",
		TailscaleIP: "100.64.0.2",
		Description: "Test device 2",
	}

	deviceStorage.AddOrUpdateDevice(device1)
	deviceStorage.AddOrUpdateDevice(device2)

	err := handler.saveDeviceStorage(deviceStorage)
	require.NoError(t, err)
}

func TestNewDeviceHandler(t *testing.T) {
	testDir := t.TempDir()
	filePath := filepath.Join(testDir, "test.json")
	storage := persistence.NewFileStorageWithPath(filePath, 3)

	handler := NewDeviceHandler(storage)

	assert.NotNil(t, handler)
	assert.Equal(t, storage, handler.storage)
}

func TestNewDeviceHandlerWithDefaults(t *testing.T) {
	handler, err := NewDeviceHandlerWithDefaults()

	assert.NoError(t, err)
	assert.NotNil(t, handler)
	assert.NotNil(t, handler.storage)
}

func TestDeviceHandler_ListDevices(t *testing.T) {
	tests := []struct {
		name           string
		method         string
		setupData      bool
		expectedStatus int
		expectedCount  int
	}{
		{
			name:           "Valid GET request with data",
			method:         http.MethodGet,
			setupData:      true,
			expectedStatus: http.StatusOK,
			expectedCount:  2,
		},
		{
			name:           "Valid GET request with no data",
			method:         http.MethodGet,
			setupData:      false,
			expectedStatus: http.StatusOK,
			expectedCount:  0,
		},
		{
			name:           "Invalid method",
			method:         http.MethodPost,
			setupData:      false,
			expectedStatus: http.StatusMethodNotAllowed,
			expectedCount:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, _ := setupTestHandler(t)

			if tt.setupData {
				setupTestDeviceData(t, handler)
			}

			req := httptest.NewRequest(tt.method, "/list-devices", nil)
			w := httptest.NewRecorder()

			handler.ListDevices(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if tt.expectedStatus == http.StatusOK {
				var devices []tailscale.PersistedDevice
				err := json.Unmarshal(w.Body.Bytes(), &devices)
				assert.NoError(t, err)
				assert.Len(t, devices, tt.expectedCount)

				if tt.expectedCount > 0 {
					assert.Equal(t, "test-device-1", devices[0].Name)
					assert.Equal(t, "100.64.0.1", devices[0].TailscaleIP)
					assert.Equal(t, "Test device 1", devices[0].Description)
				}
			} else {
				var errorResp map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &errorResp)
				assert.NoError(t, err)
				assert.Equal(t, true, errorResp["error"])
			}
		})
	}
}

func TestDeviceHandler_AnnotateDevice(t *testing.T) {
	tests := []struct {
		name           string
		method         string
		body           interface{}
		setupData      bool
		expectedStatus int
		expectedError  string
	}{
		{
			name:   "Valid annotation request",
			method: http.MethodPost,
			body: tailscale.AnnotationRequest{
				Name:     "test-device-1",
				Property: "description",
				Value:    "Updated description",
			},
			setupData:      true,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Invalid method",
			method:         http.MethodGet,
			body:           nil,
			setupData:      false,
			expectedStatus: http.StatusMethodNotAllowed,
		},
		{
			name:           "Invalid JSON body",
			method:         http.MethodPost,
			body:           "invalid json",
			setupData:      false,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:   "Empty device name",
			method: http.MethodPost,
			body: tailscale.AnnotationRequest{
				Name:     "",
				Property: "description",
				Value:    "test",
			},
			setupData:      false,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:   "Immutable property",
			method: http.MethodPost,
			body: tailscale.AnnotationRequest{
				Name:     "test-device-1",
				Property: "name",
				Value:    "new-name",
			},
			setupData:      true,
			expectedStatus: http.StatusUnprocessableEntity,
		},
		{
			name:   "Unknown property",
			method: http.MethodPost,
			body: tailscale.AnnotationRequest{
				Name:     "test-device-1",
				Property: "unknown",
				Value:    "test",
			},
			setupData:      true,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:   "Device not found",
			method: http.MethodPost,
			body: tailscale.AnnotationRequest{
				Name:     "nonexistent-device",
				Property: "description",
				Value:    "test",
			},
			setupData:      true,
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, _ := setupTestHandler(t)

			if tt.setupData {
				setupTestDeviceData(t, handler)
			}

			var reqBody []byte
			var err error
			if tt.body != nil {
				if str, ok := tt.body.(string); ok {
					reqBody = []byte(str)
				} else {
					reqBody, err = json.Marshal(tt.body)
					require.NoError(t, err)
				}
			}

			req := httptest.NewRequest(tt.method, "/annotate-device", bytes.NewReader(reqBody))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			handler.AnnotateDevice(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			var response map[string]interface{}
			err = json.Unmarshal(w.Body.Bytes(), &response)
			assert.NoError(t, err)

			if tt.expectedStatus == http.StatusOK {
				assert.Equal(t, true, response["success"])
				assert.Contains(t, response["message"], "updated successfully")

				// Verify the device was actually updated
				if tt.body != nil {
					if req, ok := tt.body.(tailscale.AnnotationRequest); ok {
						deviceStorage, err := handler.loadDeviceStorage()
						require.NoError(t, err)
						device, _ := deviceStorage.FindDevice(req.Name)
						assert.NotNil(t, device)
						assert.Equal(t, req.Value, device.Description)
					}
				}
			} else {
				assert.Equal(t, true, response["error"])
				assert.NotEmpty(t, response["message"])
			}
		})
	}
}

func TestDeviceHandler_GetStorageInfo(t *testing.T) {
	tests := []struct {
		name           string
		method         string
		setupData      bool
		expectedStatus int
	}{
		{
			name:           "Valid GET request",
			method:         http.MethodGet,
			setupData:      true,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Invalid method",
			method:         http.MethodPost,
			setupData:      false,
			expectedStatus: http.StatusMethodNotAllowed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, filePath := setupTestHandler(t)

			if tt.setupData {
				setupTestDeviceData(t, handler)
			}

			req := httptest.NewRequest(tt.method, "/device-storage-info", nil)
			w := httptest.NewRecorder()

			handler.GetStorageInfo(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			var response map[string]interface{}
			err := json.Unmarshal(w.Body.Bytes(), &response)
			assert.NoError(t, err)

			if tt.expectedStatus == http.StatusOK {
				assert.Equal(t, filePath, response["file_path"])
				assert.Equal(t, float64(3), response["backup_count"]) // JSON numbers are float64

				if tt.setupData {
					assert.Equal(t, true, response["exists"])
					assert.Greater(t, response["size"], float64(0))
				}
			} else {
				assert.Equal(t, true, response["error"])
			}
		})
	}
}

func TestDeviceHandler_LoadDeviceStorage(t *testing.T) {
	tests := []struct {
		name          string
		setupData     bool
		corruptData   bool
		expectedError bool
		expectedCount int
	}{
		{
			name:          "Load existing data",
			setupData:     true,
			corruptData:   false,
			expectedError: false,
			expectedCount: 2,
		},
		{
			name:          "Load non-existent data",
			setupData:     false,
			corruptData:   false,
			expectedError: false,
			expectedCount: 0,
		},
		{
			name:          "Load corrupted data",
			setupData:     false,
			corruptData:   true,
			expectedError: false,
			expectedCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, filePath := setupTestHandler(t)

			if tt.setupData {
				setupTestDeviceData(t, handler)
			} else if tt.corruptData {
				// Write corrupted JSON data
				err := os.WriteFile(filePath, []byte("invalid json data"), 0644)
				require.NoError(t, err)
			}

			deviceStorage, err := handler.loadDeviceStorage()

			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, deviceStorage)
				assert.Len(t, deviceStorage.Devices, tt.expectedCount)
			}
		})
	}
}

func TestDeviceHandler_SaveDeviceStorage(t *testing.T) {
	handler, filePath := setupTestHandler(t)

	deviceStorage := tailscale.NewDeviceStorage()
	device := tailscale.PersistedDevice{
		Name:        "test-device",
		TailscaleIP: "100.64.0.1",
		Description: "Test device",
	}
	deviceStorage.AddOrUpdateDevice(device)

	err := handler.saveDeviceStorage(deviceStorage)
	assert.NoError(t, err)

	// Verify file was created and contains expected data
	assert.FileExists(t, filePath)

	data, err := os.ReadFile(filePath)
	assert.NoError(t, err)

	var savedStorage tailscale.DeviceStorage
	err = json.Unmarshal(data, &savedStorage)
	assert.NoError(t, err)
	assert.Len(t, savedStorage.Devices, 1)
	assert.Equal(t, "test-device", savedStorage.Devices[0].Name)
}

func TestDeviceHandler_WriteErrorResponse(t *testing.T) {
	handler, _ := setupTestHandler(t)

	w := httptest.NewRecorder()
	handler.writeErrorResponse(w, http.StatusBadRequest, "Test error message")

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	assert.Equal(t, true, response["error"])
	assert.Equal(t, "Test error message", response["message"])
	assert.Equal(t, float64(http.StatusBadRequest), response["status"])
}

func TestDeviceHandler_GetStorage(t *testing.T) {
	testDir := t.TempDir()
	filePath := filepath.Join(testDir, "test.json")
	storage := persistence.NewFileStorageWithPath(filePath, 3)

	handler := NewDeviceHandler(storage)

	assert.Equal(t, storage, handler.GetStorage())
}

func TestDeviceHandler_Integration(t *testing.T) {
	// Integration test that exercises the full workflow
	handler, _ := setupTestHandler(t)

	// Step 1: List devices (should be empty)
	req := httptest.NewRequest(http.MethodGet, "/list-devices", nil)
	w := httptest.NewRecorder()
	handler.ListDevices(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var devices []tailscale.PersistedDevice
	err := json.Unmarshal(w.Body.Bytes(), &devices)
	assert.NoError(t, err)
	assert.Len(t, devices, 0)

	// Step 2: Setup test data manually (simulating sync operation)
	setupTestDeviceData(t, handler)

	// Step 3: List devices (should have data)
	req = httptest.NewRequest(http.MethodGet, "/list-devices", nil)
	w = httptest.NewRecorder()
	handler.ListDevices(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	err = json.Unmarshal(w.Body.Bytes(), &devices)
	assert.NoError(t, err)
	assert.Len(t, devices, 2)

	// Step 4: Annotate a device
	annotationReq := tailscale.AnnotationRequest{
		Name:     "test-device-1",
		Property: "description",
		Value:    "Updated via annotation",
	}
	reqBody, err := json.Marshal(annotationReq)
	assert.NoError(t, err)

	req = httptest.NewRequest(http.MethodPost, "/annotate-device", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	handler.AnnotateDevice(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	// Step 5: Verify annotation was applied
	req = httptest.NewRequest(http.MethodGet, "/list-devices", nil)
	w = httptest.NewRecorder()
	handler.ListDevices(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	err = json.Unmarshal(w.Body.Bytes(), &devices)
	assert.NoError(t, err)

	// Find the updated device
	var updatedDevice *tailscale.PersistedDevice
	for _, device := range devices {
		if device.Name == "test-device-1" {
			updatedDevice = &device
			break
		}
	}
	assert.NotNil(t, updatedDevice)
	assert.Equal(t, "Updated via annotation", updatedDevice.Description)

	// Step 6: Check storage info
	req = httptest.NewRequest(http.MethodGet, "/device-storage-info", nil)
	w = httptest.NewRecorder()
	handler.GetStorageInfo(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var storageInfo map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &storageInfo)
	assert.NoError(t, err)
	assert.Equal(t, true, storageInfo["exists"])
	assert.Greater(t, storageInfo["size"], float64(0))
}
