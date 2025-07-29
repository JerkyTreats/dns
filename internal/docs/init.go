package docs

import (
	"github.com/jerkytreats/dns/internal/api/types"
)

func init() {
	// Register Swagger UI endpoint
	types.RegisterRoute(types.RouteInfo{
		Method:       "GET",
		Path:         "/swagger",
		Handler:      nil, // Will be set during handler initialization
		RequestType:  nil, // GET request has no body
		ResponseType: nil, // Returns HTML, not JSON
		Module:       "docs",
		Summary:      "Swagger UI for API documentation",
	})

	// Register OpenAPI spec endpoint
	types.RegisterRoute(types.RouteInfo{
		Method:       "GET",
		Path:         "/docs/openapi.yaml",
		Handler:      nil, // Will be set during handler initialization
		RequestType:  nil, // GET request has no body
		ResponseType: nil, // Returns YAML, not JSON
		Module:       "docs",
		Summary:      "OpenAPI specification file",
	})

	// Register docs directory handler (for any additional static files)
	types.RegisterRoute(types.RouteInfo{
		Method:       "GET",
		Path:         "/docs",
		Handler:      nil, // Will be set during handler initialization
		RequestType:  nil, // GET request has no body
		ResponseType: nil, // Returns various file types
		Module:       "docs",
		Summary:      "Documentation static files",
	})
}