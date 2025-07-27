package record

import (
	"fmt"
	"strings"

	"github.com/jerkytreats/dns/pkg/validation"
)

// RecordValidator implements the Validator interface
type RecordValidator struct{}

// NewValidator creates a new record validator
func NewValidator() Validator {
	return &RecordValidator{}
}

// ValidateCreateRequest validates a CreateRecordRequest
func (v *RecordValidator) ValidateCreateRequest(req CreateRecordRequest) error {
	// Validate required fields
	if req.ServiceName == "" {
		return fmt.Errorf("service_name is required")
	}

	if req.Name == "" {
		return fmt.Errorf("name is required")
	}

	// Validate service name format
	if err := v.validateServiceName(req.ServiceName); err != nil {
		return fmt.Errorf("invalid service_name: %w", err)
	}

	// Validate record name format
	if err := v.validateRecordName(req.Name); err != nil {
		return fmt.Errorf("invalid name: %w", err)
	}

	// Validate port if specified
	if req.Port != nil {
		if err := v.validatePort(*req.Port); err != nil {
			return fmt.Errorf("invalid port: %w", err)
		}
	}

	return nil
}

// validateServiceName validates the service name format
func (v *RecordValidator) validateServiceName(serviceName string) error {
	if serviceName == "" {
		return fmt.Errorf("service name cannot be empty")
	}

	// Service name should be alphanumeric with hyphens and dots allowed
	if !isValidServiceName(serviceName) {
		return fmt.Errorf("service name must contain only lowercase letters, numbers, hyphens, and dots")
	}

	// Check length constraints
	if len(serviceName) > 63 {
		return fmt.Errorf("service name cannot exceed 63 characters")
	}

	return nil
}

// validateRecordName validates the DNS record name format
func (v *RecordValidator) validateRecordName(name string) error {
	if name == "" {
		return fmt.Errorf("record name cannot be empty")
	}

	// Record name should be a valid DNS label
	if !isValidDNSLabel(name) {
		return fmt.Errorf("record name must be a valid DNS label")
	}

	// Check length constraints
	if len(name) > 63 {
		return fmt.Errorf("record name cannot exceed 63 characters")
	}

	return nil
}

// validatePort validates the port number
func (v *RecordValidator) validatePort(port int) error {
	if port <= 0 || port > 65535 {
		return fmt.Errorf("port must be between 1 and 65535, got %d", port)
	}

	return nil
}

// validateFQDN validates a fully qualified domain name
func (v *RecordValidator) validateFQDN(fqdn string) error {
	if err := validation.ValidateFQDN(fqdn); err != nil {
		return fmt.Errorf("invalid FQDN: %w", err)
	}
	return nil
}

// validateIP validates an IP address
func (v *RecordValidator) validateIP(ip string) error {
	if ip == "" {
		return fmt.Errorf("IP address cannot be empty")
	}

	// Use existing validation package if available, otherwise basic check
	if !isValidIP(ip) {
		return fmt.Errorf("invalid IP address format: %s", ip)
	}

	return nil
}

// isValidServiceName checks if a service name contains only allowed characters
func isValidServiceName(name string) bool {
	if name == "" {
		return false
	}

	for _, char := range name {
		if !((char >= 'a' && char <= 'z') || 
			 (char >= '0' && char <= '9') || 
			 char == '-' || char == '.') {
			return false
		}
	}

	// Cannot start or end with hyphen or dot
	if strings.HasPrefix(name, "-") || strings.HasSuffix(name, "-") ||
	   strings.HasPrefix(name, ".") || strings.HasSuffix(name, ".") {
		return false
	}

	return true
}

// isValidDNSLabel checks if a string is a valid DNS label
func isValidDNSLabel(label string) bool {
	if label == "" || len(label) > 63 {
		return false
	}

	// DNS labels can contain letters, numbers, and hyphens
	// Cannot start or end with hyphen
	if strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
		return false
	}

	for _, char := range label {
		if !((char >= 'a' && char <= 'z') || 
			 (char >= 'A' && char <= 'Z') || 
			 (char >= '0' && char <= '9') || 
			 char == '-') {
			return false
		}
	}

	return true
}

// isValidIP performs basic IP address validation
func isValidIP(ip string) bool {
	parts := strings.Split(ip, ".")
	if len(parts) != 4 {
		return false
	}

	for _, part := range parts {
		if part == "" {
			return false
		}

		// Check if part is numeric and in valid range
		num := 0
		for _, char := range part {
			if char < '0' || char > '9' {
				return false
			}
			num = num*10 + int(char-'0')
			if num > 255 {
				return false
			}
		}

		// Leading zeros not allowed (except for "0")
		if len(part) > 1 && part[0] == '0' {
			return false
		}
	}

	return true
}
