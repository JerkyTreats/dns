package handler

import (
	"encoding/json"
	"net/http"

	"github.com/jerkytreats/dns/internal/dns/coredns"
	"github.com/jerkytreats/dns/internal/logging"
	"github.com/jerkytreats/dns/internal/proxy"
)

// AddRecordRequest represents the JSON payload for adding DNS records with optional proxy support
type AddRecordRequest struct {
	ServiceName  string `json:"service_name"`
	Name         string `json:"name"`
	IP           string `json:"ip"`
	Port         *int   `json:"port,omitempty"`          // Optional: target service port
	ForwardToIP  string `json:"forward_to_ip,omitempty"` // Optional: target service IP (for cross-device proxying)
	ProxyEnabled bool   `json:"proxy_enabled,omitempty"` // Optional: enable reverse proxy rule
}

// RecordHandler handles DNS record operations
type RecordHandler struct {
	manager      *coredns.Manager
	proxyManager *proxy.Manager
}

// NewRecordHandler creates a new record handler
func NewRecordHandler(manager *coredns.Manager, proxyManager *proxy.Manager) (*RecordHandler, error) {
	return &RecordHandler{
		manager:      manager,
		proxyManager: proxyManager,
	}, nil
}

// AddRecord handles adding a new DNS record with optional reverse proxy rule
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
	if req.ServiceName == "" || req.Name == "" || req.IP == "" {
		http.Error(w, "Missing required fields: service_name, name, ip", http.StatusBadRequest)
		return
	}

	// Validate proxy configuration if enabled
	if req.ProxyEnabled {
		if req.Port == nil {
			http.Error(w, "Port is required when proxy_enabled is true", http.StatusBadRequest)
			return
		}
		if req.ForwardToIP == "" {
			http.Error(w, "forward_to_ip is required when proxy_enabled is true", http.StatusBadRequest)
			return
		}
		if *req.Port <= 0 || *req.Port > 65535 {
			http.Error(w, "Port must be between 1 and 65535", http.StatusBadRequest)
			return
		}
	}

	// Create DNS record
	if err := h.manager.AddRecord(req.ServiceName, req.Name, req.IP); err != nil {
		logging.Error("Failed to add DNS record: %v", err)
		http.Error(w, "Failed to add DNS record", http.StatusInternalServerError)
		return
	}

	// Create proxy rule if requested
	if req.ProxyEnabled && req.Port != nil && h.proxyManager != nil {
		if err := h.proxyManager.AddRule(req.Name, req.ForwardToIP, *req.Port); err != nil {
			logging.Error("Failed to add proxy rule: %v", err)
			// Don't fail the entire request, but log the error
			logging.Warn("DNS record created but proxy rule failed - service accessible via port only")
		} else {
			logging.Info("Successfully added proxy rule: %s -> %s:%d", req.Name, req.ForwardToIP, *req.Port)
		}
	}

	logging.Info("Successfully added record for %s -> %s", req.Name, req.IP)
	if req.ProxyEnabled {
		logging.Info("Proxy enabled: %s will forward to %s:%d", req.Name, req.ForwardToIP, *req.Port)
	}

	w.WriteHeader(http.StatusCreated)
	w.Write([]byte("Record added successfully"))
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
