package healthcheck

import (
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"github.com/jerkytreats/dns/internal/logging"
)

// IsDockerEnvironment detects if the code is running inside a Docker container.
func IsDockerEnvironment() bool {
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return true
	}

	if data, err := os.ReadFile("/proc/1/cgroup"); err == nil {
		content := string(data)
		if strings.Contains(content, "docker") || strings.Contains(content, "/docker-") {
			return true
		}
	}

	if hostname, err := os.Hostname(); err == nil {
		if len(hostname) == 12 && isHexString(hostname) {
			return true
		}
	}

	return false
}

func isHexString(s string) bool {
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

// DNSHealthChecker performs UDP-based health checks against a DNS server.
// It implements the Checker interface.

type DNSHealthChecker struct {
	server  string
	timeout time.Duration
	retries int
	delay   time.Duration
}

// NewDNSHealthChecker constructs a DNSHealthChecker with the provided settings.
func NewDNSHealthChecker(server string, timeout time.Duration, retries int, delay time.Duration) *DNSHealthChecker {
	return &DNSHealthChecker{
		server:  server,
		timeout: timeout,
		retries: retries,
		delay:   delay,
	}
}

func (hc *DNSHealthChecker) Name() string { return "coredns" }

// CheckOnce performs a single UDP query to the configured DNS server.
// It returns success bool, response time, and optional error.
func (hc *DNSHealthChecker) CheckOnce() (bool, time.Duration, error) {
	start := time.Now()

	conn, err := net.DialTimeout("udp", hc.server, hc.timeout)
	if err != nil {
		return false, 0, err
	}
	defer conn.Close()

	// Build a minimal DNS query for the root label.
	query := []byte{
		0x12, 0x34, // ID
		0x01, 0x00, // Flags
		0x00, 0x01, // Questions
		0x00, 0x00, // Answer RRs
		0x00, 0x00, // Authority RRs
		0x00, 0x00, // Additional RRs
		0x00,       // Root label
		0x00, 0x01, // Type A
		0x00, 0x01, // Class IN
	}

	if _, err := conn.Write(query); err != nil {
		return false, 0, err
	}

	conn.SetReadDeadline(time.Now().Add(hc.timeout))

	resp := make([]byte, 512)
	n, err := conn.Read(resp)
	if err != nil {
		return false, 0, err
	}

	if n < 12 {
		return false, 0, fmt.Errorf("DNS response too short: %d bytes", n)
	}

	if resp[0] != 0x12 || resp[1] != 0x34 {
		return false, 0, fmt.Errorf("DNS response ID mismatch")
	}

	return true, time.Since(start), nil
}

// WaitHealthy performs repeated health checks until success or retries exhausted.
// Returns true if a check succeeded.
func (hc *DNSHealthChecker) WaitHealthy() bool {
	for i := 0; i < hc.retries; i++ {
		ok, _, _ := hc.CheckOnce()
		if ok {
			return true
		}
		time.Sleep(hc.delay)
	}
	return false
}

// TestBasicConnectivity performs a basic UDP connection test to verify the server is reachable.
// This is a simple connectivity test that doesn't perform actual DNS queries.
func TestBasicConnectivity(server string, timeout time.Duration) error {
	logging.Info("Testing basic connectivity to CoreDNS at %s...", server)
	conn, err := net.DialTimeout("udp", server, timeout)
	if err != nil {
		return fmt.Errorf("cannot establish UDP connection to CoreDNS: %v", err)
	}
	conn.Close()
	logging.Info("Basic UDP connectivity to CoreDNS successful")
	return nil
}

// WaitForHealthyWithDiagnostics performs sophisticated health checking with detailed logging and diagnostics.
// It uses the provided checker to perform health checks with retry logic and diagnostic information.
func WaitForHealthyWithDiagnostics(checker Checker, maxAttempts int, retryDelay time.Duration) error {
	logging.Info("Waiting for %s to report healthy status...", checker.Name())

	healthCheckAttempts := 0

	for i := 0; i < maxAttempts; i++ {
		healthCheckAttempts++
		logging.Info("Health check attempt %d/%d...", healthCheckAttempts, maxAttempts)

		ok, latency, err := checker.CheckOnce()
		if ok {
			logging.Info("%s is healthy (responded in %v)", checker.Name(), latency)
			return nil
		} else {
			if err != nil {
				logging.Warn("Health check failed: %v", err)

				// Extract server address for diagnostic testing
				// This assumes the checker is a DNSHealthChecker - we could make this more generic
				if dnsChecker, isDNSChecker := checker.(*DNSHealthChecker); isDNSChecker {
					testConn, dialErr := net.DialTimeout("udp", dnsChecker.server, 2*time.Second)
					if dialErr != nil {
						logging.Warn("%s port appears to be closed or unreachable: %v", checker.Name(), dialErr)
					} else {
						testConn.Close()
						logging.Info("%s port is open, but DNS query failed (%s may still be starting)", checker.Name(), checker.Name())
					}
				}
			} else {
				logging.Warn("Health check failed: no error details")
			}

			if i < maxAttempts-1 {
				logging.Info("Retrying in %v...", retryDelay)
				time.Sleep(retryDelay)
			}
		}

		if i == maxAttempts-1 {
			totalWaitTime := time.Duration(maxAttempts) * retryDelay
			return fmt.Errorf("%s did not become healthy after %d attempts over %v", checker.Name(), maxAttempts, totalWaitTime)
		}
	}

	return fmt.Errorf("unexpected end of health check loop")
}
