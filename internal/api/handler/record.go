package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/jerkytreats/dns/internal/config"
	"github.com/jerkytreats/dns/internal/dns/coredns"
	"github.com/jerkytreats/dns/internal/logging"
	"github.com/jerkytreats/dns/internal/proxy"
	"github.com/jerkytreats/dns/internal/tailscale"
)

// AddRecordRequest represents the simplified JSON payload for adding DNS records with automatic proxy setup
type AddRecordRequest struct {
	ServiceName string `json:"service_name"`
	Name        string `json:"name"`
	Port        *int   `json:"port,omitempty"` // Optional: triggers automatic proxy setup
}

// RecordHandler handles DNS record operations
type RecordHandler struct {
	manager         *coredns.Manager
	proxyManager    *proxy.Manager
	tailscaleClient *tailscale.Client
}

// NewRecordHandler creates a new record handler
func NewRecordHandler(manager *coredns.Manager, proxyManager *proxy.Manager, tailscaleClient *tailscale.Client) (*RecordHandler, error) {
	return &RecordHandler{
		manager:         manager,
		proxyManager:    proxyManager,
		tailscaleClient: tailscaleClient,
	}, nil
}

// getDNSManagerIP returns the Tailscale IP of the current DNS Manager device
func (h *RecordHandler) getDNSManagerIP() (string, error) {
	if h.tailscaleClient == nil {
		return "", fmt.Errorf("tailscale client not available")
	}

	var ip string
	var err error

	// Check if a specific device name is configured
	deviceName := config.GetString(tailscale.TailscaleDeviceNameKey)
	if deviceName != "" {
		logging.Debug("Using configured Tailscale device name for DNS Manager: %s", deviceName)
		ip, err = h.tailscaleClient.GetCurrentDeviceIPByName(deviceName)
	} else {
		logging.Debug("No device name configured, using hostname-based detection for DNS Manager")
		ip, err = h.tailscaleClient.GetCurrentDeviceIP()
	}

	if err != nil {
		logging.Error("Failed to get DNS Manager IP: %v", err)
		return "", fmt.Errorf("failed to get DNS Manager IP: %w", err)
	}

	logging.Debug("DNS Manager Tailscale IP: %s", ip)
	return ip, nil
}

// getDNSManagerDevice returns the Tailscale device information for the current DNS Manager
func (h *RecordHandler) getDNSManagerDevice() (*tailscale.Device, error) {
	if h.tailscaleClient == nil {
		return nil, fmt.Errorf("tailscale client not available")
	}

	devices, err := h.tailscaleClient.ListDevices()
	if err != nil {
		return nil, fmt.Errorf("failed to list devices: %w", err)
	}

	// Get current hostname to identify this device
	hostname, err := os.Hostname()
	if err != nil {
		return nil, fmt.Errorf("failed to get hostname: %w", err)
	}

	// Find current device by hostname matching
	for _, device := range devices {
		if device.Hostname == hostname || strings.Split(device.Hostname, ".")[0] == strings.Split(hostname, ".")[0] {
			logging.Debug("Found DNS Manager device: %s (%s)", device.Name, device.Hostname)
			return &device, nil
		}
	}

	return nil, fmt.Errorf("DNS Manager device not found in Tailscale network")
}

// AddRecord handles adding a new DNS record with automatic device detection and optional reverse proxy rule
func (h *RecordHandler) AddRecord(w http.ResponseWriter, r *http.Request) {
	logging.Info("Processing add record request")

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req AddRecordRequest

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate required fields
	if req.ServiceName == "" || req.Name == "" {
		http.Error(w, "Missing required fields: service_name, name", http.StatusBadRequest)
		return
	}

	// Validate port if specified
	if req.Port != nil && (*req.Port <= 0 || *req.Port > 65535) {
		http.Error(w, "Port must be between 1 and 65535", http.StatusBadRequest)
		return
	}

	// Get DNS Manager IP (where all DNS records will point)
	dnsManagerIP, err := h.getDNSManagerIP()
	if err != nil {
		logging.Error("Failed to get DNS Manager IP: %v", err)

		// Fallback for testing or when Tailscale client is unavailable
		if h.tailscaleClient == nil {
			logging.Warn("Tailscale client not available - using fallback IP for DNS records")
			dnsManagerIP = "127.0.0.1" // Test fallback
		} else {
			http.Error(w, "Failed to determine DNS Manager IP", http.StatusInternalServerError)
			return
		}
	}

	// Create DNS record pointing to DNS Manager
	if err := h.manager.AddRecord(req.ServiceName, req.Name, dnsManagerIP); err != nil {
		logging.Error("Failed to add DNS record: %v", err)
		http.Error(w, "Failed to add DNS record", http.StatusInternalServerError)
		return
	}

	logging.Info("Successfully added DNS record: %s -> %s", req.Name, dnsManagerIP)

	// If port is specified, create automatic proxy rule
	if req.Port != nil && h.proxyManager != nil {
		h.proxyManager.SetupAutomaticProxy(r, req.Name, dnsManagerIP, *req.Port, h.tailscaleClient)
	}

	w.WriteHeader(http.StatusCreated)
	response := map[string]interface{}{
		"message":    "Record added successfully",
		"dns_record": fmt.Sprintf("%s -> %s", req.Name, dnsManagerIP),
	}

	if req.Port != nil {
		response["proxy_port"] = *req.Port
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// ListRecords handles listing all DNS records from the internal zone
func (h *RecordHandler) ListRecords(w http.ResponseWriter, r *http.Request) {
	logging.Info("Processing list records request")

	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	records, err := h.manager.ListRecords("internal")
	if err != nil {
		logging.Error("Failed to list records: %v", err)
		http.Error(w, "Failed to list records", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(records); err != nil {
		logging.Error("Failed to encode records to JSON: %v", err)
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}

	logging.Info("Successfully returned %d records", len(records))
}
