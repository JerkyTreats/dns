package handler

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/jerkytreats/dns/internal/config"
	"github.com/jerkytreats/dns/internal/dns/coredns"
	"github.com/jerkytreats/dns/internal/dns/record"
	"github.com/jerkytreats/dns/internal/logging"
)

// RecordHandler handles DNS record operations using the record service layer
type RecordHandler struct {
	recordService     *record.Service
	certificateManager interface {
		AddDomainToSAN(domain string) error
		RemoveDomainFromSAN(domain string) error
	}
}

// NewRecordHandler creates a new record handler with the record service
func NewRecordHandler(recordService *record.Service, certificateManager interface {
	AddDomainToSAN(domain string) error
	RemoveDomainFromSAN(domain string) error
}) (*RecordHandler, error) {
	return &RecordHandler{
		recordService:     recordService,
		certificateManager: certificateManager,
	}, nil
}

// AddRecord handles adding a new DNS record with automatic device detection and optional reverse proxy rule
func (h *RecordHandler) AddRecord(w http.ResponseWriter, r *http.Request) {
	logging.Info("Processing add record request")

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req record.CreateRecordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Create record using service layer with device detection
	recordResult, err := h.recordService.CreateRecord(req, r)
	if err != nil {
		logging.Error("Failed to create record: %v", err)
		http.Error(w, "Failed to create record", http.StatusInternalServerError)
		return
	}

	// Add domain to certificate SAN list asynchronously to avoid blocking the API response
	if h.certificateManager != nil && req.Name != "" {
		dnsDomain := config.GetString(coredns.DNSDomainKey)
		if dnsDomain != "" {
			domain := fmt.Sprintf("%s.%s", req.Name, dnsDomain)
			go func(domainToAdd string) {
				logging.Info("Starting asynchronous certificate SAN check for domain: %s", domainToAdd)
				if err := h.certificateManager.AddDomainToSAN(domainToAdd); err != nil {
					logging.Warn("Failed to add domain to certificate SAN: %v", err)
				} else {
					logging.Info("Successfully processed domain for certificate SAN: %s", domainToAdd)
				}
			}(domain)
			logging.Info("Initiated asynchronous certificate SAN processing for domain: %s", domain)
		}
	}

	w.WriteHeader(http.StatusCreated)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(recordResult)
}

// ListRecords handles listing all DNS records with proxy information
func (h *RecordHandler) ListRecords(w http.ResponseWriter, r *http.Request) {
	logging.Info("Processing list records request")

	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get records using service layer
	records, err := h.recordService.ListRecords()
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

// RemoveRecord handles removing a DNS record and associated proxy rule
func (h *RecordHandler) RemoveRecord(w http.ResponseWriter, r *http.Request) {
	logging.Info("Processing remove record request")

	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req record.RemoveRecordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Remove record using service layer
	err := h.recordService.RemoveRecord(req)
	if err != nil {
		logging.Error("Failed to remove record: %v", err)
		http.Error(w, "Failed to remove record", http.StatusInternalServerError)
		return
	}

	// Remove domain from certificate SAN list asynchronously
	if h.certificateManager != nil && req.Name != "" {
		dnsDomain := config.GetString(coredns.DNSDomainKey)
		if dnsDomain != "" {
			domain := fmt.Sprintf("%s.%s", req.Name, dnsDomain)
			go func(domainToRemove string) {
				logging.Info("Starting asynchronous certificate update for domain removal: %s", domainToRemove)
				if err := h.certificateManager.RemoveDomainFromSAN(domainToRemove); err != nil {
					logging.Warn("Failed to remove domain from certificate SAN: %v", err)
				} else {
					logging.Info("Successfully removed domain from certificate SAN: %s", domainToRemove)
				}
			}(domain)
			logging.Info("Initiated asynchronous certificate update for domain removal: %s", domain)
		}
	}

	w.WriteHeader(http.StatusOK)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "Record removed successfully"})
}
