#!/bin/sh
cd /volume1/docker/dns

# Pull latest (optional, only if Git is working)
git pull origin main

# Stop old container if it exists
docker stop internal-dns 2>/dev/null
docker rm internal-dns 2>/dev/null

# Run container (adjust ports if needed)
docker run -d \
  --name internal-dns \
  --restart unless-stopped \
  -p 53:53/udp \
  -v $(pwd)/zones:/zones \
  internal-dns
