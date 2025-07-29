package analyzer

import (
	"fmt"
	"strings"

	"github.com/jerkytreats/dns/internal/api/types"
	"gopkg.in/yaml.v3"
)

// OpenAPISpec represents the complete OpenAPI 3.0 specification structure
type OpenAPISpec struct {
	OpenAPI    string                 `yaml:"openapi"`
	Info       Info                   `yaml:"info"`
	Servers    []Server               `yaml:"servers"`
	Paths      map[string]PathItem    `yaml:"paths"`
	Components Components             `yaml:"components"`
}

// Info contains API metadata
type Info struct {
	Title       string `yaml:"title"`
	Description string `yaml:"description"`
	Version     string `yaml:"version"`
}

// Server represents an API server
type Server struct {
	URL         string `yaml:"url"`
	Description string `yaml:"description"`
}

// PathItem describes operations available on a single path
type PathItem struct {
	Get    *Operation `yaml:"get,omitempty"`
	Post   *Operation `yaml:"post,omitempty"`
	Put    *Operation `yaml:"put,omitempty"`
	Delete *Operation `yaml:"delete,omitempty"`
}

// Operation describes a single API operation
type Operation struct {
	Tags        []string            `yaml:"tags,omitempty"`
	Summary     string              `yaml:"summary,omitempty"`
	Description string              `yaml:"description,omitempty"`
	OperationID string              `yaml:"operationId,omitempty"`
	RequestBody *RequestBody        `yaml:"requestBody,omitempty"`
	Responses   map[string]Response `yaml:"responses"`
}

// RequestBody describes the request body
type RequestBody struct {
	Description string                     `yaml:"description,omitempty"`
	Required    bool                       `yaml:"required,omitempty"`
	Content     map[string]MediaTypeObject `yaml:"content"`
}

// MediaTypeObject provides schema and examples for media type
type MediaTypeObject struct {
	Schema SchemaRef `yaml:"schema"`
}

// Response describes a single response
type Response struct {
	Description string                     `yaml:"description"`
	Content     map[string]MediaTypeObject `yaml:"content,omitempty"`
}

// SchemaRef is a reference to a schema
type SchemaRef struct {
	Ref string `yaml:"$ref,omitempty"`
}

// Components holds reusable objects for different aspects of the OAS
type Components struct {
	Schemas map[string]interface{} `yaml:"schemas"`
}

// buildOpenAPISpec builds the complete OpenAPI specification
func (g *Generator) buildOpenAPISpec() string {
	spec := OpenAPISpec{
		OpenAPI: "3.0.3",
		Info: Info{
			Title:       "DNS Manager API",
			Description: "Auto-generated API documentation for DNS Manager service with zero-maintenance updates",
			Version:     "1.0.0",
		},
		Servers: []Server{
			{
				URL:         "https://dns.internal.jerkytreats.dev",
				Description: "Production server",
			},
			{
				URL:         "http://localhost:8080",
				Description: "Development server",
			},
		},
		Paths:      g.buildPaths(),
		Components: Components{Schemas: g.typeSchemas},
	}

	// Convert to YAML
	yamlData, err := yaml.Marshal(spec)
	if err != nil {
		return fmt.Sprintf("# Error generating YAML: %v\n", err)
	}

	// Add header comment
	header := "# Auto-generated OpenAPI specification\n# DO NOT EDIT MANUALLY - Changes will be overwritten\n\n"
	
	return header + string(yamlData)
}

// buildPaths builds the paths section of the OpenAPI spec
func (g *Generator) buildPaths() map[string]PathItem {
	paths := make(map[string]PathItem)

	for _, route := range g.routes {
		pathItem, exists := paths[route.Path]
		if !exists {
			pathItem = PathItem{}
		}

		operation := g.buildOperation(route)
		
		switch strings.ToUpper(route.Method) {
		case "GET":
			pathItem.Get = operation
		case "POST":
			pathItem.Post = operation
		case "PUT":
			pathItem.Put = operation
		case "DELETE":
			pathItem.Delete = operation
		}

		paths[route.Path] = pathItem
	}

	return paths
}

// buildOperation builds an Operation from a RouteInfo
func (g *Generator) buildOperation(route types.RouteInfo) *Operation {
	operation := &Operation{
		Tags:        []string{route.Module},
		Summary:     route.Summary,
		OperationID: g.generateOperationID(route),
		Responses:   g.buildResponses(route),
	}

	// Add request body for non-GET methods
	if route.RequestType != nil && strings.ToUpper(route.Method) != "GET" {
		operation.RequestBody = g.buildRequestBody(route)
	}

	return operation
}

// generateOperationID generates a unique operation ID
func (g *Generator) generateOperationID(route types.RouteInfo) string {
	// Convert path to camelCase operation name
	pathParts := strings.Split(strings.Trim(route.Path, "/"), "/")
	var allParts []string
	
	// Process each path segment, splitting on hyphens too
	for _, pathPart := range pathParts {
		if pathPart == "" {
			continue
		}
		
		// Split each path part on hyphens
		hyphenParts := strings.Split(pathPart, "-")
		allParts = append(allParts, hyphenParts...)
	}
	
	var operationParts []string
	
	// Add method prefix
	switch strings.ToUpper(route.Method) {
	case "GET":
		if strings.Contains(route.Path, "list") {
			operationParts = append(operationParts, "list")
		} else {
			operationParts = append(operationParts, "get")
		}
	case "POST":
		if strings.Contains(route.Path, "add") || strings.Contains(route.Path, "create") {
			operationParts = append(operationParts, "create")
		} else {
			operationParts = append(operationParts, "post")
		}
	case "PUT":
		operationParts = append(operationParts, "update")
	case "DELETE":
		operationParts = append(operationParts, "delete")
	default:
		operationParts = append(operationParts, strings.ToLower(route.Method))
	}

	// Add path parts
	for i, part := range allParts {
		if part == "" {
			continue
		}
		if i == 0 {
			operationParts = append(operationParts, part)
		} else {
			operationParts = append(operationParts, strings.Title(part))
		}
	}

	return strings.Join(operationParts, "")
}

// buildRequestBody builds the request body specification
func (g *Generator) buildRequestBody(route types.RouteInfo) *RequestBody {
	typeName := g.getTypeName(route.RequestType)
	
	return &RequestBody{
		Description: fmt.Sprintf("Request body for %s", route.Summary),
		Required:    true,
		Content: map[string]MediaTypeObject{
			"application/json": {
				Schema: SchemaRef{
					Ref: fmt.Sprintf("#/components/schemas/%s", typeName),
				},
			},
		},
	}
}

// buildResponses builds the responses specification
func (g *Generator) buildResponses(route types.RouteInfo) map[string]Response {
	responses := make(map[string]Response)

	// Success response
	if route.ResponseType != nil {
		typeName := g.getTypeName(route.ResponseType)
		responses["200"] = Response{
			Description: "Success",
			Content: map[string]MediaTypeObject{
				"application/json": {
					Schema: SchemaRef{
						Ref: fmt.Sprintf("#/components/schemas/%s", typeName),
					},
				},
			},
		}
	} else {
		responses["200"] = Response{
			Description: "Success",
		}
	}

	// Standard error responses
	responses["400"] = Response{
		Description: "Bad Request",
		Content: map[string]MediaTypeObject{
			"application/json": {
				Schema: SchemaRef{
					Ref: "#/components/schemas/ErrorResponse",
				},
			},
		},
	}

	responses["500"] = Response{
		Description: "Internal Server Error",
		Content: map[string]MediaTypeObject{
			"application/json": {
				Schema: SchemaRef{
					Ref: "#/components/schemas/ErrorResponse",
				},
			},
		},
	}

	// Add method-specific responses
	if strings.ToUpper(route.Method) != "GET" {
		responses["422"] = Response{
			Description: "Unprocessable Entity",
			Content: map[string]MediaTypeObject{
				"application/json": {
					Schema: SchemaRef{
						Ref: "#/components/schemas/ErrorResponse",
					},
				},
			},
		}
	}

	return responses
}