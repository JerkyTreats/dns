package coredns

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSanitizeDNSName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
		reason   string
	}{
		{
			name:     "underscore replacement",
			input:    "foo_example",
			expected: "foo-example",
			reason:   "underscores are invalid in DNS names and should be replaced with hyphens",
		},
		{
			name:     "problematic API input",
			input:    "bad_foo-dot-com._-",
			expected: "bad-foo-dot-com",
			reason:   "complex problematic name should be sanitized to DNS-compliant form",
		},
		{
			name:     "mixed case conversion",
			input:    "MyDevice",
			expected: "mydevice",
			reason:   "DNS names should be lowercase",
		},
		{
			name:     "multiple underscores",
			input:    "test_device_name",
			expected: "test-device-name",
			reason:   "multiple underscores should all be replaced with hyphens",
		},
		{
			name:     "special characters removal",
			input:    "device@#$%name",
			expected: "devicename",
			reason:   "special characters should be removed",
		},
		{
			name:     "leading and trailing hyphens",
			input:    "-device-name-",
			expected: "device-name",
			reason:   "leading and trailing hyphens should be removed",
		},
		{
			name:     "consecutive hyphens",
			input:    "device--name",
			expected: "device-name",
			reason:   "consecutive hyphens should be collapsed to single hyphen",
		},
		{
			name:     "unicode characters",
			input:    "dévice-naïve",
			expected: "device-naive",
			reason:   "unicode characters should be converted to ASCII equivalents",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
			reason:   "empty string should remain empty",
		},
		{
			name:     "only special characters",
			input:    "@#$%",
			expected: "",
			reason:   "string with only special characters should become empty",
		},
		{
			name:     "only hyphens",
			input:    "---",
			expected: "",
			reason:   "string with only hyphens should become empty after trimming",
		},
		{
			name:     "valid DNS name unchanged",
			input:    "valid-dns-name",
			expected: "valid-dns-name",
			reason:   "already valid DNS names should remain unchanged",
		},
		{
			name:     "length limit enforcement",
			input:    "this-is-a-very-long-hostname-that-exceeds-the-dns-limit-of-63-characters-and-should-be-truncated",
			expected: "this-is-a-very-long-hostname-that-exceeds-the-dns-limit-of-63-c",
			reason:   "DNS labels must be 63 characters or less",
		},
		{
			name:     "length limit with trailing hyphen cleanup",
			input:    "this-is-a-very-long-hostname-that-exceeds-the-dns-limit-of-63-",
			expected: "this-is-a-very-long-hostname-that-exceeds-the-dns-limit-of-63",
			reason:   "after truncation, trailing hyphens should be removed",
		},
		{
			name:     "complex mixed example",
			input:    "MyDevice_Name@123",
			expected: "mydevice-name123",
			reason:   "combination of case, underscores, and special characters",
		},
		{
			name:     "numeric only",
			input:    "12345",
			expected: "12345",
			reason:   "numeric-only names should be preserved",
		},
		{
			name:     "generic Windows style",
			input:    "DESKTOP-ABC123",
			expected: "desktop-abc123",
			reason:   "Windows-style computer names should be converted to lowercase",
		},
		{
			name:     "generic mobile device",
			input:    "Phone_Device_6",
			expected: "phone-device-6",
			reason:   "mobile device names with underscores should use hyphens",
		},
		{
			name:     "FQDN in hostname",
			input:    "Host-VM.domain.com",
			expected: "host-vm-domain-com",
			reason:   "dots in names should be converted to hyphens for DNS compliance",
		},
		{
			name:     "generic server",
			input:    "SERVERBOX",
			expected: "serverbox",
			reason:   "all-caps server names should be lowercase",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeDNSName(tt.input)
			assert.Equal(t, tt.expected, result, "Test '%s' failed: %s", tt.name, tt.reason)
		})
	}
}

func TestHandleDNSNameCollision(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		existingNames map[string]bool
		expected      string
	}{
		{
			name:          "no collision",
			input:         "device",
			existingNames: map[string]bool{"other": true},
			expected:      "device",
		},
		{
			name:          "collision with numeric suffix",
			input:         "device",
			existingNames: map[string]bool{"device": true},
			expected:      "device-2",
		},
		{
			name:          "collision with multiple taken suffixes",
			input:         "device",
			existingNames: map[string]bool{"device": true, "device-2": true, "device-3": true},
			expected:      "device-4",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := handleDNSNameCollision(tt.input, tt.existingNames)
			assert.Equal(t, tt.expected, result)
		})
	}
}
