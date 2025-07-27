package record

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewValidator(t *testing.T) {
	validator := NewValidator()
	assert.NotNil(t, validator)
	assert.IsType(t, &RecordValidator{}, validator)
}

func TestValidateCreateRequest(t *testing.T) {
	validator := NewValidator()

	testCases := []struct {
		name        string
		request     CreateRecordRequest
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid request without port",
			request: CreateRecordRequest{
				ServiceName: "internal",
				Name:        "testrecord",
			},
			expectError: false,
		},
		{
			name: "valid request with port",
			request: CreateRecordRequest{
				ServiceName: "internal",
				Name:        "testrecord",
				Port:        intPtr(8080),
			},
			expectError: false,
		},
		{
			name: "missing service name",
			request: CreateRecordRequest{
				Name: "testrecord",
			},
			expectError: true,
			errorMsg:    "service_name is required",
		},
		{
			name: "missing record name",
			request: CreateRecordRequest{
				ServiceName: "internal",
			},
			expectError: true,
			errorMsg:    "name is required",
		},
		{
			name: "invalid service name with uppercase",
			request: CreateRecordRequest{
				ServiceName: "Internal",
				Name:        "testrecord",
			},
			expectError: true,
			errorMsg:    "invalid service_name",
		},
		{
			name: "invalid service name with special chars",
			request: CreateRecordRequest{
				ServiceName: "internal@domain",
				Name:        "testrecord",
			},
			expectError: true,
			errorMsg:    "invalid service_name",
		},
		{
			name: "invalid record name with special chars",
			request: CreateRecordRequest{
				ServiceName: "internal",
				Name:        "test_record",
			},
			expectError: true,
			errorMsg:    "invalid name",
		},
		{
			name: "invalid port number (negative)",
			request: CreateRecordRequest{
				ServiceName: "internal",
				Name:        "testrecord",
				Port:        intPtr(-80),
			},
			expectError: true,
			errorMsg:    "invalid port",
		},
		{
			name: "invalid port number (too large)",
			request: CreateRecordRequest{
				ServiceName: "internal",
				Name:        "testrecord",
				Port:        intPtr(70000),
			},
			expectError: true,
			errorMsg:    "invalid port",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := validator.ValidateCreateRequest(tc.request)

			if tc.expectError {
				assert.Error(t, err)
				if tc.errorMsg != "" {
					assert.Contains(t, err.Error(), tc.errorMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateServiceName(t *testing.T) {
	validator := &RecordValidator{}

	testCases := []struct {
		name        string
		serviceName string
		expectError bool
	}{
		{"valid simple name", "internal", false},
		{"valid with hyphen", "my-service", false},
		{"valid with dot", "service.local", false},
		{"valid with numbers", "service123", false},
		{"empty name", "", true},
		{"name with uppercase", "Internal", true},
		{"name with underscore", "my_service", true},
		{"name with special chars", "service@domain", true},
		{"name starting with hyphen", "-service", true},
		{"name ending with hyphen", "service-", true},
		{"name starting with dot", ".service", true},
		{"name ending with dot", "service.", true},
		{"name too long", strings.Repeat("a", 64), true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := validator.validateServiceName(tc.serviceName)

			if tc.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateRecordName(t *testing.T) {
	validator := &RecordValidator{}

	testCases := []struct {
		name       string
		recordName string
		expectError bool
	}{
		{"valid simple name", "web", false},
		{"valid with hyphen", "web-app", false},
		{"valid with uppercase", "WebApp", false},
		{"valid with numbers", "app123", false},
		{"empty name", "", true},
		{"name with underscore", "web_app", true},
		{"name with dot", "web.app", true},
		{"name with special chars", "app@web", true},
		{"name starting with hyphen", "-app", true},
		{"name ending with hyphen", "app-", true},
		{"name too long", strings.Repeat("a", 64), true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := validator.validateRecordName(tc.recordName)

			if tc.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidatePort(t *testing.T) {
	validator := &RecordValidator{}

	testCases := []struct {
		name        string
		port        int
		expectError bool
	}{
		{"valid low port", 80, false},
		{"valid high port", 8080, false},
		{"valid max port", 65535, false},
		{"zero port", 0, true},
		{"negative port", -1, true},
		{"port too large", 65536, true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := validator.validatePort(tc.port)

			if tc.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestIsValidServiceName(t *testing.T) {
	testCases := []struct {
		name     string
		input    string
		expected bool
	}{
		{"valid simple name", "internal", true},
		{"valid with hyphen", "my-service", true},
		{"valid with dot", "service.local", true},
		{"valid with numbers", "service123", true},
		{"empty name", "", false},
		{"name with uppercase", "Internal", false},
		{"name with underscore", "my_service", false},
		{"name with special chars", "service@domain", false},
		{"name starting with hyphen", "-service", false},
		{"name ending with hyphen", "service-", false},
		{"name starting with dot", ".service", false},
		{"name ending with dot", "service.", false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := isValidServiceName(tc.input)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestIsValidDNSLabel(t *testing.T) {
	testCases := []struct {
		name     string
		input    string
		expected bool
	}{
		{"valid simple name", "web", true},
		{"valid with hyphen", "web-app", true},
		{"valid with uppercase", "WebApp", true},
		{"valid with numbers", "app123", true},
		{"empty name", "", false},
		{"name with underscore", "web_app", false},
		{"name with dot", "web.app", false},
		{"name with special chars", "app@web", false},
		{"name starting with hyphen", "-app", false},
		{"name ending with hyphen", "app-", false},
		{"name too long", strings.Repeat("a", 64), false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := isValidDNSLabel(tc.input)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestIsValidIP(t *testing.T) {
	testCases := []struct {
		name     string
		input    string
		expected bool
	}{
		{"valid IP", "192.168.1.1", true},
		{"valid IP with zeros", "10.0.0.1", true},
		{"valid Tailscale IP", "100.64.0.1", true},
		{"empty IP", "", false},
		{"missing octet", "192.168.1", false},
		{"too many octets", "192.168.1.1.5", false},
		{"octet too large", "192.168.1.256", false},
		{"invalid format", "192.168.1", false},
		{"non-numeric", "192.168.1.a", false},
		{"leading zeros", "192.168.01.1", false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := isValidIP(tc.input)
			assert.Equal(t, tc.expected, result)
		})
	}
}

// Helper function to create int pointer
func intPtr(i int) *int {
	return &i
}
