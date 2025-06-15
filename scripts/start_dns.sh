#!/bin/sh

set -e

echo "[INFO] Starting baked-in internal-dns container"

# Optional: mark repo as safe for Git if pulling
git config --global --add safe.directory /volume1/docker/dns || true

cd /volume1/docker/dns

# Optional: pull latest repo updates
# git pull origin main

# Stop and remove any existing container
docker stop internal-dns 2>/dev/null || true
docker rm internal-dns 2>/dev/null || true

# Build fresh image from baked-in files
docker build -t internal-dns .

# Run with no volume mounts
docker run -d \
  --name internal-dns \
  --restart unless-stopped \
  -p 53:53/udp \
  internal-dns

echo "[INFO] internal-dns started using baked Corefile and zone definitions."
