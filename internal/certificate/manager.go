package certificate

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/go-acme/lego/v4/certcrypto"
	"github.com/go-acme/lego/v4/certificate"
	"github.com/go-acme/lego/v4/lego"
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
	CertRenewalEnabledKey       = "certificate.renewal.enabled"
	CertRenewalRenewBeforeKey   = "certificate.renewal.renew_before"
	CertRenewalCheckIntervalKey = "certificate.renewal.check_interval"
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
}

// NewManager creates a new certificate manager.
func NewManager() (*Manager, error) {
	logging.Info("Creating certificate manager")

	email := config.GetString(CertEmailKey)
	certPath := config.GetString(CertCertFileKey)
	keyPath := config.GetString(CertKeyFileKey)
	zonesPath := config.GetString("dns.coredns.zones_path")

	dnsProvider := coredns.NewDNSProvider(zonesPath)

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

	client, err := lego.NewClient(legoConfig)
	if err != nil {
		return nil, fmt.Errorf("could not create lego client: %w", err)
	}

	err = client.Challenge.SetDNS01Provider(dnsProvider)
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
	return m.saveCertificate(certs)
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

	if err := m.ObtainCertificate(domain); err != nil {
		logging.Error("Failed to obtain new certificate for domain %s: %v", domain, err)
		return
	}

	logging.Info("Successfully renewed certificate for domain: %s", domain)
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
