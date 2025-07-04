#!/bin/bash

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Logging function
log() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Check if docker is running
if ! docker info > /dev/null 2>&1; then
    error "Docker is not running. Please start Docker and try again."
    exit 1
fi

# Get the directory where the script is located
SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

# Change to project root
cd "$PROJECT_ROOT"

log "Starting Tailscale Internal DNS Manager with Dynamic Configuration..."

# Check for required files
if [ ! -f "configs/config.yaml" ]; then
    error "Configuration file configs/config.yaml not found!"
    echo "Please run './scripts/setup.sh' first to create the configuration."
    exit 1
fi

if [ ! -f "configs/coredns/Corefile.template" ]; then
    error "CoreDNS template file configs/coredns/Corefile.template not found!"
    echo "This file should be committed to the repository."
    exit 1
fi

log "Configuration files found"
log "Using dynamic CoreDNS configuration system"

# Create ssl directory if it doesn't exist
if [ ! -d "ssl" ]; then
    log "Creating ssl directory for certificates..."
    mkdir -p ssl
fi

log "Starting services..."
docker-compose up -d

# Wait for services to be ready with health checks
log "Waiting for services to be ready..."
sleep 3

# Check API service health
MAX_ATTEMPTS=20
ATTEMPT=1

log "Checking API service health..."
while [ $ATTEMPT -le $MAX_ATTEMPTS ]; do
    if curl -f -s http://localhost:8080/health > /dev/null 2>&1; then
        log "API service is healthy!"
        break
    fi

    if [ $ATTEMPT -eq $MAX_ATTEMPTS ]; then
        error "API service health check failed after $MAX_ATTEMPTS attempts"
        echo "Check logs with: docker-compose logs api"
        exit 1
    fi

    echo -n "."
    sleep 2
    ATTEMPT=$((ATTEMPT + 1))
done

# Check CoreDNS service
log "Checking CoreDNS service..."
if docker-compose ps coredns | grep -q "Up"; then
    # Test DNS resolution if dig is available
    if command -v dig >/dev/null 2>&1; then
        if timeout 5 dig @localhost version.bind TXT CH +short > /dev/null 2>&1; then
            log "CoreDNS is responding to queries!"
        else
            warn "CoreDNS is running but not responding to test queries"
        fi
    else
        log "CoreDNS container is running (dig not available for testing)"
    fi
else
    error "CoreDNS service failed to start"
    echo "Check logs with: docker-compose logs coredns"
    exit 1
fi

# Show service status
echo
log "Services started successfully!"
echo

# Display service information
log "Service Status:"
docker-compose ps

echo
log "Service URLs:"
echo "  API Server: http://localhost:8080"
echo "  Health Check: http://localhost:8080/health"
echo "  CoreDNS: localhost:53 (UDP/TCP)"

# Show certificate information if TLS is enabled
if grep -q "enabled: true" configs/config.yaml | head -n 20; then
    echo "  HTTPS: https://localhost:8443 (when certificates are ready)"
fi

echo
log "Useful commands:"
echo "  docker-compose logs -f           # View all logs"
echo "  docker-compose logs -f api       # View API logs"
echo "  docker-compose logs -f coredns   # View CoreDNS logs"
echo "  docker-compose down              # Stop services"
echo "  docker-compose restart           # Restart services"
echo

# Show quick test commands
log "Quick Tests:"
echo "1. Health check: curl http://localhost:8080/health"

# Get domain from config for DNS testing
DOMAIN=$(grep "domain:" configs/config.yaml | head -n 1 | awk '{print $2}' | tr -d '"')
if [ ! -z "$DOMAIN" ]; then
    echo "2. DNS test: dig @localhost $DOMAIN"
fi

echo
log "Dynamic Configuration System:"
echo "- CoreDNS will generate its configuration automatically"
echo "- DNS zones and records are managed dynamically"
echo "- TLS integration happens automatically when certificates are available"
echo "- API-driven configuration updates with automatic restarts"
echo

log "Start script completed!"
