# Development Guide

This guide provides instructions for setting up a local development environment for the DNS Manager service.

## Environment Setup

1.  **Prerequisites**
    - Go 1.24 or later
    - Docker and Docker Compose
    - Git

2.  **Local Setup**
    ```bash
    git clone https://github.com/jerkytreats/dns.git
    cd dns
    go mod download
    ```

3.  **Configuration**
    - The application is configured using `configs/config.yaml`.
    - For local development, you can use the default settings, which use Let's Encrypt's staging environment.
    - You must provide a valid email and a domain you control in the `certificate` section for the DNS-01 challenge to work.

## Running Locally

The recommended way to run the services for local development is with `docker-compose`.

```bash
# Build and start all services
docker-compose up --build

# To view logs
docker-compose logs -f

# To stop services
docker-compose down
```
The API server will be available at `http://localhost:8080` (or `https://localhost:8443` if TLS is enabled).

## Testing

- **Run all tests:**
  ```bash
  go test ./...
  ```
- **Run tests with coverage:**
  ```bash
  go test ./... -cover
  ```
- **Test Structure**: Unit tests are co-located with the code they test (e.g., `manager_test.go` is in the same package as `manager.go`).

## API Development

- **Handlers**: API handlers are located in `internal/api/handler`.
- **Routing**: Routes are defined in `cmd/api/main.go`.
- When adding a new endpoint, you will need to create a handler function and add it to the router in `main.go`.

## CoreDNS Integration

- **Zone Files**: The DNS zone files are in `configs/coredns/zones/`. The main zone file is `internal.jerkytreats.dev.db`.
- **Dynamic Zones**: Zone files for ACME challenges (e.g., `_acme-challenge.internal.jerkytreats.dev.zone`) are created dynamically by the application.
- **CoreDNS Configuration**: The main CoreDNS configuration is in `configs/coredns/Corefile`.

## Troubleshooting

- **CoreDNS Errors**: Check the CoreDNS logs with `docker-compose logs coredns`. Ensure that the zone files have the correct permissions and that the `Corefile` syntax is valid.
- **API Errors**: Check the API logs with `docker-compose logs api`.
- **Enable Debug Logging**: To get more detailed logs, set `logging.level` to `debug` in `configs/config.yaml`.

## Contributing

1. Follow Go code style guidelines
2. Write tests for new features
3. Update documentation
4. Submit PR with clear description

## Resources

- [Go Documentation](https://golang.org/doc/)
- [CoreDNS Documentation](https://coredns.io/manual/)
- [OpenAPI Specification](https://swagger.io/specification/)
