#!/bin/bash

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Function to print colored output
print_status() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

print_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

if [ ! -f "docker-compose.test.yml" ]; then
    print_error "docker-compose.test.yml not found!"
    print_error "Please run this script from the tests directory: cd tests && ./run-integration-tests.sh"
    exit 1
fi

if ! docker info >/dev/null 2>&1; then
    print_error "Docker is not running. Please start Docker and try again."
    exit 1
fi

if ! command -v docker-compose >/dev/null 2>&1; then
    print_error "docker-compose is not installed or not in PATH"
    exit 1
fi

print_status "🐳 Running integration tests in Docker containers"



print_status "🚀 Starting integration test run..."

# Clean up any existing containers
print_status "🧹 Cleaning up any existing test containers..."
docker-compose -f docker-compose.test.yml down -v --remove-orphans >/dev/null 2>&1 || true

# Function to handle cleanup on exit
cleanup() {
    print_status "🧹 Cleaning up test environment..."
    docker-compose -f docker-compose.test.yml down -v >/dev/null 2>&1 || true
}

# Set trap to cleanup on exit
trap cleanup EXIT

# Check Docker Compose file syntax
print_status "🔍 Validating Docker Compose configuration..."
if ! docker-compose -f docker-compose.test.yml config >/dev/null 2>&1; then
    print_error "Invalid docker-compose.test.yml configuration"
    exit 1
fi

print_success "Docker Compose configuration is valid"

print_status "🏗️  Building and starting test services..."

# Use standard Docker build (no BuildKit)
print_status "🔧 Using standard Docker build"
export DOCKER_BUILDKIT=0
unset COMPOSE_DOCKER_CLI_BUILD

# Pre-pull base images in smaller batches for better Colima compatibility
print_status "📦 Pre-pulling base images..."
docker pull golang:1.24.3-alpine &
docker pull alpine:3.19 &
wait  # Wait for first batch

docker pull nginx:alpine &
docker pull coredns/coredns:1.11.1 &
wait  # Wait for second batch

docker pull ghcr.io/letsencrypt/pebble:latest &
wait  # Wait for final image

# Run tests entirely within Docker with parallel startup
if ! docker-compose -f docker-compose.test.yml up --build --exit-code-from test-runner test-runner; then
    print_error "❌ Integration tests failed!"

    print_warning "Collecting diagnostic information..."

    # Get service logs for debugging
    echo ""
    print_status "📋 Recent service logs:"
    docker-compose -f docker-compose.test.yml logs --tail=20 || true

    # Check service status
    echo ""
    print_status "🔍 Service status:"
    docker-compose -f docker-compose.test.yml ps || true

    exit 1
fi

print_success "🎉 All integration tests passed!"
