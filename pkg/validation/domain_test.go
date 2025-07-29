package validation

import (
	"testing"
)

func TestValidateDNSName(t *testing.T) {
	tests := []struct {
		name           string
		input          string
		expectedValid  bool
		expectedNorm   string
		expectedErrors int
	}{
		{
			name:           "valid lowercase name",
			input:          "anton",
			expectedValid:  true,
			expectedNorm:   "anton",
			expectedErrors: 0,
		},
		{
			name:           "valid name with numbers",
			input:          "server01",
			expectedValid:  true,
			expectedNorm:   "server01",
			expectedErrors: 0,
		},
		{
			name:           "valid name with hyphen",
			input:          "dns-server",
			expectedValid:  true,
			expectedNorm:   "dns-server",
			expectedErrors: 0,
		},
		{
			name:           "uppercase gets normalized",
			input:          "ANTON",
			expectedValid:  false,
			expectedNorm:   "anton",
			expectedErrors: 1,
		},
		{
			name:           "mixed case gets normalized",
			input:          "Anton",
			expectedValid:  false,
			expectedNorm:   "anton",
			expectedErrors: 1,
		},
		{
			name:           "uppercase with mixed case gets normalized",
			input:          "Revenantor",
			expectedValid:  false,
			expectedNorm:   "revenantor",
			expectedErrors: 1,
		},
		{
			name:           "underscore gets normalized to hyphen",
			input:          "dns_server",
			expectedValid:  false,
			expectedNorm:   "dns-server",
			expectedErrors: 1,
		},
		{
			name:           "invalid characters removed",
			input:          "dns@server!",
			expectedValid:  false,
			expectedNorm:   "dnsserver",
			expectedErrors: 1,
		},
		{
			name:           "empty string",
			input:          "",
			expectedValid:  false,
			expectedNorm:   "",
			expectedErrors: 1,
		},
		{
			name:           "starts with hyphen",
			input:          "-server",
			expectedValid:  false,
			expectedNorm:   "server",
			expectedErrors: 1,
		},
		{
			name:           "ends with hyphen",
			input:          "server-",
			expectedValid:  false,
			expectedNorm:   "server",
			expectedErrors: 1,
		},
		{
			name:           "consecutive hyphens",
			input:          "dns--server",
			expectedValid:  false,
			expectedNorm:   "dns-server",
			expectedErrors: 1,
		},
		{
			name:           "too long name gets truncated",
			input:          "this-is-a-very-long-dns-name-that-exceeds-the-sixty-three-character-limit-for-dns-labels",
			expectedValid:  false,
			expectedNorm:   "this-is-a-very-long-dns-name-that-exceeds-the-sixty-three-chara",
			expectedErrors: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ValidateDNSName(tt.input)

			if result.IsValid != tt.expectedValid {
				t.Errorf("ValidateDNSName(%q).IsValid = %v, want %v", tt.input, result.IsValid, tt.expectedValid)
			}

			if result.NormalizedName != tt.expectedNorm {
				t.Errorf("ValidateDNSName(%q).NormalizedName = %q, want %q", tt.input, result.NormalizedName, tt.expectedNorm)
			}

			if len(result.Errors) != tt.expectedErrors {
				t.Errorf("ValidateDNSName(%q) errors = %d, want %d. Errors: %v", tt.input, len(result.Errors), tt.expectedErrors, result.Errors)
			}
		})
	}
}

func TestValidateServiceName(t *testing.T) {
	tests := []struct {
		name           string
		input          string
		expectedValid  bool
		expectedNorm   string
		expectedErrors int
	}{
		{
			name:           "valid simple service name",
			input:          "web",
			expectedValid:  true,
			expectedNorm:   "web",
			expectedErrors: 0,
		},
		{
			name:           "valid hierarchical service name",
			input:          "internal.web",
			expectedValid:  true,
			expectedNorm:   "internal.web",
			expectedErrors: 0,
		},
		{
			name:           "mixed case service name gets normalized",
			input:          "DNS-API",
			expectedValid:  false,
			expectedNorm:   "dns-api",
			expectedErrors: 1,
		},
		{
			name:           "hierarchical with mixed case",
			input:          "Internal.Web-Service",
			expectedValid:  false,
			expectedNorm:   "internal.web-service",
			expectedErrors: 1,
		},
		{
			name:           "empty string",
			input:          "",
			expectedValid:  false,
			expectedNorm:   "",
			expectedErrors: 1,
		},
		{
			name:           "dots only",
			input:          "...",
			expectedValid:  false,
			expectedNorm:   "",
			expectedErrors: 2,
		},
		{
			name:           "empty part in middle",
			input:          "web..api",
			expectedValid:  false,
			expectedNorm:   "web.api",
			expectedErrors: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ValidateServiceName(tt.input)

			if result.IsValid != tt.expectedValid {
				t.Errorf("ValidateServiceName(%q).IsValid = %v, want %v", tt.input, result.IsValid, tt.expectedValid)
			}

			if result.NormalizedName != tt.expectedNorm {
				t.Errorf("ValidateServiceName(%q).NormalizedName = %q, want %q", tt.input, result.NormalizedName, tt.expectedNorm)
			}

			if len(result.Errors) != tt.expectedErrors {
				t.Errorf("ValidateServiceName(%q) errors = %d, want %d. Errors: %v", tt.input, len(result.Errors), tt.expectedErrors, result.Errors)
			}
		})
	}
}

func TestNormalizeDNSName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"lowercase unchanged", "anton", "anton"},
		{"uppercase to lowercase", "ANTON", "anton"},
		{"mixed case to lowercase", "Anton", "anton"},
		{"underscores to hyphens", "dns_server", "dns-server"},
		{"invalid chars removed", "dns@server!", "dnsserver"},
		{"leading hyphen removed", "-server", "server"},
		{"trailing hyphen removed", "server-", "server"},
		{"consecutive hyphens merged", "dns--server", "dns-server"},
		{"multiple issues", "DNS_Server@123!", "dns-server123"},
		{"empty string", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NormalizeDNSName(tt.input)
			if result != tt.expected {
				t.Errorf("NormalizeDNSName(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestIsValidDNSName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{"valid lowercase", "anton", true},
		{"valid with numbers", "server01", true},
		{"valid with hyphen", "dns-server", true},
		{"uppercase invalid", "ANTON", false},
		{"mixed case invalid", "Anton", false},
		{"underscore invalid", "dns_server", false},
		{"special chars invalid", "dns@server", false},
		{"starts with hyphen invalid", "-server", false},
		{"ends with hyphen invalid", "server-", false},
		{"consecutive hyphens invalid", "dns--server", false},
		{"empty invalid", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsValidDNSName(tt.input)
			if result != tt.expected {
				t.Errorf("IsValidDNSName(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestIsValidServiceName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{"valid simple", "web", true},
		{"valid hierarchical", "internal.web", true},
		{"valid with numbers", "api.v2", true},
		{"uppercase invalid", "WEB", false},
		{"mixed case invalid", "Internal.Web", false},
		{"underscore invalid", "web_api", false},
		{"empty invalid", "", false},
		{"dots only invalid", "...", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsValidServiceName(tt.input)
			if result != tt.expected {
				t.Errorf("IsValidServiceName(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}