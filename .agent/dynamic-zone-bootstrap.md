# Dynamic Zone Bootstrap Design Brief

## Problem Statement

### Security Concern
The static zone file `configs/coredns/zones/internal.jerkytreats.dev.db` currently checked into git contains:
- Internal Tailscale IP addresses (100.x.x.x range)
- Network topology information
- Device naming conventions
- Service mappings

This presents a **minor security threat** by exposing internal network architecture, even though it's behind Tailscale.

### Current Static Configuration
```dns
$ORIGIN internal.jerkytreats.dev.
; === Core Devices ===
ns              IN  A   100.65.225.93   ; omnitron (NAS, DNS host)
omnitron        IN  A   100.65.225.93
dns             IN  A   100.65.225.93
macbook         IN  A   100.115.251.3   ; revenantor (MacBook)
dev             IN  A   100.115.251.3
```

## Proposed Solution

### Dynamic Zone Generation with Tailscale Integration
Replace static zone files with **runtime zone generation** using:
1. **Configurable origin domain** - make `internal.jerkytreats.dev` a config value
2. **Tailscale device names** - reference devices by Tailscale name, not IP
3. **Runtime IP resolution** - query Tailscale for current device IPs
4. **Leverage existing add-record logic** - reuse the dynamic record creation system

## Architecture Design

### 1. Configuration Schema Extension

```yaml
# New configuration structure - NO IP ADDRESSES
dns:
  domain: "jerkytreats.dev"  # Base domain
  internal:
    origin: "internal.jerkytreats.dev"  # Internal zone origin
    bootstrap_devices:
      - name: "ns"
        tailscale_name: "omnitron"  # Tailscale device name
        aliases: ["omnitron", "dns"]
        description: "NAS, DNS host"
      - name: "macbook"
        tailscale_name: "revenantor"  # Tailscale device name
        aliases: ["dev", "revenantor"]
        description: "MacBook development"
      - name: "android"
        tailscale_name: "google-pixel-6"  # Tailscale device name
        aliases: ["pixel"]
        description: "Google Pixel 6"
        enabled: false  # Optional devices
  tailscale:
    auth_key: "${TAILSCALE_AUTH_KEY}"  # Environment variable
    api_key: "${TAILSCALE_API_KEY}"    # For API access
    tailnet: "${TAILSCALE_TAILNET}"    # Tailnet identifier
  coredns:
    config_path: "/etc/coredns/Corefile"
    zones_path: "/etc/coredns/zones"
    reload_command: ["systemctl", "reload", "coredns"]
```

### 2. Bootstrap Service Architecture

```go
// New bootstrap service with Tailscale integration
type BootstrapService struct {
    manager       *coredns.Manager
    config        *BootstrapConfig
    tailscale     *TailscaleClient
}

type BootstrapConfig struct {
    Origin    string   `yaml:"origin"`
    Devices   []Device `yaml:"bootstrap_devices"`
}

type Device struct {
    Name           string   `yaml:"name"`
    TailscaleName  string   `yaml:"tailscale_name"`  // NEW: Tailscale device name
    Aliases        []string `yaml:"aliases,omitempty"`
    Description    string   `yaml:"description,omitempty"`
    Enabled        bool     `yaml:"enabled"`
}

// NEW: Tailscale integration
type TailscaleClient struct {
    apiKey   string
    tailnet  string
}

type TailscaleDevice struct {
    Name      string   `json:"name"`
    Hostname  string   `json:"hostname"`
    Addresses []string `json:"addresses"`
    Online    bool     `json:"online"`
}
```

### 3. Component Integration

#### A. Zone Bootstrap Flow with Tailscale Resolution
```
Application Startup
    ↓
1. Load bootstrap configuration
    ↓
2. Initialize Tailscale client
    ↓
3. Check if internal zone exists
    ↓
4. If not exists: CreateInternalZone()
    ↓
5. For each enabled device:
   a. Query Tailscale for device IP
   b. AddRecord(device.Name, resolvedIP)
   c. For each alias: AddRecord(alias, resolvedIP)
    ↓
6. CoreDNS reload
    ↓
Application Ready
```

#### B. Tailscale IP Resolution
```go
// NEW: Runtime IP resolution
func (t *TailscaleClient) GetDeviceIP(deviceName string) (string, error) {
    devices, err := t.ListDevices()
    if err != nil {
        return "", err
    }

    for _, device := range devices {
        if device.Name == deviceName || device.Hostname == deviceName {
            if !device.Online {
                return "", fmt.Errorf("device %s is offline", deviceName)
            }
            // Return the Tailscale IP (typically 100.x.x.x)
            for _, addr := range device.Addresses {
                if strings.HasPrefix(addr, "100.") {
                    return addr, nil
                }
            }
        }
    }
    return "", fmt.Errorf("device %s not found", deviceName)
}

// Use existing manager.AddRecord with resolved IPs
for _, device := range config.Devices {
    if device.Enabled {
        ip, err := tailscale.GetDeviceIP(device.TailscaleName)
        if err != nil {
            logging.Error("Failed to resolve IP for device %s: %v", device.TailscaleName, err)
            continue
        }

        manager.AddRecord("internal", device.Name, ip)
        for _, alias := range device.Aliases {
            manager.AddRecord("internal", alias, ip)
        }
    }
}
```

### 4. New Components Required

#### A. Bootstrap Manager with Tailscale Integration
```go
// internal/dns/bootstrap/manager.go
type Manager struct {
    corednsManager *coredns.Manager
    config         BootstrapConfig
    tailscale      *TailscaleClient
}

func (m *Manager) EnsureInternalZone() error
func (m *Manager) BootstrapDevices() error
func (m *Manager) IsZoneBootstrapped() bool
func (m *Manager) RefreshDeviceIPs() error  // NEW: Refresh IPs from Tailscale
```

#### B. Tailscale Client
```go
// internal/tailscale/client.go
type Client struct {
    apiKey   string
    tailnet  string
    baseURL  string
}

func NewClient(apiKey, tailnet string) *Client
func (c *Client) ListDevices() ([]Device, error)
func (c *Client) GetDevice(nameOrHostname string) (*Device, error)
func (c *Client) IsDeviceOnline(name string) (bool, error)
```

#### C. Configuration Keys
```go
// New configuration constants
const (
    DNSInternalOriginKey     = "dns.internal.origin"
    DNSBootstrapDevicesKey   = "dns.internal.bootstrap_devices"
    TailscaleAPIKeyKey       = "tailscale.api_key"
    TailscaleTailnetKey      = "tailscale.tailnet"
)
```

#### D. Integration Points
```go
// cmd/api/main.go - startup integration
manager := coredns.NewManager(...)
tailscaleClient := tailscale.NewClient(
    config.GetString(TailscaleAPIKeyKey),
    config.GetString(TailscaleTailnetKey),
)
bootstrap := bootstrap.NewManager(manager, config.GetBootstrapConfig(), tailscaleClient)

// Bootstrap internal zone with live Tailscale data
if err := bootstrap.EnsureInternalZone(); err != nil {
    logging.Error("Failed to bootstrap internal zone: %v", err)
    return err
}
```

## Implementation Plan

### Phase 1: Tailscale Integration Infrastructure
1. **Create Tailscale client** - API integration for device lookup
2. **Extend config schema** - Add Tailscale device names, remove IPs
3. **Add configuration validation** - Ensure Tailscale credentials present

### Phase 2: Dynamic IP Resolution
1. **Implement device IP resolution** - Query Tailscale for current IPs
2. **Error handling** - Handle offline devices, API failures
3. **Caching strategy** - Cache resolved IPs with TTL

### Phase 3: Bootstrap Service Enhancement
1. **Update bootstrap manager** - Integrate Tailscale IP resolution
2. **Zone refresh capability** - Periodic IP updates from Tailscale
3. **Device status monitoring** - Track online/offline status

### Phase 4: Integration & Testing
1. **Startup integration** - Call bootstrap with Tailscale resolution
2. **Configuration migration** - Move from IP-based to name-based config
3. **Testing** - Mock Tailscale API for unit tests, integration tests

### Phase 5: Security Hardening
1. **Remove static zone file** - Delete from git, add to .gitignore
2. **Environment separation** - Different Tailscale credentials per environment
3. **Sensitive config handling** - Use environment variables for API keys

## Security Benefits

### Immediate Gains
- ✅ **Zero IP addresses in configuration** - Complete IP elimination from VCS
- ✅ **Dynamic IP adaptation** - Automatically handles Tailscale IP changes
- ✅ **Environment-specific configs** - Different device sets per environment
- ✅ **Real-time device status** - Only bootstrap online devices

### Long-term Advantages
- **Automatic network updates** - No manual IP management needed
- **Device lifecycle management** - Automatic handling of device changes
- **Audit trail** - All changes tracked through Tailscale
- **Zero-trust principles** - Always verify device status at runtime

## Migration Strategy

### Backward Compatibility
1. **Detect static zone files** - Check for existing zone files on startup
2. **Migration warning** - Log migration advice if static files detected
3. **Graceful fallback** - Continue with static files if Tailscale unavailable

### Deployment Process
1. **Deploy new code** with Tailscale integration
2. **Configure Tailscale credentials** in environment variables
3. **Add bootstrap device configuration** with Tailscale names
4. **Verify bootstrap functionality** in staging environment
5. **Remove static zone files** from git
6. **Update .gitignore** to prevent future static zone commits

## Configuration Example

### Development Environment
```yaml
dns:
  internal:
    origin: "internal.dev.jerkytreats.dev"
    bootstrap_devices:
      - name: "dev-machine"
        tailscale_name: "dev-laptop"
        aliases: ["dev"]
        enabled: true
tailscale:
  api_key: "${TAILSCALE_DEV_API_KEY}"
  tailnet: "${TAILSCALE_DEV_TAILNET}"
```

### Production Environment
```yaml
dns:
  internal:
    origin: "internal.jerkytreats.dev"
    bootstrap_devices:
      - name: "ns"
        tailscale_name: "omnitron"  # No IP addresses!
        aliases: ["omnitron", "dns"]
        enabled: true
      - name: "macbook"
        tailscale_name: "revenantor"
        aliases: ["dev", "revenantor"]
        enabled: true
tailscale:
  api_key: "${TAILSCALE_API_KEY}"
  tailnet: "${TAILSCALE_TAILNET}"
```

## Tailscale API Integration

### API Authentication
```bash
# Environment variables for Tailscale access
export TAILSCALE_API_KEY="tskey-api-xxxxx"
export TAILSCALE_TAILNET="example@gmail.com"
```

### Device Resolution Examples
```go
// Get current IP for a device
func (c *TailscaleClient) GetDeviceIP(name string) (string, error) {
    resp, err := http.Get(fmt.Sprintf(
        "https://api.tailscale.com/api/v2/tailnet/%s/devices",
        c.tailnet,
    ))
    // Parse response and extract IP for device name
}

// Check if device is online
func (c *TailscaleClient) IsDeviceOnline(name string) (bool, error) {
    device, err := c.GetDevice(name)
    if err != nil {
        return false, err
    }
    return device.Online, nil
}
```

## Risk Assessment

### Low Risk Changes
- ✅ Leverages existing AddRecord infrastructure
- ✅ Graceful fallback to static files during transition
- ✅ Configuration-driven approach (familiar pattern)
- ✅ Well-documented Tailscale API

### Mitigation Strategies
- **Comprehensive testing** - Mock Tailscale API for unit tests
- **Phased rollout** - Deploy to staging first
- **Monitoring** - Alert on Tailscale API failures and bootstrap failures
- **Caching** - Cache resolved IPs to handle temporary API outages
- **Documentation** - Clear migration instructions and troubleshooting

## Testing Requirements

### Unit Tests Required

#### A. Tailscale Client Tests (`internal/tailscale/client_test.go`)
```go
func TestTailscaleClient_ListDevices(t *testing.T)
func TestTailscaleClient_GetDevice(t *testing.T)
func TestTailscaleClient_GetDeviceIP(t *testing.T)
func TestTailscaleClient_IsDeviceOnline(t *testing.T)
func TestTailscaleClient_HandleAPIErrors(t *testing.T)
func TestTailscaleClient_InvalidTailnet(t *testing.T)
func TestTailscaleClient_MalformedResponse(t *testing.T)
```

**Mock Strategy**: Use `httptest.Server` to simulate Tailscale API responses
```go
func setupMockTailscaleAPI(t *testing.T) *httptest.Server {
    return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        switch r.URL.Path {
        case "/api/v2/tailnet/test-tailnet/devices":
            mockDevices := []TailscaleDevice{
                {Name: "omnitron", Addresses: []string{"100.65.225.93"}, Online: true},
                {Name: "revenantor", Addresses: []string{"100.115.251.3"}, Online: true},
                {Name: "offline-device", Addresses: []string{"100.1.1.1"}, Online: false},
            }
            json.NewEncoder(w).Encode(map[string]interface{}{"devices": mockDevices})
        }
    }))
}
```

#### B. Bootstrap Manager Tests (`internal/dns/bootstrap/manager_test.go`)
```go
func TestBootstrapManager_EnsureInternalZone(t *testing.T)
func TestBootstrapManager_BootstrapDevices(t *testing.T)
func TestBootstrapManager_IsZoneBootstrapped(t *testing.T)
func TestBootstrapManager_RefreshDeviceIPs(t *testing.T)
func TestBootstrapManager_HandleOfflineDevices(t *testing.T)
func TestBootstrapManager_HandleTailscaleFailure(t *testing.T)
func TestBootstrapManager_SkipExistingZone(t *testing.T)
func TestBootstrapManager_CreateZoneWithAliases(t *testing.T)
func TestBootstrapManager_ConfigurationValidation(t *testing.T)
```

**Test Scenarios**:
- ✅ Bootstrap with all devices online
- ✅ Bootstrap with some devices offline (should skip gracefully)
- ✅ Bootstrap when zone already exists (should not overwrite)
- ✅ Tailscale API failure (should handle gracefully)
- ✅ Invalid device configurations
- ✅ IP resolution for devices with multiple addresses
- ✅ Alias creation for devices

#### C. Configuration Tests (`internal/config/bootstrap_test.go`)
```go
func TestBootstrapConfig_LoadFromYAML(t *testing.T)
func TestBootstrapConfig_ValidateDevices(t *testing.T)
func TestBootstrapConfig_ValidateTailscaleCredentials(t *testing.T)
func TestBootstrapConfig_MissingRequiredFields(t *testing.T)
func TestBootstrapConfig_DuplicateDeviceNames(t *testing.T)
func TestBootstrapConfig_InvalidTailscaleNames(t *testing.T)
```

#### D. Integration with Existing CoreDNS Manager Tests
```go
func TestBootstrap_WithExistingManager(t *testing.T)
func TestBootstrap_ZoneFileCreation(t *testing.T)
func TestBootstrap_RecordPersistence(t *testing.T)
func TestBootstrap_CoreDNSReload(t *testing.T)
```

### Integration Tests Required

#### A. Full Bootstrap Flow Tests (`tests/bootstrap_integration_test.go`)
```go
func TestIntegration_FullBootstrapFlow(t *testing.T) {
    // 1. Start with empty zones directory
    // 2. Configure mock Tailscale API with test devices
    // 3. Start DNS manager with bootstrap configuration
    // 4. Verify zone file created with correct records
    // 5. Verify CoreDNS can resolve bootstrap devices
    // 6. Verify API can still add additional records
}

func TestIntegration_BootstrapWithOfflineDevices(t *testing.T) {
    // Test bootstrap when some Tailscale devices are offline
}

func TestIntegration_BootstrapSkipExisting(t *testing.T) {
    // Test that bootstrap doesn't overwrite existing zone files
}

func TestIntegration_TailscaleAPIFailure(t *testing.T) {
    // Test graceful handling when Tailscale API is unavailable
}
```

#### B. DNS Resolution Integration Tests
```go
func TestIntegration_DNSResolutionAfterBootstrap(t *testing.T) {
    // 1. Bootstrap internal zone
    // 2. Query CoreDNS for bootstrap device names
    // 3. Verify correct IP addresses returned
    // 4. Verify aliases resolve to same IPs
}

func TestIntegration_MixedBootstrapAndRuntimeRecords(t *testing.T) {
    // 1. Bootstrap with initial devices
    // 2. Add runtime records via API
    // 3. Restart service
    // 4. Verify both bootstrap and runtime records persist
    // 5. Verify no duplicate bootstrap records created
}
```

#### C. Docker Compose Test Environment (`docker-compose.bootstrap-test.yml`)
```yaml
version: '3.8'
services:
  api-bootstrap-test:
    build:
      context: .
      dockerfile: Dockerfile.api
    environment:
      - CONFIG_PATH=/app/configs/bootstrap-test.yaml
      - TAILSCALE_API_KEY=mock-api-key
      - TAILSCALE_TAILNET=test-tailnet
    volumes:
      - ./tests/configs:/app/configs
      - ./tests/zones:/etc/coredns/zones
    ports:
      - "8082:8080"
    depends_on:
      - coredns-bootstrap-test
      - tailscale-mock

  coredns-bootstrap-test:
    build:
      context: .
      dockerfile: Dockerfile
    volumes:
      - ./tests/configs/coredns-bootstrap-test:/etc/coredns
      - ./tests/zones:/zones
    ports:
      - "1054:53/udp"

  tailscale-mock:
    image: mockserver/mockserver:latest
    ports:
      - "1080:1080"
    environment:
      - MOCKSERVER_INITIALIZATION_JSON_PATH=/config/tailscale-mock.json
    volumes:
      - ./tests/mocks:/config
```

#### D. Mock Tailscale API Server (`tests/mocks/tailscale-mock.json`)
```json
[
  {
    "httpRequest": {
      "method": "GET",
      "path": "/api/v2/tailnet/test-tailnet/devices"
    },
    "httpResponse": {
      "statusCode": 200,
      "body": {
        "devices": [
          {
            "name": "omnitron",
            "hostname": "omnitron.test-tailnet.ts.net",
            "addresses": ["100.65.225.93", "fd7a:115c:a1e0::1"],
            "online": true
          },
          {
            "name": "revenantor",
            "hostname": "revenantor.test-tailnet.ts.net",
            "addresses": ["100.115.251.3", "fd7a:115c:a1e0::2"],
            "online": true
          },
          {
            "name": "offline-device",
            "hostname": "offline.test-tailnet.ts.net",
            "addresses": ["100.1.1.1"],
            "online": false
          }
        ]
      }
    }
  }
]
```

### Test Configuration Files

#### A. Bootstrap Test Config (`tests/configs/bootstrap-test.yaml`)
```yaml
dns:
  domain: "test.local"
  internal:
    origin: "internal.test.local"
    bootstrap_devices:
      - name: "ns"
        tailscale_name: "omnitron"
        aliases: ["omnitron", "dns"]
        enabled: true
      - name: "dev"
        tailscale_name: "revenantor"
        aliases: ["macbook", "revenantor"]
        enabled: true
      - name: "offline"
        tailscale_name: "offline-device"
        enabled: true  # Should be skipped due to offline status
  coredns:
    config_path: "/etc/coredns/Corefile"
    zones_path: "/zones"
    reload_command: []

tailscale:
  api_key: "mock-api-key"
  tailnet: "test-tailnet"
  base_url: "http://tailscale-mock:1080"  # Point to mock server
```

### Test Execution Strategy

#### A. Unit Test Execution
```bash
# All bootstrap-related unit tests
go test ./internal/tailscale/... -v
go test ./internal/dns/bootstrap/... -v
go test ./internal/config/... -v -run Bootstrap

# With coverage
go test ./internal/tailscale/... -coverprofile=tailscale.out
go test ./internal/dns/bootstrap/... -coverprofile=bootstrap.out
```

#### B. Integration Test Execution
```bash
# Bootstrap integration tests
docker-compose -f docker-compose.bootstrap-test.yml up -d --build
go test -tags=bootstrap_integration ./tests/... -v
docker-compose -f docker-compose.bootstrap-test.yml down
```

#### C. End-to-End Test Scenarios
```go
func TestE2E_BootstrapToProduction(t *testing.T) {
    // Comprehensive test simulating production bootstrap scenario
    // 1. Clean environment
    // 2. Bootstrap with production-like config
    // 3. Verify all expected records created
    // 4. Add runtime records via API
    // 5. Simulate restart
    // 6. Verify persistence and no duplication
}
```

### Test Coverage Requirements

#### Minimum Coverage Targets
- **Tailscale Client**: 95% line coverage
- **Bootstrap Manager**: 90% line coverage
- **Configuration Loading**: 85% line coverage
- **Integration Tests**: All critical paths covered

#### Critical Test Scenarios
- ✅ **Happy Path**: All devices online, successful bootstrap
- ✅ **Partial Failure**: Some devices offline, bootstrap continues
- ✅ **API Failure**: Tailscale API unavailable, graceful fallback
- ✅ **Configuration Errors**: Invalid configs handled properly
- ✅ **Persistence**: Zone files survive restarts correctly
- ✅ **No Duplication**: Bootstrap doesn't duplicate existing records
- ✅ **Mixed Records**: Bootstrap + runtime records coexist properly

### Continuous Integration Requirements

#### A. GitHub Actions Workflow (`.github/workflows/bootstrap-tests.yml`)
```yaml
name: Bootstrap Tests
on: [push, pull_request]
jobs:
  unit-tests:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v4
        with:
          go-version: '1.21'
      - name: Run Bootstrap Unit Tests
        run: |
          go test ./internal/tailscale/... -v
          go test ./internal/dns/bootstrap/... -v

  integration-tests:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - name: Run Bootstrap Integration Tests
        run: |
          docker-compose -f docker-compose.bootstrap-test.yml up -d --build
          go test -tags=bootstrap_integration ./tests/... -v
          docker-compose -f docker-compose.bootstrap-test.yml down
```

## Success Criteria

### Primary Objectives
1. ✅ **Zero IP addresses in configuration files**
2. ✅ **Dynamic IP resolution from Tailscale working**
3. ✅ **Existing functionality preserved**
4. ✅ **Automatic device status handling**

### Testing Verification Checklist
- [ ] **Unit Tests**: All new components have >90% test coverage
- [ ] **Integration Tests**: Full bootstrap flow tested end-to-end
- [ ] **Mock Integration**: Tailscale API properly mocked for testing
- [ ] **Error Handling**: All failure scenarios tested and handled gracefully
- [ ] **Persistence**: Zone file persistence verified across restarts
- [ ] **DNS Resolution**: CoreDNS properly serves bootstrap records
- [ ] **Mixed Records**: Bootstrap + runtime records coexist without conflicts
- [ ] **Configuration Validation**: Invalid configs properly rejected
- [ ] **Offline Devices**: Offline devices skipped with appropriate logging
- [ ] **API Compatibility**: Existing AddRecord API continues to work

### Verification Tests
- [ ] Bootstrap creates zone with all online devices from Tailscale
- [ ] Offline devices are skipped with appropriate logging
- [ ] IP changes in Tailscale automatically reflected in DNS
- [ ] Existing AddRecord API continues to work
- [ ] Zone reload functionality preserved
- [ ] Integration tests pass with Tailscale mocking
- [ ] No static zone files or IP addresses remain in repository
- [ ] All unit tests pass with required coverage thresholds
- [ ] End-to-end tests demonstrate complete functionality

This enhanced design completely eliminates IP addresses from configuration while providing dynamic, real-time device management through Tailscale integration, backed by comprehensive testing at all levels.
