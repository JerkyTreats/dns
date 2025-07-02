# Tailscale Internal DNS Manager

A lightweight API for managing internal DNS records with dynamic zone synchronization using Tailscale device discovery. Provides automated DNS record management, CoreDNS integration, and Let's Encrypt SSL/TLS certification.

## Features

- **Dynamic Device Discovery**: Automatically discovers Tailscale devices and creates DNS records
- **RESTful API**: Simple interface for DNS record management
- **CoreDNS Integration**: Reliable DNS resolution with automatic zone file management
- **Let's Encrypt Integration**: Automated SSL/TLS certificate management via DNS-01 challenge
- **Firewall Management**: Automatic ipset/iptables configuration for Tailscale CIDR protection
- **Device Persistence**: Persistent storage of device metadata with backup support

## Prerequisites

- **Docker and Docker Compose**
- **Tailscale Account** with API access enabled
- **Domain Control** for DNS-01 challenge validation

## Quick Start

### 1. Setup

```bash
git clone https://github.com/jerkytreats/dns.git
cd dns
./scripts/setup.sh
```

The setup script will prompt for:
- Tailscale API key and tailnet name
- Internal domain and Let's Encrypt email
- Creates personalized configuration

### 2. Deploy

```bash
docker-compose up --build -d
```

### 3. Verify

```bash
# Check service health
curl http://localhost:8080/health

# Test DNS resolution
dig @localhost your-device.internal.yourdomain.com
```

## Configuration

The main configuration file `configs/config.yaml` contains:

```yaml
dns:
  domain: "internal.yourdomain.com"    # Base domain for DNS records
  internal:
    enabled: true                      # Enable dynamic device sync
    origin: "internal.yourdomain.com"  # Zone origin for device records
    polling:
      enabled: true                    # Enable periodic sync
      interval: "1h"                   # Sync interval

tailscale:
  api_key: "your-api-key"             # Tailscale API key
  tailnet: "your-tailnet"             # Tailscale tailnet name
  device_name: "your-device-name"     # Optional: specific device name

certificate:
  email: "your-email@domain.com"      # Let's Encrypt email
  domain: "internal.yourdomain.com"   # Domain for certificates
  renewal:
    enabled: true                     # Enable auto-renewal
    renew_before: "720h"              # Renew 30 days before expiry
```

## API Reference

### Health Check
```bash
curl http://localhost:8080/health
```

### DNS Record Management
```bash
# Add a DNS record
curl -X POST http://localhost:8080/add-record \
  -H "Content-Type: application/json" \
  -d '{"service_name": "my-service", "name": "my-service", "ip": "192.168.1.100"}'

# List DNS records
curl http://localhost:8080/list-records?service_name=my-service
```

### Device Management
```bash
# List Tailscale devices
curl http://localhost:8080/list-devices

# Annotate a device
curl -X POST http://localhost:8080/annotate-device \
  -H "Content-Type: application/json" \
  -d '{"hostname": "my-device", "annotations": {"description": "Main server"}}'

# Get device storage info
curl http://localhost:8080/device-storage-info
```

## Architecture

### Core Components

- **API Service** (`cmd/api/main.go`): Go application managing DNS records, certificates, and device sync
- **CoreDNS Manager** (`internal/dns/coredns/`): Manages CoreDNS configuration and zone files
- **Tailscale Client** (`internal/tailscale/`): Integrates with Tailscale API for device discovery
- **Sync Manager** (`internal/tailscale/sync/`): Handles dynamic zone synchronization
- **Certificate Manager** (`internal/certificate/`): Manages Let's Encrypt certificates via DNS-01 challenge
- **Firewall Manager** (`internal/firewall/`): Configures ipset/iptables for Tailscale CIDR protection
- **Device Persistence** (`internal/persistence/`): Stores device metadata with backup support

### Data Flow

1. **Startup**: API service initializes Tailscale client, CoreDNS manager, and firewall rules
2. **Device Discovery**: Sync manager fetches devices from Tailscale API
3. **DNS Record Creation**: CoreDNS manager creates zone files and DNS records
4. **Certificate Management**: Certificate manager handles Let's Encrypt challenges via CoreDNS
5. **Ongoing Sync**: Periodic polling keeps DNS records synchronized with Tailscale devices

## Security

- **Network Security**: All services run behind Tailscale network isolation
- **TLS Encryption**: Automatic Let's Encrypt certificates with modern cipher suites
- **Firewall Protection**: Automatic ipset/iptables configuration for Tailscale CIDR (100.64.0.0/10)
- **Private Keys**: Securely managed with proper file permissions
- **API Authentication**: Protected by Tailscale network access

## Troubleshooting

### Common Issues

**Tailscale API Connection Failed**:
- Verify `tailscale.api_key` is correct
- Check your `tailscale.tailnet` name
- Ensure API access is enabled in Tailscale admin console

**Device Not Found**:
- Check device name matches exactly in Tailscale admin console
- Ensure device is online and connected to Tailscale
- Verify device appears in `tailscale status` output

**Let's Encrypt Certificate Issues**:
- Ensure your domain's DNS points to your server
- Check firewall allows ports 80, 443, 53, and 853
- Verify email address is valid in configuration

**DNS Resolution Issues**:
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
```

## Maintenance

### Certificate Renewal
- Certificates auto-renew 30 days before expiration
- Monitor certificate status via `/health` endpoint
- Manual renewal: restart the API service

### Device Updates
- New devices are automatically discovered on service restart
- Dynamic sync updates DNS records when device IPs change
- Use the API to add temporary records without config changes

### Updates
```bash
git pull origin main
docker-compose down
docker-compose up --build -d
```

## License

This project is licensed under the MIT License - see the LICENSE file for details.
