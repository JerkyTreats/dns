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
// If httpRequest is provided, it will attempt to create a proxy rule with automatic device detection
// Otherwise, it will use the DNS Manager IP for the proxy rule target
func (s *Service) CreateRecord(req CreateRecordRequest, httpRequest ...*http.Request) (*Record, error) {
	// Determine if we're using automatic device detection based on HTTP request
	usingDeviceDetection := len(httpRequest) > 0 && httpRequest[0] != nil
	if usingDeviceDetection {
		logging.Info("Creating record with device detection: %s.%s", req.Name, req.ServiceName)
	} else {
		logging.Info("Creating record: %s.%s", req.Name, req.ServiceName)
	}

	// 1. Normalize and validate input
	normalizedReq, err := s.validator.NormalizeCreateRequest(req)
	if err != nil {
		return nil, fmt.Errorf("normalization failed: %w", err)
	}
	
	// Log if normalization changed the input
	if normalizedReq.ServiceName != req.ServiceName || normalizedReq.Name != req.Name {
		logging.Info("Normalized record request: %s.%s -> %s.%s", 
			req.Name, req.ServiceName, normalizedReq.Name, normalizedReq.ServiceName)
	}
	
	// Use the normalized request for the rest of the operation
	req = normalizedReq

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
		var proxyRule *proxy.ProxyRule
		var err error

		if usingDeviceDetection {
			// Use device detection for proxy target
			proxyRule, err = s.createProxyRuleWithDeviceDetection(req)
		} else {
			// Use DNS Manager IP for proxy target
			proxyRule, err = s.createProxyRule(req, dnsManagerIP)
		}

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

// RemoveRecord removes a DNS record and associated proxy rule if exists
func (s *Service) RemoveRecord(req RemoveRecordRequest) error {
	logging.Info("Removing record: %s.%s", req.Name, req.ServiceName)

	// Validate the request
	if err := s.validator.ValidateRemoveRequest(req); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	// Remove DNS record
	if err := s.dnsManager.RemoveRecord(req.ServiceName, req.Name); err != nil {
		return fmt.Errorf("failed to remove DNS record: %w", err)
	}

	// Remove proxy rule if proxy manager is available
	if s.proxyManager != nil && s.proxyManager.IsEnabled() {
		domain := config.GetString(coredns.DNSDomainKey)
		if domain != "" {
			fqdn := fmt.Sprintf("%s.%s", req.Name, domain)
			if err := s.proxyManager.RemoveRule(fqdn); err != nil {
				logging.Warn("Failed to remove proxy rule for %s: %v", fqdn, err)
			} else {
				logging.Info("Removed proxy rule for %s", fqdn)
			}
		}
	}

	logging.Info("Successfully removed record: %s.%s", req.Name, req.ServiceName)
	return nil
}

// getDNSManagerIP returns the Tailscale IP of the current DNS Manager device
func (s *Service) getDNSManagerIP() (string, error) {
	if s.tailscaleClient == nil {
		logging.Warn("Tailscale client not available - using fallback IP for DNS records")
		return "", fmt.Errorf("tailscale client not available")
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

// createProxyRuleWithDeviceDetection creates a proxy rule with automatic device detection
func (s *Service) createProxyRuleWithDeviceDetection(req CreateRecordRequest) (*proxy.ProxyRule, error) {
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
