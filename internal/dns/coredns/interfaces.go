package coredns

// RestartManagerInterface defines the interface for restart management
type RestartManagerInterface interface {
	RestartCoreDNS() error
	RestartCoreDNSWithRollback(backupPath string) error
	IsHealthy() bool
	GetHealthStatus() *HealthStatus
}
