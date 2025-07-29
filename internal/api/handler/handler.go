package handler

import (
	"net/http"

	"github.com/jerkytreats/dns/internal/api/types"
	"github.com/jerkytreats/dns/internal/dns/coredns"
	"github.com/jerkytreats/dns/internal/dns/record"
	"github.com/jerkytreats/dns/internal/docs"
	"github.com/jerkytreats/dns/internal/healthcheck"
	"github.com/jerkytreats/dns/internal/logging"
	"github.com/jerkytreats/dns/internal/proxy"
	"github.com/jerkytreats/dns/internal/tailscale"
	devicehandler "github.com/jerkytreats/dns/internal/tailscale/handler"
	"github.com/jerkytreats/dns/internal/tailscale/sync"
)

// HandlerRegistry manages all HTTP handlers for the application
type HandlerRegistry struct {
	recordHandler *RecordHandler
	healthHandler *healthcheck.Handler
	deviceHandler *devicehandler.DeviceHandler
	docsHandler   *docs.DocsHandler
	mux           *http.ServeMux
}

// NewHandlerRegistry creates a new handler registry with all handlers initialized
func NewHandlerRegistry(dnsManager *coredns.Manager, dnsChecker healthcheck.Checker, syncManager *sync.Manager, proxyManager *proxy.Manager, tailscaleClient *tailscale.Client, certificateManager interface {
	AddDomainToSAN(domain string) error
	RemoveDomainFromSAN(domain string) error
}) (*HandlerRegistry, error) {
	logging.Info("Initializing handler registry with all application handlers")

	// Create record service with all dependencies
	recordService := record.NewService(dnsManager, proxyManager, tailscaleClient)

	// Initialize handlers from their respective domains
	recordHandler, err := NewRecordHandler(recordService, certificateManager)
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

	docsHandler, err := docs.NewDocsHandler()
	if err != nil {
		return nil, err
	}

	registry := &HandlerRegistry{
		recordHandler: recordHandler,
		healthHandler: healthHandler,
		deviceHandler: deviceHandler,
		docsHandler:   docsHandler,
		mux:           http.NewServeMux(),
	}

	registry.RegisterHandlers(registry.mux)
	logging.Info("Handler registry initialized successfully with all handlers")

	return registry, nil
}

// RegisterHandlers registers all application handlers using the RouteInfo registry
func (hr *HandlerRegistry) RegisterHandlers(mux *http.ServeMux) {
	logging.Info("Registering all application handlers from RouteInfo registry")

	// Update RouteInfo registry with actual handler functions
	hr.updateRouteHandlers()

	// Register all routes from the central registry
	routes := GetRegisteredRoutes()
	for _, route := range routes {
		if route.Handler != nil {
			mux.HandleFunc(route.Path, route.Handler)
			logging.Debug("Registered %s %s from %s module", route.Method, route.Path, route.Module)
		} else {
			logging.Warn("Skipping route %s %s - handler is nil", route.Method, route.Path)
		}
	}

	logging.Info("Successfully registered %d handlers from RouteInfo registry", len(routes))
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

// GetDocsHandler returns the docs handler instance for direct access if needed
func (hr *HandlerRegistry) GetDocsHandler() *docs.DocsHandler {
	return hr.docsHandler
}

// updateRouteHandlers updates the RouteInfo registry with actual handler function references
func (hr *HandlerRegistry) updateRouteHandlers() {
	routes := GetRegisteredRoutes()
	for i, route := range routes {
		switch route.Path {
		case "/health":
			if hr.healthHandler != nil {
				routes[i].Handler = hr.healthHandler.ServeHTTP
			}
		case "/add-record":
			if hr.recordHandler != nil {
				routes[i].Handler = hr.recordHandler.AddRecord
			}
		case "/list-records":
			if hr.recordHandler != nil {
				routes[i].Handler = hr.recordHandler.ListRecords
			}
		case "/list-devices":
			if hr.deviceHandler != nil {
				routes[i].Handler = hr.deviceHandler.ListDevices
			}
		case "/annotate-device":
			if hr.deviceHandler != nil {
				routes[i].Handler = hr.deviceHandler.AnnotateDevice
			}
		case "/device-storage-info":
			if hr.deviceHandler != nil {
				routes[i].Handler = hr.deviceHandler.GetStorageInfo
			}
		case "/swagger":
			if hr.docsHandler != nil {
				routes[i].Handler = hr.docsHandler.ServeSwaggerUI
			}
		case "/docs/openapi.yaml":
			if hr.docsHandler != nil {
				routes[i].Handler = hr.docsHandler.ServeOpenAPISpec
			}
		case "/docs":
			if hr.docsHandler != nil {
				routes[i].Handler = hr.docsHandler.ServeDocs
			}
		}
	}
	
	// Update the global registry with the handler references
	types.UpdateRouteRegistry(routes)
}
