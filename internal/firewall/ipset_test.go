package firewall

import (
	"testing"

	"github.com/jerkytreats/dns/internal/config"
)

func TestNewManager_WithConfig(t *testing.T) {
	// Reset config for clean test state
	config.ResetForTest()

	// Set server configuration
	config.SetForTest(ServerPortKey, 8080)
	config.SetForTest(ServerTLSPortKey, 8443)
	config.SetForTest("server.tls.enabled", false)

	manager, err := NewManager()
	if err != nil {
		t.Fatalf("Expected no error for properly configured firewall, got: %v", err)
	}

	// Firewall is now always enabled, no need to check

	if manager.GetIpsetName() != TailscaleIpsetName {
		t.Errorf("Expected ipset name '%s', got: %s", TailscaleIpsetName, manager.GetIpsetName())
	}

	if manager.GetTailscaleCIDR() != "100.64.0.0/10" {
		t.Errorf("Expected CIDR '100.64.0.0/10', got: %s", manager.GetTailscaleCIDR())
	}
}

func TestNewManager_WithDefaults(t *testing.T) {
	// Reset config for clean test state
	config.ResetForTest()

	// Set minimal server configuration
	config.SetForTest(ServerPortKey, 8080)
	config.SetForTest(ServerTLSPortKey, 8443)
	config.SetForTest("server.tls.enabled", false)

	manager, err := NewManager()
	if err != nil {
		t.Fatalf("Expected no error when using defaults, got: %v", err)
	}

	// Firewall is now always enabled, no need to check

	if manager.GetIpsetName() != TailscaleIpsetName {
		t.Errorf("Expected ipset name '%s', got: %s", TailscaleIpsetName, manager.GetIpsetName())
	}

	if manager.GetTailscaleCIDR() != TailscaleCIDR {
		t.Errorf("Expected CIDR '%s', got: %s", TailscaleCIDR, manager.GetTailscaleCIDR())
	}
}

func TestManagerConfiguration(t *testing.T) {
	// Reset config for clean test state
	config.ResetForTest()

	// Set server configuration
	config.SetForTest(ServerPortKey, 8080)
	config.SetForTest(ServerTLSPortKey, 8443)
	config.SetForTest("server.tls.enabled", false)

	manager, err := NewManager()
	if err != nil {
		t.Fatalf("Expected no error for manager creation, got: %v", err)
	}

	// Firewall is now always enabled, no need to check

	// Test that hardcoded ipset name is used
	if manager.GetIpsetName() != TailscaleIpsetName {
		t.Errorf("Expected ipset name '%s', got: %s", TailscaleIpsetName, manager.GetIpsetName())
	}
}

func TestFirewallManagerOperations(t *testing.T) {
	// Reset config for clean test state
	config.ResetForTest()

	config.SetForTest(ServerPortKey, 8080)
	config.SetForTest(ServerTLSPortKey, 8443)
	config.SetForTest("server.tls.enabled", false)

	manager, err := NewManager()
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	// Test that manager operations don't panic (we can't test actual iptables/ipset in unit tests)
	// These will fail with command execution errors but should not panic

	err = manager.EnsureFirewallRules()
	// We expect this to fail in test environment without iptables/ipset
	if err == nil {
		t.Log("EnsureFirewallRules succeeded (unexpected in test environment)")
	}

	err = manager.ValidateFirewallSetup()
	// We expect this to fail in test environment without iptables/ipset
	if err == nil {
		t.Log("ValidateFirewallSetup succeeded (unexpected in test environment)")
	}

	rules, err := manager.ListCurrentRules()
	// We expect this to fail in test environment without iptables/ipset
	if err == nil {
		t.Logf("ListCurrentRules succeeded: %v (unexpected in test environment)", rules)
	}

	err = manager.RemoveFirewallRules()
	// We expect this to fail in test environment without iptables/ipset
	if err == nil {
		t.Log("RemoveFirewallRules succeeded (unexpected in test environment)")
	}
}

func TestDNSServerPorts(t *testing.T) {
	// Reset config for clean test state
	config.ResetForTest()

	testCases := []struct {
		name       string
		httpPort   int
		httpsPort  int
		tlsEnabled bool
		expected   []string
	}{
		{
			name:       "http only",
			httpPort:   8080,
			httpsPort:  8443,
			tlsEnabled: false,
			expected:   []string{"53", "8080"},
		},
		{
			name:       "http and https",
			httpPort:   8080,
			httpsPort:  8443,
			tlsEnabled: true,
			expected:   []string{"53", "8080", "8443"},
		},
		{
			name:       "custom ports",
			httpPort:   9000,
			httpsPort:  9443,
			tlsEnabled: true,
			expected:   []string{"53", "9000", "9443"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Reset config for each test case
			config.ResetForTest()

			config.SetForTest(ServerPortKey, tc.httpPort)
			config.SetForTest(ServerTLSPortKey, tc.httpsPort)
			config.SetForTest("server.tls.enabled", tc.tlsEnabled)

			ports := getDNSServerPorts()

			if len(ports) != len(tc.expected) {
				t.Errorf("Expected %d ports, got %d: %v", len(tc.expected), len(ports), ports)
				return
			}

			for i, expected := range tc.expected {
				if ports[i] != expected {
					t.Errorf("Expected port %s at index %d, got %s", expected, i, ports[i])
				}
			}
		})
	}
}
