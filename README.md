# Tailscale Internal DNS Manager

A lightweight API for managing internal DNS records. This service provides a simple interface to manage DNS records for internal services, with CoreDNS integration for reliable DNS resolution and automated Let's Encrypt SSL/TLS certification.

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
- CoreDNS integration for reliable DNS resolution
- Automated Let's Encrypt SSL/TLS certification using `go-acme/lego`
- In-application certificate issuance and renewal (no `certbot` required)
- Docker-based deployment for easy setup
- OpenAPI/Swagger documentation
- Health monitoring endpoints with certificate status
- Comprehensive test coverage

## Prerequisites

- Go 1.24 or later
- Docker and Docker Compose
- A domain you control for DNS-01 challenge validation

## Quick Start

1.  **Clone the repository:**
    ```bash
    git clone https://github.com/jerkytreats/dns.git
    cd dns
    ```

2.  **Configure your domain and email** in `configs/config.yaml`. The `server.tls.enabled` flag controls whether the server starts with HTTPS.
    ```yaml
    server:
      tls:
        enabled: true # Set to true to enable HTTPS

    certificate:
      email: "your-email@example.com"
      domain: "internal.yourdomain.com"
    ```

3.  **Deploy the services:**
    ```bash
    docker-compose up --build
    ```

The application will start, obtain an SSL certificate from Let's Encrypt's staging environment, and serve traffic:
- **HTTP:** `http://localhost:8080`
- **HTTPS:** `https://localhost:8443`
- **CoreDNS:** `localhost:53` (UDP/TCP) and `localhost:853` (DNS-over-TLS)

## SSL/TLS Configuration

Certificate management is handled directly within the Go application using the `go-acme/lego` library, removing the need for an external `certbot` container.

- **Automated Issuance and Renewal**: The application obtains and renews certificates automatically. On the first run, it will generate a new certificate. A background process checks daily and renews the certificate if it is within the configured renewal window (default: 30 days).
- **DNS-01 Challenge**: Wildcard certificates (`*.internal.yourdomain.com`) are supported using the DNS-01 challenge method. The application temporarily creates the required TXT records in a CoreDNS zone file to prove ownership of the domain.
- **Staging and Production**: By default, the service uses Let's Encrypt's staging environment to avoid hitting rate limits during development. To use the production environment, update the `ca_dir_url` in `configs/config.yaml`.

## API Endpoints

### Health Check
```http
GET /health
```
Returns the health status of the API and CoreDNS services. If TLS is enabled, it also includes details about the current SSL certificate, such as its expiration date.

### Add DNS Record
```http
POST /add-record
Content-Type: application/json

{
    "service_name": "my-service"
}
```
Creates a new DNS `A` record for `<service_name>.internal.yourdomain.com`.

## Configuration

The service is configured through `configs/config.yaml`. Key options include:

```yaml
server:
  host: 0.0.0.0
  port: 8080
  tls:
    enabled: true
    port: 8443
    cert_file: /etc/letsencrypt/live/internal.jerkytreats.dev/cert.pem
    key_file: /etc/letsencrypt/live/internal.jerkytreats.dev/privkey.pem

dns:
  coredns:
    config_path: /etc/coredns/Corefile
    zones_path: /etc/coredns/zones
    tls:
      enabled: true
      port: 853

certificate:
  email: "admin@jerkytreats.dev"
  domain: "internal.jerkytreats.dev"
  ca_dir_url: "https://acme-staging-v02.api.letsencrypt.org/directory" # Staging URL
  renewal:
    enabled: true
    renew_before: "720h" # 30 days
    check_interval: "24h"
```

## Development

### Running Tests
To run the test suite:
```bash
go test ./...
```

### Local Environment
To run the services locally for development:
```bash
docker-compose up --build
```
The API server will start with hot-reloading enabled.

## Docker Deployment

The `docker-compose.yml` file orchestrates the `api` and `coredns` services.
- The `api` service builds from `Dockerfile.api`.
- A shared volume (`./ssl`) is used to store the SSL certificates, which are written by the `api` service and read by the `coredns` service.
- DNS zone files are located in `configs/coredns/zones` and mounted into the `coredns` container.

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
