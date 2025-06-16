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

# Stop existing containers if they exist
log "Stopping existing containers..."
docker-compose down --remove-orphans || true

# Build the images
log "Building Docker images..."
docker-compose build --no-cache

# Start the services
log "Starting services..."
docker-compose up -d

# Wait for services to be ready
log "Waiting for services to be ready..."
sleep 5

# Check if services are running
if docker-compose ps | grep -q "Up"; then
    log "Services are running successfully!"
    log "API server is available at: http://localhost:8080"
    log "CoreDNS is available at: localhost:53"
else
    error "Failed to start services. Check logs with: docker-compose logs"
    exit 1
fi

# Print helpful commands
echo
log "Useful commands:"
echo "  docker-compose logs -f    # View logs"
echo "  docker-compose down       # Stop services"
echo "  docker-compose restart    # Restart services"
