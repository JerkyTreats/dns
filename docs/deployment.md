# Deployment Guide

This guide provides instructions for deploying the DNS Manager service to a production environment.

## Prerequisites

- Docker and Docker Compose installed on the target server.
- A domain name that you control.
- Firewall rules configured to allow traffic on ports 80, 443, 53, and 853.

## Deployment Steps

1.  **Clone the Repository**
    ```bash
    git clone https://github.com/jerkytreats/dns.git
    cd dns
    ```

2.  **Configure the Environment**
    - Edit `configs/config.yaml` to set your production settings. You must update the `certificate.email` and `certificate.domain` fields.
    - To use Let's Encrypt's production environment, change `certificate.ca_dir_url` to `https://acme-v02.api.letsencrypt.org/directory`.
    - Enable TLS in production by setting `server.tls.enabled: true`.

3.  **Build and Deploy**
    Use the `deploy.sh` script to build and start the services.
    ```bash
    ./scripts/deploy.sh
    ```
    This script will build the Docker images, create the network, and start the `api` and `coredns` containers in detached mode.

## Production Configuration

Your `configs/config.yaml` and `configs/coredns/Corefile` should be configured for your production environment.

### API Server (`configs/config.yaml`)
```yaml
app:
  name: dns-manager
  version: 1.0.0
  environment: production

server:
  host: 0.0.0.0
  port: 8080
  tls:
    enabled: true # Enable for production
    port: 8443

logging:
  level: info
  format: json

certificate:
  email: "your-email@yourdomain.com"
  domain: "internal.yourdomain.com"
  ca_dir_url: "https://acme-v02.api.letsencrypt.org/directory" # Production URL
```

### CoreDNS (`configs/coredns/Corefile`)
```corefile
. {
    errors
    log
    forward . /etc/resolv.conf
}

internal.jerkytreats.dev:53 {
    errors
    log
    file /etc/coredns/zones/internal.jerkytreats.dev.db
}

internal.jerkytreats.dev:853 {
    tls /etc/letsencrypt/live/internal.jerkytreats.dev/cert.pem /etc/letsencrypt/live/internal.jerkytreats.dev/privkey.pem
    errors
    log
    file /etc/coredns/zones/internal.jerkytreats.dev.db
}

_acme-challenge.internal.jerkytreats.dev:53 {
    errors
    log
    file /etc/coredns/zones/_acme-challenge.internal.jerkytreats.dev.zone
}
```

## Security

- **Network Security**: Ensure that the API server is not publicly exposed. If it must be, restrict access to trusted IP addresses.
- **File Permissions**: The `ssl` directory contains your private keys. The `docker-compose.yml` file mounts this as read-only for the `coredns` service. Ensure the permissions on the host are secure.

## Monitoring

- **Health Checks**: The `/health` endpoint provides the status of the API and certificate information. Monitor this endpoint to ensure the service is running correctly.
- **Logging**: Both the `api` and `coredns` services log to `stdout`. Configure your Docker daemon to forward logs to a log aggregation service.

## Backup and Recovery

- **Certificates**: The SSL certificates are stored in the `./ssl` directory. Back up this directory regularly.
- **Zone Files**: The DNS zone files are in `configs/coredns/zones`. Since these are managed by the application, ensure your Git repository is backed up.

## Maintenance

- **Updates**: To update the application, pull the latest changes from the Git repository and re-run the deployment script:
  ```bash
  git pull origin main
  ./scripts/deploy.sh
  ```
- **Cleanup**: To remove old containers and images, use the Docker `prune` commands:
  ```bash
  docker system prune -f
  docker image prune -f
  ```

## Troubleshooting

- **API Issues**: Check the API logs with `docker-compose logs api`.
- **DNS Issues**: Check the CoreDNS logs with `docker-compose logs coredns`.
- **Test DNS Resolution**: Use `dig @localhost internal.yourdomain.com` to test DNS resolution.

## Rollback

To roll back to a previous version, check out the desired Git commit and re-run the deployment script.
```bash
git checkout <previous-commit-hash>
./scripts/deploy.sh
```

## Support

For issues and support:
1. Check the troubleshooting guide
2. Review logs and error messages
3. Contact the development team
4. Submit an issue on GitHub
