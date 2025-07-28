#!/bin/bash
set -e

# Colors for output
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

log() { echo -e "${GREEN}[INIT]${NC} $1"; }
warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
error() { echo -e "${RED}[ERROR]${NC} $1"; }

log "Starting DNS Manager initialization..."

# Debug: List what files are actually present
log "Debug: Files in /app/configs/ (mounted volume):"
ls -la /app/configs/ || true
log "Debug: Files in /app/templates/ (built-in):"
ls -la /app/templates/ || true
log "Debug: Files in /app/scripts/:"
ls -la /app/scripts/ || true

# Always regenerate config.yaml from template with current environment variables
log "Creating configuration from template and environment variables..."

if [ ! -f "/app/templates/config.yaml.template" ]; then
    error "Template file not found!"
    error "Contents of /app/templates/:"
    ls -la /app/templates/ || true
    exit 1
fi

# Copy template to config (from internal templates to mounted volume)
cp /app/templates/config.yaml.template /app/configs/config.yaml

    # Substitute environment variables using Python
    if [ -n "$INTERNAL_DOMAIN" ]; then
        python3 /app/scripts/template_substitute.py /app/configs/config.yaml /app/configs/config.yaml \
            -s "INTERNAL_DOMAIN_PLACEHOLDER" "$INTERNAL_DOMAIN"
    fi

    if [ -n "$LETSENCRYPT_EMAIL" ]; then
        python3 /app/scripts/template_substitute.py /app/configs/config.yaml /app/configs/config.yaml \
            -s "LETSENCRYPT_EMAIL_PLACEHOLDER" "$LETSENCRYPT_EMAIL"
    fi

    if [ -n "$TAILSCALE_API_KEY" ]; then
        python3 /app/scripts/template_substitute.py /app/configs/config.yaml /app/configs/config.yaml \
            -s "TAILSCALE_API_KEY_PLACEHOLDER" "$TAILSCALE_API_KEY"
    fi

    if [ -n "$TAILSCALE_TAILNET" ]; then
        python3 /app/scripts/template_substitute.py /app/configs/config.yaml /app/configs/config.yaml \
            -s "TAILSCALE_TAILNET_PLACEHOLDER" "$TAILSCALE_TAILNET"
    fi

    # Set Let's Encrypt URL based on USE_PRODUCTION_CERTS setting
    if [ "$USE_PRODUCTION_CERTS" = "false" ]; then
        LETSENCRYPT_URL="https://acme-staging-v02.api.letsencrypt.org/directory"
    else
        LETSENCRYPT_URL="https://acme-v02.api.letsencrypt.org/directory"
    fi
    python3 /app/scripts/template_substitute.py /app/configs/config.yaml /app/configs/config.yaml \
        -s "LETSENCRYPT_URL_PLACEHOLDER" "$LETSENCRYPT_URL"

    # Set server port based on environment
    SERVER_PORT="${SERVER_PORT:-8080}"
    python3 /app/scripts/template_substitute.py /app/configs/config.yaml /app/configs/config.yaml \
        -s "SERVER_PORT_PLACEHOLDER" "$SERVER_PORT"

    # Configure TLS if enabled
    if [ "$ENABLE_TLS" = "true" ]; then
        python3 /app/scripts/template_substitute.py /app/configs/config.yaml /app/configs/config.yaml \
            -s "enabled: false" "enabled: true"
    fi

    # Configure Cloudflare API token (required)
    if [ -n "$CLOUDFLARE_API_TOKEN" ]; then
        python3 /app/scripts/template_substitute.py /app/configs/config.yaml /app/configs/config.yaml \
            -s "CLOUDFLARE_API_TOKEN_PLACEHOLDER" "$CLOUDFLARE_API_TOKEN"
    else
        warn "CLOUDFLARE_API_TOKEN not provided - certificate obtainment will fail"
    fi

    # Configure proxy enabled setting
    if [ -n "$PROXY_ENABLED" ]; then
        if [ "$PROXY_ENABLED" = "true" ]; then
            python3 /app/scripts/template_substitute.py /app/configs/config.yaml /app/configs/config.yaml \
                -s "enabled: true" "enabled: true"
        else
            python3 /app/scripts/template_substitute.py /app/configs/config.yaml /app/configs/config.yaml \
                -s "enabled: true" "enabled: false"
        fi
    fi

    # Configure Caddy port
    if [ -n "$CADDY_PORT" ]; then
        python3 /app/scripts/template_substitute.py /app/configs/config.yaml /app/configs/config.yaml \
            -s "CADDY_PORT_PLACEHOLDER" "$CADDY_PORT"
    else
        python3 /app/scripts/template_substitute.py /app/configs/config.yaml /app/configs/config.yaml \
            -s "CADDY_PORT_PLACEHOLDER" "80"
    fi

    log "Configuration created successfully"

# Try to auto-configure Tailscale device name if not set and Python/API are available
if [ -z "$TAILSCALE_DEVICE_NAME" ]; then
    log "Attempting to auto-detect Tailscale device..."
    if command -v python3 >/dev/null 2>&1; then
        python3 /app/scripts/configure_ns.py || warn "Auto-detection failed, will use manual configuration"
    else
        warn "Python3 not available for auto-detection"
    fi
elif [ -n "$TAILSCALE_DEVICE_NAME" ]; then
    # Update config with provided device name
    python3 /app/scripts/template_substitute.py /app/configs/config.yaml /app/configs/config.yaml \
        -s "# device_name: \"your-device-name\"" "device_name: \"$TAILSCALE_DEVICE_NAME\""
fi

# Copy CoreDNS template to mounted config location if it doesn't exist
if [ ! -f "/etc/coredns/Corefile.template" ]; then
    log "Copying CoreDNS template to mounted location..."
    mkdir -p /etc/coredns
    cp /app/templates/coredns/Corefile.template /etc/coredns/
fi

# Ensure CoreDNS template exists
if [ ! -f "/etc/coredns/Corefile.template" ]; then
    error "CoreDNS template not found!"
    exit 1
fi

# Ensure Caddyfile template exists
if [ ! -f "/etc/caddy/Caddyfile.template" ]; then
    log "Copying Caddyfile template to mounted location..."
    mkdir -p /etc/caddy
    cp /app/templates/Caddyfile.template /etc/caddy/
fi

# Create initial Caddyfile if it doesn't exist
if [ ! -f "/app/configs/Caddyfile" ]; then
    log "Creating initial Caddyfile..."
    cat > /app/configs/Caddyfile << 'CADDY_EOF'
# Initial Caddyfile - will be updated by proxy manager
{
    admin off
    auto_https off
    log {
        output stdout
        format console
        level INFO
    }
}

# Health check endpoint
:2019 {
    respond /health "OK" 200
}
CADDY_EOF
fi

log "Initialization complete, starting services..."