package coredns

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/go-acme/lego/v4/challenge/dns01"
	"github.com/jerkytreats/dns/internal/logging"
)

type DNSProvider struct {
	zonesPath string
}

func NewDNSProvider(zonesPath string) *DNSProvider {
	return &DNSProvider{zonesPath: zonesPath}
}

func (d *DNSProvider) Present(domain, token, keyAuth string) error {
	logging.Info("Presenting DNS challenge for domain: %s", domain)

	fqdn, value := dns01.GetRecord(domain, keyAuth)

	challengeContent := fmt.Sprintf(`$ORIGIN _acme-challenge.internal.jerkytreats.dev.
@	60 IN	TXT	"%s"`, value)

	challengeFile := d.getChallengeFilePath(fqdn)

	// Ensure the directory exists
	if err := os.MkdirAll(filepath.Dir(challengeFile), 0755); err != nil {
		logging.Error("Failed to create challenge file directory: %v", err)
		return fmt.Errorf("failed to create challenge file directory: %w", err)
	}

	if err := os.WriteFile(challengeFile, []byte(challengeContent), 0644); err != nil {
		logging.Error("Failed to write challenge file: %v", err)
		return fmt.Errorf("failed to write challenge file to %s: %w", challengeFile, err)
	}

	logging.Info("Successfully created challenge file: %s", challengeFile)
	logging.Info("Challenge content: %s", challengeContent)
	return nil
}

func (d *DNSProvider) CleanUp(domain, token, keyAuth string) error {
	logging.Info("Cleaning up DNS challenge for domain: %s", domain)

	fqdn, _ := dns01.GetRecord(domain, keyAuth)
	challengeFile := d.getChallengeFilePath(fqdn)

	if err := os.Remove(challengeFile); err != nil {
		if os.IsNotExist(err) {
			logging.Warn("Challenge file already removed or never existed: %s", challengeFile)
			return nil
		}
		logging.Error("Failed to remove challenge file: %v", err)
		return fmt.Errorf("failed to remove challenge file %s: %w", challengeFile, err)
	}

	logging.Info("Successfully removed challenge file: %s", challengeFile)
	return nil
}

func (d *DNSProvider) getChallengeFilePath(fqdn string) string {
	// Always create the static ACME challenge zone file that matches the Corefile configuration
	// This should be _acme-challenge.internal.jerkytreats.dev.zone regardless of the domain
	return filepath.Join(d.zonesPath, "_acme-challenge.internal.jerkytreats.dev.zone")
}
