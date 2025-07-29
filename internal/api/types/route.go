package types

import (
	"net/http"
	"reflect"
	"sync"

	"github.com/jerkytreats/dns/internal/logging"
)

// RouteInfo contains metadata for API route registration and documentation generation
type RouteInfo struct {
	Method       string           // HTTP method (GET, POST, etc.)
	Path         string           // Route path (/list-devices)
	Handler      http.HandlerFunc // Handler function
	RequestType  reflect.Type     // Request body type (nil for GET)
	ResponseType reflect.Type     // Success response type
	Module       string           // Module name for documentation grouping
	Summary      string           // Optional operation summary
}

var (
	// routeRegistry holds all registered routes
	routeRegistry []RouteInfo
	// registryMutex protects concurrent access to routeRegistry
	registryMutex sync.RWMutex
)

// RegisterRoute adds a new route to the global registry
// This function is called by modules during their init() phase
func RegisterRoute(route RouteInfo) {
	registryMutex.Lock()
	defer registryMutex.Unlock()
	
	routeRegistry = append(routeRegistry, route)
	logging.Debug("Registered route: %s %s from module %s", route.Method, route.Path, route.Module)
}

// GetRegisteredRoutes returns a copy of all registered routes
// This is used by the OpenAPI generator and handler registry
func GetRegisteredRoutes() []RouteInfo {
	registryMutex.RLock()
	defer registryMutex.RUnlock()
	
	// Return a copy to prevent external modification
	routes := make([]RouteInfo, len(routeRegistry))
	copy(routes, routeRegistry)
	return routes
}

// UpdateRouteRegistry updates the entire route registry (used by handler registry)
func UpdateRouteRegistry(routes []RouteInfo) {
	registryMutex.Lock()
	defer registryMutex.Unlock()
	
	routeRegistry = routes
}

// ClearRegistry clears all registered routes (used for testing)
func ClearRegistry() {
	registryMutex.Lock()
	defer registryMutex.Unlock()
	
	routeRegistry = nil
}