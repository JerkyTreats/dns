package validation

import (
	"fmt"
	"strings"
)

// ValidateFQDN checks if a hostname is a valid FQDN
func ValidateFQDN(hostname string) error {
	if hostname == "" {
		return fmt.Errorf("hostname cannot be empty")
	}

	// Check if it contains at least one dot (domain separator)
	if !strings.Contains(hostname, ".") {
		return fmt.Errorf("hostname '%s' is not a valid FQDN - must contain at least one domain separator (.)", hostname)
	}

	// Check if it starts or ends with a dot
	if strings.HasPrefix(hostname, ".") || strings.HasSuffix(hostname, ".") {
		return fmt.Errorf("hostname '%s' is not a valid FQDN - cannot start or end with a dot", hostname)
	}

	// Check for valid characters (letters, digits, hyphens, dots)
	// Must start and end with alphanumeric
	if !IsValidFQDN(hostname) {
		return fmt.Errorf("hostname '%s' is not a valid FQDN - contains invalid characters or format", hostname)
	}

	return nil
}

// IsValidFQDN checks if a string is a valid FQDN format
func IsValidFQDN(hostname string) bool {
	// Must not be empty
	if hostname == "" {
		return false
	}

	// Must contain at least one dot
	if !strings.Contains(hostname, ".") {
		return false
	}

	// Must not start or end with dot
	if strings.HasPrefix(hostname, ".") || strings.HasSuffix(hostname, ".") {
		return false
	}

	// Split by dots and validate each part
	parts := strings.Split(hostname, ".")
	for _, part := range parts {
		// Each part must not be empty
		if part == "" {
			return false
		}

		// Each part must start and end with alphanumeric
		if len(part) == 0 || !isAlphanumeric(part[0]) || !isAlphanumeric(part[len(part)-1]) {
			return false
		}

		// Each part must only contain alphanumeric and hyphens
		for _, char := range part {
			if !isAlphanumericOrHyphen(char) {
				return false
			}
		}
	}

	return true
}

// isAlphanumeric checks if a byte is alphanumeric
func isAlphanumeric(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9')
}

// isAlphanumericOrHyphen checks if a rune is alphanumeric or hyphen
func isAlphanumericOrHyphen(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-'
}
