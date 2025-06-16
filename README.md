# DNS Manager API

A lightweight API for managing internal DNS records within a Tailscale network. This service provides a simple interface to manage DNS records for internal services, with CoreDNS integration for reliable DNS resolution.

## Features

- RESTful API for DNS record management
- CoreDNS integration for reliable DNS resolution
- Docker-based deployment
- OpenAPI/Swagger documentation
- Health monitoring endpoints
- Comprehensive test coverage

## Prerequisites

- Go 1.24 or later
- Docker and Docker Compose
- Make (optional, for using Makefile commands)

## Quick Start

1. Clone the repository:
   ```bash
   git clone https://github.com/jerkytreats/dns.git
   cd dns
   ```

2. Deploy using the provided script:
   ```bash
   ./scripts/deploy.sh
   ```

The API will be available at `http://localhost:8080` and CoreDNS at `localhost:53`.

## API Endpoints

### Health Check
```http
GET /health
```
Returns the health status of the API and CoreDNS services.

### Add DNS Record
```http
POST /add-record
Content-Type: application/json

{
    "service_name": "my-service"
}
```
Creates a new DNS record for the specified service.

## Configuration

The service is configured through `configs/config.yaml`. Key configuration options:

```yaml
app:
  name: dns-manager
  version: 1.0.0
  environment: development

server:
  host: 0.0.0.0
  port: 8080
  read_timeout: 5s
  write_timeout: 10s
  idle_timeout: 120s

dns:
  domain: internal.jerkytreats.dev
  coredns:
    config_path: /etc/coredns/Corefile
    zones_path: /zones
    reload_command: ["kill", "-SIGUSR1", "1"]
```

## Development

### Local Development

1. Install dependencies:
   ```bash
   go mod download
   ```

2. Run tests:
   ```bash
   go test ./...
   ```

3. Start services with hot-reload:
   ```bash
   docker-compose up
   ```

### Project Structure

```
.
├── cmd/                    # Application entry points
├── configs/               # Configuration files
├── docs/                  # Documentation
├── internal/              # Internal packages
│   ├── api/              # API handlers and middleware
│   └── dns/              # DNS management logic
├── scripts/              # Utility scripts
└── coredns/              # CoreDNS configuration
```

## Docker Deployment

The service is containerized using Docker:

- `Dockerfile.api`: Multi-stage build for the API server
- `Dockerfile`: CoreDNS configuration
- `docker-compose.yml`: Local development setup

### Building Images

```bash
docker-compose build
```

### Running Services

```bash
docker-compose up -d
```

## API Documentation

API documentation is available through Swagger UI at `/swagger` when the service is running.

## Contributing

1. Fork the repository
2. Create a feature branch
3. Commit your changes
4. Push to the branch
5. Create a Pull Request

## License

This project is licensed under the MIT License - see the LICENSE file for details.
