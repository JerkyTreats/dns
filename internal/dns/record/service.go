package record

import (
	"fmt"
	"net/http"

	"github.com/jerkytreats/dns/internal/config"
	"github.com/jerkytreats/dns/internal/dns/coredns"
	"github.com/jerkytreats/dns/internal/logging"
	"github.com/jerkytreats/dns/internal/proxy"
)

// Service orchestrates record creation, validation, and coordination
type Service struct {
	dnsManager      DNSManagerInterface
	proxyManager    ProxyManagerInterface
	tailscaleClient TailscaleClientInterface
	generator       Generator
	validator       Validator
}

// NewService creates a new record service
func NewService(
	dnsManager DNSManagerInterface,
	proxyManager ProxyManagerInterface,
	tailscaleClient TailscaleClientInterface,
) *Service {
	generator := NewGenerator(dnsManager, proxyManager)
	validator := NewValidator()

	return &Service{
		dnsManager:      dnsManager,
		proxyManager:    proxyManager,
		tailscaleClient: tailscaleClient,
		generator:       generator,
		validator:       validator,
	}
}

// CreateRecord creates a new record with DNS and optional proxy rule
func (s *Service) CreateRecord(req CreateRecordRequest) (*Record, error) {
	logging.Info("Creating record: %s.%s", req.Name, req.ServiceName)

	// 1. Validate input
	if err := s.validator.ValidateCreateRequest(req); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	// 2. Resolve DNS Manager IP for DNS record
	dnsManagerIP, err := s.getDNSManagerIP()
	if err != nil {
		return nil, fmt.Errorf("failed to get DNS Manager IP: %w", err)
	}

	// 3. Create DNS record pointing to DNS Manager
	if err := s.dnsManager.AddRecord(req.ServiceName, req.Name, dnsManagerIP); err != nil {
		return nil, fmt.Errorf("failed to add DNS record: %w", err)
	}

	logging.Info("Successfully added DNS record: %s -> %s", req.Name, dnsManagerIP)

	// 4. Create unified record response
	record := NewRecord(req.Name, "A", dnsManagerIP)

	// 5. Create proxy rule if port is specified and proxy manager is available
	if req.Port != nil && s.proxyManager != nil && s.proxyManager.IsEnabled() {
		proxyRule, err := s.createProxyRule(req, dnsManagerIP)
		if err != nil {
			logging.Error("Failed to create proxy rule: %v", err)
			// Don't fail the entire operation, just log the error
		} else {
			record.ProxyRule = FromProxyRule(proxyRule)
			logging.Info("Successfully created proxy rule: %s -> %s:%d", 
				req.Name, proxyRule.TargetIP, proxyRule.TargetPort)
		}
	}

	return record, nil
}

// CreateRecordWithSourceIP creates a record with automatic device detection from source IP
func (s *Service) CreateRecordWithSourceIP(req CreateRecordRequest, r *http.Request) (*Record, error) {
	logging.Info("Creating record with source IP detection: %s.%s", req.Name, req.ServiceName)

	// 1. Validate input
	if err := s.validator.ValidateCreateRequest(req); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	// 2. Resolve DNS Manager IP for DNS record
	dnsManagerIP, err := s.getDNSManagerIP()
	if err != nil {
		return nil, fmt.Errorf("failed to get DNS Manager IP: %w", err)
	}

	// 3. Create DNS record pointing to DNS Manager
	if err := s.dnsManager.AddRecord(req.ServiceName, req.Name, dnsManagerIP); err != nil {
		return nil, fmt.Errorf("failed to add DNS record: %w", err)
	}

	logging.Info("Successfully added DNS record: %s -> %s", req.Name, dnsManagerIP)

	// 4. Create unified record response
	record := NewRecord(req.Name, "A", dnsManagerIP)

	// 5. Create proxy rule if port is specified and proxy manager is available
	if req.Port != nil && s.proxyManager != nil && s.proxyManager.IsEnabled() {
		proxyRule, err := s.createProxyRuleFromRequest(req, r)
		if err != nil {
			logging.Error("Failed to create proxy rule: %v", err)
			// Don't fail the entire operation, just log the error
		} else {
			record.ProxyRule = FromProxyRule(proxyRule)
			logging.Info("Successfully created proxy rule: %s -> %s:%d", 
				req.Name, proxyRule.TargetIP, proxyRule.TargetPort)
		}
	}

	return record, nil
}

// ListRecords returns all records by generating them from persisted sources
func (s *Service) ListRecords() ([]Record, error) {
	logging.Debug("Listing all records")
	return s.generator.GenerateRecords()
}

// getDNSManagerIP returns the Tailscale IP of the current DNS Manager device
func (s *Service) getDNSManagerIP() (string, error) {
	if s.tailscaleClient == nil {
		logging.Warn("Tailscale client not available - using fallback IP for DNS records")
		return "127.0.0.1", nil // Test fallback
	}

	var ip string
	var err error

	// Check if a specific device name is configured
	deviceName := config.GetString("tailscale.device_name")
	if deviceName != "" {
		logging.Debug("Using configured Tailscale device name for DNS Manager: %s", deviceName)
		ip, err = s.tailscaleClient.GetCurrentDeviceIPByName(deviceName)
	} else {
		logging.Debug("No device name configured, using hostname-based detection for DNS Manager")
		ip, err = s.tailscaleClient.GetCurrentDeviceIP()
	}

	if err != nil {
		logging.Error("Failed to get DNS Manager IP: %v", err)
		return "", fmt.Errorf("failed to get DNS Manager IP: %w", err)
	}

	logging.Debug("DNS Manager Tailscale IP: %s", ip)
	return ip, nil
}

// createProxyRule creates a proxy rule for the given request
func (s *Service) createProxyRule(req CreateRecordRequest, dnsManagerIP string) (*proxy.ProxyRule, error) {
	if req.Port == nil {
		return nil, fmt.Errorf("port is required for proxy rule creation")
	}

	// For this method, we assume the target IP is the DNS Manager IP
	// This is a simplified version - in practice, you might want to resolve the actual target
	targetIP := dnsManagerIP

	// Build FQDN
	domain := config.GetString(coredns.DNSDomainKey)
	fqdn := fmt.Sprintf("%s.%s", req.Name, domain)

	// Create proxy rule
	proxyRule, err := proxy.NewProxyRule(fqdn, targetIP, *req.Port, "http")
	if err != nil {
		return nil, fmt.Errorf("failed to create proxy rule: %w", err)
	}

	// Add rule to proxy manager
	if err := s.proxyManager.AddRule(proxyRule); err != nil {
		return nil, fmt.Errorf("failed to add proxy rule: %w", err)
	}

	return proxyRule, nil
}

// createProxyRuleFromRequest creates a proxy rule with automatic device detection from HTTP request
func (s *Service) createProxyRuleFromRequest(req CreateRecordRequest, r *http.Request) (*proxy.ProxyRule, error) {
	if req.Port == nil {
		return nil, fmt.Errorf("port is required for proxy rule creation")
	}

	if s.tailscaleClient == nil {
		return nil, fmt.Errorf("tailscale client not available")
	}

	// Get device name from config or use the one from the request header if available
	deviceName := config.GetString("tailscale.device_name")
	
	// Try to get device IP directly using the device name
	var deviceIP string
	var err error
	
	if deviceName != "" {
		logging.Debug("Using configured device name for proxy target: %s", deviceName)
		deviceIP, err = s.tailscaleClient.GetCurrentDeviceIPByName(deviceName)
		if err != nil || deviceIP == "" {
			return nil, fmt.Errorf("failed to get Tailscale IP for device '%s': %w", deviceName, err)
		}
	} else {
		// Fallback to hostname-based detection
		logging.Debug("No device name configured, using hostname-based detection for proxy target")
		deviceIP, err = s.tailscaleClient.GetCurrentDeviceIP()
		if err != nil || deviceIP == "" {
			return nil, fmt.Errorf("failed to get current device Tailscale IP: %w", err)
		}
	}

	logging.Info("Using Tailscale device IP for proxy rule: %s", deviceIP)

	// Build FQDN
	domain := config.GetString(coredns.DNSDomainKey)
	fqdn := fmt.Sprintf("%s.%s", req.Name, domain)

	// Create proxy rule
	proxyRule, err := proxy.NewProxyRule(fqdn, deviceIP, *req.Port, "http")
	if err != nil {
		return nil, fmt.Errorf("failed to create proxy rule: %w", err)
	}

	// Add rule to proxy manager
	if err := s.proxyManager.AddRule(proxyRule); err != nil {
		return nil, fmt.Errorf("failed to add proxy rule: %w", err)
	}

	return proxyRule, nil
}
