# Certificate SAN Management Architecture

## Problem Statement
Current certificate manager only supports single domain certificates. Need dynamic SAN (Subject Alternative Names) support to automatically include DNS records as they're created, avoiding wildcard certificate issues with CoreDNS record management.

## Current Architecture Analysis

### Records System
- **Location**: `internal/dns/record/types.go:19`
- **Behavior**: Records are **generated at runtime** from DNS zones and proxy rules
- **No Persistence**: Records themselves are not persisted, only their underlying sources

### Certificate Manager
- **Location**: `internal/certificate/manager.go:295`
- **Current**: Hardcoded single domain: `Domains: []string{domain}`
- **ACME Support**: Uses `go-acme/lego` which fully supports multiple domains

### Persistence Layer
- **Location**: `internal/persistence/file.go`
- **Features**: Atomic operations, backup support, thread-safe
- **Pattern**: Used by device storage for persistent data

## ACME SAN Capabilities ✅
- **ACME v2**: Fully supports multiple domains in SAN certificates
- **Let's Encrypt Limits**: Up to 100 SANs per certificate
- **go-acme/lego**: Supports via `certificate.ObtainRequest.Domains: []string{...}`
- **DNS-01 Challenge**: Required for multiple domains, already implemented

## Proposed Architecture

### 1. Certificate Domain Storage
**New File**: `internal/certificate/domain_storage.go`

```go
type CertificateDomainStorage struct {
    storage *persistence.FileStorage
}

type CertificateDomains struct {
    BaseDomain string    `json:"base_domain"`           // internal.jerkytreats.dev
    SANDomains []string  `json:"san_domains"`           // [dns.internal.jerkytreats.dev, api.internal.jerkytreats.dev]
    UpdatedAt  time.Time `json:"updated_at"`
}

// Methods
func NewCertificateDomainStorage() *CertificateDomainStorage
func (s *CertificateDomainStorage) LoadDomains() (*CertificateDomains, error)
func (s *CertificateDomainStorage) SaveDomains(domains *CertificateDomains) error
func (s *CertificateDomainStorage) AddDomain(domain string) error
func (s *CertificateDomainStorage) RemoveDomain(domain string) error
```

**Storage Path**: `data/certificate_domains.json`

### 2. Certificate Manager Integration
**Modify**: `internal/certificate/manager.go`

```go
// Add domain storage field
type Manager struct {
    // ... existing fields
    domainStorage *CertificateDomainStorage
}

// Modify ObtainCertificate to support multiple domains
func (m *Manager) ObtainCertificate(domains []string) error {
    // ... existing logic
    request := certificate.ObtainRequest{
        Domains: domains,  // Changed from single domain
        Bundle:  true,
    }
    // ... rest unchanged
}

// Add method to get all domains for certificate
func (m *Manager) GetDomainsForCertificate() ([]string, error) {
    certDomains, err := m.domainStorage.LoadDomains()
    if err != nil {
        return nil, err
    }
    
    allDomains := []string{certDomains.BaseDomain}
    allDomains = append(allDomains, certDomains.SANDomains...)
    return allDomains, nil
}
```

### 3. Record Handler Integration
**Modify**: `internal/api/handler/record.go`

```go
type RecordHandler struct {
    recordService     *record.Service
    certificateManager *certificate.Manager  // Add this
}

func (h *RecordHandler) AddRecord(w http.ResponseWriter, r *http.Request) {
    // ... existing record creation logic
    
    // After successful record creation:
    if req.Name != "" {
        domain := fmt.Sprintf("%s.%s", req.Name, config.GetString("dns.domain"))
        if err := h.certificateManager.AddDomainToSAN(domain); err != nil {
            logging.Warn("Failed to add domain to certificate SAN: %v", err)
            // Don't fail the record creation
        }
    }
    
    // ... rest unchanged
}
```

### 4. Certificate Manager SAN Methods
**Add to**: `internal/certificate/manager.go`

```go
// AddDomainToSAN adds a new domain to the certificate SAN list and triggers renewal
func (m *Manager) AddDomainToSAN(domain string) error {
    if err := m.domainStorage.AddDomain(domain); err != nil {
        return fmt.Errorf("failed to add domain to storage: %w", err)
    }
    
    // Trigger certificate renewal with new domains
    domains, err := m.GetDomainsForCertificate()
    if err != nil {
        return fmt.Errorf("failed to get domains for certificate: %w", err)
    }
    
    return m.ObtainCertificateWithRetry(domains)
}

// RemoveDomainFromSAN removes a domain from SAN list and triggers renewal
func (m *Manager) RemoveDomainFromSAN(domain string) error {
    if err := m.domainStorage.RemoveDomain(domain); err != nil {
        return fmt.Errorf("failed to remove domain from storage: %w", err)
    }
    
    domains, err := m.GetDomainsForCertificate()
    if err != nil {
        return fmt.Errorf("failed to get domains for certificate: %w", err)
    }
    
    return m.ObtainCertificateWithRetry(domains)
}
```

### 5. Initialization Integration
**Modify**: `internal/certificate/manager.go` NewManager function

```go
func NewManager() (*Manager, error) {
    // ... existing initialization
    
    domainStorage := NewCertificateDomainStorage()
    
    manager := &Manager{
        // ... existing fields
        domainStorage: domainStorage,
    }
    
    // Initialize with base domain if no domains stored
    if !domainStorage.storage.Exists() {
        baseDomain := config.GetString(CertDomainKey)
        initialDomains := &CertificateDomains{
            BaseDomain: baseDomain,
            SANDomains: []string{},
            UpdatedAt:  time.Now(),
        }
        if err := domainStorage.SaveDomains(initialDomains); err != nil {
            logging.Warn("Failed to initialize certificate domains: %v", err)
        }
    }
    
    return manager, nil
}
```

## Implementation Flow

### Adding a Record
1. `POST /add-record` with `name: "dns"`
2. `RecordHandler.AddRecord()` creates DNS record
3. Handler calls `certificateManager.AddDomainToSAN("dns.internal.jerkytreats.dev")`
4. Domain storage adds to SAN list and persists
5. Certificate manager triggers renewal with updated domain list
6. New certificate includes all domains in SAN

### Certificate Initialization
1. Certificate manager loads domains from `data/certificate_domains.json`
2. Combines base domain + SAN domains into single array
3. ACME request includes all domains for validation
4. Single certificate covers all current DNS records

### Benefits
- **No Wildcard Issues**: Specific domains only, maintains CoreDNS compatibility
- **Multi-Domain Support**: Supports `foo.bar.internal.jerkytreats.dev` scenarios
- **Automatic Management**: DNS records automatically trigger certificate updates
- **Persistent Tracking**: Domains survive restarts and system changes
- **Incremental Updates**: Add/remove domains without affecting others
- **Existing Pattern**: Uses proven persistence layer architecture

### Configuration Requirements
- Enable certificate renewal (`certificate.renewal.enabled: true`)
- DNS-01 challenge already configured with Cloudflare
- Adequate Let's Encrypt rate limits for renewal frequency

## Files to Modify/Create

1. **CREATE**: `internal/certificate/domain_storage.go`
2. **MODIFY**: `internal/certificate/manager.go` - Add SAN support
3. **MODIFY**: `internal/api/handler/record.go` - Add certificate integration
4. **DATA**: `data/certificate_domains.json` - Persistent storage

## Testing Strategy
1. Unit tests for domain storage operations
2. Integration tests for certificate-record coordination
3. End-to-end tests with multiple domain certificate requests
4. Rate limit testing with Let's Encrypt staging environment