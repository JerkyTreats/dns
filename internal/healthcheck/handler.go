package healthcheck

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/jerkytreats/dns/internal/certificate"
	"github.com/jerkytreats/dns/internal/config"
	"github.com/jerkytreats/dns/internal/logging"
	"github.com/jerkytreats/dns/internal/tailscale/sync"
)

// HealthStatus represents the status of a component
type HealthStatus struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

// HealthResponse represents the full health check response
type HealthResponse struct {
	Status     string                  `json:"status"`
	Version    string                  `json:"version"`
	Components map[string]HealthStatus `json:"components"`
	// Certificate info can be added later without import cycles
	Certificate map[string]interface{} `json:"certificate,omitempty"`
}

// Handler handles health check HTTP requests
type Handler struct {
	dnsChecker         Checker
	syncManager        *sync.Manager
	getCertificateInfo func() map[string]interface{} // Injected function to avoid import cycles
}

// NewHandler creates a new health check handler with all necessary dependencies
func NewHandler(dnsChecker Checker, syncManager *sync.Manager) (*Handler, error) {
	logging.Info("Initializing health check handler")

	// Create certificate info function internally
	getCertificateInfo := func() map[string]interface{} {
		serverTLSCertFileKey := "server.tls.cert_file"
		certPath := config.GetString(serverTLSCertFileKey)

		if certInfo, err := certificate.GetCertificateInfo(certPath); err == nil {
			return map[string]interface{}{
				"subject":    certInfo.Subject.CommonName,
				"issuer":     certInfo.Issuer.CommonName,
				"not_before": certInfo.NotBefore,
				"not_after":  certInfo.NotAfter,
				"expires_in": time.Until(certInfo.NotAfter).String(),
			}
		}

		// Return nil on error - certificate info is optional
		return nil
	}

	handler := &Handler{
		dnsChecker:         dnsChecker,
		syncManager:        syncManager,
		getCertificateInfo: getCertificateInfo,
	}

	logging.Info("Health check handler initialized successfully")
	return handler, nil
}

// ServeHTTP handles health check requests
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	response := h.buildHealthResponse()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// buildHealthResponse constructs the complete health response
func (h *Handler) buildHealthResponse() HealthResponse {
	components := map[string]HealthStatus{
		"api": {
			Status:  "healthy",
			Message: "API is running",
		},
	}

	// Dynamic CoreDNS health status
	if h.dnsChecker != nil {
		if ok, latency, err := h.dnsChecker.CheckOnce(); ok {
			components["coredns"] = HealthStatus{
				Status:  "healthy",
				Message: fmt.Sprintf("CoreDNS responded in %v", latency),
			}
		} else {
			msg := "CoreDNS probe failed"
			if err != nil {
				msg = err.Error()
			}
			components["coredns"] = HealthStatus{
				Status:  "error",
				Message: msg,
			}
		}
	} else {
		components["coredns"] = HealthStatus{
			Status:  "unknown",
			Message: "DNS checker not initialized",
		}
	}

	// Sync status
	syncEnabledKey := "dns.internal.enabled"
	if config.GetBool(syncEnabledKey) {
		if h.syncManager != nil && h.syncManager.IsZoneSynced() {
			components["sync"] = HealthStatus{
				Status:  "healthy",
				Message: "Dynamic zone sync is active",
			}
		} else if h.syncManager != nil {
			components["sync"] = HealthStatus{
				Status:  "warning",
				Message: "Sync manager created but zone not synced",
			}
		} else {
			components["sync"] = HealthStatus{
				Status:  "starting",
				Message: "Dynamic zone sync is starting in background",
			}
		}
	} else {
		components["sync"] = HealthStatus{
			Status:  "disabled",
			Message: "Dynamic zone sync is disabled",
		}
	}

	// Build response
	appVersionKey := "app.version"
	response := HealthResponse{
		Status:     "healthy",
		Version:    config.GetString(appVersionKey),
		Components: components,
	}

	// Add certificate info if available and TLS is enabled
	serverTLSEnabledKey := "server.tls.enabled"
	if config.GetBool(serverTLSEnabledKey) && h.getCertificateInfo != nil {
		if certInfo := h.getCertificateInfo(); certInfo != nil {
			response.Certificate = certInfo
		}
	}

	return response
}
