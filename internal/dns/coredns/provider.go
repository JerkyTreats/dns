package coredns

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-acme/lego/v4/challenge/dns01"
)

type DNSProvider struct {
	zonesPath string
}

func NewDNSProvider(zonesPath string) *DNSProvider {
	return &DNSProvider{zonesPath: zonesPath}
}

func (d *DNSProvider) Present(domain, token, keyAuth string) error {
	fqdn, value := dns01.GetRecord(domain, keyAuth)

	// The fqdn from GetRecord has a trailing dot, which we need to remove for the ORIGIN
	origin := strings.TrimSuffix(fqdn, ".")

	challengeContent := fmt.Sprintf(`$ORIGIN %s.
@	60 IN	TXT	"%s"`, origin, value)

	challengeFile := d.getChallengeFilePath(fqdn)
	return os.WriteFile(challengeFile, []byte(challengeContent), 0644)
}

func (d *DNSProvider) CleanUp(domain, token, keyAuth string) error {
	fqdn, _ := dns01.GetRecord(domain, keyAuth)
	challengeFile := d.getChallengeFilePath(fqdn)
	return os.Remove(challengeFile)
}

func (d *DNSProvider) getChallengeFilePath(fqdn string) string {
	return filepath.Join(d.zonesPath, fmt.Sprintf("%s.zone", fqdn))
}
