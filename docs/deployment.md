# Deployment Guide

## Production Deployment

### Prerequisites

- Docker and Docker Compose installed
- Access to the target server
- Proper network configuration
- SSL certificates (if using HTTPS)

### Deployment Steps

1. **Clone Repository**
   ```bash
   git clone https://github.com/jerkytreats/dns.git
   cd dns
   ```

2. **Configure Environment**
   - Copy and edit configuration files:
     ```bash
     cp configs/config.yaml.example configs/config.yaml
     ```
   - Update production settings in `configs/config.yaml`
   - Configure CoreDNS settings in `coredns/Corefile`

3. **Build and Deploy**
   ```bash
   # Build and start services
   ./scripts/deploy.sh
   ```

### Production Configuration

#### API Server

```yaml
# configs/config.yaml
app:
  name: dns-manager
  version: 1.0.0
  environment: production

server:
  host: 0.0.0.0
  port: 8080
  read_timeout: 10s
  write_timeout: 20s
  idle_timeout: 120s

logging:
  level: info
  format: json
  output: stdout
```

#### CoreDNS

```yaml
# coredns/Corefile
.:53 {
    errors
    health
    log
    forward . /etc/resolv.conf
}

internal.jerkytreats.dev:53 {
    errors
    health
    log
    file /zones/internal.jerkytreats.dev.zone
}
```

### Security Considerations

1. **Network Security**
   - Use internal network for API-CoreDNS communication
   - Restrict API access to trusted IPs
   - Use HTTPS for API endpoints

2. **Authentication**
   - Enable basic auth for API endpoints
   - Use secure credentials
   - Rotate credentials regularly

3. **File Permissions**
   - Restrict access to configuration files
   - Use non-root user for containers
   - Secure zone file permissions

### Monitoring

1. **Health Checks**
   - Monitor `/health` endpoint
   - Set up alerts for unhealthy status
   - Monitor CoreDNS health

2. **Logging**
   - Configure log aggregation
   - Set up log rotation
   - Monitor error rates

3. **Metrics**
   - Track API request rates
   - Monitor DNS query volumes
   - Set up performance alerts

### Backup and Recovery

1. **Zone Files**
   ```bash
   # Backup zone files
   tar -czf zones-backup.tar.gz coredns/zones/

   # Restore zone files
   tar -xzf zones-backup.tar.gz
   ```

2. **Configuration**
   ```bash
   # Backup configuration
   tar -czf config-backup.tar.gz configs/ coredns/Corefile

   # Restore configuration
   tar -xzf config-backup.tar.gz
   ```

### Scaling

1. **API Server**
   - Deploy multiple API instances
   - Use load balancer
   - Configure session management

2. **CoreDNS**
   - Deploy multiple CoreDNS instances
   - Use DNS load balancing
   - Configure zone synchronization

### Maintenance

1. **Updates**
   ```bash
   # Pull latest changes
   git pull origin main

   # Rebuild and restart
   ./scripts/deploy.sh
   ```

2. **Log Rotation**
   ```bash
   # Configure log rotation
   /var/log/dns-manager/*.log {
       daily
       rotate 7
       compress
       delaycompress
       missingok
       notifempty
   }
   ```

3. **Cleanup**
   ```bash
   # Remove old containers
   docker system prune -f

   # Clean up old images
   docker image prune -f
   ```

### Troubleshooting

1. **API Issues**
   ```bash
   # Check API logs
   docker-compose logs api

   # Check API health
   curl http://localhost:8080/health
   ```

2. **DNS Issues**
   ```bash
   # Check CoreDNS logs
   docker-compose logs coredns

   # Test DNS resolution
   dig @localhost internal.jerkytreats.dev
   ```

3. **Container Issues**
   ```bash
   # Check container status
   docker-compose ps

   # Check container logs
   docker-compose logs
   ```

### Rollback Procedure

1. **API Rollback**
   ```bash
   # Revert to previous version
   git checkout <previous-version>
   ./scripts/deploy.sh
   ```

2. **Configuration Rollback**
   ```bash
   # Restore previous configuration
   cp config-backup.tar.gz .
   tar -xzf config-backup.tar.gz
   docker-compose restart
   ```

## Support

For issues and support:
1. Check the troubleshooting guide
2. Review logs and error messages
3. Contact the development team
4. Submit an issue on GitHub
