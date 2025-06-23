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

# Check if template exists
if [ ! -f "configs/config.yaml.template" ]; then
    error "Configuration template configs/config.yaml.template not found!"
    exit 1
fi

log "Prerequisites check passed!"
echo

# Step 1: Check if config already exists
if [ -f "configs/config.yaml" ]; then
    warn "Configuration file configs/config.yaml already exists."
    echo "Do you want to recreate it? This will overwrite your current configuration. (y/n) [n]:"
    read -p "Recreate config: " RECREATE_CONFIG
    RECREATE_CONFIG=${RECREATE_CONFIG:-n}

    if [[ ! $RECREATE_CONFIG =~ ^[Yy]$ ]]; then
        log "Keeping existing configuration. Exiting setup."
        exit 0
    fi

    # Backup existing config
    log "Backing up existing config to configs/config.yaml.backup"
    cp configs/config.yaml configs/config.yaml.backup
fi

# Step 2: Environment Variables Setup
log "Step 1: Setting up Configuration"
echo

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

# Ask whether to use Cloudflare as public DNS provider for ACME
echo "Do you want to use Cloudflare for ACME DNS challenges (cloudflare DNS-01)? (y/n) [n]:"
read -p "Use Cloudflare: " USE_CLOUDFLARE
USE_CLOUDFLARE=${USE_CLOUDFLARE:-n}

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

# Step 3: Create configuration file from template
log "Creating configuration file from template..."

# Copy template to config file
cp configs/config.yaml.template configs/config.yaml

# Substitute placeholders in the config file
log "Substituting configuration values..."

# Use sed to replace placeholders (macOS compatible)
if [[ "$OSTYPE" == "darwin"* ]]; then
    # macOS
    sed -i '' "s/INTERNAL_DOMAIN_PLACEHOLDER/$INTERNAL_DOMAIN/g" configs/config.yaml
    sed -i '' "s/LETSENCRYPT_EMAIL_PLACEHOLDER/$LETSENCRYPT_EMAIL/g" configs/config.yaml
    sed -i '' "s|LETSENCRYPT_URL_PLACEHOLDER|$LETSENCRYPT_URL|g" configs/config.yaml
    sed -i '' "s/TAILSCALE_API_KEY_PLACEHOLDER/$TAILSCALE_API_KEY/g" configs/config.yaml
    sed -i '' "s/TAILSCALE_TAILNET_PLACEHOLDER/$TAILSCALE_TAILNET/g" configs/config.yaml
    if [[ $USE_CLOUDFLARE =~ ^[Yy]$ ]]; then
        sed -i '' "/provider: \"lego\"/a\  dns_provider: cloudflare" configs/config.yaml
    fi
    # Only enable TLS if using production certificates
    if [[ $TLS_ENABLED == "true" ]]; then
        sed -i '' "s/enabled: false/enabled: true/g" configs/config.yaml
    fi
else
    # Linux
    sed -i "s/INTERNAL_DOMAIN_PLACEHOLDER/$INTERNAL_DOMAIN/g" configs/config.yaml
    sed -i "s/LETSENCRYPT_EMAIL_PLACEHOLDER/$LETSENCRYPT_EMAIL/g" configs/config.yaml
    sed -i "s|LETSENCRYPT_URL_PLACEHOLDER|$LETSENCRYPT_URL|g" configs/config.yaml
    sed -i "s/TAILSCALE_API_KEY_PLACEHOLDER/$TAILSCALE_API_KEY/g" configs/config.yaml
    sed -i "s/TAILSCALE_TAILNET_PLACEHOLDER/$TAILSCALE_TAILNET/g" configs/config.yaml
    if [[ $USE_CLOUDFLARE =~ ^[Yy]$ ]]; then
        sed -i "/provider: \"lego\"/a\  dns_provider: cloudflare" configs/config.yaml
    fi
    # Only enable TLS if using production certificates
    if [[ $TLS_ENABLED == "true" ]]; then
        sed -i "s/enabled: false/enabled: true/g" configs/config.yaml
    fi
fi

log "Configuration file created successfully!"

# Setup directories for dynamic CoreDNS configuration
log "Setting up directories for dynamic CoreDNS configuration..."
mkdir -p ssl/ configs/coredns/zones/
chmod 755 configs/coredns/
chmod 755 configs/coredns/zones/

# Ensure the Corefile template exists
if [ ! -f "configs/coredns/Corefile.template" ]; then
    error "CoreDNS template file configs/coredns/Corefile.template not found!"
    error "This file should be committed to the repository."
    exit 1
fi

# Set proper permissions on template files for Docker containers
log "Setting proper permissions on configuration files..."
chmod 644 configs/coredns/Corefile.template
if [ -d "tests/configs/coredns-test" ]; then
    chmod 644 tests/configs/coredns-test/Corefile.template 2>/dev/null || true
fi

log "Dynamic CoreDNS configuration setup complete"
info "The application will generate the Corefile dynamically from the template"
info "No static Corefile is needed - configuration is template-based"

echo

# Step 4: Bootstrap Devices Configuration
log "Step 2: Bootstrap Devices Configuration"
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

# Step 5: Final Instructions
log "Setup Complete!"
echo

info "Configuration file created:"
echo "- configs/config.yaml (contains your secrets - not committed to git)"
echo
info "Next steps:"
echo "1. Edit configs/config.yaml to configure your Tailscale devices"
echo "2. Deploy the services with: ./scripts/deploy.sh"
echo "3. Verify deployment with: curl http://localhost:8080/health"
echo "4. Test DNS resolution with: dig @localhost your-device.$INTERNAL_DOMAIN"
echo
info "Dynamic Configuration Features:"
echo "- CoreDNS configuration is generated dynamically from templates"
echo "- New domains and DNS records are added automatically"
echo "- TLS certificates are integrated automatically when available"
echo "- No manual CoreDNS configuration file editing required"
echo
info "Useful commands:"
echo "- ./scripts/deploy.sh    # Deploy services"
echo "- ./scripts/start.sh     # Start existing services"
echo "- docker-compose logs -f # View logs"
echo "- docker-compose down    # Stop services"
echo

# Step 6: Handle Cloudflare token in .env if needed
if [[ $USE_CLOUDFLARE =~ ^[Yy]$ ]]; then
    echo
    log "Cloudflare DNS provider selected. A Cloudflare API token is required."
    echo "Please create a token with DNS:Edit permission for the 'jerkytreats.dev' zone."
    read -p "Cloudflare API Token: " -s CF_TOKEN
    echo

    if [ -z "$CF_TOKEN" ]; then
        error "Cloudflare API token cannot be empty when Cloudflare provider is selected."
        exit 1
    fi

    # Insert token into certificate section
    if [[ "$OSTYPE" == "darwin"* ]]; then
        sed -i '' "/dns_provider: cloudflare/a\  cloudflare_api_token: \"$CF_TOKEN\"" configs/config.yaml
    else
        sed -i "/dns_provider: cloudflare/a\  cloudflare_api_token: \"$CF_TOKEN\"" configs/config.yaml
    fi

    log "Cloudflare token added to configs/config.yaml (git-ignored)."
fi

log "Setup script completed successfully!"
