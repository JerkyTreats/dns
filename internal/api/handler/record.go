package handler

import (
	"encoding/json"
	"net/http"

	"github.com/jerkytreats/dns/internal/dns/coredns"
	"github.com/jerkytreats/dns/internal/logging"
)

// RecordHandler handles DNS record operations
type RecordHandler struct {
	manager *coredns.Manager
}

// NewRecordHandler creates a new record handler
func NewRecordHandler(manager *coredns.Manager) (*RecordHandler, error) {
	return &RecordHandler{
		manager: manager,
	}, nil
}

// AddRecord handles adding a new DNS record
func (h *RecordHandler) AddRecord(w http.ResponseWriter, r *http.Request) {
	logging.Info("Processing add record request")

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		ServiceName string `json:"service_name"`
		Name        string `json:"name"`
		IP          string `json:"ip"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if err := h.manager.AddRecord(req.ServiceName, req.Name, req.IP); err != nil {
		logging.Error("Failed to add record: %v", err)
		http.Error(w, "Failed to add record", http.StatusInternalServerError)
		return
	}

	logging.Info("Successfully added record for %s -> %s", req.Name, req.IP)
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
