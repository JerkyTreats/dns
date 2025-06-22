package coredns

// RestartManagerInterface defines the interface for restart management
type RestartManagerInterface interface {
	RestartCoreDNS() error
	RestartCoreDNSWithRollback(backupPath string) error
	IsHealthy() bool
	GetHealthStatus() *HealthStatus
}

// ConfigManagerInterface defines the interface for configuration management
type ConfigManagerInterface interface {
	GenerateCorefile() error
	AddDomain(domain string, tlsConfig *TLSConfig) error
	RemoveDomain(domain string) error
	EnableTLS(domain, certFile, keyFile string) error
	DisableTLS(domain string) error
	GetAllDomains() map[string]*DomainConfig
	GetConfigVersion() int
}
