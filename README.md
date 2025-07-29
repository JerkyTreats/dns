# DNS Manager

A comprehensive DNS management solution for Tailscale networks, providing automated DNS record management, SSL certificates, and reverse proxy configuration in a single unified container.

## Features

### 🎯 Core DNS Management
- **Dynamic DNS Records**: Create and manage DNS records with automatic zone updates
- **CoreDNS Integration**: Robust DNS server with health monitoring
- **Zone Management**: Template-based zone files with automatic serial updates
- **DNS-over-TLS**: Secure DNS with SSL/TLS support

### 🔗 Tailscale Integration
- **Device Discovery**: Automatic discovery and sync of Tailscale devices
- **IP Resolution**: Maps device names to Tailscale IPs (100.64.x.x range)
- **Device Annotations**: Custom metadata and descriptions for devices
- **Polling Sync**: Configurable automatic device synchronization

### 🔄 Reverse Proxy Automation
- **Caddy Integration**: Automatic proxy rules for services with ports
- **SSL Termination**: Automatic HTTPS for proxied services
- **Dynamic Configuration**: Real-time proxy updates via Caddy's admin API
- **Service Discovery**: Auto-proxy setup for DNS records with ports

### 🔐 Security & Certificates
- **Let's Encrypt Integration**: Automatic SSL certificate provisioning and renewal
- **DNS-01 Challenge**: Cloudflare DNS challenge support for wildcard certificates
- **SAN Management**: Subject Alternative Names for multi-domain certificates
- **Firewall Integration**: Automatic IPSet management for Tailscale CIDR ranges
- **TLS Configuration**: Customizable TLS versions and cipher suites

### 🌐 API & Documentation
- **REST API**: Complete HTTP API for DNS and device management
- **OpenAPI/Swagger**: Auto-generated interactive documentation
- **Route Registry**: Centralized route management
- **Input Validation**: Request validation and error handling

## Architecture

The DNS Manager runs as a unified container using supervisord to manage multiple services:

- **API Service**: Go-based REST API for DNS and device management
- **CoreDNS**: DNS server with health monitoring and TLS support
- **Caddy**: Reverse proxy with automatic SSL termination
- **Certificate Manager**: Background SSL certificate provisioning and renewal

## Quick Start

### Docker Compose

```yaml
version: '3.8'

services:
  dns-manager:
    build:
      context: .
      dockerfile: Dockerfile.all
    ports:
      - "8080:8080"    # API HTTP
      - "8443:8443"    # API HTTPS
      - "53:53/udp"    # DNS
      - "53:53/tcp"    # DNS
      - "853:853/tcp"  # DNS-over-TLS
      - "80:80"        # Caddy HTTP
      - "2019:2019"    # Caddy Admin
    volumes:
      - ./data:/app/data
      - ./configs:/app/configs
      - ./ssl:/etc/letsencrypt
    environment:
      - TAILSCALE_API_KEY=your_api_key
      - TAILSCALE_TAILNET=your_tailnet
      - INTERNAL_DOMAIN=internal.yourdomain.com
      - CLOUDFLARE_API_TOKEN=your_cloudflare_token
    privileged: true  # Required for firewall management
    cap_add:
      - NET_ADMIN
      - NET_RAW
    restart: unless-stopped
```

### Environment Variables

| Variable | Description | Required |
|----------|-------------|----------|
| `TAILSCALE_API_KEY` | Tailscale API key for device discovery | Yes |
| `TAILSCALE_TAILNET` | Your Tailscale tailnet name | Yes |
| `INTERNAL_DOMAIN` | Internal domain for DNS records | Yes |
| `CLOUDFLARE_API_TOKEN` | Cloudflare API token for DNS challenges | Yes |
| `LETSENCRYPT_EMAIL` | Email for Let's Encrypt registration | Yes |
| `SERVER_PORT` | API server port (default: 8080) | No |
| `USE_PRODUCTION_CERTS` | Use production Let's Encrypt (default: false) | No |

## Configuration

The application uses a YAML configuration file (`config.yaml`) with the following sections:

- **Server**: HTTP/HTTPS server settings
- **DNS**: CoreDNS and zone management configuration
- **Tailscale**: API integration settings
- **Proxy**: Caddy reverse proxy configuration
- **Certificate**: SSL certificate management
- **Logging**: Log level and format settings

See `configs/config.yaml.template` for a complete configuration example.

## API Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/add-record` | POST | Create DNS record with optional proxy |
| `/list-records` | GET | List all DNS records |
| `/list-devices` | GET | List Tailscale devices |
| `/annotate-device` | POST | Update device metadata |
| `/health` | GET | Service health status |
| `/swagger` | GET | Interactive API documentation |

## Usage Examples

### Create a DNS Record

```bash
curl -X POST http://localhost:8080/add-record \
  -H "Content-Type: application/json" \
  -d '{
    "service_name": "web-service",
    "name": "webapp",
    "port": 8080
  }'
```

This creates:
- DNS A record: `webapp.internal.yourdomain.com → 100.64.1.5`
- Reverse proxy rule: `webapp.internal.yourdomain.com → service_ip:8080`
- SSL certificate for the domain

### List All Records

```bash
curl http://localhost:8080/list-records
```

### Check Health Status

```bash
curl http://localhost:8080/health
```

## Ports

| Port | Service | Protocol | Purpose |
|------|---------|----------|---------|
| 8080 | API | HTTP | REST API |
| 8443 | API | HTTPS | Secure REST API |
| 53 | CoreDNS | UDP/TCP | DNS resolution |
| 853 | CoreDNS | TCP | DNS-over-TLS |
| 80 | Caddy | HTTP | Reverse proxy |
| 2019 | Caddy | HTTP | Admin API |
| 8082 | CoreDNS | HTTP | Health check |

## Security

- **Privileged Mode**: Required for firewall (IPSet/iptables) management
- **Network Capabilities**: NET_ADMIN and NET_RAW for network configuration
- **TLS**: End-to-end encryption for all HTTP and DNS traffic
- **Firewall**: Automatic Tailscale CIDR range management

## Monitoring & Health

Comprehensive health monitoring for all services:

- **Health Checks**: Individual service status via `/health` endpoint
- **Component Status**: DNS, API, and certificate service monitoring
- **Readiness Probes**: Service readiness validation during startup
- **Container Health**: Docker health check on port 8080
- **Certificate Health**: SSL certificate expiration monitoring
- **Logging**: Structured JSON logging with configurable levels

## Development

### Building

```bash
# Build API only
docker build -f Dockerfile.api -t dns-manager-api .

# Build unified container
docker build -f Dockerfile.all -t dns-manager .
```

### Testing

```bash
go test ./...
```

### Generating Documentation

```bash
go run cmd/generate-openapi/main.go
```

## License

MIT