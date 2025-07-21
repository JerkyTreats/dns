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

# Check for optional tools (for auto-detection)
if ! command -v curl &> /dev/null; then
    warn "curl not found. Auto-detection of Tailscale device will be disabled."
fi

if ! command -v jq &> /dev/null; then
    warn "jq not found. Auto-detection of Tailscale device will be disabled."
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

# Use Python for robust template substitution (handles any special characters safely)
if command -v python3 &> /dev/null; then
    log "Using Python for safe template substitution..."

    # Build substitution arguments for Python script
    PYTHON_ARGS=(
        -s "INTERNAL_DOMAIN_PLACEHOLDER" "$INTERNAL_DOMAIN"
        -s "LETSENCRYPT_EMAIL_PLACEHOLDER" "$LETSENCRYPT_EMAIL"
        -s "LETSENCRYPT_URL_PLACEHOLDER" "$LETSENCRYPT_URL"
        -s "TAILSCALE_API_KEY_PLACEHOLDER" "$TAILSCALE_API_KEY"
        -s "TAILSCALE_TAILNET_PLACEHOLDER" "$TAILSCALE_TAILNET"
        -s "SERVER_PORT_PLACEHOLDER" "${SERVER_PORT:-8080}"
    )

    # Run Python template substitution
    python3 scripts/template_substitute.py configs/config.yaml.template configs/config.yaml "${PYTHON_ARGS[@]}"

    if [[ $USE_CLOUDFLARE =~ ^[Yy]$ ]]; then
        python3 scripts/template_substitute.py configs/config.yaml configs/config.yaml \
            -s "provider: \"lego\"" "provider: \"lego\"
  dns_provider: cloudflare"
    fi

    # Only enable TLS if using production certificates
    if [[ $TLS_ENABLED == "true" ]]; then
        python3 scripts/template_substitute.py configs/config.yaml configs/config.yaml \
            -s "enabled: false" "enabled: true"
    fi

elif command -v perl &> /dev/null; then
    log "Python not found, falling back to perl for variable substitution..."

    perl -i -pe "s/INTERNAL_DOMAIN_PLACEHOLDER/\Q$INTERNAL_DOMAIN\E/g" configs/config.yaml
    perl -i -pe "s/LETSENCRYPT_EMAIL_PLACEHOLDER/\Q$LETSENCRYPT_EMAIL\E/g" configs/config.yaml
    perl -i -pe "s|LETSENCRYPT_URL_PLACEHOLDER|\Q$LETSENCRYPT_URL\E|g" configs/config.yaml
    perl -i -pe "s/TAILSCALE_API_KEY_PLACEHOLDER/\Q$TAILSCALE_API_KEY\E/g" configs/config.yaml
    perl -i -pe "s/TAILSCALE_TAILNET_PLACEHOLDER/\Q$TAILSCALE_TAILNET\E/g" configs/config.yaml
    perl -i -pe "s/SERVER_PORT_PLACEHOLDER/\Q${SERVER_PORT:-8080}\E/g" configs/config.yaml

    if [[ $USE_CLOUDFLARE =~ ^[Yy]$ ]]; then
        perl -i -pe 's/(provider: "lego")/$1\n  dns_provider: cloudflare/g' configs/config.yaml
    fi

    # Only enable TLS if using production certificates
    if [[ $TLS_ENABLED == "true" ]]; then
        perl -i -pe 's/enabled: false/enabled: true/g' configs/config.yaml
    fi

else
    error "Neither Python nor Perl found. Please install Python 3 or Perl to continue."
    error "Python 3 is recommended for the most robust template substitution."
    exit 1
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

echo

# Step 5: Final Instructions
log "Setup Complete!"
echo

info "Configuration file created:"
echo "- configs/config.yaml (contains your secrets - not committed to git)"
echo
info "Next steps:"
echo "1. Deploy the services with: ./scripts/deploy.sh"
echo "2. Verify deployment with: curl http://localhost:8080/health"
echo "3. Test DNS resolution with: dig @localhost your-device.$INTERNAL_DOMAIN"
echo
info "Dynamic Configuration Features:"
echo "- CoreDNS configuration is generated dynamically from templates"
echo "- New domains and DNS records are added automatically"
echo "- TLS certificates are integrated automatically when available"
echo "- NS records automatically use the correct Tailscale IP address"
echo "- No manual CoreDNS configuration file editing required"
echo
info "Useful commands:"
echo "- ./scripts/deploy.sh    # Deploy services"
echo "- ./scripts/start.sh     # Start existing services"
echo "- $COMPOSE_CMD logs -f   # View logs"
echo "- $COMPOSE_CMD down      # Stop services"
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
    if command -v python3 &> /dev/null; then
        python3 scripts/template_substitute.py configs/config.yaml configs/config.yaml \
            -s "dns_provider: cloudflare" "dns_provider: cloudflare
  cloudflare_api_token: \"$CF_TOKEN\""
    elif command -v perl &> /dev/null; then
        perl -i -pe "s/(dns_provider: cloudflare)/\$1\n  cloudflare_api_token: \"\Q$CF_TOKEN\E\"/g" configs/config.yaml
    else
        error "Neither Python nor Perl found. Cannot add Cloudflare token."
        exit 1
    fi

    log "Cloudflare token added to configs/config.yaml (git-ignored)."
fi

log "Setup script completed successfully!"

# Step 7: Configure NS device automatically
log "Configuring Tailscale NS device..."
if command -v python3 &> /dev/null; then
    # Check if Python 3 is recent enough (3.6+ for f-strings, pathlib, etc.)
    PYTHON_MAJOR=$(python3 -c "import sys; print(sys.version_info.major)" 2>/dev/null)
    PYTHON_MINOR=$(python3 -c "import sys; print(sys.version_info.minor)" 2>/dev/null)
    if [[ "$PYTHON_MAJOR" -eq 3 && "$PYTHON_MINOR" -ge 6 ]] || [[ "$PYTHON_MAJOR" -gt 3 ]]; then
        python3 scripts/configure_ns.py
        if [ $? -eq 0 ]; then
            log "NS device configuration completed!"
        else
            warn "NS device configuration failed. You may need to manually configure the device_name in configs/config.yaml"
        fi
    else
        warn "Python 3.6+ required for automatic NS configuration. Found: $PYTHON_MAJOR.$PYTHON_MINOR"
        warn "Please manually configure the device_name in configs/config.yaml"
    fi
else
    warn "Python 3 not found. Please manually configure the device_name in configs/config.yaml"
fi
