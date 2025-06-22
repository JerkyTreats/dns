# Tailscale Internal DNS Manager

A lightweight API for managing internal DNS records with dynamic zone bootstrapping using Tailscale device discovery. This service provides automated DNS record management for internal services, with CoreDNS integration for reliable DNS resolution and automated Let's Encrypt SSL/TLS certification.

## Overview

This project automatically discovers devices in your Tailscale network and creates DNS records for them, eliminating the need to manually manage static IP addresses. It provides:

- **Dynamic Zone Bootstrap**: Automatically discovers Tailscale devices and creates DNS records
- **RESTful API**: Simple interface for DNS record management
- **CoreDNS Integration**: Reliable DNS resolution with automatic zone file management
- **Let's Encrypt Integration**: Automated SSL/TLS certificate management
- **Docker Deployment**: Easy containerized setup

## Prerequisites

### Required Software
- **Docker and Docker Compose**: For containerized deployment
- **Tailscale Account**: With API access enabled
- **Domain Control**: A domain you control for DNS-01 challenge validation

### Tailscale Setup

1. **Create a Tailscale API Key**:
   - Go to the [Tailscale Admin Console](https://login.tailscale.com/admin/settings/keys)
   - Generate an API key with appropriate permissions
   - Note your Tailnet name (usually your organization name or email domain)

2. **Enable API Access**:
   - Ensure your Tailscale account has API access enabled
   - Verify you can see your devices in the admin console

## First-Time Setup

### 1. Clone and Configure

```bash
git clone https://github.com/jerkytreats/dns.git
cd dns
```

### 2. Environment Variables

Create a `.env` file or set environment variables:

```bash
# Required Tailscale Configuration
export TAILSCALE_API_KEY="tskey-api-xxxxx"     # Your Tailscale API key
export TAILSCALE_TAILNET="your-tailnet-name"   # Your Tailnet identifier

# Optional
export APP_ENV=production
```

### 3. Configuration Setup

Copy the bootstrap example configuration:

```bash
cp configs/config-bootstrap-example.yaml configs/config.yaml
```

Edit `configs/config.yaml` to match your setup:

```yaml
dns:
  domain: internal.yourdomain.com  # Change to your domain
  internal:
    enabled: true  # Enable dynamic bootstrap
    origin: "internal.yourdomain.com"  # Your internal domain
    bootstrap_devices:
      - name: "server"
        tailscale_name: "your-server-name"  # Tailscale device name
        aliases: ["api", "dns"]
        description: "Main server"
        enabled: true
      # Add more devices as needed

# Tailscale configuration uses environment variables
tailscale:
  api_key: "${TAILSCALE_API_KEY}"
  tailnet: "${TAILSCALE_TAILNET}"

certificate:
  email: "your-email@yourdomain.com"  # Required for Let's Encrypt
  domain: "internal.yourdomain.com"   # Must match your domain
  ca_dir_url: "https://acme-v02.api.letsencrypt.org/directory"  # Production
```

### 4. Let's Encrypt Configuration

**For Production SSL Certificates**:

1. **Update Certificate Settings** in `configs/config.yaml`:
   ```yaml
   certificate:
     email: "your-email@yourdomain.com"          # Required
     domain: "internal.yourdomain.com"           # Your domain
     ca_dir_url: "https://acme-v02.api.letsencrypt.org/directory"  # Production

   server:
     tls:
       enabled: true  # Enable HTTPS
   ```

2. **DNS-01 Challenge**: The service uses DNS-01 challenge for wildcard certificates
   - Requires your domain's DNS to point to your CoreDNS server
   - Automatically manages challenge records during certificate issuance

**For Development/Testing**:
- Use staging URL: `https://acme-staging-v02.api.letsencrypt.org/directory`
- Set `server.tls.enabled: false` to disable HTTPS

### 5. Deploy Services

```bash
# Using Docker Compose
docker-compose up --build -d

# Or using the deployment script
./scripts/deploy.sh
```

### 6. Verify Deployment

1. **Check Service Health**:
   ```bash
   curl http://localhost:8080/health
   ```

2. **Test DNS Resolution**:
   ```bash
   # Test internal domain resolution
   dig @localhost your-device.internal.yourdomain.com

   # Test with specific device
   nslookup server.internal.yourdomain.com localhost
   ```

3. **Check Logs**:
   ```bash
   docker-compose logs api
   docker-compose logs coredns
   ```

## Configuration Reference

### Dynamic Bootstrap Configuration

The `dns.internal.bootstrap_devices` section defines which Tailscale devices to automatically create DNS records for:

```yaml
dns:
  internal:
    enabled: true
    origin: "internal.yourdomain.com"
    bootstrap_devices:
      - name: "server"                    # DNS record name
        tailscale_name: "my-server"       # Tailscale device name
        aliases: ["api", "web"]           # Additional DNS names
        description: "Main server"        # Documentation
        enabled: true                     # Enable/disable this device
```

### Environment Variables

| Variable | Required | Description |
|----------|----------|-------------|
| `TAILSCALE_API_KEY` | Yes | Tailscale API key for device discovery |
| `TAILSCALE_TAILNET` | Yes | Your Tailnet identifier/name |
| `APP_ENV` | No | Application environment (development/production) |

## API Usage

### Health Check
```bash
curl http://localhost:8080/health
```

### Add DNS Record
```bash
curl -X POST http://localhost:8080/add-record \
  -H "Content-Type: application/json" \
  -d '{"service_name": "my-new-service"}'
```

## Security

- **Network Security**: All services run behind Tailscale network isolation
- **TLS Encryption**: Automatic Let's Encrypt certificates with modern cipher suites
- **Private Keys**: Securely managed with proper file permissions
- **API Authentication**: Protected by Tailscale network access

## Troubleshooting

### Common Issues

1. **Tailscale API Connection Failed**:
   - Verify `TAILSCALE_API_KEY` is correct
   - Check your Tailnet name in `TAILSCALE_TAILNET`
   - Ensure API access is enabled in Tailscale admin console

2. **Device Not Found**:
   - Check device name matches exactly in Tailscale admin console
   - Ensure device is online and connected to Tailscale
   - Verify device appears in `tailscale status` output

3. **Let's Encrypt Certificate Issues**:
   - Ensure your domain's DNS points to your server
   - Check firewall allows ports 80, 443, 53, and 853
   - Verify email address is valid in configuration
   - For testing, use staging URL to avoid rate limits

4. **DNS Resolution Issues**:
   - Test CoreDNS directly: `dig @localhost internal.yourdomain.com`
   - Check CoreDNS logs: `docker-compose logs coredns`
   - Verify zone files in `configs/coredns/zones/`

### Debug Mode

Enable debug logging in `configs/config.yaml`:
```yaml
logging:
  level: debug
```

### Log Analysis
```bash
# API service logs
docker-compose logs -f api

# CoreDNS logs
docker-compose logs -f coredns

# Follow all logs
docker-compose logs -f
```

## Maintenance

### Certificate Renewal
- Certificates auto-renew 30 days before expiration
- Monitor certificate status via `/health` endpoint
- Manual renewal: restart the API service

### Device Updates
- New devices are automatically discovered on service restart
- Modify `bootstrap_devices` in config and restart to add/remove devices
- Use the API to add temporary records without config changes

### Updates
```bash
git pull origin main
docker-compose down
docker-compose up --build -d
```

## Architecture

- **API Service**: Go application managing DNS records and certificates
- **CoreDNS**: DNS server with dynamic zone file management
- **Tailscale Integration**: Device discovery and IP resolution
- **Let's Encrypt**: Automated certificate management via DNS-01 challenge

## License

This project is licensed under the MIT License - see the LICENSE file for details.
