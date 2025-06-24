package certificate

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
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
	legoClient *lego.Client
	user       *User
	certPath   string
	keyPath    string

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
						logging.Info("Discovered Cloudflare ZoneID %s for %s (not set in config; lego auto-detects)", id, zoneCandidate)
						break
					}
					if !strings.Contains(zoneCandidate, ".") {
						break // cannot strip further
					}
					zoneCandidate = zoneCandidate[strings.Index(zoneCandidate, ".")+1:]
				}
			}
		}

		dnsProvider, err = cloudflare.NewDNSProviderConfig(cfConfig)
		if err != nil {
			return nil, fmt.Errorf("could not create Cloudflare DNS provider: %w", err)
		}

	case "coredns":
		logging.Info("Using internal CoreDNS provider for ACME challenges")
		dnsProvider = coredns.NewDNSProvider(zonesPath)

	default:
		return nil, fmt.Errorf("unsupported certificate.dns_provider value: %s", providerType)
	}

	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("could not generate private key: %w", err)
	}

	user := &User{
		Email: email,
		key:   privateKey,
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

	// Add exponential back-off propagation checks (respecting dnsTimeout)
	dns01Options = append(dns01Options, exponentialBackoff(dnsTimeout))

	// Disable complete propagation requirement for container environments
	// This helps avoid issues where Docker's internal DNS differs from public DNS
	logging.Info("Disabling complete propagation requirement for container compatibility")
	dns01Options = append(dns01Options, dns01.DisableCompletePropagationRequirement())

	// Set the DNS01 provider with the configured options
	err = client.Challenge.SetDNS01Provider(dnsProvider, dns01Options...)
	if err != nil {
		return nil, fmt.Errorf("could not set DNS01 provider: %w", err)
	}

	return &Manager{
		legoClient: client,
		user:       user,
		certPath:   certPath,
		keyPath:    keyPath,
	}, nil
}

// SetCoreDNSManager integrates the certificate manager with CoreDNS ConfigManager for TLS enablement
func (m *Manager) SetCoreDNSManager(configManager interface {
	EnableTLS(domain, certPath, keyPath string) error
}) {
	logging.Info("Integrating certificate manager with CoreDNS ConfigManager")
	m.corednsConfigManager = configManager
}

// ObtainCertificate obtains a new certificate if one does not already exist.
func (m *Manager) ObtainCertificate(domain string) error {
	if _, err := os.Stat(m.certPath); err == nil {
		logging.Info("Certificate already exists, skipping obtainment for domain: %s", domain)
		return nil
	}

	if m.user.GetRegistration() == nil {
		reg, err := m.legoClient.Registration.Register(registration.RegisterOptions{TermsOfServiceAgreed: true})
		if err != nil {
			return fmt.Errorf("could not register user: %w", err)
		}
		m.user.Registration = reg
		logging.Info("Successfully registered user: %s", m.user.Email)
	}

	request := certificate.ObtainRequest{
		Domains: []string{domain, "*." + domain},
		Bundle:  true,
	}
	certs, err := m.legoClient.Certificate.Obtain(request)
	if err != nil {
		return fmt.Errorf("could not obtain certificate: %w", err)
	}

	logging.Info("Successfully obtained certificate for domain: %s", domain)
	if err := m.saveCertificate(certs); err != nil {
		return err
	}

	// Notify CoreDNS ConfigManager to enable TLS
	if m.corednsConfigManager != nil {
		logging.Info("Notifying CoreDNS ConfigManager to enable TLS for domain: %s", domain)
		if err := m.corednsConfigManager.EnableTLS(domain, m.certPath, m.keyPath); err != nil {
			logging.Error("Failed to enable TLS in CoreDNS configuration: %v", err)
			// Don't fail certificate obtainment if TLS enablement fails
		} else {
			logging.Info("Successfully enabled TLS in CoreDNS configuration for domain: %s", domain)
		}
	}

	return nil
}

// ObtainCertificateWithRetry obtains a certificate with exponential backoff retry logic.
// This method will keep retrying until successful or max retries is reached.
// Returns an error if max retries is reached and certificate obtainment fails.
func (m *Manager) ObtainCertificateWithRetry(domain string) error {
	const (
		initialDelay = 5 * time.Second
		maxDelay     = 5 * time.Minute
		maxRetries   = 10 // Set a reasonable max retry limit
	)

	delay := initialDelay
	attempt := 1

	for {
		logging.Info("Certificate obtainment attempt %d for domain: %s", attempt, domain)

		err := m.ObtainCertificate(domain)
		if err == nil {
			logging.Info("Certificate successfully obtained for domain: %s", domain)
			return nil
		}

		logging.Error("Certificate obtainment failed (attempt %d): %v", attempt, err)

		if attempt >= maxRetries {
			logging.Error("Certificate obtainment failed after %d attempts, giving up", attempt)
			return fmt.Errorf("certificate obtainment failed after %d attempts: %w", attempt, err)
		}

		logging.Info("Retrying certificate obtainment in %v", delay)
		time.Sleep(delay)

		// Exponential backoff with jitter
		delay *= 2
		if delay > maxDelay {
			delay = maxDelay
		}
		// Add jitter (±25%)
		jitter := time.Duration(float64(delay) * 0.25 * (2*mathrand.Float64() - 1))
		delay += jitter

		attempt++
	}
}

// StartRenewalLoop starts a ticker to periodically check and renew the certificate.
func (m *Manager) StartRenewalLoop(domain string) {
	logging.Info("Starting certificate renewal loop")

	renewalInterval := config.GetDuration(CertRenewalRenewBeforeKey)
	checkInterval := config.GetDuration(CertRenewalCheckIntervalKey)

	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()

	for {
		<-ticker.C
		m.checkAndRenew(domain, renewalInterval)
	}
}

func (m *Manager) checkAndRenew(domain string, renewalInterval time.Duration) {
	certBytes, err := os.ReadFile(m.certPath)
	if err != nil {
		logging.Error("Failed to read certificate for renewal check: %v", err)
		return
	}

	block, _ := pem.Decode(certBytes)
	if block == nil {
		logging.Error("Failed to decode PEM block from certificate")
		return
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		logging.Error("Failed to parse certificate: %v", err)
		return
	}

	expireIn := time.Until(cert.NotAfter)

	if expireIn > renewalInterval {
		logging.Info("Certificate for domain %s not due for renewal, expires in: %v", domain, expireIn)
		return
	}

	logging.Info("Certificate for domain %s is due for renewal, expires in: %v", domain, expireIn)

	if err := os.Remove(m.certPath); err != nil {
		logging.Error("Failed to remove old certificate file: %v", err)
		return
	}
	if err := os.Remove(m.keyPath); err != nil {
		logging.Error("Failed to remove old key file: %v", err)
		return
	}

	logging.Info("Removed old certificate and key for domain: %s", domain)

	// Use retry logic for certificate renewal
	if err := m.ObtainCertificateWithRetry(domain); err != nil {
		logging.Error("Certificate renewal failed after retries: %v", err)
		return
	}
	logging.Info("Certificate renewal process completed for domain: %s", domain)
}

func (m *Manager) saveCertificate(certs *certificate.Resource) error {
	// Create directory if it doesn't exist
	if err := os.MkdirAll(filepath.Dir(m.certPath), 0755); err != nil {
		return fmt.Errorf("could not create cert directory: %w", err)
	}

	if err := os.WriteFile(m.certPath, certs.Certificate, 0644); err != nil {
		return fmt.Errorf("could not save certificate: %w", err)
	}

	if err := os.WriteFile(m.keyPath, certs.PrivateKey, 0600); err != nil {
		return fmt.Errorf("could not save private key: %w", err)
	}

	logging.Info("Successfully saved certificate and key to %s and %s", m.certPath, m.keyPath)
	return nil
}

func GetCertificateInfo(certPath string) (*x509.Certificate, error) {
	certBytes, err := os.ReadFile(certPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read certificate file: %w", err)
	}

	block, _ := pem.Decode(certBytes)
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block from certificate")
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse certificate: %w", err)
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
	const maxStep = 32 * time.Second

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
			if wait < maxStep {
				wait *= 2
				if wait > maxStep {
					wait = maxStep
				}
			}
		}
	})
}
