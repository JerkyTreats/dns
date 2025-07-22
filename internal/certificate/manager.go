package certificate

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	mathrand "math/rand"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	cfapi "github.com/cloudflare/cloudflare-go"
	"github.com/go-acme/lego/v4/certcrypto"
	"github.com/go-acme/lego/v4/certificate"
	"github.com/go-acme/lego/v4/challenge"
	"github.com/go-acme/lego/v4/challenge/dns01"
	"github.com/go-acme/lego/v4/lego"
	"github.com/go-acme/lego/v4/providers/dns/cloudflare"
	"github.com/go-acme/lego/v4/registration"
	"github.com/jerkytreats/dns/internal/config"
	"github.com/jerkytreats/dns/internal/dns/coredns"
	"github.com/jerkytreats/dns/internal/logging"
)

const (
	CertEmailKey                = "certificate.email"
	CertDomainKey               = "certificate.domain"
	CertCertFileKey             = "server.tls.cert_file"
	CertKeyFileKey              = "server.tls.key_file"
	CertCADirURLKey             = "certificate.ca_dir_url"
	CertInsecureSkipVerifyKey   = "certificate.insecure_skip_verify"
	CertRenewalEnabledKey       = "certificate.renewal.enabled"
	CertRenewalRenewBeforeKey   = "certificate.renewal.renew_before"
	CertRenewalCheckIntervalKey = "certificate.renewal.check_interval"
	CertDNSResolversKey         = "certificate.dns_resolvers"
	CertDNSTimeoutKey           = "certificate.dns_timeout"
	CertDNSProviderKey          = "certificate.dns_provider"
	CertCloudflareTokenKey      = "certificate.cloudflare_api_token"
)

func init() {
	config.RegisterRequiredKey(CertEmailKey)
	config.RegisterRequiredKey(CertDomainKey)
	config.RegisterRequiredKey(CertCertFileKey)
	config.RegisterRequiredKey(CertKeyFileKey)
	config.RegisterRequiredKey(CertCADirURLKey)
	config.RegisterRequiredKey(CertRenewalEnabledKey)
	config.RegisterRequiredKey(CertRenewalRenewBeforeKey)
	config.RegisterRequiredKey(CertRenewalCheckIntervalKey)
	config.RegisterRequiredKey(CertDNSResolversKey)
	config.RegisterRequiredKey(CertDNSTimeoutKey)
	config.RegisterRequiredKey(CertCloudflareTokenKey)
}

// User implements acme.User
type User struct {
	Email        string
	Registration *registration.Resource
	key          crypto.PrivateKey
}

func (u *User) GetEmail() string                        { return u.Email }
func (u *User) GetRegistration() *registration.Resource { return u.Registration }
func (u *User) GetPrivateKey() crypto.PrivateKey        { return u.key }

// Manager handles certificate issuance and renewal.
type Manager struct {
	legoClient   *lego.Client
	user         *User
	certPath     string
	keyPath      string
	acmeUserPath string
	acmeKeyPath  string

	// CoreDNS integration for TLS enablement
	corednsConfigManager interface {
		EnableTLS(domain, certPath, keyPath string) error
	}
}

// NewManager creates a new certificate manager.
func NewManager() (*Manager, error) {
	logging.Info("Creating certificate manager")

	email := config.GetString(CertEmailKey)
	certPath := config.GetString(CertCertFileKey)
	keyPath := config.GetString(CertKeyFileKey)
	zonesPath := config.GetString("dns.coredns.zones_path")

	// Derive ACME paths from the certificate path
	acmeDir := filepath.Dir(certPath)
	acmeUserPath := filepath.Join(acmeDir, "acme_user.json")
	acmeKeyPath := filepath.Join(acmeDir, "acme_key.pem")

	var (
		dnsProvider challenge.Provider
		err         error
	)

	providerType := config.GetString(CertDNSProviderKey)
	if providerType == "" {
		providerType = "coredns"
	}

	switch providerType {
	case "cloudflare":
		logging.Info("Using Cloudflare DNS provider for ACME challenges")

		// Prefer env var for security; fall back to config key
		cfToken := os.Getenv("CLOUDFLARE_API_TOKEN")
		if cfToken == "" {
			cfToken = config.GetString(CertCloudflareTokenKey)
		}

		if cfToken == "" {
			return nil, fmt.Errorf("cloudflare api token not provided via CLOUDFLARE_API_TOKEN env var or %s config key", CertCloudflareTokenKey)
		}

		cfConfig := cloudflare.NewDefaultConfig()
		cfConfig.AuthToken = cfToken

		var discoveredZoneID string
		// Attempt to resolve ZoneID automatically based on certificate domain
		domainForCert := config.GetString(CertDomainKey)
		if domainForCert != "" {
			// Use Cloudflare API to find matching zone ID by walking up labels
			api, apiErr := cfapi.NewWithAPIToken(cfToken, cfapi.HTTPClient(&http.Client{Timeout: 10 * time.Second}))
			if apiErr == nil {
				zoneCandidate := domainForCert
				for {
					id, zidErr := api.ZoneIDByName(zoneCandidate)
					if zidErr == nil {
						logging.Info("Discovered Cloudflare ZoneID %s for %s", id, zoneCandidate)
						discoveredZoneID = id
						break
					}
					if !strings.Contains(zoneCandidate, ".") {
						break // cannot strip further
					}
					zoneCandidate = zoneCandidate[strings.Index(zoneCandidate, ".")+1:]
				}
			}
		}

		if discoveredZoneID == "" {
			return nil, fmt.Errorf("could not auto-discover Cloudflare ZoneID for domain %s", domainForCert)
		}

		dnsProvider, err = cloudflare.NewDNSProviderConfig(cfConfig)
		if err != nil {
			return nil, fmt.Errorf("could not create Cloudflare DNS provider: %w", err)
		}

		// Wrap the provider with our proactive cleanup layer.
		logging.Info("[SETUP] Wrapping Cloudflare provider with proactive cleanup layer.")
		dnsProvider, err = NewCleaningDNSProvider(dnsProvider, cfToken, discoveredZoneID)
		if err != nil {
			return nil, fmt.Errorf("could not create cleaning DNS provider: %w", err)
		}

	case "coredns":
		logging.Info("Using internal CoreDNS provider for ACME challenges")
		dnsProvider = coredns.NewDNSProvider(zonesPath)

	default:
		return nil, fmt.Errorf("unsupported certificate.dns_provider value: %s", providerType)
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

	// Set custom CA directory URL if provided
	if caURL := config.GetString(CertCADirURLKey); caURL != "" {
		legoConfig.CADirURL = caURL
	}

	// Configure DNS resolvers for challenge verification
	dnsResolvers := config.GetStringSlice(CertDNSResolversKey)
	if len(dnsResolvers) > 0 {
		logging.Info("Configuring DNS resolvers for ACME challenge verification: %v", dnsResolvers)
	} else {
		// Default to Google DNS and Cloudflare DNS for public resolution
		dnsResolvers = []string{"8.8.8.8:53", "1.1.1.1:53"}
		logging.Info("Using default DNS resolvers for ACME challenge verification: %v", dnsResolvers)
	}

	// Configure DNS timeout
	dnsTimeout := config.GetDuration(CertDNSTimeoutKey)
	if dnsTimeout == 0 {
		dnsTimeout = 10 * time.Second // Default timeout
	}
	logging.Info("Configuring DNS timeout: %v", dnsTimeout)

	// Configure insecure skip verify for test environments
	skipVerify := config.GetBool(CertInsecureSkipVerifyKey)
	logging.Info("Certificate insecure_skip_verify setting: %v", skipVerify)
	if skipVerify {
		logging.Info("Configuring HTTP client to skip certificate verification")
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

	// Configure DNS01 challenge options for better compatibility with container environments
	logging.Info("Configuring DNS01 challenge options for container environment compatibility")

	// Create DNS01 challenge options to solve Docker DNS resolution issues
	var dns01Options []dns01.ChallengeOption

	// Add custom DNS resolvers for propagation checking
	if len(dnsResolvers) > 0 {
		logging.Info("Adding recursive nameservers for DNS01 challenge: %v", dnsResolvers)
		dns01Options = append(dns01Options, dns01.AddRecursiveNameservers(dnsResolvers))
	}

	// Add DNS timeout configuration
	logging.Info("Adding DNS timeout for DNS01 challenge: %v", dnsTimeout)
	dns01Options = append(dns01Options, dns01.AddDNSTimeout(dnsTimeout))

	// Disable complete propagation requirement for container environments
	// This helps avoid issues where Docker's internal DNS differs from public DNS
	logging.Info("Disabling complete propagation requirement for container compatibility")
	dns01Options = append(dns01Options, dns01.DisableCompletePropagationRequirement())

	// Add exponential backoff for pre-check to wait for propagation
	propagationTimeout := 5 * time.Minute
	logging.Info("Adding exponential backoff for pre-check with a timeout of %v", propagationTimeout)
	dns01Options = append(dns01Options, exponentialBackoff(propagationTimeout))

	// Set the DNS01 provider with the configured options
	err = client.Challenge.SetDNS01Provider(dnsProvider, dns01Options...)
	if err != nil {
		return nil, fmt.Errorf("could not set DNS01 provider: %w", err)
	}

	return &Manager{
		legoClient:   client,
		user:         user,
		certPath:     certPath,
		keyPath:      keyPath,
		acmeUserPath: acmeUserPath,
		acmeKeyPath:  acmeKeyPath,
	}, nil
}

// SetCoreDNSManager integrates the certificate manager with CoreDNS ConfigManager for TLS enablement
func (m *Manager) SetCoreDNSManager(configManager interface {
	EnableTLS(domain, certPath, keyPath string) error
}) {
	logging.Info("Integrating certificate manager with CoreDNS ConfigManager")
	m.corednsConfigManager = configManager
}

// ObtainCertificate requests a certificate for the given domain.
func (m *Manager) ObtainCertificate(domain string) error {
	logging.Info("Obtaining certificate for domain: %s", domain)

	if m.user.Registration == nil {
		logging.Info("Registering user with ACME server")
		reg, err := m.legoClient.Registration.Register(registration.RegisterOptions{TermsOfServiceAgreed: true})
		if err != nil {
			return fmt.Errorf("could not register user: %w", err)
		}
		m.user.Registration = reg

		if err := saveUser(m.user, m.acmeUserPath, m.acmeKeyPath); err != nil {
			logging.Error("Failed to save user registration: %v", err)
		}
	}

	request := certificate.ObtainRequest{
		Domains: []string{domain},
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

// calculateBackoffWithJitter calculates exponential backoff with jitter for retry attempts
// Returns duration for backoff: attempt^2 seconds + random jitter (0-25% of backoff)
func calculateBackoffWithJitter(attempt int) time.Duration {
	if attempt <= 0 {
		return 0
	}

	backoff := time.Duration(attempt*attempt) * time.Second

	// Fix jitter calculation to handle zero/negative cases
	jitterMax := int(backoff / 4) // 25% of backoff time in nanoseconds
	var jitter time.Duration
	if jitterMax > 0 {
		jitter = time.Duration(mathrand.Intn(jitterMax))
	}

	return backoff + jitter
}

// ObtainCertificateWithRetry obtains a certificate with exponential backoff retry logic.
// This method will keep retrying until successful or max retries is reached.
// Returns an error if max retries is reached and certificate obtainment fails.
func (m *Manager) ObtainCertificateWithRetry(domain string) error {
	maxRetries := 10

	for attempt := 1; attempt <= maxRetries; attempt++ {
		err := m.ObtainCertificate(domain)
		if err == nil {
			return nil
		}

		if attempt == maxRetries {
			return fmt.Errorf("failed to obtain certificate after %d attempts: %w", maxRetries, err)
		}

		sleepTime := calculateBackoffWithJitter(attempt)

		logging.Info("Certificate obtainment failed, retrying in %v (attempt %d/%d): %v", sleepTime, attempt, maxRetries, err)
		time.Sleep(sleepTime)
	}

	return fmt.Errorf("failed to obtain certificate after %d attempts", maxRetries)
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
		m.checkAndRenew(domain, renewalInterval)
	}
}

func (m *Manager) checkAndRenew(domain string, renewalInterval time.Duration) {
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

	if err := m.ObtainCertificateWithRetry(domain); err != nil {
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

func saveUser(user *User, userFile, keyFile string) error {
	if err := os.MkdirAll(filepath.Dir(userFile), 0755); err != nil {
		return fmt.Errorf("could not create user directory: %w", err)
	}

	userData, err := json.Marshal(user.Registration)
	if err != nil {
		return fmt.Errorf("could not marshal user registration: %w", err)
	}

	if err := os.WriteFile(userFile, userData, 0644); err != nil {
		return fmt.Errorf("could not write user file: %w", err)
	}

	keyBytes, err := x509.MarshalECPrivateKey(user.key.(*ecdsa.PrivateKey))
	if err != nil {
		return fmt.Errorf("could not marshal private key: %w", err)
	}

	keyData := pem.EncodeToMemory(&pem.Block{
		Type:  "EC PRIVATE KEY",
		Bytes: keyBytes,
	})

	if err := os.WriteFile(keyFile, keyData, 0600); err != nil {
		return fmt.Errorf("could not write key file: %w", err)
	}

	return nil
}

func loadUser(userFile, keyFile string) (*User, error) {
	userData, err := os.ReadFile(userFile)
	if err != nil {
		return nil, err
	}

	keyData, err := os.ReadFile(keyFile)
	if err != nil {
		return nil, err
	}

	block, _ := pem.Decode(keyData)
	if block == nil {
		return nil, fmt.Errorf("could not decode private key")
	}

	key, err := x509.ParseECPrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("could not parse private key: %w", err)
	}

	var reg registration.Resource
	if err := json.Unmarshal(userData, &reg); err != nil {
		return nil, fmt.Errorf("could not unmarshal user registration: %w", err)
	}

	return &User{
		Registration: &reg,
		key:          key,
	}, nil
}

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

// exponentialBackoff returns a dns01.ChallengeOption that wraps lego's default
// pre-check with an exponential back-off wait loop. The first re-try happens
// after 2 s and the delay doubles every probe up to 32 s. The overall timeout
// is bounded by the dnsTimeout that the caller already configured. If the
// record does not propagate before the deadline the check returns false with
// a descriptive error, allowing lego to continue polling until its own global
// timeout triggers.
func exponentialBackoff(timeout time.Duration) dns01.ChallengeOption {
	return dns01.WrapPreCheck(func(_ string, fqdn, value string, check dns01.PreCheckFunc) (bool, error) {
		deadline := time.Now().Add(timeout)
		wait := 2 * time.Second

		for {
			ok, err := check(fqdn, value)
			if ok {
				return true, nil
			}

			if time.Now().After(deadline) {
				if err == nil {
					err = fmt.Errorf("DNS propagation timed out after %v", timeout)
				}
				return false, err
			}

			time.Sleep(wait)
			if wait < 32*time.Second {
				wait *= 2
				if wait > 32*time.Second {
					wait = 32 * time.Second
				}
			}
		}
	})
}

// CleaningDNSProvider wraps a challenge.Provider to ensure no stale TXT records
// exist before a new challenge is presented.
type CleaningDNSProvider struct {
	wrappedProvider challenge.Provider
	cfAPIToken      string
	cfZoneID        string
}

// NewCleaningDNSProvider creates a new provider that cleans up old records.
func NewCleaningDNSProvider(provider challenge.Provider, token, zoneID string) (*CleaningDNSProvider, error) {
	if token == "" || zoneID == "" {
		return nil, fmt.Errorf("Cloudflare API token and Zone ID are required for the cleaning provider")
	}
	return &CleaningDNSProvider{
		wrappedProvider: provider,
		cfAPIToken:      token,
		cfZoneID:        zoneID,
	}, nil
}

// Present ensures old records are removed before creating the new one.
func (d *CleaningDNSProvider) Present(domain, token, keyAuth string) error {
	fqdn, _ := dns01.GetRecord(domain, keyAuth)

	// Ensure the base subdomain exists in Cloudflare before creating ACME challenge
	if err := d.ensureSubdomainExists(domain); err != nil {
		logging.Error("Failed to ensure subdomain exists: %v", err)
		// Continue anyway in case it was a permission issue but domain actually exists
	}

	if err := d.cleanupRecords(fqdn); err != nil {
		logging.Error("Failed to cleanup existing records: %v", err)
	}

	time.Sleep(5 * time.Second)

	return d.wrappedProvider.Present(domain, token, keyAuth)
}

// ensureSubdomainExists creates the base subdomain in Cloudflare if it doesn't exist
func (d *CleaningDNSProvider) ensureSubdomainExists(domain string) error {
	api, err := cfapi.NewWithAPIToken(d.cfAPIToken)
	if err != nil {
		return fmt.Errorf("could not create Cloudflare API client: %w", err)
	}

	// Check if subdomain already exists
	records, _, err := api.ListDNSRecords(context.Background(), cfapi.ZoneIdentifier(d.cfZoneID), cfapi.ListDNSRecordsParams{
		Name: domain,
	})
	if err != nil {
		return fmt.Errorf("could not list DNS records for %s: %w", domain, err)
	}

	// If any record exists for this subdomain, we're good
	if len(records) > 0 {
		logging.Debug("Subdomain %s already exists in Cloudflare with %d records", domain, len(records))
		return nil
	}

	// Create a dummy A record for the subdomain to make it resolvable
	logging.Info("Creating subdomain %s in Cloudflare for ACME validation", domain)

	record := cfapi.DNSRecord{
		Type:    "A",
		Name:    domain,
		Content: "127.0.0.1", // Dummy IP - Tailscale will override this for internal routing
		TTL:     300,         // 5 minutes
		Comment: "Auto-created for Let's Encrypt ACME validation",
	}

	_, err = api.CreateDNSRecord(context.Background(), cfapi.ZoneIdentifier(d.cfZoneID), cfapi.CreateDNSRecordParams{
		Type:    record.Type,
		Name:    record.Name,
		Content: record.Content,
		TTL:     record.TTL,
		Comment: record.Comment,
	})

	if err != nil {
		return fmt.Errorf("could not create subdomain record for %s: %w", domain, err)
	}

	logging.Info("Successfully created subdomain %s in Cloudflare", domain)
	return nil
}

// CleanUp delegates to the wrapped provider.
func (d *CleaningDNSProvider) CleanUp(domain, token, keyAuth string) error {
	return d.wrappedProvider.CleanUp(domain, token, keyAuth)
}

func (d *CleaningDNSProvider) cleanupRecords(fqdn string) error {
	api, err := cfapi.NewWithAPIToken(d.cfAPIToken)
	if err != nil {
		return fmt.Errorf("could not create Cloudflare API client: %w", err)
	}

	records, _, err := api.ListDNSRecords(context.Background(), cfapi.ZoneIdentifier(d.cfZoneID), cfapi.ListDNSRecordsParams{
		Type: "TXT",
		Name: fqdn,
	})
	if err != nil {
		return fmt.Errorf("could not list DNS records: %w", err)
	}

	for _, record := range records {
		if err := api.DeleteDNSRecord(context.Background(), cfapi.ZoneIdentifier(d.cfZoneID), record.ID); err != nil {
			logging.Error("Failed to delete DNS record %s: %v", record.ID, err)
		}
	}

	return nil
}

// RestoreTLSWithExistingCertificates checks for existing certificates and enables TLS
// without attempting to obtain new certificates. This is useful when certificates
// already exist but the DNS configuration was reset.
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

	// Create the underlying certificate manager
	manager, err := NewManager()
	if err != nil {
		return nil, fmt.Errorf("failed to create certificate manager: %w", err)
	}

	// Set the CoreDNS manager dependency
	manager.SetCoreDNSManager(dnsManager)

	// Get domain from configuration
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

		// Start renewal loop if enabled
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

		// Start renewal loop if enabled
		if config.GetBool(CertRenewalEnabledKey) {
			go pm.manager.StartRenewalLoop(pm.domain)
		}
	}()

	return certReadyCh, errorCh
}

// runProcess contains the core certificate process logic
func (pm *ProcessManager) runProcess() error {
	logging.Info("Starting certificate process for domain: %s", pm.domain)

	// Try to restore existing certificates first
	if err := pm.manager.RestoreTLSWithExistingCertificates(pm.domain); err != nil {
		logging.Warn("Could not restore existing certificates: %v", err)
		logging.Info("Attempting to obtain new certificate...")

		// Obtain new certificate with retry logic
		if err := pm.manager.ObtainCertificateWithRetry(pm.domain); err != nil {
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
