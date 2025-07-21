#!/bin/bash

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

log() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

# Get the directory where the script is located
SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

# Change to project root
cd "$PROJECT_ROOT"

echo "============================================="
echo "    Building Unified DNS Manager for Unraid"
echo "============================================="
echo

# Check if Docker is available
if ! command -v docker &> /dev/null; then
    error "Docker is not installed or not in PATH"
    exit 1
fi

# Check if Docker is running
if ! docker info > /dev/null 2>&1; then
    error "Docker is not running. Please start Docker and try again."
    exit 1
fi

# Default values
IMAGE_NAME="jerkytreats/dns"
TAG="unified"
PUSH="false"
PLATFORM=""

# Parse command line arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --push)
            PUSH="true"
            shift
            ;;
        --tag)
            TAG="$2"
            shift 2
            ;;
        --name)
            IMAGE_NAME="$2"
            shift 2
            ;;
        --platform)
            PLATFORM="--platform $2"
            shift 2
            ;;
        --help)
            echo "Usage: $0 [OPTIONS]"
            echo "Options:"
            echo "  --push              Push image to registry after building"
            echo "  --tag TAG           Docker image tag (default: unified)"
            echo "  --name NAME         Docker image name (default: jerkytreats/dns)"
            echo "  --platform PLATFORM Build for specific platform (e.g., linux/amd64,linux/arm64)"
            echo "  --help              Show this help message"
            exit 0
            ;;
        *)
            error "Unknown option: $1"
            echo "Use --help for usage information"
            exit 1
            ;;
    esac
done

FULL_IMAGE_NAME="${IMAGE_NAME}:${TAG}"

log "Building unified DNS manager container..."
log "Image: $FULL_IMAGE_NAME"
log "Dockerfile: Dockerfile.all"

# Build the image
if [ -n "$PLATFORM" ]; then
    log "Building for platform(s): ${PLATFORM#--platform }"
    if [ "$PUSH" = "true" ]; then
        docker buildx build $PLATFORM --push -f Dockerfile.all -t "$FULL_IMAGE_NAME" .
    else
        docker buildx build $PLATFORM --load -f Dockerfile.all -t "$FULL_IMAGE_NAME" .
    fi
else
    docker build -f Dockerfile.all -t "$FULL_IMAGE_NAME" .
fi

if [ $? -eq 0 ]; then
    log "Build completed successfully!"
else
    error "Build failed!"
    exit 1
fi

# Show image info
log "Image information:"
docker images "$IMAGE_NAME" --filter "reference=$FULL_IMAGE_NAME"

# Push if requested
if [ "$PUSH" = "true" ] && [ -z "$PLATFORM" ]; then
    log "Pushing image to registry..."
    docker push "$FULL_IMAGE_NAME"
    if [ $? -eq 0 ]; then
        log "Push completed successfully!"
    else
        error "Push failed!"
        exit 1
    fi
fi

echo
log "Build script completed!"
if [ "$PUSH" = "false" ]; then
    info "To push the image to registry, run:"
    echo "  docker push $FULL_IMAGE_NAME"
    echo "Or use: $0 --push"
fi
echo
info "To test locally:"
echo "  docker-compose -f docker-compose.unraid.yml up"
echo
info "For Unraid deployment:"
echo "  Use image: $FULL_IMAGE_NAME"
echo "  Template: unraid-template.xml"
