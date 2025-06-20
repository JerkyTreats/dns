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
func NewRecordHandler(manager *coredns.Manager) *RecordHandler {
	return &RecordHandler{
		manager: manager,
	}
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
