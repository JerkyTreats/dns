# Lightweight API Specification

## Overview

The DNS Manager API provides endpoints for managing internal DNS records within a Tailscale network. The API is designed to be lightweight and focused on specific DNS management tasks.

## API Request Format

```json
{
  "service": "dns-manager",
  "tasks": [
    {
      "name": "AddInternalTSRecord",
      "description": "Adds a service to internal.jerkytreats.dev",
      "parameters": [
        { "service_name" : "new-service" }
      ],
      "hostname": "dns.internal.jerkytreats.dev",
      "endpoint" : "add-record",
      "method" : "POST"
    }
  ]
}
```

This translates to a POST request to `dns.internal.jerkytreats.dev` endpoint `/add-record` with the associated parameters.

## Project Structure

```
.
├── .agent/                    # Agent configuration and specs
├── .github/                   # GitHub workflows and templates
├── cmd/
│   └── api/                   # API server entry point
│       └── main.go
├── configs/
│   ├── config.yaml           # Application configuration
│   └── coredns/              # CoreDNS specific configs
│       └── zones/            # DNS zone files
├── docs/
│   └── api/                  # API documentation
│       └── openapi.yaml
├── internal/
│   ├── api/
│   │   ├── handler/         # HTTP handlers
│   │   ├── middleware/      # HTTP middleware
│   │   └── server/          # Server setup
│   └── dns/
│       ├── controller/      # DNS controller logic
│       ├── coredns/         # CoreDNS integration
│       └── zone/            # Zone management
├── .gitignore
├── Dockerfile              # CoreDNS Dockerfile
├── Dockerfile.api         # API server Dockerfile
├── docker-compose.yml     # Local development setup
├── go.mod
├── go.sum
└── README.md
```

## Implementation Requirements

### 1. API Server
- Implement using standard library or lightweight framework
- Endpoint: `/add-record`
- Request validation:
  - Required field: `service_name`
  - Validate service name format
- Response format:
  ```json
  {
    "status": "success|error",
    "message": "Operation result message",
    "data": {
      "hostname": "new-service.internal.jerkytreats.dev"
    }
  }
  ```

### 2. DNS Controller
- CoreDNS integration:
  - Zone file management
  - Configuration updates
  - Service restart handling
- Operations:
  - Create new zone for service
  - Update CoreDNS configuration
  - Trigger CoreDNS reload/restart
- Error handling and rollback capabilities

### 3. Docker Setup
- Create `Dockerfile.api`:
  - Multi-stage build
  - Minimal base image
  - Proper security considerations
- Docker Compose for local development
- Networking configuration between API and CoreDNS

## API Specification

### OpenAPI Specification
```yaml
openapi: 3.0.3
info:
  title: DNS Manager API
  description: API for managing internal DNS records
  version: 1.0.0
servers:
  - url: https://dns.internal.jerkytreats.dev
    description: Production server
paths:
  /health:
    get:
      summary: Health check endpoint
      description: Returns the health status of the API and its dependencies
      operationId: healthCheck
      responses:
        '200':
          description: Service is healthy
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/HealthResponse'
        '503':
          description: Service is unhealthy
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/HealthResponse'
  /add-record:
    post:
      summary: Add a new internal DNS record
      description: Creates a new DNS record for a service in the internal domain
      operationId: addRecord
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/AddRecordRequest'
      responses:
        '200':
          description: Record added successfully
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/AddRecordResponse'
        '400':
          description: Invalid request
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/ErrorResponse'
        '500':
          description: Internal server error
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/ErrorResponse'
components:
  schemas:
    HealthResponse:
      type: object
      required:
        - status
        - version
        - components
      properties:
        status:
          type: string
          enum: [healthy, unhealthy]
          description: Overall health status
        version:
          type: string
          description: API version
        components:
          type: object
          properties:
            api:
              type: object
              properties:
                status:
                  type: string
                  enum: [healthy, unhealthy]
                message:
                  type: string
            coredns:
              type: object
              properties:
                status:
                  type: string
                  enum: [healthy, unhealthy]
                message:
                  type: string
    AddRecordRequest:
      type: object
      required:
        - service_name
      properties:
        service_name:
          type: string
          pattern: '^[a-z0-9-]+$'
          minLength: 1
          maxLength: 63
          description: Name of the service to add
          example: "new-service"
    AddRecordResponse:
      type: object
      required:
        - status
        - message
        - data
      properties:
        status:
          type: string
          enum: [success, error]
        message:
          type: string
          description: Operation result message
        data:
          type: object
          properties:
            hostname:
              type: string
              format: hostname
              example: "new-service.internal.jerkytreats.dev"
    ErrorResponse:
      type: object
      required:
        - status
        - message
      properties:
        status:
          type: string
          enum: [error]
        message:
          type: string
          description: Error message
        error_code:
          type: string
          description: Machine-readable error code
```

### Swagger UI Configuration
```yaml
# configs/swagger.yaml
swagger:
  enabled: true
  path: /swagger
  spec_path: /docs/openapi.yaml
  security:
    enabled: true
    type: basic
  ui:
    title: DNS Manager API Documentation
    theme: dark
    doc_expansion: list
    default_models_expand_depth: 3
    default_model_expand_depth: 3
    display_request_duration: true
    filter: true
    try_it_out_enabled: true
```

## Implementation Status

| Component         | Task                        | Status   | Notes                                      |
|-------------------|----------------------------|----------|--------------------------------------------|
| **Project Setup** | Initialize project structure| ✅ DONE  | go-template structure in place             |
|                   | Set up Go module            | ✅ DONE  | go.mod updated, dependencies managed       |
|                   | Configure build scripts     | ✅ DONE  | Standard Go build/test in place            |
| **API Server**    | Basic HTTP server           | ✅ DONE  | `cmd/api/main.go`                          |
|                   | Health check endpoint       | ✅ DONE  | `/health` endpoint, tested                 |
|                   | Add record endpoint         | ✅ DONE  | `/add-record` endpoint, tested             |
|                   | Request validation          | ✅ DONE  | Regex and struct validation, tested        |
|                   | Error handling              | ✅ DONE  | Consistent error responses, tested         |
|                   | Unit tests                  | ✅ DONE  | Full handler test coverage                 |
| **DNS Controller**| CoreDNS integration         | ✅ DONE  | `internal/dns/coredns/manager.go`          |
|                   | Zone file management        | ✅ DONE  | Add/remove zone, tested                    |
|                   | Configuration updates       | ✅ DONE  | CoreDNS config block add/remove, tested    |
|                   | Service restart handling    | ✅ DONE  | Reload logic, tested (mocked in tests)     |
|                   | Unit tests                  | ✅ DONE  | Full manager test coverage                 |
| **Docker Setup**  | API Dockerfile              | ⏳ TODO  | Next step                                  |
|                   | Docker Compose              | ⏳ TODO  | Next step                                  |
|                   | Networking config           | ⏳ TODO  | Next step                                  |
| **Documentation** | OpenAPI spec                | ✅ DONE  | `docs/api/openapi.yaml`                    |
|                   | Swagger UI                  | ⏳ TODO  | To be set up                               |
|                   | README updates              | ⏳ TODO  | To be updated after Docker setup           |

---

**Summary:**
- All Go code, API endpoints, validation, and DNS controller logic are implemented and fully tested.
- Next steps: Dockerize the API, set up docker-compose, and finalize documentation.
