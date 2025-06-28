package handler

import (
	"net/http"

	"github.com/jerkytreats/dns/internal/dns/coredns"
	"github.com/jerkytreats/dns/internal/healthcheck"
	"github.com/jerkytreats/dns/internal/logging"
	devicehandler "github.com/jerkytreats/dns/internal/tailscale/handler"
	"github.com/jerkytreats/dns/internal/tailscale/sync"
)

// HandlerRegistry manages all HTTP handlers for the application
type HandlerRegistry struct {
	recordHandler *RecordHandler
	healthHandler *healthcheck.Handler
	deviceHandler *devicehandler.DeviceHandler
	mux           *http.ServeMux
}

// NewHandlerRegistry creates a new handler registry with all handlers initialized
func NewHandlerRegistry(dnsManager *coredns.Manager, dnsChecker healthcheck.Checker, syncManager *sync.Manager) (*HandlerRegistry, error) {
	logging.Info("Initializing handler registry with all application handlers")

	// Initialize handlers from their respective domains
	recordHandler, err := NewRecordHandler(dnsManager)
	if err != nil {
		return nil, err
	}

	healthHandler, err := healthcheck.NewHandler(dnsChecker, syncManager)
	if err != nil {
		return nil, err
	}

	deviceHandler, err := devicehandler.NewDeviceHandlerWithDefaults()
	if err != nil {
		return nil, err
	}

	registry := &HandlerRegistry{
		recordHandler: recordHandler,
		healthHandler: healthHandler,
		deviceHandler: deviceHandler,
		mux:           http.NewServeMux(),
	}

	registry.RegisterHandlers(registry.mux)
	logging.Info("Handler registry initialized successfully with all handlers")

	return registry, nil
}

// RegisterHandlers registers all application handlers with the provided ServeMux
func (hr *HandlerRegistry) RegisterHandlers(mux *http.ServeMux) {
	logging.Info("Registering all application handlers")

	// Register all handlers
	mux.Handle("/health", hr.healthHandler)
	mux.HandleFunc("/add-record", hr.recordHandler.AddRecord)
	mux.HandleFunc("/list-records", hr.recordHandler.ListRecords)

	// Register device management endpoints
	mux.HandleFunc("/list-devices", hr.deviceHandler.ListDevices)
	mux.HandleFunc("/annotate-device", hr.deviceHandler.AnnotateDevice)
	mux.HandleFunc("/device-storage-info", hr.deviceHandler.GetStorageInfo)

	logging.Info("All application handlers registered successfully")
}

// GetServeMux returns the internal ServeMux with all handlers registered
func (hr *HandlerRegistry) GetServeMux() *http.ServeMux {
	return hr.mux
}

// GetRecordHandler returns the record handler instance for direct access if needed
func (hr *HandlerRegistry) GetRecordHandler() *RecordHandler {
	return hr.recordHandler
}

// GetHealthHandler returns the health handler instance for direct access if needed
func (hr *HandlerRegistry) GetHealthHandler() *healthcheck.Handler {
	return hr.healthHandler
}

// GetDeviceHandler returns the device handler instance for direct access if needed
func (hr *HandlerRegistry) GetDeviceHandler() *devicehandler.DeviceHandler {
	return hr.deviceHandler
}
