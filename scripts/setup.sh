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

# Step 2.5: Auto-detect current Tailscale device
log "Detecting current Tailscale device..."
echo

# Function to get Tailscale devices via API
get_tailscale_devices() {
    local api_key="$1"
    local tailnet="$2"

    log "Fetching Tailscale devices from API..."
    local response=$(curl -s -H "Authorization: Bearer $api_key" \
        "https://api.tailscale.com/api/v2/tailnet/$tailnet/devices" 2>/dev/null)

    if [ $? -ne 0 ] || [ -z "$response" ]; then
        return 1
    fi

    echo "$response"
}

# Function to detect current device
detect_current_device() {
    local api_response="$1"
    local current_hostname="$2"

    # Try to find the current device by hostname or IP
    local current_ip=""
    local device_name=""

    # Check if we have a Tailscale IP address on this machine
    if command -v ip &> /dev/null; then
        # Linux
        current_ip=$(ip addr show | grep -o '100\.[0-9]\+\.[0-9]\+\.[0-9]\+' | head -1)
    elif command -v ifconfig &> /dev/null; then
        # macOS/BSD
        current_ip=$(ifconfig | grep -o '100\.[0-9]\+\.[0-9]\+\.[0-9]\+' | head -1)
    fi

    if [ -n "$current_ip" ]; then
        log "Found Tailscale IP on this machine: $current_ip"
        # Extract device name from API response matching this IP
        device_name=$(echo "$api_response" | jq -r --arg ip "$current_ip" '
            .devices[] | select(.addresses[]? == $ip) | .name' 2>/dev/null)
    fi

    # Fallback: try to match by hostname
    if [ -z "$device_name" ]; then
        log "Trying to match by hostname: $current_hostname"
        device_name=$(echo "$api_response" | jq -r --arg hostname "$current_hostname" '
            .devices[] | select(.hostname == $hostname or (.hostname | split(".")[0]) == $hostname) | .name' 2>/dev/null)
    fi

    echo "$device_name"
}

# Auto-detect current device
CURRENT_HOSTNAME=$(hostname)
log "Current hostname: $CURRENT_HOSTNAME"

if command -v curl &> /dev/null && command -v jq &> /dev/null; then
    API_RESPONSE=$(get_tailscale_devices "$TAILSCALE_API_KEY" "$TAILSCALE_TAILNET")

    if [ -n "$API_RESPONSE" ]; then
        DETECTED_DEVICE=$(detect_current_device "$API_RESPONSE" "$CURRENT_HOSTNAME")

        if [ -n "$DETECTED_DEVICE" ]; then
            log "Auto-detected current Tailscale device: $DETECTED_DEVICE"
            echo "Use this device for NS records? (y/n) [y]:"
            read -p "Use detected device: " USE_DETECTED
            USE_DETECTED=${USE_DETECTED:-y}

            if [[ $USE_DETECTED =~ ^[Yy]$ ]]; then
                TAILSCALE_DEVICE_NAME="$DETECTED_DEVICE"
                log "Using detected device: $TAILSCALE_DEVICE_NAME"
            fi
        else
            warn "Could not auto-detect current Tailscale device"
            echo "Available devices in your Tailscale network:"
            echo "$API_RESPONSE" | jq -r '.devices[] | "- \(.name) (\(.hostname)) - \(if .online then "online" else "offline" end)"' 2>/dev/null || echo "Failed to parse device list"
        fi

        # If auto-detection failed or user declined, prompt manually
        if [ -z "$TAILSCALE_DEVICE_NAME" ]; then
            echo
            echo "Please specify which Tailscale device this DNS server should represent:"
            echo "This is important for NS records to point to the correct IP address."
            read -p "Tailscale device name: " TAILSCALE_DEVICE_NAME
        fi
    else
        warn "Failed to fetch Tailscale devices from API. You may need to configure device name manually."
    fi
else
    warn "curl or jq not found. Cannot auto-detect Tailscale device."
    warn "Please install curl and jq for automatic device detection, or configure manually."
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

# Use perl for safer substitutions (handles special characters better than sed)
if command -v perl &> /dev/null; then
    log "Using perl for safe variable substitution..."

    perl -i -pe "s/INTERNAL_DOMAIN_PLACEHOLDER/\Q$INTERNAL_DOMAIN\E/g" configs/config.yaml
    perl -i -pe "s/LETSENCRYPT_EMAIL_PLACEHOLDER/\Q$LETSENCRYPT_EMAIL\E/g" configs/config.yaml
    perl -i -pe "s|LETSENCRYPT_URL_PLACEHOLDER|\Q$LETSENCRYPT_URL\E|g" configs/config.yaml
    perl -i -pe "s/TAILSCALE_API_KEY_PLACEHOLDER/\Q$TAILSCALE_API_KEY\E/g" configs/config.yaml
    perl -i -pe "s/TAILSCALE_TAILNET_PLACEHOLDER/\Q$TAILSCALE_TAILNET\E/g" configs/config.yaml

    # Add Tailscale device name if detected/specified
    if [ -n "$TAILSCALE_DEVICE_NAME" ]; then
        perl -i -pe "s/# device_name: \"your-device-name\"/device_name: \"\Q$TAILSCALE_DEVICE_NAME\E\"/g" configs/config.yaml
        log "Added Tailscale device name: $TAILSCALE_DEVICE_NAME"
    fi

    if [[ $USE_CLOUDFLARE =~ ^[Yy]$ ]]; then
        perl -i -pe 's/(provider: "lego")/$1\n  dns_provider: cloudflare/g' configs/config.yaml
    fi

    # Only enable TLS if using production certificates
    if [[ $TLS_ENABLED == "true" ]]; then
        perl -i -pe 's/enabled: false/enabled: true/g' configs/config.yaml
    fi

else
    # Fallback to sed with manual escaping if perl is not available
    log "Perl not found, using sed with manual escaping..."

    # Function to escape sed replacement strings more carefully
    escape_for_sed() {
        # Escape special characters that can break sed when using | as delimiter
        # Escape backslashes first, then pipes and ampersands
        printf '%s\n' "$1" | sed 's/\\/\\\\/g; s/|/\\|/g; s/&/\\&/g'
    }

    # Escape variables for sed
    ESCAPED_INTERNAL_DOMAIN=$(escape_for_sed "$INTERNAL_DOMAIN")
    ESCAPED_LETSENCRYPT_EMAIL=$(escape_for_sed "$LETSENCRYPT_EMAIL")
    ESCAPED_LETSENCRYPT_URL=$(escape_for_sed "$LETSENCRYPT_URL")
    ESCAPED_TAILSCALE_API_KEY=$(escape_for_sed "$TAILSCALE_API_KEY")
    ESCAPED_TAILSCALE_TAILNET=$(escape_for_sed "$TAILSCALE_TAILNET")
    if [ -n "$TAILSCALE_DEVICE_NAME" ]; then
        ESCAPED_TAILSCALE_DEVICE_NAME=$(escape_for_sed "$TAILSCALE_DEVICE_NAME")
    fi

    # Use sed with safer approach
    if [[ "$OSTYPE" == "darwin"* ]]; then
        # macOS
        sed -i '' "s|INTERNAL_DOMAIN_PLACEHOLDER|$ESCAPED_INTERNAL_DOMAIN|g" configs/config.yaml
        sed -i '' "s|LETSENCRYPT_EMAIL_PLACEHOLDER|$ESCAPED_LETSENCRYPT_EMAIL|g" configs/config.yaml
        sed -i '' "s|LETSENCRYPT_URL_PLACEHOLDER|$ESCAPED_LETSENCRYPT_URL|g" configs/config.yaml
        sed -i '' "s|TAILSCALE_API_KEY_PLACEHOLDER|$ESCAPED_TAILSCALE_API_KEY|g" configs/config.yaml
        sed -i '' "s|TAILSCALE_TAILNET_PLACEHOLDER|$ESCAPED_TAILSCALE_TAILNET|g" configs/config.yaml

        # Add Tailscale device name if detected/specified
        if [ -n "$TAILSCALE_DEVICE_NAME" ]; then
            sed -i '' "s|# device_name: \"your-device-name\"|device_name: \"$ESCAPED_TAILSCALE_DEVICE_NAME\"|g" configs/config.yaml
            log "Added Tailscale device name: $TAILSCALE_DEVICE_NAME"
        fi

        if [[ $USE_CLOUDFLARE =~ ^[Yy]$ ]]; then
            sed -i '' '/provider: "lego"/a\
  dns_provider: cloudflare' configs/config.yaml
        fi

        # Only enable TLS if using production certificates
        if [[ $TLS_ENABLED == "true" ]]; then
            sed -i '' "s|enabled: false|enabled: true|g" configs/config.yaml
        fi
    else
        # Linux
        sed -i "s|INTERNAL_DOMAIN_PLACEHOLDER|$ESCAPED_INTERNAL_DOMAIN|g" configs/config.yaml
        sed -i "s|LETSENCRYPT_EMAIL_PLACEHOLDER|$ESCAPED_LETSENCRYPT_EMAIL|g" configs/config.yaml
        sed -i "s|LETSENCRYPT_URL_PLACEHOLDER|$ESCAPED_LETSENCRYPT_URL|g" configs/config.yaml
        sed -i "s|TAILSCALE_API_KEY_PLACEHOLDER|$ESCAPED_TAILSCALE_API_KEY|g" configs/config.yaml
        sed -i "s|TAILSCALE_TAILNET_PLACEHOLDER|$ESCAPED_TAILSCALE_TAILNET|g" configs/config.yaml

        # Add Tailscale device name if detected/specified
        if [ -n "$TAILSCALE_DEVICE_NAME" ]; then
            sed -i "s|# device_name: \"your-device-name\"|device_name: \"$ESCAPED_TAILSCALE_DEVICE_NAME\"|g" configs/config.yaml
            log "Added Tailscale device name: $TAILSCALE_DEVICE_NAME"
        fi

        if [[ $USE_CLOUDFLARE =~ ^[Yy]$ ]]; then
            sed -i '/provider: "lego"/a\
  dns_provider: cloudflare' configs/config.yaml
        fi

        # Only enable TLS if using production certificates
        if [[ $TLS_ENABLED == "true" ]]; then
            sed -i "s|enabled: false|enabled: true|g" configs/config.yaml
        fi
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
if [ -n "$TAILSCALE_DEVICE_NAME" ]; then
    echo "- Auto-configured Tailscale device: $TAILSCALE_DEVICE_NAME"
fi
echo
info "Next steps:"
echo "1. Edit configs/config.yaml to configure your Tailscale devices (if needed)"
echo "2. Deploy the services with: ./scripts/deploy.sh"
echo "3. Verify deployment with: curl http://localhost:8080/health"
echo "4. Test DNS resolution with: dig @localhost your-device.$INTERNAL_DOMAIN"
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
    if command -v perl &> /dev/null; then
        perl -i -pe "s/(dns_provider: cloudflare)/\$1\n  cloudflare_api_token: \"\Q$CF_TOKEN\E\"/g" configs/config.yaml
    else
        ESCAPED_CF_TOKEN=$(escape_for_sed "$CF_TOKEN")
        if [[ "$OSTYPE" == "darwin"* ]]; then
            sed -i '' "/dns_provider: cloudflare/a\\
  cloudflare_api_token: \"$ESCAPED_CF_TOKEN\"" configs/config.yaml
        else
            sed -i "/dns_provider: cloudflare/a\\
  cloudflare_api_token: \"$ESCAPED_CF_TOKEN\"" configs/config.yaml
        fi
    fi

    log "Cloudflare token added to configs/config.yaml (git-ignored)."
fi

log "Setup script completed successfully!"
