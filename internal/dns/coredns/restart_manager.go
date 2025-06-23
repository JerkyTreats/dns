package coredns

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"time"

	"github.com/jerkytreats/dns/internal/config"
	"github.com/jerkytreats/dns/internal/healthcheck"
	"github.com/jerkytreats/dns/internal/logging"
)

const (
	// Default timeouts
	defaultRestartTimeout = 30 * time.Second
	defaultHealthTimeout  = 10 * time.Second

	// Health check configuration
	defaultHealthRetries = 5
	defaultHealthDelay   = 2 * time.Second

	// DNS test configuration
	defaultDNSServer = "127.0.0.1:53" // Default for local/single container setup
	dockerDNSServer  = "coredns:53"   // For multi-container Docker setup
)

// RestartManager handles CoreDNS container lifecycle management
type RestartManager struct {
	restartCommand  []string
	restartTimeout  time.Duration
	lastRestartTime time.Time
	restartCount    int
	dnsServer       string // DNS server address for health checks
	healthChecker   healthcheck.Checker
}

// RestartResult represents the result of a restart operation
type RestartResult struct {
	Success         bool
	RestartTime     time.Duration
	HealthCheckTime time.Duration
	Error           error
	RestartOutput   string
	HealthStatus    string
}

// NewRestartManager creates a new RestartManager instance
func NewRestartManager() *RestartManager {
	logging.Info("Creating CoreDNS RestartManager")

	// Get restart command from configuration
	restartCmd := config.GetStringSlice("dns.coredns.reload_command")

	// Get timeouts from configuration with defaults
	restartTimeout := config.GetDuration(DNSRestartTimeoutKey)
	if restartTimeout == 0 {
		restartTimeout = defaultRestartTimeout
	}

	// Use a portion of restart timeout for health checks
	healthTimeout := restartTimeout / 3
	if healthTimeout < defaultHealthTimeout {
		healthTimeout = defaultHealthTimeout
	}

	healthRetries := config.GetInt(DNSHealthCheckRetriesKey)
	if healthRetries == 0 {
		healthRetries = defaultHealthRetries
	}

	// Determine DNS server address based on environment
	dnsServer := defaultDNSServer
	if healthcheck.IsDockerEnvironment() {
		dnsServer = dockerDNSServer
		logging.Info("Detected Docker environment, using DNS server: %s", dnsServer)
	} else {
		logging.Info("Using local DNS server: %s", dnsServer)
	}

	hc := healthcheck.NewDNSHealthChecker(dnsServer, healthTimeout, healthRetries, defaultHealthDelay)

	return &RestartManager{
		restartCommand: restartCmd,
		restartTimeout: restartTimeout,
		dnsServer:      dnsServer,
		healthChecker:  hc,
	}
}

// RestartCoreDNS restarts the CoreDNS container with health checks
func (rm *RestartManager) RestartCoreDNS() error {
	startTime := time.Now()

	logging.Info("Starting CoreDNS restart operation")

	// Perform restart with rollback capability
	result := rm.performRestart()

	// Log result
	rm.logRestartResult(result, time.Since(startTime))

	if !result.Success {
		return fmt.Errorf("CoreDNS restart failed: %w", result.Error)
	}

	rm.lastRestartTime = time.Now()
	rm.restartCount++

	logging.Info("CoreDNS restart completed successfully")
	return nil
}

// RestartCoreDNSWithRollback restarts CoreDNS with automatic rollback on failure
func (rm *RestartManager) RestartCoreDNSWithRollback(backupConfigPath string) error {
	logging.Info("Starting CoreDNS restart with rollback capability")

	// Perform restart
	result := rm.performRestart()

	if !result.Success {
		logging.Error("CoreDNS restart failed, attempting rollback")

		// Attempt rollback
		if rollbackErr := rm.rollbackConfiguration(backupConfigPath); rollbackErr != nil {
			logging.Error("Rollback failed: %v", rollbackErr)
			return fmt.Errorf("restart failed and rollback failed: restart=%w, rollback=%w", result.Error, rollbackErr)
		}

		logging.Info("Successfully rolled back configuration")
		return fmt.Errorf("restart failed but rollback succeeded: %w", result.Error)
	}

	rm.lastRestartTime = time.Now()
	rm.restartCount++

	logging.Info("CoreDNS restart with rollback completed successfully")
	return nil
}

// IsHealthy checks if CoreDNS is responding to DNS queries
func (rm *RestartManager) IsHealthy() bool {
	// If no restart command is configured, assume healthy since we rely on CoreDNS reload plugin
	if len(rm.restartCommand) == 0 {
		logging.Debug("No restart command configured - assuming healthy (CoreDNS reload plugin)")
		return true
	}

	result := rm.performHealthCheck()
	return result.Success
}

// GetHealthStatus returns detailed health status information
func (rm *RestartManager) GetHealthStatus() *HealthStatus {
	result := rm.performHealthCheck()

	return &HealthStatus{
		Healthy:      result.Success,
		ResponseTime: result.HealthCheckTime,
		LastCheck:    time.Now(),
		Error:        result.Error,
		CheckDetails: result.HealthStatus,
		RestartCount: rm.restartCount,
		LastRestart:  rm.lastRestartTime,
	}
}

// HealthStatus represents the health status of CoreDNS
type HealthStatus struct {
	Healthy      bool
	ResponseTime time.Duration
	LastCheck    time.Time
	Error        error
	CheckDetails string
	RestartCount int
	LastRestart  time.Time
}

// performRestart executes the restart command and validates the result
func (rm *RestartManager) performRestart() *RestartResult {
	result := &RestartResult{}
	restartStart := time.Now()

	// If no restart command is configured, rely on CoreDNS reload plugin
	if len(rm.restartCommand) == 0 {
		logging.Info("No restart command configured - relying on CoreDNS reload plugin")
		result.Success = true
		result.RestartTime = 0
		result.RestartOutput = "no restart needed - CoreDNS reload plugin handles config changes"
		result.HealthStatus = "healthy - relying on CoreDNS reload plugin"
		// Skip health checks entirely when no restart is needed
		result.HealthCheckTime = 0
		return result
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), rm.restartTimeout)
	defer cancel()

	// Execute restart command
	cmd := exec.CommandContext(ctx, rm.restartCommand[0], rm.restartCommand[1:]...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	logging.Info("Executing restart command: %v", rm.restartCommand)

	if err := cmd.Run(); err != nil {
		result.Error = fmt.Errorf("restart command failed: %w: %s", err, stderr.String())
		result.RestartOutput = stdout.String() + stderr.String()
		return result
	}

	result.RestartTime = time.Since(restartStart)
	result.RestartOutput = stdout.String()

	// Wait a moment for the service to start
	time.Sleep(2 * time.Second)

	// Perform health check
	healthStart := time.Now()
	if !rm.healthChecker.WaitHealthy() {
		result.Error = fmt.Errorf("health check failed after restart")
		result.HealthStatus = "service not responding after restart"
		return result
	}

	result.HealthCheckTime = time.Since(healthStart)
	result.Success = true
	result.HealthStatus = "healthy"

	return result
}

// performHealthCheck delegates to DNSHealthChecker for a single check and wraps
// the result in a RestartResult for legacy callers.
func (rm *RestartManager) performHealthCheck() *RestartResult {
	ok, dur, err := rm.healthChecker.CheckOnce()
	status := "healthy"
	if err != nil {
		status = err.Error()
	}
	return &RestartResult{
		Success:         ok,
		HealthCheckTime: dur,
		Error:           err,
		HealthStatus:    status,
	}
}

// rollbackConfiguration attempts to rollback to a previous configuration
func (rm *RestartManager) rollbackConfiguration(backupConfigPath string) error {
	logging.Info("Attempting configuration rollback")

	// This would typically involve:
	// 1. Restoring backup configuration file
	// 2. Restarting CoreDNS with the backup config
	// 3. Verifying the rollback was successful

	// For now, we'll implement a basic version
	// In a real implementation, this would integrate with the ConfigManager
	// to restore a previous known-good configuration

	if backupConfigPath == "" {
		return fmt.Errorf("no backup configuration path provided")
	}

	// TODO: Implement actual rollback logic
	// This should coordinate with ConfigManager to restore previous config

	logging.Warn("Rollback functionality not fully implemented")
	return fmt.Errorf("rollback functionality not yet implemented")
}

// logRestartResult logs the detailed restart operation result
func (rm *RestartManager) logRestartResult(result *RestartResult, totalTime time.Duration) {
	if result.Success {
		logging.Info("CoreDNS restart successful - Total: %v, Restart: %v, Health: %v",
			totalTime, result.RestartTime, result.HealthCheckTime)
	} else {
		logging.Error("CoreDNS restart failed - Total: %v, Error: %v", totalTime, result.Error)
		if result.RestartOutput != "" {
			logging.Error("Restart output: %s", result.RestartOutput)
		}
	}
}
