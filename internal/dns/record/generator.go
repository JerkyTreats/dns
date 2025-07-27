package record

import (
	"fmt"
	"time"

	"github.com/jerkytreats/dns/internal/config"
	"github.com/jerkytreats/dns/internal/dns/coredns"
	"github.com/jerkytreats/dns/internal/logging"
)

// RecordGenerator implements the Generator interface
type RecordGenerator struct {
	dnsManager   DNSManagerInterface
	proxyManager ProxyManagerInterface
}

// NewGenerator creates a new record generator
func NewGenerator(dnsManager DNSManagerInterface, proxyManager ProxyManagerInterface) Generator {
	return &RecordGenerator{
		dnsManager:   dnsManager,
		proxyManager: proxyManager,
	}
}

// GenerateRecords creates unified Record structs from existing DNS zones and proxy rules
func (g *RecordGenerator) GenerateRecords() ([]Record, error) {
	logging.Debug("Generating unified records from DNS zones and proxy rules")

	// Get the internal service name from configuration
	serviceName := "internal" // Default service name for internal zone
	
	// Get DNS records from the internal zone
	dnsRecords, err := g.dnsManager.ListRecords(serviceName)
	if err != nil {
		return nil, fmt.Errorf("failed to list DNS records: %w", err)
	}

	// Get proxy rules if proxy manager is available
	var proxyRules []*ProxyRule
	if g.proxyManager != nil && g.proxyManager.IsEnabled() {
		rawProxyRules := g.proxyManager.ListRules()
		proxyRules = make([]*ProxyRule, len(rawProxyRules))
		for i, rule := range rawProxyRules {
			proxyRules[i] = FromProxyRule(rule)
		}
	}

	// Create map of proxy rules by hostname for efficient lookup
	proxyRuleMap := make(map[string]*ProxyRule)
	for _, rule := range proxyRules {
		// Extract hostname without domain suffix for matching
		hostname := g.extractHostnameFromFQDN(rule.Hostname)
		proxyRuleMap[hostname] = rule
	}

	// Create unified records by joining DNS records with proxy rules
	records := make([]Record, 0, len(dnsRecords))
	for _, dnsRecord := range dnsRecords {
		record := Record{
			Name:      dnsRecord.Name,
			Type:      dnsRecord.Type,
			IP:        dnsRecord.IP,
			CreatedAt: time.Now(), // We don't have creation time from DNS records
			UpdatedAt: time.Now(),
		}

		// Check if there's a corresponding proxy rule
		if proxyRule, exists := proxyRuleMap[dnsRecord.Name]; exists {
			record.ProxyRule = proxyRule
		}

		records = append(records, record)
	}

	logging.Debug("Generated %d unified records (%d DNS records, %d proxy rules)", 
		len(records), len(dnsRecords), len(proxyRules))

	return records, nil
}

// extractHostnameFromFQDN extracts the hostname part from a FQDN
// e.g., "llm.internal.jerkytreats.dev" -> "llm"
func (g *RecordGenerator) extractHostnameFromFQDN(fqdn string) string {
	domain := config.GetString(coredns.DNSDomainKey)
	if domain == "" {
		// Fallback: just take the first part before the first dot
		parts := splitHostname(fqdn)
		if len(parts) > 0 {
			return parts[0]
		}
		return fqdn
	}

	// Remove the domain suffix to get the hostname
	expectedSuffix := "." + domain
	if len(fqdn) > len(expectedSuffix) && 
	   fqdn[len(fqdn)-len(expectedSuffix):] == expectedSuffix {
		hostname := fqdn[:len(fqdn)-len(expectedSuffix)]
		
		// If there are still dots, take the last part (service name)
		parts := splitHostname(hostname)
		if len(parts) > 0 {
			return parts[len(parts)-1]
		}
		return hostname
	}

	// Fallback: take the first part before the first dot
	parts := splitHostname(fqdn)
	if len(parts) > 0 {
		return parts[0]
	}
	
	return fqdn
}

// splitHostname splits a hostname by dots
func splitHostname(hostname string) []string {
	if hostname == "" {
		return []string{}
	}

	var parts []string
	current := ""
	
	for _, char := range hostname {
		if char == '.' {
			if current != "" {
				parts = append(parts, current)
				current = ""
			}
		} else {
			current += string(char)
		}
	}
	
	if current != "" {
		parts = append(parts, current)
	}
	
	return parts
}
