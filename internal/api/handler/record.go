package handler

import (
	"encoding/json"
	"net/http"
	"regexp"

	"github.com/jerkytreats/dns/internal/dns/coredns"
	"go.uber.org/zap"
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
	logger  *zap.Logger
	manager *coredns.Manager
}

// NewRecordHandler creates a new record handler
func NewRecordHandler(logger *zap.Logger, manager *coredns.Manager) *RecordHandler {
	return &RecordHandler{
		logger:  logger,
		manager: manager,
	}
}

// AddRecord handles adding a new DNS record
func (h *RecordHandler) AddRecord(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req AddRecordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendError(w, "Invalid request body", "INVALID_REQUEST", http.StatusBadRequest)
		return
	}

	// Validate service name
	if !serviceNameRegex.MatchString(req.ServiceName) {
		sendError(w, "Invalid service name format", "INVALID_SERVICE_NAME", http.StatusBadRequest)
		return
	}

	// Add zone to CoreDNS
	if err := h.manager.AddZone(req.ServiceName); err != nil {
		h.logger.Error("Failed to add zone",
			zap.String("service", req.ServiceName),
			zap.Error(err))
		sendError(w, "Failed to add DNS record", "DNS_ERROR", http.StatusInternalServerError)
		return
	}

	response := AddRecordResponse{
		Status:  "success",
		Message: "Record added successfully",
	}
	response.Data.Hostname = req.ServiceName + ".internal.jerkytreats.dev"

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
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
