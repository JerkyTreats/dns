package handler

import (
	"encoding/json"
	"net/http"
	"regexp"

	"github.com/jerkytreats/dns/internal/dns/coredns"
	"github.com/jerkytreats/dns/internal/logging"
)

var serviceNameRegex = regexp.MustCompile(`^[a-z0-9-]+$`)

type AddRecordRequest struct {
	ServiceName string `json:"service_name"`
}

type AddRecordResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
	Data    struct {
		Hostname string `json:"hostname"`
	} `json:"data"`
}

type ErrorResponse struct {
	Status    string `json:"status"`
	Message   string `json:"message"`
	ErrorCode string `json:"error_code,omitempty"`
}

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
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Name string `json:"name"`
		IP   string `json:"ip"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if err := h.manager.AddRecord(req.Name, req.IP); err != nil {
		logging.Error("Failed to add record: %v", err)
		http.Error(w, "Failed to add record", http.StatusInternalServerError)
		return
	}

	logging.Info("Successfully added record for %s -> %s", req.Name, req.IP)
	w.WriteHeader(http.StatusCreated)
	w.Write([]byte("Record added successfully"))
}

func sendError(w http.ResponseWriter, message, errorCode string, statusCode int) {
	response := ErrorResponse{
		Status:    "error",
		Message:   message,
		ErrorCode: errorCode,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(response)
}
