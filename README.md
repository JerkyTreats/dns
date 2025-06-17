# Tailscale Interal DNS Manager

A lightweight API for managing internal DNS records within a Tailscale network. This service provides a simple interface to manage DNS records for internal services, with CoreDNS integration for reliable DNS resolution.

## The Point

---

I'm experimenting with AI, and want to run local LLM. 

It's compute intensive, so I want to use my gaming PC. 

---

I want to run from mobile/laptop/etc, which means I need a Tailscale network. 

I don't like the `*.tailscale.ts.net` endpoints, I want to use `*.internal.jerkytreats.dev` 

---

So I need a CoreDNS server that is the NS for internal.jerkytreats.dev. 

Then I can build an API in front of the LLM at `llm.internal.jerkytreats.dev` 

But I am NOT going to manually add an entry to the CoreDNS server. It's a moral imperative as a platform engineer to automate domain resolution. 

So I build an API around the CoreDNS server to `add-record/` that will start resolving against `llm.internal.jerkytreats.dev` 

---

The best part is, the security is free because its all behind Tailscale. Only in-network devices can call thesee endpoints. 

And when I say "I built", I meant I "one-shot vibe coded it" over the course of an hour or two. 

So now the game is to build out as many of these little services as I can, create an "API Mesh" behind the TS network and see I can built a custom AI agent around it. 

It's all a bit nuts, but its been fun and I'm working _fast_.

## Features

- RESTful API for DNS record management
- CoreDNS integration for DNS resolution
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
