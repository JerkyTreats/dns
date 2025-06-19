# Let's Encrypt Certification for *.internal.jerkytreats.dev

## Overview

Implementation of Let's Encrypt SSL/TLS certification for the wildcard domain `*.internal.jerkytreats.dev` using the `go-acme/lego` library for renewal. This approach integrates directly into the Go application, simplifying the architecture by removing the need for external services like `certbot`.

## Requirements

| Component | Specification |
|-----------|---------------|
| **Certificate** | Wildcard *.internal.jerkytreats.dev, 90-day validity |
| **Validation** | DNS-01 challenge (required for wildcard) |
| **Authority** | Let's Encrypt Authority X3 |
| **Key Type** | RSA 2048-bit or ECDSA P-256 |
| **DNS Record** | TXT record for _acme-challenge.internal.jerkytreats.dev |
| **TTL** | 60 seconds (ACME challenge minimum) |

## Architecture Integration


### In-Application Certificate Management

Instead of an external `certbot` container, we will use the `go-acme/lego` library to handle certificate lifecycle events directly within the Go application.

| Component | Extension | Purpose |
|-----------|-----------|---------|
| `internal/certificate/manager.go` | New component using `go-acme/lego` | Certificate issuance, renewal, and ACME challenge handling |
| `internal/dns/coredns/manager.go` | DNS-01 challenge provider for `lego` | Manages DNS records for ACME validation |
| `configs/config.yaml` | TLS configuration sections | Certificate & security settings for `lego` |
| `cmd/api/main.go` | Background worker for certificate management | Runs renewal checks periodically |
| `docker-compose.yml` | Removed `certbot` service | Simplified setup with no external dependencies for certs |

### Implementation Phases

| Phase | Duration | Tasks |
|-------|----------|-------|
| **Lego Integration** | 2 days | Add `go-acme/lego`, create certificate manager, implement DNS provider |
| **Service Integration** | 2 days | CoreDNS TLS, API server TLS, health check integration |
| **Automation** | 1 day | Automated renewal, monitoring, documentation |

## Technical Implementation

### Docker Compose Simplification

The `certbot` service is no longer needed. Services that require certificates will mount a shared volume where the application will store them.

```yaml
# docker-compose.yml simplified
services:
  # api and coredns services will use a shared volume for certificates
  api:
    ports: ["8080:8080", "8443:8443"]  # Added HTTPS
    volumes:
      - ./ssl:/etc/letsencrypt:rw  # Writable by the app, read-only for others
    environment: [TLS_ENABLED=true]

  coredns:
    ports: ["53:53/udp", "53:53/tcp", "8053:8053/tcp"] # Changed DoH port to avoid conflict
    volumes:
      - ./ssl:/etc/letsencrypt:ro  # Direct read-only mount
      - ./configs/coredns/zones:/zones # Zones for coredns
```

### Lego Certificate Manager

A new manager will be responsible for the certificate lifecycle.

```go
// internal/certificate/manager.go
package certificate

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"log"
	"time"

	"github.com/go-acme/lego/v4/certcrypto"
	"github.com/go-acme/lego/v4/certificate"
	"github.com/go-acme/lego/v4/lego"
	"github.com/go-acme/lego/v4/registration"
)

// User implements acme.User
type User struct {
	Email        string
	Registration *registration.Resource
	key          crypto.PrivateKey
}

func (u *User) GetEmail() string                       { return u.Email }
func (u *User) GetRegistration() *registration.Resource { return u.Registration }
func (u *User) GetPrivateKey() crypto.PrivateKey       { return u.key }

// Manager handles certificate issuance and renewal.
type Manager struct {
	legoClient *lego.Client
	// ... other fields like logger, config
}

// NewManager creates a new certificate manager.
func NewManager(email string, dnsProvider challenge.Provider) (*Manager, error) {
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}

	user := &User{
		Email: email,
		key:   privateKey,
	}

	config := lego.NewConfig(user)
	config.Certificate.KeyType = certcrypto.RSA2048

	client, err := lego.NewClient(config)
	if err != nil {
		return nil, err
	}

	err = client.Challenge.SetDNS01Provider(dnsProvider)
	if err != nil {
		return nil, err
	}

	// ... register user, etc.

	return &Manager{legoClient: client}, nil
}

// ObtainCertificate obtains a new certificate.
func (m *Manager) ObtainCertificate(domains []string) (*certificate.Resource, error) {
	request := certificate.ObtainRequest{
		Domains:    domains,
		Bundle:     true,
	}
	return m.legoClient.Certificate.Obtain(request)
}

// Renewal function to be called periodically
func (m *Manager) StartRenewal() {
    // Logic to check certificate expiry and renew
    ticker := time.NewTicker(24 * time.Hour)
    for range ticker.C {
        // check and renew logic
    }
}
```

### DNS-01 Challenge Provider for CoreDNS

We'll implement `lego`'s `challenge.Provider` interface.

```go
// internal/dns/coredns/provider.go
package coredns

import (
	"fmt"
	"os"
	"path/filepath"
)

type DNSProvider struct {
	zonesPath string
}

func NewDNSProvider(zonesPath string) *DNSProvider {
	return &DNSProvider{zonesPath: zonesPath}
}

func (d *DNSProvider) Present(domain, token, keyAuth string) error {
	fqdn, value := dns01.GetRecord(domain, keyAuth)

	challengeContent := fmt.Sprintf(`$ORIGIN %s.
_acme-challenge	60 IN	TXT	"%s"`, domain, value)

	challengeFile := filepath.Join(d.zonesPath, "_acme-challenge.zone")
	return os.WriteFile(challengeFile, []byte(challengeContent), 0644)
}

func (d *DNSProvider) CleanUp(domain, token, keyAuth string) error {
	challengeFile := filepath.Join(d.zonesPath, "_acme-challenge.zone")
	return os.Remove(challengeFile)
}
```

### Configuration Extensions

```yaml
# configs/config.yaml additions
server:
  tls:
    enabled: true
    port: 8443
    cert_file: /etc/letsencrypt/live/internal.jerkytreats.dev/cert.pem
    key_file: /etc/letsencrypt/live/internal.jerkytreats.dev/privkey.pem
    min_version: "1.2"
    max_version: "1.3"

dns:
  coredns:
    zones_path: /zones # path inside coredns container
    tls:
      enabled: true
      cert_file: /etc/letsencrypt/live/internal.jerkytreats.dev/cert.pem
      key_file: /etc/letsencrypt/live/internal.jerkytreats.dev/privkey.pem
      port: 8053

certificate:
  provider: "lego"
  email: "admin@jerkytreats.dev"
  domain: "internal.jerkytreats.dev"
  renewal:
    enabled: true
    renew_before: "30d" # 720h
    check_interval: "24h"
  monitoring:
    enabled: true
    alert_threshold: 7d
```

### Corefile Configuration

```corefile
# configs/coredns/Corefile additions
. {
    errors
    log
    forward . /etc/resolv.conf
}

internal.jerkytreats.dev:53 {
    errors
    log
    file /zones/internal.jerkytreats.dev.db internal.jerkytreats.dev
}

# TLS-enabled zone for HTTPS access
internal.jerkytreats.dev:8053 {
    tls /etc/letsencrypt/live/internal.jerkytreats.dev/cert.pem /etc/letsencrypt/live/internal.jerkytreats.dev/privkey.pem {
        protocols tls1.2 tls1.3
        curves x25519 p256 p384 p521
    }
    errors
    log
    file /zones/internal.jerkytreats.dev.db internal.jerkytreats.dev
}

# ACME challenge zone (dynamic)
_acme-challenge.internal.jerkytreats.dev:53 {
    errors
    log
    file /zones/_acme-challenge.zone _acme-challenge.internal.jerkytreats.dev
}
```

### API Server TLS Integration

The API server will start the certificate manager.

```go
// cmd/api/main.go additions
func main() {
    // ... existing initialization ...

    if viper.GetBool("certificate.renewal.enabled") {
        go func() {
            // Initialize lego manager
            dnsProvider := coredns.NewDNSProvider(viper.GetString("dns.coredns.zones_path"))
            certManager, err := certificate.NewManager(viper.GetString("certificate.email"), dnsProvider)
            if err != nil {
                logger.Fatal("Failed to create certificate manager", zap.Error(err))
            }

            // Initial certificate obtention if not present
            // ...

            // Start renewal loop
            certManager.StartRenewal()
        }()
    }

    var server *http.Server
    if viper.GetBool("server.tls.enabled") {
        tlsConfig := &tls.Config{
            MinVersion: tls.VersionTLS12,
            MaxVersion: tls.VersionTLS13,
            CipherSuites: []uint16{
                tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
                tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
                tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
                tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
            },
        }

        server = &http.Server{
            Addr:      fmt.Sprintf("%s:%d", viper.GetString("server.host"), viper.GetInt("server.tls.port")),
            TLSConfig: tlsConfig,
            // ... timeout settings ...
        }
    }

    // Start server
    go func() {
        var err error
        if viper.GetBool("server.tls.enabled") {
            err = server.ListenAndServeTLS(viper.GetString("server.tls.cert_file"), viper.GetString("server.tls.key_file"))
        } else {
            err = server.ListenAndServe()
        }
        if err != nil && err != http.ErrServerClosed {
            logger.Fatal("Failed to start server", zap.Error(err))
        }
    }()
}
```

### Enhanced Health Check

```go
// Enhanced health check with certificate monitoring
func healthCheckHandler(w http.ResponseWriter, r *http.Request) {
    response := map[string]interface{}{
        "status":  "healthy",
        "version": viper.GetString("app.version"),
        "components": map[string]interface{}{
            "api":     map[string]string{"status": "healthy", "message": "API is running"},
            "coredns": map[string]string{"status": "healthy", "message": "CoreDNS is running"},
        },
    }

    // Add certificate info if TLS enabled
    if viper.GetBool("server.tls.enabled") {
        if manager, ok := getManagerFromContext(r.Context()); ok {
            if info, err := manager.GetCertificateInfo(); err == nil {
                response["certificate"] = map[string]interface{}{
                    "not_before": info.NotBefore,
                    "not_after":  info.NotAfter,
                    "subject":    info.Subject,
                    "issuer":     info.Issuer,
                    "expires_in": time.Until(info.NotAfter).String(),
                }
            }
        }
    }

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(response)
}
```

## Automation Scripts (Simplified)

The renewal script is no longer needed. The initial setup is handled by the application on first run.

### Initial Setup

The application will obtain a certificate on its first run if one doesn't exist. No script is needed.

## Implementation Plan

| Status      | Task                                                                | Component/File                                      |
|-------------|---------------------------------------------------------------------|-----------------------------------------------------|
| **Done**    | Create foundational certificate manager                             | `internal/certificate/manager.go`                   |
| **Done**    | Implement DNS-01 challenge provider                                 | `internal/dns/coredns/provider.go`                  |
| **Done** | Implement certificate renewal logic                               | `internal/certificate/manager.go`                   |
| **Done**   | Add TLS and certificate configuration                               | `configs/config.yaml`                               |
| **Done**   | Integrate certificate manager and TLS into the main application     | `cmd/api/main.go`                                   |
| **Done**   | Update Docker environment for TLS and certificate volumes           | `docker-compose.yml`                                |
| **Done**   | Configure CoreDNS for TLS and ACME challenges                       | `configs/coredns/Corefile`                          |
| **Done**   | Enhance health check to include certificate status                  | `cmd/api/main.go`                                   |
| **Done**   | Simplify automation scripts                                         | `scripts/`                                          |
