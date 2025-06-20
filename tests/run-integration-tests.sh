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

# Check if we're in the right directory
if [ ! -f "docker-compose.test.yml" ]; then
    print_error "docker-compose.test.yml not found!"
    print_error "Please run this script from the tests directory: cd tests && ./run-integration-tests.sh"
    exit 1
fi

# Check if Docker is running
if ! docker info >/dev/null 2>&1; then
    print_error "Docker is not running. Please start Docker and try again."
    exit 1
fi

# Check if Docker Compose is available
if ! command -v docker-compose >/dev/null 2>&1; then
    print_error "docker-compose is not installed or not in PATH"
    exit 1
fi

# Check for --docker flag to run tests entirely in Docker
DOCKER_MODE=false
if [ "$1" = "--docker" ]; then
    DOCKER_MODE=true
    print_status "🐳 Running tests entirely within Docker containers"
else
    print_status "🏠 Running tests from host system (legacy mode)"
fi

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

if [ "$DOCKER_MODE" = true ]; then
    # Run tests entirely within Docker
    print_status "🏗️  Building and starting test services..."

    # Start all services including test-runner
    if ! DOCKER_BUILDKIT=0 docker-compose -f docker-compose.test.yml up --build --exit-code-from test-runner test-runner; then
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

    print_success "🎉 All integration tests passed in Docker!"
else
    # Original host-based approach
    print_status "🏃 Running integration tests from host..."
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

    if go test -v integration_test.go; then
        echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
        print_success "🎉 All integration tests passed!"
    else
        echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
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
fi

print_success "✨ Integration test run completed successfully!"
print_status "💡 Tip: Use './run-integration-tests.sh --docker' to run tests entirely within Docker containers"
