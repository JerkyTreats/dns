# Dynamic CoreDNS Configuration Management

## Problem Statement

The current static `Corefile` configuration creates a chicken-and-egg problem during first-time setup:
1. CoreDNS fails to start due to hardcoded TLS certificate paths that don't exist
2. Certificates can't be obtained because CoreDNS isn't running to serve DNS challenges
3. System is stuck in a loop preventing successful initialization

## Solution Overview

Implement **Dynamic CoreDNS Configuration Management** that:
- Starts with a minimal, certificate-free configuration
- Dynamically adds domain configurations based on application config
- Progressively enables TLS when certificates become available
- Manages CoreDNS restarts when configuration changes

## Design Changes Required

### 1. Remove Static Configuration
- Remove `configs/coredns/Corefile`
- Generate Corefile dynamically at runtime

### 2. Dynamic Configuration Manager
- Create `CoreDNSConfigManager` to generate and manage Corefile
- Read internal domain from application configuration
- Generate minimal initial configuration
- Add domain blocks dynamically
- Handle TLS configuration when certificates are available

### 3. Certificate Integration
- Certificate manager triggers CoreDNS configuration updates
- Add TLS routes only after certificate validation
- Coordinate between certificate and DNS managers

## Implementation Plan

### Phase 1: Core Infrastructure

#### A. Dynamic Configuration Generator
```go
// internal/dns/coredns/config_manager.go
type ConfigManager struct {
    basePath     string
    domain       string
    tlsEnabled   bool
    certificates map[string]*TLSConfig
}

type TLSConfig struct {
    CertFile string
    KeyFile  string
    Port     int
}

func (cm *ConfigManager) GenerateCorefile() error
func (cm *ConfigManager) AddDomain(domain string, tlsConfig *TLSConfig) error
func (cm *ConfigManager) RemoveDomain(domain string) error
func (cm *ConfigManager) EnableTLS(domain string, certPath, keyPath string) error
func (cm *ConfigManager) WriteConfig() error
func (cm *ConfigManager) RestartCoreDNS() error
```

#### B. Startup Sequence
```go
// cmd/api/main.go - Updated startup order
1. Initialize Configuration
2. Create CoreDNS Config Manager
3. Generate minimal Corefile (no TLS)
4. Start CoreDNS container
5. Wait for CoreDNS to be ready
6. Initialize Certificate Manager
7. Start API Server
8. Begin certificate obtainment (async)
9. Update CoreDNS config when certificates ready
```

### Phase 2: Integration Points

#### A. Certificate Manager Integration
```go
// internal/certificate/manager.go
func (m *Manager) OnCertificateObtained(domain string, certPath, keyPath string) {
    // Notify CoreDNS config manager to enable TLS
    m.corednsConfigManager.EnableTLS(domain, certPath, keyPath)
}
```

#### B. Bootstrap Integration
```go
// internal/dns/bootstrap/manager.go
func (m *Manager) EnsureInternalZone() error {
    // Use dynamic config manager instead of static files
    return m.configManager.AddDomain(m.config.Origin, nil)
}
```

## File Changes Required

### New Files

#### 1. `internal/dns/coredns/config_manager.go`
- **Purpose**: Dynamic CoreDNS configuration generation and management
- **Key Functions**:
  - Generate base Corefile template
  - Add/remove domain configurations
  - Manage TLS configuration
  - Handle CoreDNS restarts

#### 2. `configs/coredns/Corefile.template`
- **Purpose**: Base template for dynamic generation
- **Content**: Minimal configuration without hardcoded domains
```
. {
    errors
    log
    forward . /etc/resolv.conf
}

# Dynamic domain configurations will be appended here
# Generated by internal/dns/coredns/config_manager.go
```

#### 3. `internal/dns/coredns/restart_manager.go`
- **Purpose**: Handle CoreDNS container restarts
- **Key Functions**:
  - Docker container restart coordination
  - Health checks after restart
  - Rollback on failure

### Modified Files

#### 1. `cmd/api/main.go`
- **Changes**: Updated startup sequence
- **New Dependencies**: ConfigManager initialization
- **Key Changes**:
  - Initialize ConfigManager before CoreDNS
  - Generate initial Corefile
  - Coordinate between managers

#### 2. `internal/dns/coredns/manager.go`
- **Changes**: Integration with ConfigManager
- **New Methods**:
  - `SetConfigManager(cm *ConfigManager)`
  - Update `AddZone` to use dynamic config
  - Update `updateConfig` to use ConfigManager

#### 3. `internal/certificate/manager.go`
- **Changes**: Add CoreDNS integration hooks
- **New Methods**:
  - `SetCoreDNSManager(cm *ConfigManager)`
  - `notifyTLSReady(domain, certPath, keyPath string)`

#### 4. `docker-compose.yml`
- **Changes**: Update CoreDNS volume mounting
- **Modifications**:
  - Remove static Corefile dependency
  - Add restart policy for dynamic updates

#### 5. `scripts/setup.sh`
- **Changes**: Remove Corefile permission setup
- **Additions**: Create template directory structure

### Removed Files

#### 1. `configs/coredns/Corefile`
- **Action**: Remove from git, add to .gitignore
- **Replacement**: Generated dynamically at runtime

## Startup Operation Sequence

### Initial Boot (No Certificates)
```
1. main.go: Load application configuration
2. main.go: Initialize CoreDNS ConfigManager
3. ConfigManager: Generate minimal Corefile from template
4. ConfigManager: Write Corefile (no TLS, no domain-specific routes)
5. Docker: Start CoreDNS container
6. main.go: Wait for CoreDNS health check
7. main.go: Initialize Certificate Manager
8. main.go: Initialize Bootstrap Manager
9. Bootstrap: Request domain addition via ConfigManager
10. ConfigManager: Add internal domain block (no TLS)
11. ConfigManager: Restart CoreDNS
12. main.go: Start API Server
13. Certificate Manager: Begin certificate obtainment (async)
```

### Certificate Obtained (TLS Enablement)
```
1. Certificate Manager: Certificate validation successful
2. Certificate Manager: Notify ConfigManager.EnableTLS()
3. ConfigManager: Add TLS block for domain
4. ConfigManager: Update Corefile with TLS configuration
5. ConfigManager: Restart CoreDNS container
6. ConfigManager: Verify TLS endpoint health
7. Certificate Manager: Log TLS enablement success
```

### Runtime Zone Addition
```
1. API Request: Add new zone
2. CoreDNS Manager: Call ConfigManager.AddDomain()
3. ConfigManager: Generate domain block
4. ConfigManager: Append to Corefile
5. ConfigManager: Restart CoreDNS
6. CoreDNS Manager: Verify zone availability
```

## Error Handling & Rollback

### Configuration Generation Failures
- Validate generated Corefile syntax before writing
- Keep backup of last known good configuration
- Rollback on CoreDNS startup failure

### CoreDNS Restart Failures
- Attempt restart with exponential backoff
- Rollback to previous configuration on repeated failures
- Health check verification after restart

### Certificate Integration Failures
- Continue operation without TLS if certificate fails
- Retry TLS enablement on certificate renewal
- Log certificate-related errors without breaking DNS

## Testing Requirements

### Unit Tests

#### 1. `internal/dns/coredns/config_manager_test.go`
```go
func TestConfigManager_GenerateCorefile()
func TestConfigManager_AddDomain()
func TestConfigManager_EnableTLS()
func TestConfigManager_RemoveDomain()
func TestConfigManager_WriteConfig()
```

#### 2. `internal/dns/coredns/restart_manager_test.go`
```go
func TestRestartManager_RestartCoreDNS()
func TestRestartManager_HealthCheck()
func TestRestartManager_Rollback()
```

### Integration Tests

#### 1. `tests/dynamic_config_test.go`
```go
func TestDynamicConfiguration_InitialBoot()
func TestDynamicConfiguration_DomainAddition()
func TestDynamicConfiguration_TLSEnablement()
func TestDynamicConfiguration_CertificateIntegration()
func TestDynamicConfiguration_ErrorRecovery()
```

#### 2. Update existing integration tests
- `TestServiceStartupIntegration`: Verify dynamic config generation
- `TestCertificateManagement`: Test TLS enablement flow
- `TestEndToEndBootstrapWorkflow`: Verify dynamic domain addition

### Docker Integration Tests

#### 1. `tests/docker_restart_test.go`
```go
func TestDockerRestart_CoreDNSContainer()
func TestDockerRestart_ConfigurationPersistence()
func TestDockerRestart_HealthCheckVerification()
```

## Configuration Schema Updates

### Application Configuration
```yaml
dns:
  domain: "INTERNAL_DOMAIN_PLACEHOLDER"
  coredns:
    config_path: /etc/coredns/Corefile
    template_path: /etc/coredns/Corefile.template  # NEW
    zones_path: /etc/coredns/zones
    reload_command: ["docker-compose", "restart", "coredns"]  # NEW
    restart_timeout: "30s"  # NEW
    health_check_retries: 5  # NEW
```

### Runtime State Management
```yaml
# Generated at runtime in memory
coredns_state:
  domains:
    - name: "internal.jerkytreats.dev"
      tls_enabled: false
      cert_path: ""
      key_path: ""
      zones: ["internal"]
  last_restart: "2024-03-15T10:30:00Z"
  config_version: 2
```

## Deployment Considerations

### First-Time Setup
1. Setup script creates template structure
2. No static Corefile committed to git
3. Dynamic generation on first boot
4. Graceful certificate obtainment

## Implementation Phase Status

Based on codebase analysis, here's the current implementation status and step-by-step plan:

### **Phase 1: Core Infrastructure**

| Component | Status | Description | Dependencies | Priority |
|-----------|--------|-------------|--------------|----------|
| **A. Dynamic Configuration Generator** | ✅ COMPLETE | Created `internal/dns/coredns/config_manager.go` with ConfigManager struct | None | HIGH |
| A1. ConfigManager struct | ✅ COMPLETE | Defined base configuration management structure | None | HIGH |
| A2. GenerateCorefile() | ✅ COMPLETE | Generate dynamic Corefile from template | ConfigManager | HIGH |
| A3. AddDomain() method | ✅ COMPLETE | Add domain configuration blocks dynamically | ConfigManager | HIGH |
| A4. EnableTLS() method | ✅ COMPLETE | Add TLS configuration when certificates available | ConfigManager | HIGH |
| A5. WriteConfig() method | ✅ COMPLETE | Write generated config to filesystem | ConfigManager | HIGH |
| **B. Corefile Template** | ✅ COMPLETE | Created `configs/coredns/Corefile.template` | None | HIGH |
| B1. Base template structure | ✅ COMPLETE | Minimal configuration without hardcoded domains | None | HIGH |
| B2. Dynamic section markers | ✅ COMPLETE | Template variables for dynamic content injection | Template | MEDIUM |
| **C. Restart Manager** | ✅ COMPLETE | Created `internal/dns/coredns/restart_manager.go` | None | MEDIUM |
| C1. Docker restart logic | ✅ COMPLETE | Handle CoreDNS container restarts | RestartManager | MEDIUM |
| C2. Health check validation | ✅ COMPLETE | Verify CoreDNS health after restart | RestartManager | MEDIUM |
| C3. Rollback mechanism | ✅ COMPLETE | Rollback to previous config on failure | RestartManager | MEDIUM |

### **Phase 2: Integration Points** ✅ **COMPLETE**

| Component | Status | Description | Dependencies | Priority |
|-----------|--------|-------------|--------------|----------|
| **A. Startup Sequence** | ✅ COMPLETE | Updated `cmd/api/main.go` startup order | ConfigManager | HIGH |
| A1. ConfigManager initialization | ✅ COMPLETE | Initialize before CoreDNS startup | ConfigManager | HIGH |
| A2. Generate initial Corefile | ✅ COMPLETE | Create minimal config before container start | ConfigManager | HIGH |
| A3. Certificate integration | ✅ COMPLETE | Coordinate with certificate manager | ConfigManager, CertManager | HIGH |
| **B. CoreDNS Manager Updates** | ✅ COMPLETE | Updated `internal/dns/coredns/manager.go` | ConfigManager | HIGH |
| B1. ConfigManager integration | ✅ COMPLETE | Added SetConfigManager() method | ConfigManager | HIGH |
| B2. AddZone() refactor | ✅ COMPLETE | Use ConfigManager with fallback to direct config | ConfigManager | HIGH |
| B3. UpdateConfig() replacement | ✅ COMPLETE | Integrated with ConfigManager calls | ConfigManager | HIGH |
| **C. Certificate Manager Integration** | ✅ COMPLETE | Updated `internal/certificate/manager.go` | ConfigManager | MEDIUM |
| C1. TLS notification hooks | ✅ COMPLETE | Added SetCoreDNSManager() integration | ConfigManager | MEDIUM |
| C2. Certificate callback | ✅ COMPLETE | EnableTLS() call on cert obtainment and renewal | ConfigManager | MEDIUM |
| **D. Bootstrap Integration** | ✅ COMPLETE | `internal/dns/bootstrap/manager.go` exists | None | - |
| D1. Dynamic zone creation | ✅ COMPLETE | Bootstrap manager creates zones dynamically | Existing | - |
| D2. Device record creation | ✅ COMPLETE | Creates DNS records for devices | Existing | - |

### **Phase 3: Configuration & Deployment** ✅ **COMPLETE**

| Component | Status | Description | Dependencies | Priority |
|-----------|--------|-------------|--------------|----------|
| **A. Configuration Updates** | ✅ COMPLETE | Updated config templates and schema | None | MEDIUM |
| A1. Template path config | ✅ COMPLETE | Added template_path to config schema and constants | None | MEDIUM |
| A2. Restart timeout config | ✅ COMPLETE | Added restart_timeout configuration to templates and constants | None | LOW |
| A3. Health check retries | ✅ COMPLETE | Added health_check_retries configuration to templates and constants | None | LOW |
| **B. Docker Integration** | ✅ COMPLETE | Updated docker-compose and test configs | None | MEDIUM |
| B1. Remove static Corefile | ✅ COMPLETE | Removed static Corefile, now generated dynamically | ConfigManager | MEDIUM |
| B2. Template volume mount | ✅ COMPLETE | Updated test docker-compose to mount templates | None | MEDIUM |
| B3. Restart policy update | ✅ COMPLETE | Added restart policies (unless-stopped) for dynamic updates | None | LOW |
| **C. Static File Cleanup** | ✅ COMPLETE | Cleaned up static configuration files | None | LOW |
| C1. Remove static Corefile | ✅ COMPLETE | Deleted `configs/coredns/Corefile` | ConfigManager | LOW |
| C2. Update .gitignore | ✅ COMPLETE | Added generated Corefile to .gitignore | None | LOW |
| C3. Update setup scripts | ✅ COMPLETE | Removed static Corefile operations from setup.sh | None | LOW |

### **Phase 4: Testing & Validation** ✅ **COMPLETE**

| Component | Status | Description | Dependencies | Priority |
|-----------|--------|-------------|--------------|----------|
| **A. Unit Tests** | ✅ COMPLETE | Created comprehensive unit tests | Implementation | HIGH |
| A1. ConfigManager tests | ✅ COMPLETE | All ConfigManager methods tested and passing | ConfigManager | HIGH |
| A2. RestartManager tests | ✅ COMPLETE | All RestartManager functionality tested and passing | RestartManager | HIGH |
| A3. Integration tests | ✅ COMPLETE | Manager integration tested - all tests passing | All managers | HIGH |
| **B. Integration Tests** | ✅ COMPLETE | Updated existing integration tests | Implementation | MEDIUM |
| B1. Startup sequence test | ✅ COMPLETE | Dynamic config generation verified in tests | Implementation | MEDIUM |
| B2. Certificate integration test | ✅ COMPLETE | TLS enablement flow verified in tests | Implementation | MEDIUM |
| B3. Error recovery test | ✅ COMPLETE | Enhanced error recovery tests added and passing | Implementation | MEDIUM |
| **C. Docker Tests** | ✅ COMPLETE | Test environment updated for dynamic config | Implementation | LOW |
| C1. Container restart test | ✅ COMPLETE | Container restart integration tests added and passing | Implementation | LOW |
| C2. Configuration persistence | ✅ COMPLETE | Template mounting and volume persistence verified | Implementation | LOW |
