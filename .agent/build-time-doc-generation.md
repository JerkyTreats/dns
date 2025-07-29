# Build-Time Documentation Generation Specification

## Overview

The DNS Manager implements zero-maintenance OpenAPI documentation through automated build-time generation. This system ensures API documentation is always current with zero developer effort by analyzing Go code at build time and auto-generating OpenAPI specifications.

## Architecture

### Decentralized Route Registration

**Problem:** Current `HandlerRegistry` violates separation of concerns by knowing about module-specific routes (e.g., `/list-devices` from Tailscale module).

**Solution:** Module-based route registration pattern, similar to the existing config system.

### Core Components

**1. Modular Route Registry**
- Modules self-register routes using `RouteInfo` structs
- Central registry collects all routes at initialization
- Eliminates coupling between API layer and module internals

**2. Route Discovery Engine**  
- Analyzes all `RegisterRoute()` calls across codebase using Go AST
- Extracts `RouteInfo` metadata from module registrations
- Builds complete route inventory from decentralized sources

**3. Type Schema Generator**
- Uses `RouteInfo.RequestType` and `ResponseType` for schema generation
- Generates JSON schemas from reflection metadata
- Handles nested types, slices, and custom validation tags

**4. OpenAPI Spec Builder**
- Consumes centralized route registry as single source of truth
- Groups routes by module for organized documentation
- Generates complete OpenAPI 3.0 specification

### Automation Flow

```
Code Change → GitHub Action → AST Analysis → Type Introspection → Spec Generation → Git Commit
```

## Implementation Design

### Generator Tool Structure

```
cmd/generate-openapi/
├── main.go              # Entry point and orchestration
├── analyzer/
│   ├── handler.go       # Handler discovery and route extraction
│   ├── types.go         # Type reflection and schema generation
│   └── spec.go          # OpenAPI spec building
└── templates/
    └── openapi.tmpl     # OpenAPI spec template
```

### Modular Route Registration Pattern

**RouteInfo Structure:**
```go
type RouteInfo struct {
    Method       string           // HTTP method (GET, POST, etc.)
    Path         string           // Route path (/list-devices)  
    Handler      http.HandlerFunc // Handler function
    RequestType  reflect.Type     // Request body type (nil for GET)
    ResponseType reflect.Type     // Success response type
    Module       string           // Module name for documentation grouping
    Summary      string           // Optional operation summary
}
```

**Module Self-Registration:**
```go
// internal/tailscale/handler/init.go
func init() {
    handler.RegisterRoute(handler.RouteInfo{
        Method: "GET", 
        Path: "/list-devices",
        Handler: deviceHandler.ListDevices,
        ResponseType: reflect.TypeOf([]tailscale.Device{}),
        Module: "tailscale",
        Summary: "List all Tailscale devices",
    })
}
```

**Central Registry Collection:**
```go
// internal/api/handler/registry.go
var routeRegistry []RouteInfo

func RegisterRoute(route RouteInfo) {
    routeRegistry = append(routeRegistry, route)
}

func (hr *HandlerRegistry) RegisterHandlers(mux *http.ServeMux) {
    for _, route := range routeRegistry {
        mux.HandleFunc(route.Path, route.Handler)
    }
}
```

### Type Introspection Rules

**Request Types:**
- Functions with `*http.Request` parameter and JSON decoder usage
- Struct analysis for body schema generation
- Validation tag support (`json`, `validate`)

**Response Types:**
- Functions with `json.NewEncoder(w).Encode()` calls
- Return type analysis for response schemas
- HTTP status code detection from `w.WriteHeader()` calls

### Zero-Touch Workflow

**Developer Experience:**
1. **Add new handler:** `func (h *FooHandler) CreateFoo(w http.ResponseWriter, r *http.Request)`
2. **Register in module init():** 
   ```go
   func init() {
       handler.RegisterRoute(handler.RouteInfo{
           Method: "POST", Path: "/foo",
           Handler: fooHandler.CreateFoo,
           RequestType: reflect.TypeOf(CreateFooRequest{}),
           ResponseType: reflect.TypeOf(CreateFooResponse{}),
           Module: "foo",
       })
   }
   ```
3. **Push to repository**
4. **GitHub Action automatically updates OpenAPI spec**

**No Manual Steps Required:**
- No central handler registry updates
- No spec file editing  
- No documentation maintenance
- Modules manage their own routes

## GitHub Actions Integration

### Workflow Enhancement

**Addition to `.github/workflows/docker-publish.yml`:**

```yaml
# After existing test steps (line 52)
- name: Generate OpenAPI Documentation
  run: go run cmd/generate-openapi/main.go

- name: Commit Updated Spec
  run: |
    if ! git diff --quiet docs/api/openapi.yaml; then
      git config --local user.email "action@github.com"
      git config --local user.name "OpenAPI Generator"
      git add docs/api/openapi.yaml
      git commit -m "Auto-update OpenAPI spec [skip ci]"
      git push
    fi
```

### Build Integration

**Dependency Management:**
- Generator runs before tests to ensure spec validity
- Spec validation as part of CI pipeline
- Automatic rollback if generation fails

## Route Discovery and Documentation Generation

### Generator Analysis Process

**Build-Time Route Discovery:**
1. **Scan for RegisterRoute() calls** across all Go files using AST parsing
2. **Extract RouteInfo metadata** from each module's registration  
3. **Build complete route inventory** from distributed sources
4. **Generate OpenAPI spec** from centralized route registry

**Example Analysis Flow:**
```go
// Generator finds this in tailscale/handler/init.go:
handler.RegisterRoute(handler.RouteInfo{
    Method: "GET", 
    Path: "/list-devices",
    ResponseType: reflect.TypeOf([]tailscale.Device{}),
    Module: "tailscale",
    Summary: "List all Tailscale devices",
})

// Generates this OpenAPI specification:
"/list-devices": {
  "get": {
    "tags": ["tailscale"],
    "summary": "List all Tailscale devices", 
    "responses": {
      "200": {
        "description": "Success",
        "content": {
          "application/json": {
            "schema": {"$ref": "#/components/schemas/TailscaleDeviceArray"}
          }
        }
      }
    }
  }
}
```

### Type Convention Rules

**Request Types:**
- Must be JSON-decodable structs
- Located in handler function parameters
- Validation through struct tags

**Response Types:**
- Identified through `json.NewEncoder` usage
- HTTP status codes from `WriteHeader` calls
- Error types from `http.Error` calls

## Generated Documentation Features

### Automatic Schema Generation

**Request/Response Models:**
- Complete JSON schemas for all types
- Validation rules from struct tags
- Example values from field tags

**API Metadata:**
- Operation summaries from function names
- Parameter descriptions from struct field tags
- HTTP status code documentation

### Integration with Existing Swagger Config

**Leverages `configs/swagger.yaml`:**
- UI theme and configuration
- Security scheme definitions
- Server information and base URLs

## Maintenance and Updates

### Zero-Maintenance Promise

**Automatic Updates:**
- New handlers automatically documented
- Type changes reflected immediately
- Route modifications tracked automatically

**Version Control Integration:**
- Spec changes tracked in Git history
- Automated commit messages for traceability
- No merge conflicts on documentation

### Error Handling

**Generation Failures:**
- CI pipeline fails if spec generation errors
- Clear error messages for resolution
- Fallback to previous valid specification

## Benefits

### Developer Experience
- **Zero Documentation Debt:** Always current documentation
- **No Context Switching:** No manual spec editing required
- **Type Safety:** Compile-time validation of API contracts

### API Consumers
- **Always Accurate:** Documentation matches implementation exactly
- **Rich Schemas:** Complete type information for client generation
- **Interactive Testing:** Swagger UI for live API exploration

### Maintenance
- **Automated Workflow:** Fits existing CI/CD pipeline
- **Git Integration:** Documentation changes tracked with code
- **Version Consistency:** Spec version matches code version

## Future Enhancements

### Advanced Features
- Request/response example generation
- OpenAPI validation middleware integration
- Client SDK auto-generation from specs

### Tool Evolution
- Custom annotation support for enhanced metadata
- Multi-version API documentation
- Performance optimization for large codebases

This specification enables truly zero-maintenance API documentation where adding a new handler automatically generates complete, accurate OpenAPI documentation without any developer intervention.