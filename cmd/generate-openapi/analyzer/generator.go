package analyzer

import (
	"fmt"
	"go/parser"
	"go/token"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/jerkytreats/dns/internal/api/types"
)

// Generator handles the generation of OpenAPI specifications from Go code
type Generator struct {
	fileSet      *token.FileSet
	routes       []types.RouteInfo
	typeSchemas  map[string]interface{}
}

// NewGenerator creates a new OpenAPI generator
func NewGenerator() *Generator {
	return &Generator{
		fileSet:     token.NewFileSet(),
		typeSchemas: make(map[string]interface{}),
	}
}

// GenerateSpec generates a complete OpenAPI specification
func (g *Generator) GenerateSpec() (string, error) {
	// Force import of modules to trigger init() functions
	if err := g.discoverRoutes(); err != nil {
		return "", fmt.Errorf("failed to discover routes: %w", err)
	}

	// Get routes from the registry (populated by init() functions)
	g.routes = types.GetRegisteredRoutes()
	
	if len(g.routes) == 0 {
		return "", fmt.Errorf("no routes discovered in registry")
	}

	// Generate type schemas
	if err := g.generateSchemas(); err != nil {
		return "", fmt.Errorf("failed to generate schemas: %w", err)
	}
	
	// Add standard schemas
	g.addStandardSchemas()

	// Build the OpenAPI spec
	spec := g.buildOpenAPISpec()
	
	return spec, nil
}

// discoverRoutes scans the codebase for init() functions that register routes
func (g *Generator) discoverRoutes() error {
	// Parse Go files to trigger module loading and init() functions
	packageDirs := []string{
		"internal/api/handler",
		"internal/tailscale/handler", 
		"internal/healthcheck",
	}

	for _, dir := range packageDirs {
		if err := g.parsePackageDir(dir); err != nil {
			// Log warning but continue - some packages might not exist
			fmt.Printf("Warning: failed to parse package %s: %v\n", dir, err)
		}
	}

	return nil
}

// parsePackageDir parses all Go files in a directory
func (g *Generator) parsePackageDir(dir string) error {
	pattern := filepath.Join(dir, "*.go")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return fmt.Errorf("failed to glob pattern %s: %w", pattern, err)
	}

	for _, file := range matches {
		if strings.HasSuffix(file, "_test.go") {
			continue // Skip test files
		}

		_, err := parser.ParseFile(g.fileSet, file, nil, parser.ParseComments)
		if err != nil {
			// Log warning but continue
			fmt.Printf("Warning: failed to parse file %s: %v\n", file, err)
		}
	}

	return nil
}

// generateSchemas generates JSON schemas for request/response types
func (g *Generator) generateSchemas() error {
	for _, route := range g.routes {
		if route.RequestType != nil {
			schema, err := g.generateTypeSchema(route.RequestType)
			if err != nil {
				return fmt.Errorf("failed to generate schema for request type %v: %w", route.RequestType, err)
			}
			g.typeSchemas[g.getTypeName(route.RequestType)] = schema
		}

		if route.ResponseType != nil {
			schema, err := g.generateTypeSchema(route.ResponseType)
			if err != nil {
				return fmt.Errorf("failed to generate schema for response type %v: %w", route.ResponseType, err)
			}
			g.typeSchemas[g.getTypeName(route.ResponseType)] = schema
		}
	}

	return nil
}

// generateTypeSchema generates a JSON schema for a Go type using reflection
func (g *Generator) generateTypeSchema(t reflect.Type) (map[string]interface{}, error) {
	return g.generateSchemaForType(t, make(map[reflect.Type]bool))
}

// generateSchemaForType recursively generates schema, handling circular references
func (g *Generator) generateSchemaForType(t reflect.Type, visited map[reflect.Type]bool) (map[string]interface{}, error) {
	// Dereference pointers first
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	// Handle primitive types immediately (no circular reference issues)
	switch t.Kind() {
	case reflect.String:
		return map[string]interface{}{"type": "string"}, nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return map[string]interface{}{"type": "integer"}, nil
	case reflect.Float32, reflect.Float64:
		return map[string]interface{}{"type": "number"}, nil
	case reflect.Bool:
		return map[string]interface{}{"type": "boolean"}, nil
	}

	// Handle circular references for complex types only
	if visited == nil {
		visited = make(map[reflect.Type]bool)
	}
	
	if visited[t] {
		return map[string]interface{}{
			"type": "object",
			"description": fmt.Sprintf("Circular reference to %s", t.String()),
		}, nil
	}
	visited[t] = true

	switch t.Kind() {
	case reflect.Struct:
		// Special handling for time.Time
		if t.PkgPath() == "time" && t.Name() == "Time" {
			return map[string]interface{}{
				"type": "string",
				"format": "date-time",
			}, nil
		}
		return g.generateStructSchema(t, visited)
	case reflect.Slice, reflect.Array:
		elemSchema, err := g.generateSchemaForType(t.Elem(), visited)
		if err != nil {
			return nil, err
		}
		return map[string]interface{}{
			"type":  "array",
			"items": elemSchema,
		}, nil
	case reflect.Map:
		return map[string]interface{}{
			"type": "object",
			"additionalProperties": true,
		}, nil
	case reflect.Interface:
		return map[string]interface{}{
			"type": "object",
			"additionalProperties": true,
		}, nil
	default:
		return map[string]interface{}{
			"type": "string",
			"description": fmt.Sprintf("Unsupported type: %s", t.Kind()),
		}, nil
	}
}

// generateStructSchema generates a schema for a struct type
func (g *Generator) generateStructSchema(t reflect.Type, visited map[reflect.Type]bool) (map[string]interface{}, error) {
	properties := make(map[string]interface{})
	required := []string{}

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		
		// Skip unexported fields
		if !field.IsExported() {
			continue
		}

		jsonTag := field.Tag.Get("json")
		if jsonTag == "-" {
			continue // Skip fields marked with json:"-"
		}

		fieldName := field.Name
		if jsonTag != "" {
			// Parse json tag (e.g., "field_name,omitempty")
			parts := strings.Split(jsonTag, ",")
			if parts[0] != "" {
				fieldName = parts[0]
			}
			
			// Check if field is optional (has omitempty)
			omitempty := false
			for _, part := range parts[1:] {
				if part == "omitempty" {
					omitempty = true
					break
				}
			}
			
			if !omitempty {
				required = append(required, fieldName)
			}
		} else {
			// No json tag, field is required by default
			required = append(required, fieldName)
		}

		fieldSchema, err := g.generateSchemaForType(field.Type, visited)
		if err != nil {
			return nil, fmt.Errorf("failed to generate schema for field %s: %w", field.Name, err)
		}

		properties[fieldName] = fieldSchema
	}

	schema := map[string]interface{}{
		"type":       "object",
		"properties": properties,
	}

	if len(required) > 0 {
		schema["required"] = required
	}

	return schema, nil
}

// getTypeName returns a clean name for a type to use as a schema reference
func (g *Generator) getTypeName(t reflect.Type) string {
	// Handle array/slice types first
	if t.Kind() == reflect.Slice || t.Kind() == reflect.Array {
		elemType := t.Elem()
		elemName := g.getTypeName(elemType)
		return elemName + "Array"
	}
	
	// Remove package path, keep only the type name
	name := t.String()
	if lastDot := strings.LastIndex(name, "."); lastDot != -1 {
		name = name[lastDot+1:]
	}
	
	return name
}

// addStandardSchemas adds common schemas used across all APIs
func (g *Generator) addStandardSchemas() {
	// Standard error response schema
	g.typeSchemas["ErrorResponse"] = map[string]interface{}{
		"type": "object",
		"required": []string{"error", "message", "status"},
		"properties": map[string]interface{}{
			"error": map[string]interface{}{
				"type": "boolean",
				"description": "Indicates this is an error response",
			},
			"message": map[string]interface{}{
				"type": "string",
				"description": "Human-readable error message",
			},
			"status": map[string]interface{}{
				"type": "integer",
				"description": "HTTP status code",
			},
		},
	}
}

// GetDiscoveredRoutes returns the routes discovered by the generator
func (g *Generator) GetDiscoveredRoutes() []types.RouteInfo {
	return g.routes
}