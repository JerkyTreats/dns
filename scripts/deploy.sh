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

# Detect container runtime
DOCKER_CMD=""
COMPOSE_CMD=""

# Check for Docker
if command -v docker &> /dev/null; then
    DOCKER_CMD="docker"
    # Check if Docker Compose is available
    if docker-compose --version &> /dev/null; then
        COMPOSE_CMD="docker-compose"
    elif docker compose version &> /dev/null; then
        COMPOSE_CMD="docker compose"
    fi
fi

# Check for nerdctl
if command -v nerdctl &> /dev/null; then
    DOCKER_CMD="nerdctl"
    # Check if nerdctl compose is available
    if nerdctl compose version &> /dev/null; then
        COMPOSE_CMD="nerdctl compose"
    fi
fi

# Validate we have both a container runtime and compose
if [ -z "$DOCKER_CMD" ]; then
    error "Neither Docker nor nerdctl is installed. Please install Docker or nerdctl and try again."
    exit 1
fi

if [ -z "$COMPOSE_CMD" ]; then
    error "Docker Compose is not available. Please install Docker Compose or ensure nerdctl compose is available."
    exit 1
fi

log "Using container runtime: $DOCKER_CMD"
log "Using compose command: $COMPOSE_CMD"

# Check if container runtime is running
if ! $DOCKER_CMD info > /dev/null 2>&1; then
    error "$DOCKER_CMD is not running. Please start $DOCKER_CMD and try again."
    exit 1
fi

# Get the directory where the script is located
SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

# Change to project root
cd "$PROJECT_ROOT"

log "Starting deployment of Tailscale Internal DNS Manager with Dynamic Configuration..."

# Check for required files
log "Checking required configuration files..."

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

# Stop existing containers if they exist
log "Stopping existing containers..."
$COMPOSE_CMD down --remove-orphans || true

# Build the images
log "Building Docker images..."
$COMPOSE_CMD build

# Start the services
log "Starting services..."
$COMPOSE_CMD up -d

# Wait for services to be ready with better health checks
log "Waiting for services to be ready..."
sleep 3

# Check API service health
MAX_ATTEMPTS=30
ATTEMPT=1

log "Checking API service health..."
while [ $ATTEMPT -le $MAX_ATTEMPTS ]; do
    if curl -f -s http://localhost:8080/health > /dev/null 2>&1; then
        log "API service is healthy!"
        break
    fi

    if [ $ATTEMPT -eq $MAX_ATTEMPTS ]; then
        error "API service health check failed after $MAX_ATTEMPTS attempts"
        echo "Check logs with: $COMPOSE_CMD logs api"
        exit 1
    fi

    echo -n "."
    sleep 2
    ATTEMPT=$((ATTEMPT + 1))
done

# Check CoreDNS service
log "Checking CoreDNS service..."
if $COMPOSE_CMD ps coredns | grep -q "Up"; then
    # Test DNS resolution
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
    echo "Check logs with: $COMPOSE_CMD logs coredns"
    exit 1
fi

# Show service status
echo
log "Deployment completed successfully!"
echo

# Display service information
log "Service Status:"
$COMPOSE_CMD ps

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
echo "  $COMPOSE_CMD logs -f           # View all logs"
echo "  $COMPOSE_CMD logs -f api       # View API logs"
echo "  $COMPOSE_CMD logs -f coredns   # View CoreDNS logs"
echo "  $COMPOSE_CMD down              # Stop services"
echo "  $COMPOSE_CMD restart           # Restart services"
echo

# Show next steps
log "Next Steps:"
echo "1. Test the health endpoint: curl http://localhost:8080/health"

# Get domain from config for DNS testing
DOMAIN=$(grep "domain:" configs/config.yaml | head -n 1 | awk '{print $2}' | tr -d '"')
if [ ! -z "$DOMAIN" ]; then
    echo "2. Test DNS resolution: dig @localhost $DOMAIN"
fi

echo "3. View logs: $COMPOSE_CMD logs -f"
echo "4. Check the documentation in docs/ for more information"
echo
log "Dynamic Configuration Features Active:"
echo "- CoreDNS configuration is generated automatically on startup"
echo "- New zones and records are added dynamically via API"
echo "- TLS certificates are integrated automatically when available"
echo "- Configuration updates trigger automatic CoreDNS restarts"
echo "- No manual CoreDNS Corefile editing required"
echo

log "Deployment script completed!"
