package healthcheck

import (
	"fmt"
	"net"
	"os"
	"strings"
	"time"
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
