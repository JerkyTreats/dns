package record

import (
	"fmt"
	"time"

	"github.com/jerkytreats/dns/internal/proxy"
)

// ToProxyRule converts a Record's ProxyRule to the proxy package's ProxyRule type
func (r *Record) ToProxyRule() (*proxy.ProxyRule, error) {
	if r.ProxyRule == nil {
		return nil, fmt.Errorf("record has no proxy rule")
	}

	return &proxy.ProxyRule{
		Hostname:   r.ProxyRule.Hostname,
		TargetIP:   r.ProxyRule.TargetIP,
		TargetPort: r.ProxyRule.TargetPort,
		Protocol:   r.ProxyRule.Protocol,
		Enabled:    r.ProxyRule.Enabled,
		CreatedAt:  r.ProxyRule.CreatedAt,
	}, nil
}

// FromProxyRule creates a ProxyRule from the proxy package's ProxyRule type
func FromProxyRule(proxyRule *proxy.ProxyRule) *ProxyRule {
	if proxyRule == nil {
		return nil
	}

	return &ProxyRule{
		Enabled:    proxyRule.Enabled,
		TargetIP:   proxyRule.TargetIP,
		TargetPort: proxyRule.TargetPort,
		Protocol:   proxyRule.Protocol,
		Hostname:   proxyRule.Hostname,
		CreatedAt:  proxyRule.CreatedAt,
	}
}

// NewRecord creates a new Record with the provided DNS information
func NewRecord(name, recordType, ip string) *Record {
	now := time.Now()
	return &Record{
		Name:      name,
		Type:      recordType,
		IP:        ip,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

// AddProxyRule adds a proxy rule to the record
func (r *Record) AddProxyRule(targetIP string, targetPort int, protocol, hostname string) {
	r.ProxyRule = &ProxyRule{
		Enabled:    true,
		TargetIP:   targetIP,
		TargetPort: targetPort,
		Protocol:   protocol,
		Hostname:   hostname,
		CreatedAt:  time.Now(),
	}
	r.UpdatedAt = time.Now()
}

// HasProxyRule returns true if the record has an associated proxy rule
func (r *Record) HasProxyRule() bool {
	return r.ProxyRule != nil
}

// IsProxyEnabled returns true if the record has a proxy rule and it's enabled
func (r *Record) IsProxyEnabled() bool {
	return r.HasProxyRule() && r.ProxyRule.Enabled
}

// Update updates the record's timestamp
func (r *Record) Update() {
	r.UpdatedAt = time.Now()
}
