#!/bin/bash
set -e

GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

log() { echo -e "${GREEN}[COREDNS]${NC} $1"; }
warn() { echo -e "${YELLOW}[COREDNS]${NC} $1"; }

log "Waiting for CoreDNS configuration to be ready..."

# Wait for Corefile to exist
MAX_WAIT=60
WAIT_COUNT=0
while [ ! -f "/etc/coredns/Corefile" ] && [ $WAIT_COUNT -lt $MAX_WAIT ]; do
    warn "Corefile not found, waiting... ($WAIT_COUNT/$MAX_WAIT)"
    sleep 2
    WAIT_COUNT=$((WAIT_COUNT + 1))
done

if [ ! -f "/etc/coredns/Corefile" ]; then
    echo "ERROR: Corefile not found after $MAX_WAIT attempts"
    exit 1
fi

log "Corefile found, starting CoreDNS..."
exec /usr/local/bin/coredns -conf /etc/coredns/Corefile