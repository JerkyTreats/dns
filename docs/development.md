# Development Guide

## Development Environment Setup

1. **Prerequisites**
   - Go 1.24 or later
   - Docker and Docker Compose
   - Git

2. **Local Setup**
   ```bash
   # Clone the repository
   git clone https://github.com/jerkytreats/dns.git
   cd dns

   # Install dependencies
   go mod download
   ```

3. **Configuration**
   - Copy `configs/config.yaml.example` to `configs/config.yaml`
   - Update configuration values as needed
   - Ensure CoreDNS paths are correctly set

## Running Locally

### Using Docker Compose (Recommended)

```bash
# Start all services
docker-compose up -d

# View logs
docker-compose logs -f

# Stop services
docker-compose down
```

### Manual Development

1. **Start CoreDNS**
   ```bash
   docker run -d \
     --name coredns \
     -p 53:53/udp \
     -v $(pwd)/coredns:/etc/coredns \
     coredns/coredns:1.11.1
   ```

2. **Run API Server**
   ```bash
   go run cmd/api/main.go
   ```

## Testing

### Running Tests

```bash
# Run all tests
go test ./...

# Run tests with coverage
go test ./... -cover

# Run specific package tests
go test ./internal/api/handler/...
```

### Test Structure

- Unit tests are co-located with the code they test
- Integration tests are in `internal/tests`
- End-to-end tests are in `tests/e2e`

## API Development

### Adding New Endpoints

1. Update OpenAPI specification in `docs/api/openapi.yaml`
2. Create handler in `internal/api/handler`
3. Add tests in corresponding `_test.go` file
4. Update main.go to wire up the new endpoint

### Example: Adding a New Endpoint

```go
// internal/api/handler/example.go
func NewExampleHandler(logger *zap.Logger) *ExampleHandler {
    return &ExampleHandler{logger: logger}
}

func (h *ExampleHandler) Handle(w http.ResponseWriter, r *http.Request) {
    // Implementation
}

// cmd/api/main.go
mux.HandleFunc("/example", exampleHandler.Handle)
```

## CoreDNS Integration

### Zone Management

- Zone files are stored in `coredns/zones/`
- CoreDNS configuration is in `coredns/Corefile`
- The API server manages zones through the CoreDNS manager

### Adding a New Zone

```go
manager := coredns.NewManager(logger, configPath, zonesPath, reloadCmd)
err := manager.AddZone("new-service")
```

## Deployment

### Building for Production

```bash
# Build API server
docker build -t dns-manager-api -f Dockerfile.api .

# Build CoreDNS
docker build -t dns-manager-coredns .
```

### Production Configuration

1. Update `configs/config.yaml` for production
2. Set appropriate environment variables
3. Configure logging for production
4. Set up monitoring and alerts

## Troubleshooting

### Common Issues

1. **CoreDNS not responding**
   - Check CoreDNS logs: `docker-compose logs coredns`
   - Verify zone files exist
   - Check CoreDNS configuration

2. **API server errors**
   - Check API logs: `docker-compose logs api`
   - Verify configuration
   - Check CoreDNS connectivity

### Debugging

1. **Enable Debug Logging**
   ```yaml
   # configs/config.yaml
   logging:
     level: debug
     format: json
   ```

2. **CoreDNS Debug Mode**
   ```yaml
   # coredns/Corefile
   . {
       debug
       log
   }
   ```

## Contributing

1. Follow Go code style guidelines
2. Write tests for new features
3. Update documentation
4. Submit PR with clear description

## Resources

- [Go Documentation](https://golang.org/doc/)
- [CoreDNS Documentation](https://coredns.io/manual/)
- [OpenAPI Specification](https://swagger.io/specification/)
