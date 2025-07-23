package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/jerkytreats/dns/internal/config"
	"github.com/jerkytreats/dns/internal/dns/coredns"
	"github.com/jerkytreats/dns/internal/logging"
	"github.com/jerkytreats/dns/internal/proxy"
	"github.com/jerkytreats/dns/internal/tailscale"
)

// AddRecordRequest represents the simplified JSON payload for adding DNS records with automatic proxy setup
type AddRecordRequest struct {
	ServiceName string `json:"service_name"`
	Name        string `json:"name"`
	Port        *int   `json:"port,omitempty"` // Optional: triggers automatic proxy setup
}

// Record represents a DNS record with optional proxy information
type Record struct {
	*coredns.Record                  // Embed CoreDNS record
	ProxyRule       *proxy.ProxyRule `json:"proxy_rule,omitempty"`
}

// RecordHandler handles DNS record operations
type RecordHandler struct {
	manager         *coredns.Manager
	proxyManager    proxy.ProxyManagerInterface
	tailscaleClient tailscale.TailscaleClientInterface
}

// NewRecordHandler creates a new record handler
func NewRecordHandler(manager *coredns.Manager, proxyManager proxy.ProxyManagerInterface, tailscaleClient tailscale.TailscaleClientInterface) (*RecordHandler, error) {
	return &RecordHandler{
		manager:         manager,
		proxyManager:    proxyManager,
		tailscaleClient: tailscaleClient,
	}, nil
}

// getDNSManagerIP returns the Tailscale IP of the current DNS Manager device
func (h *RecordHandler) getDNSManagerIP() (string, error) {
	if h.tailscaleClient == nil {
		return "", fmt.Errorf("tailscale client not available")
	}

	var ip string
	var err error

	// Check if a specific device name is configured
	deviceName := config.GetString(tailscale.TailscaleDeviceNameKey)
	if deviceName != "" {
		logging.Debug("Using configured Tailscale device name for DNS Manager: %s", deviceName)
		ip, err = h.tailscaleClient.GetCurrentDeviceIPByName(deviceName)
	} else {
		logging.Debug("No device name configured, using hostname-based detection for DNS Manager")
		ip, err = h.tailscaleClient.GetCurrentDeviceIP()
	}

	if err != nil {
		logging.Error("Failed to get DNS Manager IP: %v", err)
		return "", fmt.Errorf("failed to get DNS Manager IP: %w", err)
	}

	logging.Debug("DNS Manager Tailscale IP: %s", ip)
	return ip, nil
}

// getDNSManagerDevice returns the Tailscale device information for the current DNS Manager
func (h *RecordHandler) getDNSManagerDevice() (*tailscale.Device, error) {
	if h.tailscaleClient == nil {
		return nil, fmt.Errorf("tailscale client not available")
	}

	devices, err := h.tailscaleClient.ListDevices()
	if err != nil {
		return nil, fmt.Errorf("failed to list devices: %w", err)
	}

	// Get current hostname to identify this device
	hostname, err := os.Hostname()
	if err != nil {
		return nil, fmt.Errorf("failed to get hostname: %w", err)
	}

	// Find current device by hostname matching
	for _, device := range devices {
		if device.Hostname == hostname || strings.Split(device.Hostname, ".")[0] == strings.Split(hostname, ".")[0] {
			logging.Debug("Found DNS Manager device: %s (%s)", device.Name, device.Hostname)
			return &device, nil
		}
	}

	return nil, fmt.Errorf("DNS Manager device not found in Tailscale network")
}

// AddRecord handles adding a new DNS record with automatic device detection and optional reverse proxy rule
func (h *RecordHandler) AddRecord(w http.ResponseWriter, r *http.Request) {
	logging.Info("Processing add record request")

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req AddRecordRequest

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate required fields
	if req.ServiceName == "" || req.Name == "" {
		http.Error(w, "Missing required fields: service_name, name", http.StatusBadRequest)
		return
	}

	// Validate port if specified
	if req.Port != nil && (*req.Port <= 0 || *req.Port > 65535) {
		http.Error(w, "Port must be between 1 and 65535", http.StatusBadRequest)
		return
	}

	// Get DNS Manager IP (where all DNS records will point)
	dnsManagerIP, err := h.getDNSManagerIP()
	if err != nil {
		logging.Error("Failed to get DNS Manager IP: %v", err)

		// Fallback for testing or when Tailscale client is unavailable
		if h.tailscaleClient == nil {
			logging.Warn("Tailscale client not available - using fallback IP for DNS records")
			dnsManagerIP = "127.0.0.1" // Test fallback
		} else {
			http.Error(w, "Failed to determine DNS Manager IP", http.StatusInternalServerError)
			return
		}
	}

	// Create DNS record pointing to DNS Manager
	if err := h.manager.AddRecord(req.ServiceName, req.Name, dnsManagerIP); err != nil {
		logging.Error("Failed to add DNS record: %v", err)
		http.Error(w, "Failed to add DNS record", http.StatusInternalServerError)
		return
	}

	logging.Info("Successfully added DNS record: %s -> %s", req.Name, dnsManagerIP)

	// Create unified record response
	record := Record{
		Record: &coredns.Record{
			Name: req.Name,
			Type: "A",
			IP:   dnsManagerIP,
		},
	}

	proxyRule, err := h.getProxyRule(r, &req)
	if err != nil {
		logging.Error("Failed to get proxy rule: %v", err)
	}

	if proxyRule != nil {
		record.ProxyRule = proxyRule
		err = h.proxyManager.AddRule(proxyRule)
		if err != nil {
			logging.Error("Failed to add proxy rule: %v", err)
		}
	}

	w.WriteHeader(http.StatusCreated)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(record)
}

// ListRecords handles listing all DNS records from the internal zone with proxy information
func (h *RecordHandler) ListRecords(w http.ResponseWriter, r *http.Request) {
	logging.Info("Processing list records request")

	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	dnsRecords, err := h.manager.ListRecords("internal")
	if err != nil {
		logging.Error("Failed to list records: %v", err)
		http.Error(w, "Failed to list records", http.StatusInternalServerError)
		return
	}

	// Get proxy rules to join with DNS records
	var proxyRules []*proxy.ProxyRule
	if h.proxyManager != nil {
		proxyRules = h.proxyManager.ListRules()
	}

	// Create map of proxy rules by hostname for efficient lookup
	proxyRuleMap := make(map[string]*proxy.ProxyRule)
	for _, rule := range proxyRules {
		proxyRuleMap[rule.Hostname] = rule
	}

	// Create records by joining DNS records with proxy rules
	records := make([]Record, 0, len(dnsRecords))
	for _, dnsRecord := range dnsRecords {
		record := Record{
			Record: &dnsRecord,
		}

		// Check if there's a corresponding proxy rule
		if proxyRule, exists := proxyRuleMap[dnsRecord.Name]; exists {
			record.ProxyRule = proxyRule
		}

		records = append(records, record)
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(records); err != nil {
		logging.Error("Failed to encode records to JSON: %v", err)
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}

	logging.Info("Successfully returned %d records", len(records))
}

func (h *RecordHandler) getProxyRule(r *http.Request, req *AddRecordRequest) (*proxy.ProxyRule, error) {

	if req.Port == nil {
		return nil, fmt.Errorf("port is required")
	}

	if h.proxyManager == nil {
		return nil, fmt.Errorf("proxy manager not available")
	}

	if h.tailscaleClient == nil {
		return nil, fmt.Errorf("tailscale client not available")
	}

	// 195.x.x.x
	sourceIP := proxy.GetSourceIP(r)
	if sourceIP == "" {
		return nil, fmt.Errorf("source IP not found")
	}

	// 100.x.x.x
	deviceIP, err := h.tailscaleClient.GetTailscaleIPFromSourceIP(sourceIP)
	if err != nil || deviceIP == "" {
		return nil, fmt.Errorf("failed to get device by IP: %w", err)
	}

	logging.Info("Detected source IP: %s", deviceIP)

	fqdn := fmt.Sprintf("%s.%s", req.Name, config.GetString(coredns.DNSDomainKey))

	proxyRule, err := proxy.NewProxyRule(fqdn, deviceIP, *req.Port, "http")
	if err != nil {
		return nil, fmt.Errorf("failed to create proxy rule: %w", err)
	}

	if err := h.proxyManager.AddRule(proxyRule); err != nil {
		return nil, fmt.Errorf("failed to add proxy rule: %w", err)
	}

	logging.Info("Successfully created proxy rule: %s -> %s:%d", req.Name, deviceIP, *req.Port)

	return proxyRule, nil

}
