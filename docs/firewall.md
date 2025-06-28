# Firewall Management

The DNS management application includes automated firewall protection using `ipset` and `iptables` to secure access to DNS services from Tailscale networks only.

## Overview

This feature automatically:
- Creates an ipset containing your Tailscale CIDR range
- Adds iptables rules to allow traffic from that ipset
- Runs silently in the background without user intervention
- Cleans up rules on application shutdown

## Configuration

**Zero Configuration Required!** The firewall feature automatically configures everything:
- **Ipset Name**: Hardcoded to `tailscale_allowed`
- **CIDR Range**: Automatically set to `100.64.0.0/10` (standard Tailscale CGNAT range)
- **Protocols**: Automatically set to `tcp` and `udp` (required for DNS services)
- **Ports**: Automatically derived from server configuration:
  - Port `53` (DNS)
  - `server.port` (HTTP API)
  - `server.tls.port` (HTTPS API, if TLS enabled)

## Requirements

### System Requirements
- `ipset` package installed
- `iptables` package installed
- Root privileges or appropriate capabilities

### Docker Requirements
When running in Docker, the container needs:
- `privileged: true` OR appropriate capabilities (`NET_ADMIN`, `NET_RAW`)
- Host network access for iptables manipulation

The provided `docker-compose.yml` already includes these requirements.

## Automatic Operation

The firewall operates automatically without any user intervention:

1. **Startup**: Firewall rules are configured when the application starts
2. **Operation**: Rules silently protect your DNS services
3. **Shutdown**: Rules are automatically cleaned up when the application stops

No manual management is required - the firewall "just works" to secure your services.

## Usage Examples

### Basic Usage
Firewall management is enabled automatically when you start the application. No configuration needed!

The firewall automatically configures rules for:
- DNS traffic on port 53 (tcp/udp)
- HTTP API traffic on your configured `server.port`
- HTTPS API traffic on your configured `server.tls.port` (if TLS enabled)

No configuration is required - the firewall automatically manages everything for you!



## Security Considerations

### CIDR Range
- Tailscale CIDR is hardcoded to `100.64.0.0/10` (covering 100.64.0.0 through 100.127.255.255)
- This is the standard CGNAT range used by all Tailscale networks
- No configuration needed - this range is consistent across all Tailscale deployments

### Port Configuration
- Ports are automatically determined from your server configuration
- Always includes DNS port 53 (required for DNS server operation)
- Includes HTTP port from `server.port` configuration
- Includes HTTPS port from `server.tls.port` if TLS is enabled
- No manual port configuration needed - ports match your actual services

### Container Privileges
- `privileged: true` gives full access to host system
- Alternative: use specific capabilities (`NET_ADMIN`, `NET_RAW`) for more restricted access
- Firewall rules affect the host system, not just the container

## Troubleshooting

### Permission Errors
If you see permission errors:
1. Ensure container runs with sufficient privileges
2. Check that `iptables` and `ipset` are available in container
3. Verify host system supports netfilter modules

### Rules Not Applied
If firewall rules aren't working:
1. Review application logs for error messages
2. Manually verify with `iptables -L` and `ipset list`
3. Check that iptables and ipset are available in the container
4. Ensure the container has sufficient privileges

### Host System Impact
- Rules are applied to the host system's netfilter
- Rules persist until explicitly removed or system reboot
- Application automatically cleans up on shutdown

## Integration with Existing Firewalls

This feature adds rules to the existing iptables configuration. It:
- Uses `-I INPUT` to insert rules at the beginning of the INPUT chain
- Does not flush or replace existing rules
- Only manages rules that reference the configured ipset

To avoid conflicts:
- Use unique ipset names
- Review existing iptables rules before enabling
- Test in development environment first

## Manual Verification

After enabling firewall management, you can verify the setup manually:

### Check ipset
```bash
# List all ipsets
ipset list

# Check specific ipset
ipset list tailscale_allowed
```

### Check iptables
```bash
# List INPUT chain rules
iptables -L INPUT -n

# List rules with line numbers
iptables -L INPUT -n --line-numbers
```

### Test connectivity
```bash
# From a Tailscale device, test connectivity
curl http://[DNS_SERVER_TAILSCALE_IP]:8080/health
```
