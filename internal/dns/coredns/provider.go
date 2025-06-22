package coredns

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

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

	// The fqdn from GetRecord has a trailing dot, which we need to remove for the ORIGIN
	origin := strings.TrimSuffix(fqdn, ".")

	challengeContent := fmt.Sprintf(`$ORIGIN %s.
@	60 IN	TXT	"%s"`, origin, value)

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
	// Remove trailing dot and use the full FQDN for the challenge file
	cleanFqdn := strings.TrimSuffix(fqdn, ".")
	return filepath.Join(d.zonesPath, fmt.Sprintf("%s.zone", cleanFqdn))
}
