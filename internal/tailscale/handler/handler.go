// Package handler provides HTTP handlers for Tailscale device management API endpoints
package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/jerkytreats/dns/internal/logging"
	"github.com/jerkytreats/dns/internal/persistence"
	"github.com/jerkytreats/dns/internal/tailscale"
)

// DeviceHandler handles HTTP requests for device management operations
type DeviceHandler struct {
	storage *persistence.FileStorage
}

// NewDeviceHandler creates a new device handler with the provided storage
func NewDeviceHandler(storage *persistence.FileStorage) *DeviceHandler {
	return &DeviceHandler{
		storage: storage,
	}
}

// NewDeviceHandlerWithDefaults creates a device handler with default file storage
func NewDeviceHandlerWithDefaults() (*DeviceHandler, error) {
	storage := persistence.NewFileStorage()
	return &DeviceHandler{
		storage: storage,
	}, nil
}

// ListDevices handles GET /list-devices - returns all known devices with their metadata
func (dh *DeviceHandler) ListDevices(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		dh.writeErrorResponse(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	logging.Debug("Handling list devices request")

	deviceStorage, err := dh.loadDeviceStorage()
	if err != nil {
		logging.Error("Failed to load device storage: %v", err)
		dh.writeErrorResponse(w, http.StatusInternalServerError, "Failed to load device data")
		return
	}

	devices := deviceStorage.GetDevices()
	logging.Debug("Retrieved %d devices for list request", len(devices))

	// Set response headers
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	// Encode and send response
	if err := json.NewEncoder(w).Encode(devices); err != nil {
		logging.Error("Failed to encode device list response: %v", err)
		// Response already started, can't change status code
		return
	}

	logging.Debug("Successfully returned device list with %d devices", len(devices))
}

// AnnotateDevice handles POST /annotate-device - updates annotatable device properties
func (dh *DeviceHandler) AnnotateDevice(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		dh.writeErrorResponse(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	logging.Debug("Handling annotate device request")

	// Parse request body
	var req tailscale.AnnotationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		logging.Warn("Failed to decode annotation request: %v", err)
		dh.writeErrorResponse(w, http.StatusBadRequest, "Invalid JSON request body")
		return
	}

	// Validate request
	if err := tailscale.ValidateAnnotationRequest(&req); err != nil {
		logging.Warn("Invalid annotation request: %v", err)

		// Check if it's an immutable property error
		if strings.Contains(err.Error(), "immutable") {
			dh.writeErrorResponse(w, http.StatusUnprocessableEntity, err.Error())
		} else {
			dh.writeErrorResponse(w, http.StatusBadRequest, err.Error())
		}
		return
	}

	// Load device storage
	deviceStorage, err := dh.loadDeviceStorage()
	if err != nil {
		logging.Error("Failed to load device storage: %v", err)
		dh.writeErrorResponse(w, http.StatusInternalServerError, "Failed to load device data")
		return
	}

	// Update device property
	if err := deviceStorage.UpdateDeviceProperty(req.Name, req.Property, req.Value); err != nil {
		if strings.Contains(err.Error(), "not found") {
			logging.Warn("Device not found for annotation: %s", req.Name)
			dh.writeErrorResponse(w, http.StatusBadRequest, fmt.Sprintf("Device '%s' not found", req.Name))
		} else {
			logging.Error("Failed to update device property: %v", err)
			dh.writeErrorResponse(w, http.StatusInternalServerError, "Failed to update device")
		}
		return
	}

	// Save updated storage
	if err := dh.saveDeviceStorage(deviceStorage); err != nil {
		logging.Error("Failed to save device storage: %v", err)
		dh.writeErrorResponse(w, http.StatusInternalServerError, "Failed to save device data")
		return
	}

	logging.Info("Successfully updated device '%s' property '%s' to '%s'", req.Name, req.Property, req.Value)

	// Return success response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	response := map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("Device '%s' property '%s' updated successfully", req.Name, req.Property),
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		logging.Error("Failed to encode success response: %v", err)
		return
	}
}

// GetStorageInfo handles GET /device-storage-info - returns storage information (for debugging)
func (dh *DeviceHandler) GetStorageInfo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		dh.writeErrorResponse(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	logging.Debug("Handling storage info request")

	info := dh.storage.GetStorageInfo()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	if err := json.NewEncoder(w).Encode(info); err != nil {
		logging.Error("Failed to encode storage info response: %v", err)
		return
	}

	logging.Debug("Successfully returned storage info")
}

// loadDeviceStorage loads device data from storage, creating empty storage if none exists
func (dh *DeviceHandler) loadDeviceStorage() (*tailscale.DeviceStorage, error) {
	data, err := dh.storage.Read()
	if err != nil {
		return nil, fmt.Errorf("failed to read storage: %w", err)
	}

	deviceStorage := tailscale.NewDeviceStorage()

	// If no data exists, return empty storage
	if data == nil || len(data) == 0 {
		logging.Debug("No existing device data found, using empty storage")
		return deviceStorage, nil
	}

	// Parse existing data
	if err := deviceStorage.FromJSON(data); err != nil {
		logging.Warn("Failed to parse device storage, starting with empty storage: %v", err)
		// Return empty storage rather than failing
		return tailscale.NewDeviceStorage(), nil
	}

	return deviceStorage, nil
}

// saveDeviceStorage saves device data to storage
func (dh *DeviceHandler) saveDeviceStorage(deviceStorage *tailscale.DeviceStorage) error {
	data, err := deviceStorage.ToJSON()
	if err != nil {
		return fmt.Errorf("failed to serialize device storage: %w", err)
	}

	if err := dh.storage.Write(data); err != nil {
		return fmt.Errorf("failed to write device storage: %w", err)
	}

	return nil
}

// writeErrorResponse writes a JSON error response
func (dh *DeviceHandler) writeErrorResponse(w http.ResponseWriter, statusCode int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	errorResponse := map[string]interface{}{
		"error":   true,
		"message": message,
		"status":  statusCode,
	}

	if err := json.NewEncoder(w).Encode(errorResponse); err != nil {
		logging.Error("Failed to encode error response: %v", err)
	}
}

// GetStorage returns the underlying storage (for testing)
func (dh *DeviceHandler) GetStorage() *persistence.FileStorage {
	return dh.storage
}
