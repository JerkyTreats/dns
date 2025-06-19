# DNS Manager API

A lightweight API for managing internal DNS records within a Tailscale network. This service provides a simple interface to manage DNS records for internal services, with CoreDNS integration for reliable DNS resolution and Let's Encrypt SSL/TLS certification.

## Features

- RESTful API for DNS record management
- CoreDNS integration for reliable DNS resolution
- Let's Encrypt SSL/TLS certification with automated renewal
- Docker-based deployment
- OpenAPI/Swagger documentation
- Health monitoring endpoints
- Comprehensive test coverage

## Prerequisites

- Go 1.24 or later
- Docker and Docker Compose
- Domain control for DNS-01 challenge validation
- Make (optional, for using Makefile commands)

## Quick Start

1. Clone the repository:
   ```bash
   git clone https://github.com/jerkytreats/dns.git
   cd dns
   ```

2. Configure your domain in `configs/config.yaml`:
   ```yaml
   certificate:
     email: "admin@yourdomain.com"
     domain: "yourdomain.com"
   ```

3. Deploy using the provided script:
   ```bash
   ./scripts/deploy.sh
   ```

The API will be available at:
- HTTP: `http://localhost:8080`
- HTTPS: `https://localhost:8443`
- CoreDNS: `localhost:53`

## SSL/TLS Configuration

The service automatically obtains and manages Let's Encrypt certificates:

### Certificate Management
- **Automatic issuance**: Certbot handles certificate generation
- **Direct mounting**: Certificates are mounted directly from certbot's output
- **Auto-renewal**: Certificates renew automatically every 60-90 days
- **Zero downtime**: Hot-reload capability for certificate updates

### Certificate Locations
Certificates are stored in the standard Let's Encrypt locations:
```
/etc/letsencrypt/live/yourdomain.com/
├── cert.pem          # Your certificate
├── privkey.pem       # Your private key
├── chain.pem         # Let's Encrypt's certificate chain
└── fullchain.pem     # Your cert + chain combined
```

### No Copy Process Required
Unlike traditional setups, this implementation:
- **Directly reads** certificates from certbot's output directory
- **No file copying** or permission management needed
- **Real-time updates** when certificates are renewed
- **Simplified maintenance** with fewer moving parts

## API Endpoints

### Health Check
```http
GET /health
```
Returns the health status of the API, CoreDNS services, and certificate information.

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
  tls:
    enabled: true
    port: 8443
    cert_file: /etc/letsencrypt/live/yourdomain.com/cert.pem
    key_file: /etc/letsencrypt/live/yourdomain.com/privkey.pem
  read_timeout: 5s
  write_timeout: 10s
  idle_timeout: 120s

dns:
  domain: yourdomain.com
  coredns:
    config_path: /etc/coredns/Corefile
    zones_path: /zones
    reload_command: ["kill", "-SIGUSR1", "1"]
    tls:
      enabled: true
      cert_file: /etc/letsencrypt/live/yourdomain.com/cert.pem
      key_file: /etc/letsencrypt/live/yourdomain.com/privkey.pem
      port: 443

certificate:
  provider: "certbot"
  email: "admin@yourdomain.com"
  domain: "yourdomain.com"
  renewal:
    enabled: true
    renew_before: 30d
    check_interval: 24h
  monitoring:
    enabled: true
    alert_threshold: 7d
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

The service is containerized using Docker with integrated SSL/TLS support:

- `Dockerfile.api`: Multi-stage build for the API server
- `Dockerfile`: CoreDNS configuration
- `docker-compose.yml`: Local development setup with certbot integration

### Building Images

```bash
docker-compose build
```

### Running Services

```bash
docker-compose up -d
```

### Certificate Renewal

Certificates are automatically renewed by certbot. The renewal process:
1. Runs automatically every 60-90 days
2. Uses DNS-01 challenge for wildcard certificates
3. Updates certificates in-place without service interruption
4. Triggers service reload to use new certificates

## Security Features

- **TLS 1.2/1.3 support** with modern cipher suites
- **Certificate transparency** monitoring
- **Secure private key handling** with proper permissions
- **DNS-over-HTTPS/TLS** support for CoreDNS
- **Automatic certificate rotation** and renewal

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
