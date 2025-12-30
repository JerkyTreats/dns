package certificate

import (
	"fmt"
	"time"

	"github.com/jerkytreats/dns/internal/config"
	"github.com/jerkytreats/dns/internal/logging"
)

// ProcessManager handles the complete certificate lifecycle including
// initialization, renewal, and TLS configuration
type ProcessManager struct {
	manager    *Manager
	dnsManager interface {
		EnableTLS(domain, certPath, keyPath string) error
	}
	domain string
}

// NewProcessManager creates a new certificate process manager with dependency injection
func NewProcessManager(dnsManager interface {
	EnableTLS(domain, certPath, keyPath string) error
}) (*ProcessManager, error) {
	logging.Info("Creating certificate process manager")

	manager, err := NewManager()
	if err != nil {
		return nil, fmt.Errorf("failed to create certificate manager: %w", err)
	}

	manager.SetCoreDNSManager(dnsManager)

	domain := config.GetString(CertDomainKey)
	if domain == "" {
		return nil, fmt.Errorf("certificate.domain not configured")
	}

	pm := &ProcessManager{
		manager:    manager,
		dnsManager: dnsManager,
		domain:     domain,
	}

	logging.Info("Certificate process manager created successfully for domain: %s", domain)
	return pm, nil
}

// StartWithRetry runs the certificate process with automatic retry logic (non-blocking)
// Returns a channel that will be closed when certificates are ready
func (pm *ProcessManager) StartWithRetry(retryInterval time.Duration) <-chan struct{} {
	certReadyCh := make(chan struct{})

	go func() {
		defer close(certReadyCh)

		for {
			if err := pm.runProcess(); err != nil {
				logging.Error("Certificate process failed, retrying in %v: %v", retryInterval, err)
				time.Sleep(retryInterval)
				continue
			}
			break
		}

		if config.GetBool(CertRenewalEnabledKey) {
			go pm.manager.StartRenewalLoop(pm.domain)
		}
	}()

	return certReadyCh
}

// Start runs the certificate process once (non-blocking)
// Returns a channel that will be closed when certificates are ready, or an error channel
func (pm *ProcessManager) Start() (<-chan struct{}, <-chan error) {
	certReadyCh := make(chan struct{})
	errorCh := make(chan error, 1)

	go func() {
		defer close(certReadyCh)
		defer close(errorCh)

		if err := pm.runProcess(); err != nil {
			errorCh <- err
			return
		}

		if config.GetBool(CertRenewalEnabledKey) {
			go pm.manager.StartRenewalLoop(pm.domain)
		}
	}()

	return certReadyCh, errorCh
}

// runProcess contains the core certificate process logic
func (pm *ProcessManager) runProcess() error {
	logging.Info("Starting certificate process for domain: %s", pm.domain)

	if err := pm.manager.RestoreTLSWithExistingCertificates(pm.domain); err != nil {
		logging.Warn("Could not restore existing certificates: %v", err)
		logging.Info("Attempting to obtain new certificate...")

		if err := pm.manager.ObtainCertificateWithRetryRateLimit(pm.domain); err != nil {
			return fmt.Errorf("failed to obtain certificate: %w", err)
		}
	} else {
		logging.Info("Successfully restored TLS configuration with existing certificates")
	}

	logging.Info("Certificate process completed successfully for domain: %s", pm.domain)
	return nil
}

// GetManager returns the underlying certificate manager for advanced usage
func (pm *ProcessManager) GetManager() *Manager {
	return pm.manager
}

