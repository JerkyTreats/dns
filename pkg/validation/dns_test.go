package validation

import (
	"testing"
)

func TestValidateFQDN(t *testing.T) {
	tests := []struct {
		name      string
		hostname  string
		expectErr bool
	}{
		// Valid FQDNs
		{"Valid FQDN", "example.com", false},
		{"Valid FQDN with subdomain", "api.example.com", false},
		{"Valid FQDN with hyphens", "my-app.example.com", false},
		{"Valid FQDN with numbers", "app1.example.com", false},
		{"Valid FQDN with multiple subdomains", "api.v1.example.com", false},
		{"Valid FQDN with hyphens in subdomain", "my-api.example.com", false},

		// Invalid FQDNs
		{"Empty hostname", "", true},
		{"Missing domain separator", "example", true},
		{"Starts with dot", ".example.com", true},
		{"Ends with dot", "example.com.", true},
		{"Empty domain part", "example..com", true},
		{"Invalid characters", "example@.com", true},
		{"Starts with hyphen", "-example.com", true},
		{"Ends with hyphen", "example-.com", true},
		{"Domain part starts with hyphen", "example.-com", true},
		{"Domain part ends with hyphen", "example.com-", true},
		{"Underscore not allowed", "example_test.com", true},
		{"Space not allowed", "example test.com", true},
		{"Special characters", "example#.com", true},
		{"Multiple consecutive dots", "example..com", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateFQDN(tt.hostname)
			if tt.expectErr && err == nil {
				t.Errorf("ValidateFQDN(%q) expected error, got nil", tt.hostname)
			}
			if !tt.expectErr && err != nil {
				t.Errorf("ValidateFQDN(%q) unexpected error: %v", tt.hostname, err)
			}
		})
	}
}

func TestIsValidFQDN(t *testing.T) {
	tests := []struct {
		name     string
		hostname string
		expected bool
	}{
		// Valid FQDNs
		{"Valid FQDN", "example.com", true},
		{"Valid FQDN with subdomain", "api.example.com", true},
		{"Valid FQDN with hyphens", "my-app.example.com", true},
		{"Valid FQDN with numbers", "app1.example.com", true},
		{"Valid FQDN with multiple subdomains", "api.v1.example.com", true},
		{"Valid FQDN with hyphens in subdomain", "my-api.example.com", true},

		// Invalid FQDNs
		{"Empty hostname", "", false},
		{"Missing domain separator", "example", false},
		{"Starts with dot", ".example.com", false},
		{"Ends with dot", "example.com.", false},
		{"Empty domain part", "example..com", false},
		{"Invalid characters", "example@.com", false},
		{"Starts with hyphen", "-example.com", false},
		{"Ends with hyphen", "example-.com", false},
		{"Domain part starts with hyphen", "example.-com", false},
		{"Domain part ends with hyphen", "example.com-", false},
		{"Underscore not allowed", "example_test.com", false},
		{"Space not allowed", "example test.com", false},
		{"Special characters", "example#.com", false},
		{"Multiple consecutive dots", "example..com", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsValidFQDN(tt.hostname)
			if result != tt.expected {
				t.Errorf("IsValidFQDN(%q) = %v, expected %v", tt.hostname, result, tt.expected)
			}
		})
	}
}

func TestIsAlphanumeric(t *testing.T) {
	tests := []struct {
		name     string
		input    byte
		expected bool
	}{
		{"Lowercase letter", 'a', true},
		{"Uppercase letter", 'Z', true},
		{"Digit", '5', true},
		{"Special character", '@', false},
		{"Hyphen", '-', false},
		{"Underscore", '_', false},
		{"Space", ' ', false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isAlphanumeric(tt.input)
			if result != tt.expected {
				t.Errorf("isAlphanumeric(%q) = %v, expected %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestIsAlphanumericOrHyphen(t *testing.T) {
	tests := []struct {
		name     string
		input    rune
		expected bool
	}{
		{"Lowercase letter", 'a', true},
		{"Uppercase letter", 'Z', true},
		{"Digit", '5', true},
		{"Hyphen", '-', true},
		{"Special character", '@', false},
		{"Underscore", '_', false},
		{"Space", ' ', false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isAlphanumericOrHyphen(tt.input)
			if result != tt.expected {
				t.Errorf("isAlphanumericOrHyphen(%q) = %v, expected %v", tt.input, result, tt.expected)
			}
		})
	}
}
