# Reverse Proxy Service Discovery Brief

## 1. Problem Statement

Currently, when services run on non-standard ports (like the DNS API on port 8080), users must explicitly specify the port in their URLs (e.g., `http://dns-api.internal.jerkytreats.dev:8080`). This creates several usability issues:

- **Poor User Experience**: Users must remember and type port numbers for internal services
- **Inconsistent Access Patterns**: Some services use standard ports (80/443) while others require custom ports
- **Limited Service Discovery**: There's no centralized way to map service names to their actual locations and ports across the Tailscale network

The goal is to enable seamless access to services using standard HTTP/HTTPS URLs without port specifications, while supporting services running on any device in the Tailscale network.

## 2. Proposed Solution

Implement a dynamic reverse proxy system that:

1. **Extends the DNS record API** to accept optional port and target IP information
2. **Maintains dual functionality**: Creates standard DNS A records AND reverse proxy rules
3. **Enables cross-device proxying**: Allows the DNS manager (on omnitron) to proxy requests to services on other devices (like antarus)
4. **Provides transparent port mapping**: Maps standard ports (80/443) to service-specific ports (8080, 3000, etc.)

### Architecture Overview

```
Client Request Flow:
User types: http://llm.internal.jerkytreats.dev
    ↓
DNS Resolution: Points to omnitron (proxy server) IP
    ↓
HTTP Request: Sent to omnitron:80
    ↓
Caddy Reverse Proxy: Forwards to antarus:8080
    ↓
Service Response: Relayed back to client
```

## 3. Technical Design

### A. Extended API Schema

The existing `/add-record` endpoint will be enhanced to accept proxy configuration:

```json
{
  "service_name": "llm-service",
  "name": "llm.internal.jerkytreats.dev",
  "ip": "100.1.1.1",                    // Proxy server IP (omnitron)
  "port": 8080,                         // Target service port
  "forward_to_ip": "100.2.2.2",         // Target service IP (antarus)
  "proxy_enabled": true                  // Enable reverse proxy rule
}
```

### B. New Components

#### 1. Reverse Proxy Manager (`internal/proxy/manager.go`)

```go
type ProxyManager struct {
    configPath    string
    templatePath  string
    rules         map[string]*ProxyRule
    rulesMutex    sync.RWMutex
}

type ProxyRule struct {
    Hostname     string  // llm.internal.jerkytreats.dev
    TargetIP     string  // 100.2.2.2
    TargetPort   int     // 8080
    Protocol     string  // http/https
    Enabled      bool
}

func (m *ProxyManager) AddRule(hostname, targetIP string, targetPort int) error
func (m *ProxyManager) RemoveRule(hostname string) error
func (m *ProxyManager) GenerateConfig() error
func (m *ProxyManager) ReloadProxy() error
```

#### 2. Caddy Integration

Install and configure Caddy as a lightweight reverse proxy within the existing container:

```dockerfile
# Add to Dockerfile.all
RUN wget -q https://github.com/caddyserver/caddy/releases/download/v2.7.6/caddy_2.7.6_linux_amd64.tar.gz \
    && tar -xzf caddy_2.7.6_linux_amd64.tar.gz \
    && mv caddy /usr/local/bin/ \
    && rm caddy_2.7.6_linux_amd64.tar.gz \
    && chmod +x /usr/local/bin/caddy
```

#### 3. Dynamic Caddyfile Generation

Template-based Caddyfile generation:

```caddy
# Caddyfile.template
{{range .ProxyRules}}
{{.Hostname}} {
    reverse_proxy {{.TargetIP}}:{{.TargetPort}}
    log {
        output stdout
        format console
    }
}
{{end}}
```

### C. Integration Points

#### 1. Enhanced Record Handler

Modify `internal/api/handler/record.go`:

```go
type AddRecordRequest struct {
    ServiceName   string `json:"service_name"`
    Name          string `json:"name"`
    IP            string `json:"ip"`
    Port          *int   `json:"port,omitempty"`          // Optional
    ForwardToIP   string `json:"forward_to_ip,omitempty"` // Optional
    ProxyEnabled  bool   `json:"proxy_enabled,omitempty"` // Optional
}

func (h *RecordHandler) AddRecord(w http.ResponseWriter, r *http.Request) {
    // ... existing DNS record creation ...

    // New: Create proxy rule if proxy_enabled and port specified
    if req.ProxyEnabled && req.Port != nil {
        proxyRule := &proxy.ProxyRule{
            Hostname:   req.Name,
            TargetIP:   req.ForwardToIP,
            TargetPort: *req.Port,
            Protocol:   "http",
            Enabled:    true,
        }
        if err := h.proxyManager.AddRule(proxyRule); err != nil {
            // Handle error
        }
    }
}
```

#### 2. Supervisord Integration

Add Caddy to the supervisord configuration:

```ini
[program:caddy]
command=/usr/local/bin/caddy run --config /app/configs/Caddyfile
directory=/app
autostart=true
autorestart=true
startretries=3
redirect_stderr=true
stdout_logfile=/dev/stdout
stdout_logfile_maxbytes=0
```

### D. Container Architecture

The unified container will run three main services:

1. **CoreDNS** (Port 53) - DNS resolution
2. **DNS API** (Port 8080) - Management interface
3. **Caddy Proxy** (Ports 80/443) - Reverse proxy

## 4. Implementation Phases

### Phase 1: Core Reverse Proxy Infrastructure
- Install Caddy in Dockerfile.all
- Create ProxyManager component
- Implement Caddyfile template generation
- Add Caddy to supervisord configuration

### Phase 2: API Integration
- Extend AddRecordRequest schema
- Modify RecordHandler to support proxy rules
- Implement proxy rule creation/deletion
- Add validation for proxy configurations

### Phase 3: Advanced Features
- HTTPS/TLS support for proxied services
- Health checks for target services
- Load balancing for multiple target instances
- Proxy rule persistence and recovery

## 5. Configuration Example

### Input API Call (from antarus)
```bash
curl -X POST http://dns.internal.jerkytreats.dev:8080/add-record \
  -H "Content-Type: application/json" \
  -d '{
    "service_name": "llm-service",
    "name": "llm.internal.jerkytreats.dev",
    "ip": "100.1.1.1",
    "port": 8080,
    "forward_to_ip": "100.2.2.2",
    "proxy_enabled": true
  }'
```

### Generated DNS Record
```
llm.internal.jerkytreats.dev IN A 100.1.1.1
```

### Generated Caddy Rule
```caddy
llm.internal.jerkytreats.dev {
    reverse_proxy 100.2.2.2:8080
}
```

### Result
```bash
# From any device in Tailscale network:
curl http://llm.internal.jerkytreats.dev
# Successfully reaches the LLM service on antarus:8080
```

## 6. Benefits

- **Seamless Service Discovery**: Services become accessible via clean URLs without port specifications
- **Cross-Device Proxying**: Services on any Tailscale device can be proxied through the central DNS manager
- **Backward Compatibility**: Existing DNS functionality remains unchanged
- **Scalable Architecture**: Easy to add new services and proxy rules
- **Standard Web Protocols**: Uses familiar HTTP/HTTPS patterns

## 7. Security Considerations

- All proxy traffic stays within the Tailscale network
- Target services must be reachable from the proxy server
- No external traffic is proxied (only internal Tailscale devices)
- Existing firewall rules protect against unauthorized access

## 8. Testing Strategy

- Unit tests for ProxyManager component
- Integration tests for DNS + Proxy workflows
- End-to-end tests with multiple Tailscale devices
- Performance testing for proxy throughput
- Failure scenario testing (target service down, network partition)
