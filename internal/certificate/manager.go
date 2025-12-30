package certificate

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	cfapi "github.com/cloudflare/cloudflare-go"
	"github.com/go-acme/lego/v4/certcrypto"
	"github.com/go-acme/lego/v4/certificate"
	"github.com/go-acme/lego/v4/challenge/dns01"
	"github.com/go-acme/lego/v4/lego"
	"github.com/go-acme/lego/v4/providers/dns/cloudflare"
	"github.com/go-acme/lego/v4/registration"
	"github.com/jerkytreats/dns/internal/config"
	"github.com/jerkytreats/dns/internal/logging"
)

// DNSRecord represents a DNS record for SAN validation
type DNSRecord struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// Manager handles certificate issuance and renewal.
type Manager struct {
	legoClient   *lego.Client
	user         *User
	certPath     string
	keyPath      string
	acmeUserPath string
	acmeKeyPath  string

	domainStorage *CertificateDomainStorage

	corednsConfigManager interface {
		EnableTLS(domain, certPath, keyPath string) error
	}

	dnsRecordProvider interface {
		ListRecords() ([]DNSRecord, error)
	}
}

// NewManager creates a new certificate manager.
func NewManager() (*Manager, error) {
	logging.Info("Creating certificate manager")

	email := config.GetString(CertEmailKey)
	certPath := config.GetString(CertCertFileKey)
	keyPath := config.GetString(CertKeyFileKey)

	acmeDir := filepath.Dir(certPath)
	acmeUserPath := filepath.Join(acmeDir, "acme_user.json")
	acmeKeyPath := filepath.Join(acmeDir, "acme_key.pem")

	logging.Info("Using Cloudflare DNS provider for ACME challenges")

	cfToken := os.Getenv("CLOUDFLARE_API_TOKEN")
	if cfToken == "" {
		cfToken = config.GetString(CertCloudflareTokenKey)
	}

	if cfToken == "" {
		return nil, fmt.Errorf("cloudflare api token not provided via CLOUDFLARE_API_TOKEN env var or %s config key", CertCloudflareTokenKey)
	}

	cfConfig := cloudflare.NewDefaultConfig()
	cfConfig.AuthToken = cfToken

	discoveredZoneID, err := discoverZoneID(cfToken)
	if err != nil {
		return nil, err
	}

	cloudflareProvider, err := cloudflare.NewDNSProviderConfig(cfConfig)
	if err != nil {
		return nil, fmt.Errorf("could not create Cloudflare DNS provider: %w", err)
	}

	logging.Info("[SETUP] Wrapping Cloudflare provider with proactive cleanup layer.")
	dnsProvider, err := NewCleaningDNSProvider(cloudflareProvider, cfToken, discoveredZoneID)
	if err != nil {
		return nil, fmt.Errorf("could not create cleaning DNS provider: %w", err)
	}

	user, err := loadUser(acmeUserPath, acmeKeyPath)
	if err != nil {
		if os.IsNotExist(err) {
			logging.Info("No existing ACME user found, creating a new one.")
			privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
			if err != nil {
				return nil, fmt.Errorf("could not generate private key for new user: %w", err)
			}
			user = &User{
				Email: email,
				key:   privateKey,
			}
		} else {
			return nil, fmt.Errorf("could not load ACME user: %w", err)
		}
	} else {
		logging.Info("Loaded existing ACME user from file.")
	}

	legoConfig := lego.NewConfig(user)
	legoConfig.Certificate.KeyType = certcrypto.RSA2048

	if caURL := config.GetString(CertCADirURLKey); caURL != "" {
		legoConfig.CADirURL = caURL
	}

	dnsResolvers := config.GetStringSlice(CertDNSResolversKey)
	if len(dnsResolvers) == 0 {
		dnsResolvers = []string{"8.8.8.8:53", "1.1.1.1:53"}
	}
	logging.Info("Using DNS resolvers for ACME challenge verification: %v", dnsResolvers)

	dnsTimeout := config.GetDuration(CertDNSTimeoutKey)
	if dnsTimeout == 0 {
		dnsTimeout = 10 * time.Second
	}
	logging.Info("DNS timeout: %v", dnsTimeout)

	skipVerify := config.GetBool(CertInsecureSkipVerifyKey)
	if skipVerify {
		logging.Warn("Configuring HTTP client to skip certificate verification - use only for testing!")
		legoConfig.HTTPClient = &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: true,
				},
			},
		}
	}

	client, err := lego.NewClient(legoConfig)
	if err != nil {
		return nil, fmt.Errorf("could not create lego client: %w", err)
	}

	dns01Options := []dns01.ChallengeOption{
		dns01.AddRecursiveNameservers(dnsResolvers),
		dns01.AddDNSTimeout(dnsTimeout),
		dns01.DisableCompletePropagationRequirement(),
		exponentialBackoff(10 * time.Minute),
	}

	if err := client.Challenge.SetDNS01Provider(dnsProvider, dns01Options...); err != nil {
		return nil, fmt.Errorf("could not set DNS01 provider: %w", err)
	}

	domainStorage := NewCertificateDomainStorage()

	manager := &Manager{
		legoClient:    client,
		user:          user,
		certPath:      certPath,
		keyPath:       keyPath,
		acmeUserPath:  acmeUserPath,
		acmeKeyPath:   acmeKeyPath,
		domainStorage: domainStorage,
	}

	if !domainStorage.Exists() {
		baseDomain := config.GetString(CertDomainKey)
		initialDomains := &CertificateDomains{
			BaseDomain: baseDomain,
			SANDomains: []string{},
			UpdatedAt:  time.Now(),
		}
		if err := domainStorage.SaveDomains(initialDomains); err != nil {
			logging.Warn("Failed to initialize certificate domains: %v", err)
		} else {
			logging.Info("Initialized certificate domains with base domain: %s", baseDomain)
		}
	}

	if err := manager.SyncStorageWithCertificate(); err != nil {
		logging.Warn("Failed to sync certificate storage during initialization: %v", err)
	}

	return manager, nil
}

// discoverZoneID attempts to auto-discover Cloudflare ZoneID for the certificate domain
func discoverZoneID(cfToken string) (string, error) {
	domainForCert := config.GetString(CertDomainKey)
	if domainForCert == "" {
		return "", fmt.Errorf("certificate domain not configured")
	}

	api, err := cfapi.NewWithAPIToken(cfToken, cfapi.HTTPClient(&http.Client{Timeout: 10 * time.Second}))
	if err != nil {
		return "", fmt.Errorf("could not create Cloudflare API client: %w", err)
	}

	zoneCandidate := domainForCert
	for {
		id, err := api.ZoneIDByName(zoneCandidate)
		if err == nil {
			logging.Info("Discovered Cloudflare ZoneID %s for %s", id, zoneCandidate)
			return id, nil
		}
		if !strings.Contains(zoneCandidate, ".") {
			break
		}
		zoneCandidate = zoneCandidate[strings.Index(zoneCandidate, ".")+1:]
	}

	return "", fmt.Errorf("could not auto-discover Cloudflare ZoneID for domain %s", domainForCert)
}

// SetCoreDNSManager integrates the certificate manager with CoreDNS ConfigManager
func (m *Manager) SetCoreDNSManager(configManager interface {
	EnableTLS(domain, certPath, keyPath string) error
}) {
	logging.Info("Integrating certificate manager with CoreDNS ConfigManager")
	m.corednsConfigManager = configManager
}

// SetDNSRecordProvider integrates the certificate manager with a DNS record provider
func (m *Manager) SetDNSRecordProvider(provider interface {
	ListRecords() ([]DNSRecord, error)
}) {
	logging.Info("Integrating certificate manager with DNS record provider for SAN validation")
	m.dnsRecordProvider = provider
}

// ObtainCertificate requests a certificate for the given domain.
func (m *Manager) ObtainCertificate(domain string) error {
	logging.Info("Obtaining certificate for domain: %s", domain)

	if err := m.ensureUserRegistered(); err != nil {
		return err
	}

	domains, err := m.GetDomainsForCertificate()
	if err != nil {
		logging.Warn("Failed to get domains from storage, using single domain: %v", err)
		domains = []string{domain}
	}

	request := certificate.ObtainRequest{
		Domains: domains,
		Bundle:  true,
	}

	certs, err := m.legoClient.Certificate.Obtain(request)
	if err != nil {
		return fmt.Errorf("could not obtain certificate: %w", err)
	}

	if err := m.saveCertificate(certs); err != nil {
		return fmt.Errorf("could not save certificate: %w", err)
	}

	if m.corednsConfigManager != nil {
		if err := m.corednsConfigManager.EnableTLS(domain, m.certPath, m.keyPath); err != nil {
			logging.Error("Failed to enable TLS in CoreDNS: %v", err)
		}
	}

	logging.Info("Certificate obtained and saved successfully")
	return nil
}

// ensureUserRegistered ensures the ACME user is registered
func (m *Manager) ensureUserRegistered() error {
	if m.user.Registration != nil {
		return nil
	}

	logging.Info("Registering user with ACME server")
	reg, err := m.legoClient.Registration.Register(registration.RegisterOptions{TermsOfServiceAgreed: true})
	if err != nil {
		return fmt.Errorf("could not register user: %w", err)
	}
	m.user.Registration = reg

	if err := saveUser(m.user, m.acmeUserPath, m.acmeKeyPath); err != nil {
		logging.Error("Failed to save user registration: %v", err)
	}
	return nil
}

// GetDomainsForCertificate returns all domains that should be included in the certificate
func (m *Manager) GetDomainsForCertificate() ([]string, error) {
	if m.domainStorage == nil {
		domain := config.GetString(CertDomainKey)
		return []string{domain}, nil
	}

	certDomains, err := m.domainStorage.LoadDomains()
	if err != nil {
		return nil, fmt.Errorf("failed to load certificate domains: %w", err)
	}

	allDomains := []string{certDomains.BaseDomain}
	allDomains = append(allDomains, certDomains.SANDomains...)
	return allDomains, nil
}

// CheckDomainCoverage verifies if a domain is already covered by the current certificate
func (m *Manager) CheckDomainCoverage(domain string) (bool, error) {
	if m == nil {
		return false, fmt.Errorf("certificate manager not initialized")
	}

	if _, err := os.Stat(m.certPath); os.IsNotExist(err) {
		logging.Debug("Certificate file does not exist: %s", m.certPath)
		return false, nil
	}

	certInfo, err := GetCertificateInfo(m.certPath)
	if err != nil {
		return false, fmt.Errorf("failed to read certificate info: %w", err)
	}

	if time.Until(certInfo.NotAfter) <= 0 {
		logging.Debug("Certificate is expired, considering domain %s as not covered", domain)
		return false, nil
	}

	if certInfo.Subject.CommonName == domain {
		logging.Debug("Domain %s matches certificate Common Name", domain)
		return true, nil
	}

	for _, sanDomain := range certInfo.DNSNames {
		if sanDomain == domain {
			logging.Debug("Domain %s found in certificate SAN list", domain)
			return true, nil
		}
	}

	logging.Debug("Domain %s not found in current certificate coverage", domain)
	return false, nil
}

// AddDomainToSAN adds a new domain to the certificate SAN list and triggers renewal
func (m *Manager) AddDomainToSAN(domain string) error {
	if m == nil {
		return fmt.Errorf("certificate manager not initialized")
	}
	if m.domainStorage == nil {
		return fmt.Errorf("domain storage not initialized")
	}

	covered, err := m.CheckDomainCoverage(domain)
	if err != nil {
		logging.Warn("Could not check certificate coverage for %s: %v", domain, err)
	} else if covered {
		logging.Info("Domain %s already covered by existing certificate, skipping renewal", domain)
		if err := m.domainStorage.AddDomain(domain); err != nil {
			return fmt.Errorf("failed to add domain to storage: %w", err)
		}
		return nil
	}

	logging.Info("Domain %s not covered by current certificate, adding to SAN and triggering renewal", domain)

	if err := m.domainStorage.AddDomain(domain); err != nil {
		return fmt.Errorf("failed to add domain to storage: %w", err)
	}

	domains, err := m.GetDomainsForCertificate()
	if err != nil {
		return fmt.Errorf("failed to get domains for certificate: %w", err)
	}

	logging.Info("Triggering certificate renewal for domains: %v", domains)
	return m.ObtainCertificateWithRetry(domains)
}

// RemoveDomainFromSAN removes a domain from SAN list and triggers renewal
func (m *Manager) RemoveDomainFromSAN(domain string) error {
	if m == nil {
		return fmt.Errorf("certificate manager not initialized")
	}
	if m.domainStorage == nil {
		return fmt.Errorf("domain storage not initialized")
	}

	if err := m.domainStorage.RemoveDomain(domain); err != nil {
		return fmt.Errorf("failed to remove domain from storage: %w", err)
	}

	domains, err := m.GetDomainsForCertificate()
	if err != nil {
		return fmt.Errorf("failed to get domains for certificate: %w", err)
	}

	logging.Info("Triggering certificate renewal after domain removal: %v", domains)
	return m.ObtainCertificateWithRetry(domains)
}

// ObtainCertificateWithRetry obtains certificate for the specified domains with retry logic
func (m *Manager) ObtainCertificateWithRetry(domains []string) error {
	maxRetries := 3
	for attempt := 1; attempt <= maxRetries; attempt++ {
		if err := m.obtainCertificateForDomains(domains); err != nil {
			if attempt == maxRetries {
				return fmt.Errorf("failed to obtain certificate after %d attempts: %w", maxRetries, err)
			}

			backoff := calculateBackoffWithJitter(attempt)
			logging.Warn("Certificate obtain attempt %d failed, retrying in %v: %v", attempt, backoff, err)
			time.Sleep(backoff)
			continue
		}

		logging.Info("Certificate obtained successfully on attempt %d", attempt)
		return nil
	}

	return fmt.Errorf("unexpected error in certificate retry logic")
}

// obtainCertificateForDomains is a helper method to obtain certificate for specific domains
func (m *Manager) obtainCertificateForDomains(domains []string) error {
	logging.Info("Obtaining certificate for domains: %v", domains)

	if err := m.ensureUserRegistered(); err != nil {
		return err
	}

	request := certificate.ObtainRequest{
		Domains: domains,
		Bundle:  true,
	}

	certs, err := m.legoClient.Certificate.Obtain(request)
	if err != nil {
		return fmt.Errorf("could not obtain certificate: %w", err)
	}

	if err := m.saveCertificate(certs); err != nil {
		return fmt.Errorf("could not save certificate: %w", err)
	}

	if m.corednsConfigManager != nil && len(domains) > 0 {
		if err := m.corednsConfigManager.EnableTLS(domains[0], m.certPath, m.keyPath); err != nil {
			logging.Error("Failed to enable TLS in CoreDNS: %v", err)
		}
	}

	return nil
}

// ObtainCertificateWithRetryRateLimit obtains a certificate with rate-limit-aware backoff
func (m *Manager) ObtainCertificateWithRetryRateLimit(domain string) error {
	isProduction := config.GetBool(CertUseProdCertsKey)

	maxRetries := 5
	if !isProduction {
		maxRetries = 8
	}

	for attempt := 1; attempt <= maxRetries; attempt++ {
		err := m.ObtainCertificate(domain)
		if err == nil {
			return nil
		}

		if isRateLimitError(err) {
			if isProduction {
				logging.Error("Production rate limit exceeded. Waiting 1 hour.")
				time.Sleep(1 * time.Hour)
			} else {
				logging.Error("Staging rate limit exceeded. Waiting 10 minutes.")
				time.Sleep(10 * time.Minute)
			}
			continue
		}

		if attempt == maxRetries {
			return fmt.Errorf("failed to obtain certificate after %d attempts: %w", maxRetries, err)
		}

		sleepTime := calculateRateLimitAwareBackoff(attempt, isProduction)

		envType := "production"
		if !isProduction {
			envType = "staging"
		}

		logging.Info("Certificate obtainment failed (%s), using rate-limit-aware backoff: %v (attempt %d/%d): %v",
			envType, sleepTime, attempt, maxRetries, err)
		time.Sleep(sleepTime)
	}

	return fmt.Errorf("failed to obtain certificate after %d attempts (respecting rate limits)", maxRetries)
}

// StartRenewalLoop starts a ticker to periodically check and renew the certificate.
func (m *Manager) StartRenewalLoop(domain string) {
	renewalInterval := config.GetDuration(CertRenewalCheckIntervalKey)
	if renewalInterval == 0 {
		renewalInterval = 24 * time.Hour
	}

	ticker := time.NewTicker(renewalInterval)
	defer ticker.Stop()

	for range ticker.C {
		m.checkAndRenew(domain)
	}
}

func (m *Manager) checkAndRenew(domain string) {
	logging.Info("Checking certificate renewal for domain: %s", domain)

	certInfo, err := GetCertificateInfo(m.certPath)
	if err != nil {
		logging.Error("Failed to get certificate info: %v", err)
		return
	}

	renewBefore := config.GetDuration(CertRenewalRenewBeforeKey)
	if renewBefore == 0 {
		renewBefore = 30 * 24 * time.Hour
	}

	timeUntilExpiry := time.Until(certInfo.NotAfter)
	if timeUntilExpiry > renewBefore {
		logging.Info("Certificate is still valid for %v, no renewal needed", timeUntilExpiry)
		return
	}

	logging.Info("Certificate expires in %v, initiating renewal", timeUntilExpiry)

	if err := m.ObtainCertificateWithRetryRateLimit(domain); err != nil {
		logging.Error("Certificate renewal failed: %v", err)
	} else {
		logging.Info("Certificate renewed successfully")
	}
}

func (m *Manager) saveCertificate(certs *certificate.Resource) error {
	if err := os.MkdirAll(filepath.Dir(m.certPath), 0755); err != nil {
		return fmt.Errorf("could not create certificate directory: %w", err)
	}

	if err := os.WriteFile(m.certPath, certs.Certificate, 0644); err != nil {
		return fmt.Errorf("could not write certificate file: %w", err)
	}

	if err := os.WriteFile(m.keyPath, certs.PrivateKey, 0600); err != nil {
		return fmt.Errorf("could not write private key file: %w", err)
	}

	return nil
}

// GetCertificateInfo reads and parses certificate information from a file
func GetCertificateInfo(certPath string) (*x509.Certificate, error) {
	certData, err := os.ReadFile(certPath)
	if err != nil {
		return nil, fmt.Errorf("could not read certificate file: %w", err)
	}

	block, _ := pem.Decode(certData)
	if block == nil {
		return nil, fmt.Errorf("could not decode certificate")
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("could not parse certificate: %w", err)
	}

	return cert, nil
}

// RestoreTLSWithExistingCertificates checks for existing certificates and enables TLS
func (m *Manager) RestoreTLSWithExistingCertificates(domain string) error {
	logging.Info("Checking for existing certificates for domain: %s", domain)

	if _, err := os.Stat(m.certPath); os.IsNotExist(err) {
		return fmt.Errorf("certificate file does not exist: %s", m.certPath)
	}

	if _, err := os.Stat(m.keyPath); os.IsNotExist(err) {
		return fmt.Errorf("private key file does not exist: %s", m.keyPath)
	}

	certInfo, err := GetCertificateInfo(m.certPath)
	if err != nil {
		return fmt.Errorf("could not get certificate info: %w", err)
	}

	if time.Until(certInfo.NotAfter) <= 0 {
		return fmt.Errorf("certificate is expired")
	}

	if m.corednsConfigManager != nil {
		if err := m.corednsConfigManager.EnableTLS(domain, m.certPath, m.keyPath); err != nil {
			return fmt.Errorf("could not enable TLS in CoreDNS: %w", err)
		}
	}

	logging.Info("TLS restored successfully with existing certificates")
	return nil
}

// ValidateAndUpdateSANDomains checks existing DNS records against the certificate SAN list
func (m *Manager) ValidateAndUpdateSANDomains() error {
	if m.dnsRecordProvider == nil {
		logging.Debug("DNS record provider not set, skipping SAN validation")
		return nil
	}

	if m.domainStorage == nil {
		logging.Warn("Domain storage not initialized, skipping SAN validation")
		return nil
	}

	logging.Info("Starting SAN domain validation against existing DNS records")

	dnsRecords, err := m.dnsRecordProvider.ListRecords()
	if err != nil {
		return fmt.Errorf("failed to list DNS records for SAN validation: %w", err)
	}

	baseDomain := config.GetString(CertDomainKey)
	if baseDomain == "" {
		return fmt.Errorf("certificate domain not configured")
	}

	certDomains, err := m.domainStorage.LoadDomains()
	if err != nil {
		return fmt.Errorf("failed to load certificate domains: %w", err)
	}

	existingSANs := make(map[string]bool)
	existingSANs[certDomains.BaseDomain] = true
	for _, domain := range certDomains.SANDomains {
		existingSANs[domain] = true
	}

	var domainsToAdd []string
	for _, record := range dnsRecords {
		if record.Name == "" {
			continue
		}

		fqdn := fmt.Sprintf("%s.%s", record.Name, baseDomain)

		if existingSANs[fqdn] {
			logging.Debug("Domain %s already in SAN list", fqdn)
			continue
		}

		domainsToAdd = append(domainsToAdd, fqdn)
	}

	if len(domainsToAdd) == 0 {
		logging.Info("SAN validation complete: all DNS records already covered by certificate")
		return nil
	}

	logging.Info("Found %d domains missing from SAN certificate: %v", len(domainsToAdd), domainsToAdd)

	addedCount := 0
	for _, domain := range domainsToAdd {
		if err := m.AddDomainToSAN(domain); err != nil {
			logging.Error("Failed to add domain %s to SAN during validation: %v", domain, err)
		} else {
			logging.Info("Added domain %s to SAN during validation", domain)
			addedCount++
		}
	}

	if addedCount > 0 {
		logging.Info("SAN validation complete: added %d domains to certificate", addedCount)
	}

	return nil
}

// SyncStorageWithCertificate ensures storage matches actual certificate coverage
func (m *Manager) SyncStorageWithCertificate() error {
	if m == nil {
		return fmt.Errorf("certificate manager not initialized")
	}
	if m.domainStorage == nil {
		return fmt.Errorf("domain storage not initialized")
	}

	logging.Info("Syncing certificate domain storage with actual certificate")

	if _, err := os.Stat(m.certPath); os.IsNotExist(err) {
		logging.Debug("Certificate file does not exist, keeping current storage")
		return nil
	}

	certInfo, err := GetCertificateInfo(m.certPath)
	if err != nil {
		logging.Warn("Failed to read certificate info during sync: %v", err)
		return nil
	}

	if time.Until(certInfo.NotAfter) <= 0 {
		logging.Warn("Certificate is expired, keeping current storage for renewal")
		return nil
	}

	baseDomain := certInfo.Subject.CommonName
	if baseDomain == "" && len(certInfo.DNSNames) > 0 {
		baseDomain = certInfo.DNSNames[0]
	}

	var sanDomains []string
	for _, dnsName := range certInfo.DNSNames {
		if dnsName != baseDomain {
			sanDomains = append(sanDomains, dnsName)
		}
	}

	actualDomains := &CertificateDomains{
		BaseDomain: baseDomain,
		SANDomains: sanDomains,
		UpdatedAt:  time.Now(),
	}

	if err := m.domainStorage.SaveDomains(actualDomains); err != nil {
		return fmt.Errorf("failed to sync domain storage with certificate: %w", err)
	}

	logging.Info("Successfully synced domain storage: base=%s, san_count=%d", baseDomain, len(sanDomains))
	return nil
}
