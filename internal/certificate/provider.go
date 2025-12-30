package certificate

import (
	"context"
	"fmt"
	"time"

	cfapi "github.com/cloudflare/cloudflare-go"
	"github.com/go-acme/lego/v4/challenge"
	"github.com/go-acme/lego/v4/challenge/dns01"
	"github.com/jerkytreats/dns/internal/config"
	"github.com/jerkytreats/dns/internal/logging"
	"github.com/miekg/dns"
)

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
	fqdn, value := dns01.GetRecord(domain, keyAuth)

	logging.Info("Starting ACME challenge presentation for domain: %s", domain)
	logging.Debug("ACME challenge details - FQDN: %s, Expected value: %s", fqdn, value)

	// Ensure the base subdomain exists in Cloudflare before creating ACME challenge
	if err := d.ensureSubdomainExists(domain); err != nil {
		logging.Error("Failed to ensure subdomain exists: %v", err)
	}

	// Clean up any existing TXT records for this FQDN to prevent conflicts
	if err := d.cleanupRecords(fqdn); err != nil {
		logging.Error("Failed to cleanup existing records: %v", err)
	}

	// Additional wait after cleanup to ensure DNS propagation
	creationWait := getCreationWait()
	logging.Info("Waiting %v before creating new ACME challenge record...", creationWait)
	time.Sleep(creationWait)

	// Create the new ACME challenge record
	logging.Info("Creating new ACME challenge TXT record for %s", fqdn)
	if err := d.wrappedProvider.Present(domain, token, keyAuth); err != nil {
		return err
	}

	// Verify DNS propagation with timeout
	logging.Info("Verifying DNS propagation before ACME challenge...")
	deadline := time.Now().Add(5 * time.Minute)

	for time.Now().Before(deadline) {
		if err := d.verifyDNSPropagation(fqdn, value); err == nil {
			logging.Info("DNS propagation verified successfully")
			return nil
		}

		logging.Debug("DNS not fully propagated yet, waiting 30s...")
		time.Sleep(30 * time.Second)
	}

	logging.Warn("DNS propagation verification timed out, proceeding with ACME challenge")
	return nil
}

// CleanUp delegates to the wrapped provider.
func (d *CleaningDNSProvider) CleanUp(domain, token, keyAuth string) error {
	return d.wrappedProvider.CleanUp(domain, token, keyAuth)
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

	_, err = api.CreateDNSRecord(context.Background(), cfapi.ZoneIdentifier(d.cfZoneID), cfapi.CreateDNSRecordParams{
		Type:    "A",
		Name:    domain,
		Content: "127.0.0.1",
		TTL:     300,
		Comment: "Auto-created for Let's Encrypt ACME validation",
	})

	if err != nil {
		return fmt.Errorf("could not create subdomain record for %s: %w", domain, err)
	}

	logging.Info("Successfully created subdomain %s in Cloudflare", domain)
	return nil
}

// cleanupRecords removes all existing TXT records for the given FQDN
func (d *CleaningDNSProvider) cleanupRecords(fqdn string) error {
	api, err := cfapi.NewWithAPIToken(d.cfAPIToken)
	if err != nil {
		return fmt.Errorf("could not create Cloudflare API client: %w", err)
	}

	logging.Info("Cleaning up existing TXT records for %s", fqdn)

	records, _, err := api.ListDNSRecords(context.Background(), cfapi.ZoneIdentifier(d.cfZoneID), cfapi.ListDNSRecordsParams{
		Type: "TXT",
		Name: fqdn,
	})
	if err != nil {
		return fmt.Errorf("could not list DNS records: %w", err)
	}

	if len(records) == 0 {
		logging.Debug("No existing TXT records found for %s", fqdn)
		return nil
	}

	logging.Info("Found %d existing TXT records for %s, deleting all", len(records), fqdn)

	deletedCount := 0
	for _, record := range records {
		logging.Debug("Deleting TXT record ID %s with content: %s", record.ID, record.Content)
		if err := api.DeleteDNSRecord(context.Background(), cfapi.ZoneIdentifier(d.cfZoneID), record.ID); err != nil {
			logging.Error("Failed to delete DNS record %s: %v", record.ID, err)
		} else {
			deletedCount++
			logging.Debug("Successfully deleted TXT record ID %s", record.ID)
		}
	}

	logging.Info("Cleanup completed: deleted %d/%d TXT records for %s", deletedCount, len(records), fqdn)

	if deletedCount > 0 {
		cleanupWait := getCleanupWait()
		logging.Info("Waiting %v for DNS cleanup to propagate...", cleanupWait)
		time.Sleep(cleanupWait)
	}

	return nil
}

// verifyDNSPropagation checks if a DNS record has propagated to configured resolvers
func (d *CleaningDNSProvider) verifyDNSPropagation(fqdn, expectedValue string) error {
	resolvers := config.GetStringSlice(CertDNSResolversKey)
	if len(resolvers) == 0 {
		resolvers = []string{"8.8.8.8:53", "1.1.1.1:53"}
	}

	for _, resolver := range resolvers {
		logging.Debug("Checking DNS propagation on %s for %s", resolver, fqdn)
		if !checkDNSRecord(resolver, fqdn, expectedValue) {
			return fmt.Errorf("DNS record not propagated to %s", resolver)
		}
	}

	logging.Info("DNS propagation verified across configured resolvers")
	return nil
}

// checkDNSRecord queries a specific DNS resolver for a TXT record
func checkDNSRecord(resolver, fqdn, expectedValue string) bool {
	c := dns.Client{Timeout: 10 * time.Second}
	m := dns.Msg{}
	m.SetQuestion(dns.Fqdn(fqdn), dns.TypeTXT)

	r, _, err := c.Exchange(&m, resolver)
	if err != nil {
		logging.Debug("DNS query failed for %s on %s: %v", fqdn, resolver, err)
		return false
	}

	if r.Rcode != dns.RcodeSuccess {
		logging.Debug("DNS query returned error code %d for %s on %s", r.Rcode, fqdn, resolver)
		return false
	}

	for _, ans := range r.Answer {
		if txt, ok := ans.(*dns.TXT); ok {
			for _, value := range txt.Txt {
				if value == expectedValue {
					logging.Debug("Found expected TXT record on %s: %s", resolver, value)
					return true
				}
			}
		}
	}

	logging.Debug("Expected TXT record not found on %s", resolver)
	return false
}

// getCleanupWait returns the DNS cleanup propagation wait time
func getCleanupWait() time.Duration {
	if wait := config.GetDuration(CertDNSCleanupWaitKey); wait > 0 {
		return wait
	}

	isProduction := config.GetBool(CertUseProdCertsKey)
	if isProduction {
		return 120 * time.Second
	}
	return 90 * time.Second
}

// getCreationWait returns the DNS record creation wait time
func getCreationWait() time.Duration {
	if wait := config.GetDuration(CertDNSCreationWaitKey); wait > 0 {
		return wait
	}

	isProduction := config.GetBool(CertUseProdCertsKey)
	if isProduction {
		return 90 * time.Second
	}
	return 60 * time.Second
}

// exponentialBackoff returns a dns01.ChallengeOption that wraps lego's default
// pre-check with an exponential back-off wait loop.
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

