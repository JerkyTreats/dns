package record

import (
	"fmt"
	"strings"

	"github.com/jerkytreats/dns/pkg/validation"
)

// RecordValidator implements the Validator interface
type RecordValidator struct{}

// ValidationMode determines how validation should handle normalization
type ValidationMode int

const (
	// StrictValidation rejects any input that requires normalization
	StrictValidation ValidationMode = iota
	// AutoNormalizeValidation accepts input and applies normalization
	AutoNormalizeValidation
)

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

// NormalizeCreateRequest normalizes a CreateRecordRequest and returns the normalized version
func (v *RecordValidator) NormalizeCreateRequest(req CreateRecordRequest) (CreateRecordRequest, error) {
	normalizedReq := req

	// Normalize service name
	serviceResult := validation.ValidateServiceName(req.ServiceName)
	if !serviceResult.IsValid {
		return req, fmt.Errorf("invalid service name: %s", strings.Join(serviceResult.Errors, "; "))
	}
	normalizedReq.ServiceName = serviceResult.NormalizedName

	// Normalize record name
	nameResult := validation.ValidateDNSName(req.Name)
	if !nameResult.IsValid {
		return req, fmt.Errorf("invalid record name: %s", strings.Join(nameResult.Errors, "; "))
	}
	normalizedReq.Name = nameResult.NormalizedName

	// Port validation remains the same
	if req.Port != nil {
		if err := v.validatePort(*req.Port); err != nil {
			return req, fmt.Errorf("invalid port: %w", err)
		}
	}

	return normalizedReq, nil
}

// ValidateRemoveRequest validates a RemoveRecordRequest
func (v *RecordValidator) ValidateRemoveRequest(req RemoveRecordRequest) error {
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

	return nil
}

// validateServiceName validates the service name format
func (v *RecordValidator) validateServiceName(serviceName string) error {
	result := validation.ValidateServiceName(serviceName)
	
	if !result.IsValid {
		return fmt.Errorf("invalid service name: %s", strings.Join(result.Errors, "; "))
	}

	// For strict validation, require exact match (no normalization needed)
	if result.NormalizedName != serviceName {
		return fmt.Errorf("service name contains invalid characters or format - expected: %s", result.NormalizedName)
	}

	return nil
}

// validateRecordName validates the DNS record name format
func (v *RecordValidator) validateRecordName(name string) error {
	result := validation.ValidateDNSName(name)
	
	if !result.IsValid {
		return fmt.Errorf("invalid record name: %s", strings.Join(result.Errors, "; "))
	}

	// For strict validation, require exact match (no normalization needed)
	if result.NormalizedName != name {
		return fmt.Errorf("record name contains invalid characters or format - expected: %s", result.NormalizedName)
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
