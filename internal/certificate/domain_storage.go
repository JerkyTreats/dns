package certificate

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/jerkytreats/dns/internal/persistence"
)

// CertificateDomainStorage handles persistent storage of certificate domains
type CertificateDomainStorage struct {
	storage *persistence.FileStorage
}

// CertificateDomains represents the domains included in a certificate
type CertificateDomains struct {
	BaseDomain string    `json:"base_domain"`   // internal.jerkytreats.dev
	SANDomains []string  `json:"san_domains"`   // [dns.internal.jerkytreats.dev, api.internal.jerkytreats.dev]
	UpdatedAt  time.Time `json:"updated_at"`
}

// NewCertificateDomainStorage creates a new certificate domain storage instance
func NewCertificateDomainStorage() *CertificateDomainStorage {
	storage := persistence.NewFileStorageWithPath("data/certificate_domains.json", 5)
	return &CertificateDomainStorage{
		storage: storage,
	}
}

// LoadDomains loads certificate domains from storage
func (s *CertificateDomainStorage) LoadDomains() (*CertificateDomains, error) {
	data, err := s.storage.Read()
	if err != nil {
		return nil, fmt.Errorf("failed to read certificate domains: %w", err)
	}

	var domains CertificateDomains
	if err := json.Unmarshal(data, &domains); err != nil {
		return nil, fmt.Errorf("failed to unmarshal certificate domains: %w", err)
	}

	return &domains, nil
}

// SaveDomains saves certificate domains to storage
func (s *CertificateDomainStorage) SaveDomains(domains *CertificateDomains) error {
	domains.UpdatedAt = time.Now()

	data, err := json.MarshalIndent(domains, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal certificate domains: %w", err)
	}

	if err := s.storage.Write(data); err != nil {
		return fmt.Errorf("failed to write certificate domains: %w", err)
	}

	return nil
}

// AddDomain adds a domain to the SAN list if it doesn't already exist
func (s *CertificateDomainStorage) AddDomain(domain string) error {
	domains, err := s.LoadDomains()
	if err != nil {
		return fmt.Errorf("failed to load existing domains: %w", err)
	}

	// Check if domain already exists in SAN list
	for _, existingDomain := range domains.SANDomains {
		if existingDomain == domain {
			return nil // Domain already exists, no action needed
		}
	}

	// Check if domain is the same as base domain
	if domains.BaseDomain == domain {
		return nil // Domain is already the base domain
	}

	// Add domain to SAN list
	domains.SANDomains = append(domains.SANDomains, domain)

	return s.SaveDomains(domains)
}

// RemoveDomain removes a domain from the SAN list
func (s *CertificateDomainStorage) RemoveDomain(domain string) error {
	domains, err := s.LoadDomains()
	if err != nil {
		return fmt.Errorf("failed to load existing domains: %w", err)
	}

	// Cannot remove base domain
	if domains.BaseDomain == domain {
		return fmt.Errorf("cannot remove base domain from certificate")
	}

	// Find and remove domain from SAN list
	newSANDomains := make([]string, 0, len(domains.SANDomains))
	for _, existingDomain := range domains.SANDomains {
		if existingDomain != domain {
			newSANDomains = append(newSANDomains, existingDomain)
		}
	}

	domains.SANDomains = newSANDomains

	return s.SaveDomains(domains)
}

// Exists checks if the storage file exists
func (s *CertificateDomainStorage) Exists() bool {
	return s.storage.Exists()
}