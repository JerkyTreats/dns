package validation

import (
	"fmt"
	"regexp"
	"strings"
)

// DomainValidationResult contains both validation results and normalized values
type DomainValidationResult struct {
	IsValid        bool
	NormalizedName string
	Errors         []string
}

// ValidateDNSName validates and normalizes a DNS name (short hostname)
// DNS names must:
// - Be lowercase (automatically normalized)
// - Contain only letters (a-z), numbers (0-9), and hyphens (-)
// - Not start or end with hyphens
// - Be 63 characters or less
// - Not have consecutive hyphens
func ValidateDNSName(name string) *DomainValidationResult {
	result := &DomainValidationResult{
		IsValid: true,
		Errors:  []string{},
	}

	if name == "" {
		result.IsValid = false
		result.Errors = append(result.Errors, "DNS name cannot be empty")
		return result
	}

	// Normalize the name
	normalized := NormalizeDNSName(name)
	result.NormalizedName = normalized

	// Validate normalized name
	if err := validateNormalizedDNSName(normalized); err != nil {
		result.IsValid = false
		result.Errors = append(result.Errors, err.Error())
	}

	// Check if normalization changed the name (indicating invalid input)
	if normalized != name {
		result.IsValid = false
		result.Errors = append(result.Errors, fmt.Sprintf("DNS name was normalized from '%s' to '%s' (original contained invalid characters)", name, normalized))
	}

	return result
}

// ValidateServiceName validates a service name (can contain dots for hierarchical naming)
func ValidateServiceName(serviceName string) *DomainValidationResult {
	result := &DomainValidationResult{
		IsValid: true,
		Errors:  []string{},
	}

	if serviceName == "" {
		result.IsValid = false
		result.Errors = append(result.Errors, "service name cannot be empty")
		return result
	}

	// Normalize the service name
	normalized := NormalizeServiceName(serviceName)
	result.NormalizedName = normalized

	// Validate each part of the service name (split by dots)
	parts := strings.Split(normalized, ".")
	for i, part := range parts {
		if part == "" {
			result.IsValid = false
			result.Errors = append(result.Errors, fmt.Sprintf("service name part %d is empty", i+1))
			continue
		}

		if err := validateNormalizedDNSName(part); err != nil {
			result.IsValid = false
			result.Errors = append(result.Errors, fmt.Sprintf("service name part %d (%s): %s", i+1, part, err.Error()))
		}
	}

	// Check if normalization changed the name
	if normalized != serviceName {
		result.IsValid = false
		result.Errors = append(result.Errors, fmt.Sprintf("service name was normalized from '%s' to '%s' (original contained invalid characters)", serviceName, normalized))
	}

	return result
}

// NormalizeDNSName normalizes a DNS name to be DNS-compliant
func NormalizeDNSName(name string) string {
	if name == "" {
		return ""
	}

	// Convert to lowercase
	result := strings.ToLower(name)

	// Replace underscores with hyphens
	result = strings.ReplaceAll(result, "_", "-")

	// Remove invalid characters (keep only a-z, 0-9, hyphen)
	validChars := regexp.MustCompile(`[^a-z0-9-]`)
	result = validChars.ReplaceAllString(result, "")

	// Remove consecutive hyphens
	consecutiveHyphens := regexp.MustCompile(`-+`)
	result = consecutiveHyphens.ReplaceAllString(result, "-")

	// Remove leading and trailing hyphens
	result = strings.Trim(result, "-")

	// Enforce 63 character limit for DNS labels
	if len(result) > 63 {
		result = result[:63]
		// Remove any trailing hyphen that might have been created by truncation
		result = strings.TrimSuffix(result, "-")
	}

	return result
}

// NormalizeServiceName normalizes a service name (preserves dots for hierarchical naming)
func NormalizeServiceName(serviceName string) string {
	if serviceName == "" {
		return ""
	}

	// Split by dots and normalize each part
	parts := strings.Split(serviceName, ".")
	normalizedParts := make([]string, len(parts))

	for i, part := range parts {
		normalizedParts[i] = NormalizeDNSName(part)
	}

	// Filter out empty parts
	filteredParts := make([]string, 0, len(normalizedParts))
	for _, part := range normalizedParts {
		if part != "" {
			filteredParts = append(filteredParts, part)
		}
	}

	return strings.Join(filteredParts, ".")
}

// validateNormalizedDNSName validates a DNS name that has already been normalized
func validateNormalizedDNSName(name string) error {
	if name == "" {
		return fmt.Errorf("DNS name cannot be empty")
	}

	// Check length
	if len(name) > 63 {
		return fmt.Errorf("DNS name cannot exceed 63 characters, got %d", len(name))
	}

	// Must not start or end with hyphen
	if strings.HasPrefix(name, "-") || strings.HasSuffix(name, "-") {
		return fmt.Errorf("DNS name cannot start or end with hyphen")
	}

	// Check for valid characters only (should already be normalized)
	validNameRegex := regexp.MustCompile(`^[a-z0-9-]+$`)
	if !validNameRegex.MatchString(name) {
		return fmt.Errorf("DNS name contains invalid characters (only lowercase letters, numbers, and hyphens allowed)")
	}

	// Check for consecutive hyphens
	if strings.Contains(name, "--") {
		return fmt.Errorf("DNS name cannot contain consecutive hyphens")
	}

	return nil
}

// IsValidDNSName checks if a string is a valid DNS name (strict validation)
func IsValidDNSName(name string) bool {
	result := ValidateDNSName(name)
	return result.IsValid && result.NormalizedName == name
}

// IsValidServiceName checks if a string is a valid service name (strict validation)
func IsValidServiceName(serviceName string) bool {
	result := ValidateServiceName(serviceName)
	return result.IsValid && result.NormalizedName == serviceName
}