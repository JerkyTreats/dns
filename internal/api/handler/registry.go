package handler

import (
	"github.com/jerkytreats/dns/internal/api/types"
)

// RegisterRoute is a convenience wrapper around types.RegisterRoute
func RegisterRoute(route types.RouteInfo) {
	types.RegisterRoute(route)
}

// GetRegisteredRoutes is a convenience wrapper around types.GetRegisteredRoutes
func GetRegisteredRoutes() []types.RouteInfo {
	return types.GetRegisteredRoutes()
}

// ClearRegistry is a convenience wrapper around types.ClearRegistry
func ClearRegistry() {
	types.ClearRegistry()
}