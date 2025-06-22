#!/bin/bash

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Logging functions
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
echo "    Tailscale Internal DNS Manager Setup"
echo "============================================="
echo

# Check prerequisites
log "Checking prerequisites..."

# Check if Docker is installed
if ! command -v docker &> /dev/null; then
    error "Docker is not installed. Please install Docker and try again."
    exit 1
fi

# Check if Docker Compose is available
if ! docker-compose --version &> /dev/null && ! docker compose version &> /dev/null; then
    error "Docker Compose is not installed. Please install Docker Compose and try again."
    exit 1
fi

# Check if docker is running
if ! docker info > /dev/null 2>&1; then
    error "Docker is not running. Please start Docker and try again."
    exit 1
fi

log "Prerequisites check passed!"
echo

# Step 1: Environment Variables Setup
log "Step 1: Setting up Environment Variables"
echo

# Check if .env file already exists
if [ -f ".env" ]; then
    warn ".env file already exists. Backing up to .env.backup"
    cp .env .env.backup
fi

# Prompt for Tailscale API Key
echo "Please enter your Tailscale API Key:"
echo "You can get this from: https://login.tailscale.com/admin/settings/keys"
read -p "TAILSCALE_API_KEY: " -s TAILSCALE_API_KEY
echo

# Validate API key format
if [[ ! $TAILSCALE_API_KEY =~ ^tskey-api- ]]; then
    error "Invalid Tailscale API Key format. Keys should start with 'tskey-api-'"
    exit 1
fi

# Prompt for Tailnet
echo "Please enter your Tailnet name:"
echo "This is usually your organization name or email domain (e.g., 'your-org' or 'example.com')"
read -p "TAILSCALE_TAILNET: " TAILSCALE_TAILNET

if [ -z "$TAILSCALE_TAILNET" ]; then
    error "Tailnet name cannot be empty"
    exit 1
fi

# Prompt for environment
echo "What environment is this? (development/production) [development]:"
read -p "APP_ENV: " APP_ENV
APP_ENV=${APP_ENV:-development}

# Create .env file
log "Creating .env file..."
cat > .env << EOF
# Tailscale Configuration
TAILSCALE_API_KEY="$TAILSCALE_API_KEY"
TAILSCALE_TAILNET="$TAILSCALE_TAILNET"

# Application Environment
APP_ENV="$APP_ENV"
EOF

log ".env file created successfully!"
echo

# Step 2: Configuration File Setup
log "Step 2: Setting up Configuration File"
echo

# Check if config.yaml already exists
if [ -f "configs/config.yaml" ]; then
    warn "configs/config.yaml already exists. Backing up to configs/config.yaml.backup"
    cp configs/config.yaml configs/config.yaml.backup
fi

# Copy bootstrap example
log "Copying bootstrap example configuration..."
cp configs/config-bootstrap-example.yaml configs/config.yaml

# Prompt for domain customization
echo "What domain will you use for internal DNS? (e.g., internal.yourdomain.com)"
echo "This should be a domain you control for Let's Encrypt certificates."
read -p "Internal Domain: " INTERNAL_DOMAIN

if [ -z "$INTERNAL_DOMAIN" ]; then
    error "Internal domain cannot be empty"
    exit 1
fi

# Prompt for email for Let's Encrypt
echo "What email address should be used for Let's Encrypt certificates?"
read -p "Email: " LETSENCRYPT_EMAIL

if [ -z "$LETSENCRYPT_EMAIL" ]; then
    error "Email cannot be empty"
    exit 1
fi

# Ask about environment (production vs staging)
echo "Do you want to use Let's Encrypt production certificates? (y/n) [n]:"
echo "Note: Use 'n' for development/testing to avoid rate limits"
read -p "Production certificates: " USE_PRODUCTION
USE_PRODUCTION=${USE_PRODUCTION:-n}

if [[ $USE_PRODUCTION =~ ^[Yy]$ ]]; then
    LETSENCRYPT_URL="https://acme-v02.api.letsencrypt.org/directory"
    TLS_ENABLED="true"
    log "Using Let's Encrypt production certificates"
else
    LETSENCRYPT_URL="https://acme-staging-v02.api.letsencrypt.org/directory"
    TLS_ENABLED="false"
    log "Using Let's Encrypt staging certificates"
fi

# Update configuration file
log "Updating configuration file with your settings..."

# Use sed to update the configuration file (macOS compatible)
if [[ "$OSTYPE" == "darwin"* ]]; then
    # macOS
    sed -i '' "s/internal\.jerkytreats\.dev/$INTERNAL_DOMAIN/g" configs/config.yaml
    sed -i '' "s/admin@jerkytreats\.dev/$LETSENCRYPT_EMAIL/g" configs/config.yaml
    sed -i '' "s|https://acme-staging-v02\.api\.letsencrypt\.org/directory|$LETSENCRYPT_URL|g" configs/config.yaml
    sed -i '' "s/enabled: false/enabled: $TLS_ENABLED/g" configs/config.yaml
else
    # Linux
    sed -i "s/internal\.jerkytreats\.dev/$INTERNAL_DOMAIN/g" configs/config.yaml
    sed -i "s/admin@jerkytreats\.dev/$LETSENCRYPT_EMAIL/g" configs/config.yaml
    sed -i "s|https://acme-staging-v02\.api\.letsencrypt\.org/directory|$LETSENCRYPT_URL|g" configs/config.yaml
    sed -i "s/enabled: false/enabled: $TLS_ENABLED/g" configs/config.yaml
fi

log "Configuration file updated successfully!"
echo

# Step 3: Bootstrap Devices Configuration
log "Step 3: Bootstrap Devices Configuration"
echo

info "You need to configure your Tailscale devices in configs/config.yaml"
info "Edit the 'dns.internal.bootstrap_devices' section to match your devices."
echo
info "Example device configuration:"
echo "  - name: \"server\""
echo "    tailscale_name: \"your-actual-tailscale-device-name\""
echo "    aliases: [\"api\", \"dns\"]"
echo "    description: \"Main server\""
echo "    enabled: true"
echo

# Ask if user wants to open the config file
echo "Would you like to edit the configuration file now? (y/n) [y]:"
read -p "Edit config: " EDIT_CONFIG
EDIT_CONFIG=${EDIT_CONFIG:-y}

if [[ $EDIT_CONFIG =~ ^[Yy]$ ]]; then
    if command -v code &> /dev/null; then
        log "Opening configuration file in VS Code..."
        code configs/config.yaml
    elif command -v nano &> /dev/null; then
        log "Opening configuration file in nano..."
        nano configs/config.yaml
    elif command -v vim &> /dev/null; then
        log "Opening configuration file in vim..."
        vim configs/config.yaml
    else
        warn "No suitable editor found. Please manually edit configs/config.yaml"
    fi
else
    warn "Please manually edit configs/config.yaml before deploying"
fi

echo

# Step 4: Final Instructions
log "Setup Complete!"
echo

info "Next steps:"
echo "1. Edit configs/config.yaml to configure your Tailscale devices"
echo "2. Deploy the services with: ./scripts/deploy.sh"
echo "3. Verify deployment with: curl http://localhost:8080/health"
echo "4. Test DNS resolution with: dig @localhost your-device.$INTERNAL_DOMAIN"
echo
info "Useful files created:"
echo "- .env (environment variables)"
echo "- configs/config.yaml (main configuration)"
echo
info "Useful commands:"
echo "- ./scripts/deploy.sh    # Deploy services"
echo "- ./scripts/start.sh     # Start existing services"
echo "- docker-compose logs -f # View logs"
echo "- docker-compose down    # Stop services"
echo

log "Setup script completed successfully!"
