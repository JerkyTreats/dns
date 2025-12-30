package record

import (
	"time"

	"github.com/jerkytreats/dns/internal/dns/coredns"
	"github.com/jerkytreats/dns/internal/proxy"
	"github.com/jerkytreats/dns/internal/tailscale"
)

// CreateRecordRequest represents the request to create a new record
type CreateRecordRequest struct {
	ServiceName  string  `json:"service_name"`
	Name         string  `json:"name"`
	Port         *int    `json:"port,omitempty"`          // Optional: triggers automatic proxy setup
	TargetDevice *string `json:"target_device,omitempty"` // Optional: Tailscale device name for proxy target (defaults to DNS manager device)
}

// RemoveRecordRequest represents the request to remove a record
type RemoveRecordRequest struct {
	ServiceName string `json:"service_name"`
	Name        string `json:"name"`
}

// Record represents a unified DNS record with optional proxy information
type Record struct {
	// DNS Information
	Name string `json:"name"` // e.g., "llm"
	Type string `json:"type"` // e.g., "A"
	IP   string `json:"ip"`   // e.g., "100.64.1.5" (DNS Manager IP)

	// Proxy Rule (Optional)
	ProxyRule *ProxyRule `json:"proxy_rule,omitempty"`

	// Metadata
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ProxyRule represents reverse proxy configuration
type ProxyRule struct {
	Enabled    bool   `json:"enabled"`
	TargetIP   string `json:"target_ip"`   // 100.64.1.10 (actual service IP)
	TargetPort int    `json:"target_port"` // 8080 (actual service port)
	Protocol   string `json:"protocol"`    // http/https
	Hostname   string `json:"hostname"`    // llm.internal.jerkytreats.dev
	CreatedAt  time.Time `json:"created_at"`
}

// DNSManagerInterface defines the interface for DNS operations
type DNSManagerInterface interface {
	AddRecord(serviceName, name, ip string) error
	RemoveRecord(serviceName, name string) error
	ListRecords(serviceName string) ([]coredns.Record, error)
}

// ProxyManagerInterface defines the interface for proxy operations
type ProxyManagerInterface interface {
	AddRule(proxyRule *proxy.ProxyRule) error
	RemoveRule(hostname string) error
	ListRules() []*proxy.ProxyRule
	IsEnabled() bool
}

// TailscaleClientInterface defines the interface for Tailscale operations
type TailscaleClientInterface interface {
	GetCurrentDeviceIPByName(deviceName string) (string, error)
	GetCurrentDeviceIP() (string, error)
	GetTailscaleIPFromSourceIP(sourceIP string) (string, error)
	ListDevices() ([]tailscale.Device, error)
}

// Validator defines the interface for record validation
type Validator interface {
	ValidateCreateRequest(req CreateRecordRequest) error
	ValidateRemoveRequest(req RemoveRecordRequest) error
	NormalizeCreateRequest(req CreateRecordRequest) (CreateRecordRequest, error)
}

// Generator defines the interface for runtime record generation
type Generator interface {
	GenerateRecords() ([]Record, error)
}
