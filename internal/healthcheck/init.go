package healthcheck

import (
	"reflect"

	"github.com/jerkytreats/dns/internal/api/types"
)

func init() {
	// Register health check endpoint
	types.RegisterRoute(types.RouteInfo{
		Method:       "GET",
		Path:         "/health",
		Handler:      nil, // Will be set during handler initialization
		RequestType:  nil, // GET request has no body
		ResponseType: reflect.TypeOf(HealthResponse{}),
		Module:       "healthcheck",
		Summary:      "Check the health status of the API and its dependencies",
	})
}