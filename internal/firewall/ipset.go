// Package firewall provides ipset management for allowing Tailscale CIDR ranges
package firewall

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/jerkytreats/dns/internal/config"
	"github.com/jerkytreats/dns/internal/logging"
)

const (
	// Standard Tailscale CGNAT range - this is consistent across all Tailscale networks
	TailscaleCIDR = "100.64.0.0/10"

	// Hardcoded ipset name for Tailscale firewall management
	TailscaleIpsetName = "tailscale_allowed"

	// Server configuration keys (imported from cmd/api/main.go)
	ServerPortKey    = "server.port"
	ServerTLSPortKey = "server.tls.port"
)

var (
	// DNS server protocols - tcp and udp are required for DNS services
	DNSProtocols = []string{"tcp", "udp"}
)

func init() {
	// No required keys - firewall management is always enabled with sensible defaults
}

// Manager handles ipset and iptables rules for Tailscale networks
type Manager struct {
	ports []string
}

// NewManager creates a new firewall manager with automatic Tailscale CIDR protection
func NewManager() (*Manager, error) {
	// Get ports from existing server configuration
	ports := getDNSServerPorts()

	manager := &Manager{
		ports: ports,
	}

	logging.Info("Firewall manager initialized - ipset: %s, CIDR: %s, ports: %v", TailscaleIpsetName, TailscaleCIDR, ports)
	return manager, nil
}

// getDNSServerPorts returns the ports that need firewall access for DNS server operation
func getDNSServerPorts() []string {
	ports := []string{
		"53", // DNS port (always needed)
	}

	// Add HTTP server port
	if httpPort := config.GetInt(ServerPortKey); httpPort != 0 {
		ports = append(ports, fmt.Sprintf("%d", httpPort))
	}

	// Add HTTPS server port if TLS is enabled
	if config.GetBool("server.tls.enabled") {
		if httpsPort := config.GetInt(ServerTLSPortKey); httpsPort != 0 {
			ports = append(ports, fmt.Sprintf("%d", httpsPort))
		}
	}

	return ports
}

// EnsureFirewallRules ensures that ipset and iptables rules are configured
func (m *Manager) EnsureFirewallRules() error {
	logging.Info("Setting up firewall rules for Tailscale CIDR: %s", TailscaleCIDR)

	if err := m.ensureIpsetExists(); err != nil {
		return fmt.Errorf("failed to ensure ipset exists: %w", err)
	}

	if err := m.addCIDRToIpset(); err != nil {
		return fmt.Errorf("failed to add CIDR to ipset: %w", err)
	}

	if err := m.ensureIptablesRules(); err != nil {
		return fmt.Errorf("failed to ensure iptables rules: %w", err)
	}

	logging.Info("Firewall rules configured successfully")
	return nil
}

// ensureIpsetExists creates the ipset if it doesn't exist
func (m *Manager) ensureIpsetExists() error {
	// Check if ipset exists
	cmd := exec.Command("ipset", "list", TailscaleIpsetName)
	if err := cmd.Run(); err != nil {
		// Ipset doesn't exist, create it
		logging.Info("Creating ipset: %s", TailscaleIpsetName)
		cmd = exec.Command("ipset", "create", TailscaleIpsetName, "hash:net")
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to create ipset %s: %w", TailscaleIpsetName, err)
		}
		logging.Info("Created ipset: %s", TailscaleIpsetName)
	} else {
		logging.Debug("Ipset %s already exists", TailscaleIpsetName)
	}
	return nil
}

// addCIDRToIpset adds the Tailscale CIDR to the ipset
func (m *Manager) addCIDRToIpset() error {
	logging.Info("Adding CIDR %s to ipset %s", TailscaleCIDR, TailscaleIpsetName)

	// Use -exist flag to avoid errors if the entry already exists
	cmd := exec.Command("ipset", "add", TailscaleIpsetName, TailscaleCIDR, "-exist")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to add CIDR %s to ipset %s: %w", TailscaleCIDR, TailscaleIpsetName, err)
	}

	logging.Info("Added CIDR %s to ipset %s", TailscaleCIDR, TailscaleIpsetName)
	return nil
}

// ensureIptablesRules ensures iptables rules exist for DNS protocols and server ports
func (m *Manager) ensureIptablesRules() error {
	for _, protocol := range DNSProtocols {
		if len(m.ports) > 0 {
			for _, port := range m.ports {
				if err := m.ensureIptablesRule(protocol, port); err != nil {
					return err
				}
			}
		} else {
			// Allow all ports for this protocol
			if err := m.ensureIptablesRule(protocol, ""); err != nil {
				return err
			}
		}
	}
	return nil
}

// ensureIptablesRule ensures a specific iptables rule exists
func (m *Manager) ensureIptablesRule(protocol, port string) error {
	args := []string{
		"-I", "INPUT",
		"-m", "set", "--match-set", TailscaleIpsetName, "src",
		"-p", protocol,
	}

	if port != "" {
		args = append(args, "--dport", port)
	}

	args = append(args, "-j", "ACCEPT")

	// Check if rule exists
	checkArgs := append([]string{"-C"}, args[1:]...)
	cmd := exec.Command("iptables", checkArgs...)
	if err := cmd.Run(); err != nil {
		// Rule doesn't exist, add it
		ruleDesc := fmt.Sprintf("protocol %s", protocol)
		if port != "" {
			ruleDesc += fmt.Sprintf(" port %s", port)
		}
		logging.Info("Adding iptables rule for %s from ipset %s", ruleDesc, TailscaleIpsetName)

		cmd = exec.Command("iptables", args...)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to add iptables rule for %s: %w", ruleDesc, err)
		}
		logging.Info("Added iptables rule for %s", ruleDesc)
	} else {
		logging.Debug("Iptables rule already exists for protocol %s port %s", protocol, port)
	}

	return nil
}

// RemoveFirewallRules removes the configured firewall rules
func (m *Manager) RemoveFirewallRules() error {
	logging.Info("Removing firewall rules for ipset: %s", TailscaleIpsetName)

	// Remove iptables rules
	for _, protocol := range DNSProtocols {
		if len(m.ports) > 0 {
			for _, port := range m.ports {
				if err := m.removeIptablesRule(protocol, port); err != nil {
					logging.Warn("Failed to remove iptables rule for protocol %s port %s: %v", protocol, port, err)
				}
			}
		} else {
			if err := m.removeIptablesRule(protocol, ""); err != nil {
				logging.Warn("Failed to remove iptables rule for protocol %s: %v", protocol, err)
			}
		}
	}

	// Remove ipset
	cmd := exec.Command("ipset", "destroy", TailscaleIpsetName)
	if err := cmd.Run(); err != nil {
		logging.Warn("Failed to destroy ipset %s: %v", TailscaleIpsetName, err)
	} else {
		logging.Info("Removed ipset: %s", TailscaleIpsetName)
	}

	return nil
}

// removeIptablesRule removes a specific iptables rule
func (m *Manager) removeIptablesRule(protocol, port string) error {
	args := []string{
		"-D", "INPUT",
		"-m", "set", "--match-set", TailscaleIpsetName, "src",
		"-p", protocol,
	}

	if port != "" {
		args = append(args, "--dport", port)
	}

	args = append(args, "-j", "ACCEPT")

	cmd := exec.Command("iptables", args...)
	return cmd.Run()
}

// ValidateFirewallSetup checks if the firewall setup is working correctly
func (m *Manager) ValidateFirewallSetup() error {
	logging.Info("Validating firewall setup")

	// Check if ipset exists and has the CIDR
	cmd := exec.Command("ipset", "test", TailscaleIpsetName, TailscaleCIDR)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ipset %s does not contain CIDR %s: %w", TailscaleIpsetName, TailscaleCIDR, err)
	}

	// Check if iptables rules exist
	for _, protocol := range DNSProtocols {
		if len(m.ports) > 0 {
			for _, port := range m.ports {
				if err := m.validateIptablesRule(protocol, port); err != nil {
					return err
				}
			}
		} else {
			if err := m.validateIptablesRule(protocol, ""); err != nil {
				return err
			}
		}
	}

	logging.Info("Firewall setup validation successful")
	return nil
}

// validateIptablesRule checks if a specific iptables rule exists
func (m *Manager) validateIptablesRule(protocol, port string) error {
	args := []string{
		"-C", "INPUT",
		"-m", "set", "--match-set", TailscaleIpsetName, "src",
		"-p", protocol,
	}

	if port != "" {
		args = append(args, "--dport", port)
	}

	args = append(args, "-j", "ACCEPT")

	cmd := exec.Command("iptables", args...)
	if err := cmd.Run(); err != nil {
		ruleDesc := fmt.Sprintf("protocol %s", protocol)
		if port != "" {
			ruleDesc += fmt.Sprintf(" port %s", port)
		}
		return fmt.Errorf("iptables rule missing for %s: %w", ruleDesc, err)
	}

	return nil
}

// GetIpsetName returns the ipset name
func (m *Manager) GetIpsetName() string {
	return TailscaleIpsetName
}

// GetTailscaleCIDR returns the standard Tailscale CIDR
func (m *Manager) GetTailscaleCIDR() string {
	return TailscaleCIDR
}

// ListCurrentRules returns a summary of current firewall rules
func (m *Manager) ListCurrentRules() ([]string, error) {
	var rules []string

	// Get ipset information
	cmd := exec.Command("ipset", "list", TailscaleIpsetName)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to list ipset %s: %w", TailscaleIpsetName, err)
	}

	rules = append(rules, fmt.Sprintf("Ipset %s:", TailscaleIpsetName))
	rules = append(rules, strings.Split(string(output), "\n")...)

	// Get relevant iptables rules
	cmd = exec.Command("iptables", "-L", "INPUT", "-n")
	output, err = cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to list iptables rules: %w", err)
	}

	rules = append(rules, "", "Relevant iptables rules:")
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.Contains(line, TailscaleIpsetName) {
			rules = append(rules, line)
		}
	}

	return rules, nil
}
