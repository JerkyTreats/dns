package docs

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewDocsHandler(t *testing.T) {
	handler, err := NewDocsHandler()
	if err != nil {
		t.Fatalf("NewDocsHandler() error = %v", err)
	}
	
	if handler == nil {
		t.Fatal("NewDocsHandler() returned nil")
	}
	
	if handler.swaggerConfig.Path != "/swagger" {
		t.Errorf("Expected default path '/swagger', got '%s'", handler.swaggerConfig.Path)
	}
	
	if !handler.swaggerConfig.Enabled {
		t.Error("Expected swagger to be enabled by default")
	}
}

func TestServeSwaggerUI(t *testing.T) {
	handler, err := NewDocsHandler()
	if err != nil {
		t.Fatalf("NewDocsHandler() error = %v", err)
	}

	tests := []struct {
		name           string
		method         string
		expectedStatus int
		expectHTML     bool
	}{
		{
			name:           "GET request returns HTML",
			method:         "GET",
			expectedStatus: http.StatusOK,
			expectHTML:     true,
		},
		{
			name:           "POST request not allowed",
			method:         "POST",
			expectedStatus: http.StatusMethodNotAllowed,
			expectHTML:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, "/swagger", nil)
			w := httptest.NewRecorder()

			handler.ServeSwaggerUI(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, w.Code)
			}

			if tt.expectHTML {
				contentType := w.Header().Get("Content-Type")
				if !strings.Contains(contentType, "text/html") {
					t.Errorf("Expected HTML content type, got %s", contentType)
				}

				body := w.Body.String()
				if !strings.Contains(body, "Swagger") {
					t.Error("Response should contain 'Swagger'")
				}

				if !strings.Contains(body, "DNS Manager API Documentation") {
					t.Error("Response should contain the API title")
				}
			}
		})
	}
}

func TestServeOpenAPISpec_FileNotFound(t *testing.T) {
	handler, err := NewDocsHandler()
	if err != nil {
		t.Fatalf("NewDocsHandler() error = %v", err)
	}

	req := httptest.NewRequest("GET", "/docs/openapi.yaml", nil)
	w := httptest.NewRecorder()

	handler.ServeOpenAPISpec(w, req)

	// Since the file doesn't exist in test environment, should return 404
	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status %d when file not found, got %d", http.StatusNotFound, w.Code)
	}
}

func TestServeOpenAPISpec_MethodNotAllowed(t *testing.T) {
	handler, err := NewDocsHandler()
	if err != nil {
		t.Fatalf("NewDocsHandler() error = %v", err)
	}

	req := httptest.NewRequest("POST", "/docs/openapi.yaml", nil)
	w := httptest.NewRecorder()

	handler.ServeOpenAPISpec(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status %d for POST method, got %d", http.StatusMethodNotAllowed, w.Code)
	}
}

func TestServeDocs(t *testing.T) {
	handler, err := NewDocsHandler()
	if err != nil {
		t.Fatalf("NewDocsHandler() error = %v", err)
	}

	tests := []struct {
		name           string
		path           string
		method         string
		expectedStatus int
		expectRedirect bool
	}{
		{
			name:           "Root docs path redirects to swagger",
			path:           "/docs",
			method:         "GET",
			expectedStatus: http.StatusFound,
			expectRedirect: true,
		},
		{
			name:           "Root docs path with slash redirects to swagger",
			path:           "/docs/",
			method:         "GET",
			expectedStatus: http.StatusFound,
			expectRedirect: true,
		},
		{
			name:           "Directory traversal blocked",
			path:           "/docs/../config.yaml",
			method:         "GET",
			expectedStatus: http.StatusBadRequest,
			expectRedirect: false,
		},
		{
			name:           "POST method not allowed",
			path:           "/docs",
			method:         "POST",
			expectedStatus: http.StatusMethodNotAllowed,
			expectRedirect: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			w := httptest.NewRecorder()

			handler.ServeDocs(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, w.Code)
			}

			if tt.expectRedirect {
				location := w.Header().Get("Location")
				if location != "/swagger" {
					t.Errorf("Expected redirect to '/swagger', got '%s'", location)
				}
			}
		})
	}
}

func TestGenerateSwaggerHTML_ContainsRequiredElements(t *testing.T) {
	handler, err := NewDocsHandler()
	if err != nil {
		t.Fatalf("NewDocsHandler() error = %v", err)
	}

	tests := []struct {
		name        string
		setupTLS    bool
		host        string
		expectedURL string
	}{
		{
			name:        "HTTP request generates HTTP spec URL",
			setupTLS:    false,
			host:        "localhost:8080",
			expectedURL: "http://localhost:8080/docs/openapi.yaml",
		},
		{
			name:        "HTTPS request generates HTTPS spec URL",
			setupTLS:    true,
			host:        "dns.internal.jerkytreats.dev",
			expectedURL: "https://dns.internal.jerkytreats.dev/docs/openapi.yaml",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/swagger", nil)
			req.Host = tt.host
			
			if tt.setupTLS {
				// Simulate HTTPS request by setting TLS field
				req.TLS = &tls.ConnectionState{}
			}
			
			html := handler.generateSwaggerHTML(req)

			requiredElements := []string{
				"<!DOCTYPE html>",
				"DNS Manager API Documentation",
				"swagger-ui-dist",
				"SwaggerUIBundle",
				"swagger-ui",
				tt.expectedURL,
			}

			for _, element := range requiredElements {
				if !strings.Contains(html, element) {
					t.Errorf("Generated HTML should contain '%s'", element)
				}
			}
		})
	}
}

func TestGenerateSwaggerHTML_ProtocolDetection(t *testing.T) {
	handler, err := NewDocsHandler()
	if err != nil {
		t.Fatalf("NewDocsHandler() error = %v", err)
	}

	tests := []struct {
		name             string
		host             string
		setupTLS         bool
		expectedProtocol string
		expectedHost     string
	}{
		{
			name:             "HTTP request with localhost",
			host:             "localhost:8080",
			setupTLS:         false,
			expectedProtocol: "http",
			expectedHost:     "localhost:8080",
		},
		{
			name:             "HTTP request with custom domain",
			host:             "dns.internal.jerkytreats.dev:8080",
			setupTLS:         false,
			expectedProtocol: "http",
			expectedHost:     "dns.internal.jerkytreats.dev:8080",
		},
		{
			name:             "HTTPS request with standard port",
			host:             "dns.internal.jerkytreats.dev",
			setupTLS:         true,
			expectedProtocol: "https",
			expectedHost:     "dns.internal.jerkytreats.dev",
		},
		{
			name:             "HTTPS request with custom port",
			host:             "dns.internal.jerkytreats.dev:8443",
			setupTLS:         true,
			expectedProtocol: "https",
			expectedHost:     "dns.internal.jerkytreats.dev:8443",
		},
		{
			name:             "HTTPS request with localhost",
			host:             "localhost:8443",
			setupTLS:         true,
			expectedProtocol: "https",
			expectedHost:     "localhost:8443",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/swagger", nil)
			req.Host = tt.host
			
			if tt.setupTLS {
				req.TLS = &tls.ConnectionState{}
			}
			
			html := handler.generateSwaggerHTML(req)
			
			expectedBaseURL := fmt.Sprintf("%s://%s", tt.expectedProtocol, tt.expectedHost)
			expectedSpecURL := expectedBaseURL + "/docs/openapi.yaml"
			
			if !strings.Contains(html, expectedSpecURL) {
				t.Errorf("Expected HTML to contain spec URL '%s', but it didn't. HTML snippet: %s", 
					expectedSpecURL, html[strings.Index(html, "SwaggerUIBundle"):strings.Index(html, "SwaggerUIBundle")+200])
			}
			
			// Verify the URL is used in the SwaggerUIBundle configuration
			swaggerConfigStart := strings.Index(html, "SwaggerUIBundle({")
			swaggerConfigEnd := strings.Index(html[swaggerConfigStart:], "});") + swaggerConfigStart
			swaggerConfig := html[swaggerConfigStart:swaggerConfigEnd]
			
			if !strings.Contains(swaggerConfig, fmt.Sprintf("url: '%s'", expectedSpecURL)) {
				t.Errorf("Expected Swagger config to contain 'url: %s', but it didn't. Config: %s", 
					expectedSpecURL, swaggerConfig)
			}
		})
	}
}

func TestGenerateSwaggerHTML_EdgeCases(t *testing.T) {
	handler, err := NewDocsHandler()
	if err != nil {
		t.Fatalf("NewDocsHandler() error = %v", err)
	}

	tests := []struct {
		name        string
		setupReq    func() *http.Request
		expectError bool
		description string
	}{
		{
			name: "Request with empty Host header",
			setupReq: func() *http.Request {
				req := httptest.NewRequest("GET", "/swagger", nil)
				req.Host = ""
				return req
			},
			expectError: false,
			description: "Should handle empty host gracefully with config fallback",
		},
		{
			name: "Request with IPv6 host",
			setupReq: func() *http.Request {
				req := httptest.NewRequest("GET", "/swagger", nil)
				req.Host = "[::1]:8080"
				return req
			},
			expectError: false,
			description: "Should handle IPv6 addresses",
		},
		{
			name: "TLS connection state with HTTP protocol",
			setupReq: func() *http.Request {
				req := httptest.NewRequest("GET", "/swagger", nil)
				req.Host = "example.com:8080"
				// This simulates a request that went through TLS termination
				req.TLS = &tls.ConnectionState{}
				return req
			},
			expectError: false,
			description: "Should use HTTPS when TLS connection state present",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := tt.setupReq()
			
			// Should not panic or error
			html := handler.generateSwaggerHTML(req)
			
			// Basic validation that HTML was generated
			if len(html) == 0 {
				t.Error("Generated HTML should not be empty")
			}
			
			if !strings.Contains(html, "SwaggerUIBundle") {
				t.Error("Generated HTML should contain SwaggerUIBundle")
			}
			
			if !strings.Contains(html, "/docs/openapi.yaml") {
				t.Error("Generated HTML should contain openapi.yaml URL")
			}
			
			// For IPv6 case, ensure brackets are preserved
			if req.Host == "[::1]:8080" {
				expectedURL := "http://[::1]:8080/docs/openapi.yaml"
				if !strings.Contains(html, expectedURL) {
					t.Errorf("Expected IPv6 URL %s to be present in HTML", expectedURL)
				}
			}
		})
	}
}

func TestGenerateSwaggerHTML_RequestBasedVsConfigBased(t *testing.T) {
	handler, err := NewDocsHandler()
	if err != nil {
		t.Fatalf("NewDocsHandler() error = %v", err)
	}

	// Test that the new implementation uses request data instead of config
	tests := []struct {
		name         string
		requestHost  string
		requestTLS   bool
		expectedURL  string
		description  string
	}{
		{
			name:        "Request-based HTTP URL generation",
			requestHost: "actual-request-host.com:9000",
			requestTLS:  false,
			expectedURL: "http://actual-request-host.com:9000/docs/openapi.yaml",
			description: "Should use actual request host and detect HTTP",
		},
		{
			name:        "Request-based HTTPS URL generation",
			requestHost: "secure-host.example.com:9443",
			requestTLS:  true,
			expectedURL: "https://secure-host.example.com:9443/docs/openapi.yaml",
			description: "Should use actual request host and detect HTTPS",
		},
		{
			name:        "Standard ports handled correctly",
			requestHost: "standard.example.com",
			requestTLS:  true,
			expectedURL: "https://standard.example.com/docs/openapi.yaml",
			description: "Should work with standard ports (no explicit port)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/swagger", nil)
			req.Host = tt.requestHost
			
			if tt.requestTLS {
				req.TLS = &tls.ConnectionState{}
			}
			
			html := handler.generateSwaggerHTML(req)
			
			if !strings.Contains(html, tt.expectedURL) {
				t.Errorf("Expected HTML to contain URL '%s', but it didn't", tt.expectedURL)
				
				// Find and display the actual URL for debugging
				urlStart := strings.Index(html, "url: '") + 6
				if urlStart > 5 {
					urlEnd := strings.Index(html[urlStart:], "'")
					if urlEnd > 0 {
						actualURL := html[urlStart : urlStart+urlEnd]
						t.Errorf("Actual URL found: '%s'", actualURL)
					}
				}
			}
		})
	}
}

func TestGenerateSwaggerHTML_DNSEndpoint(t *testing.T) {
	handler, err := NewDocsHandler()
	if err != nil {
		t.Fatalf("NewDocsHandler() error = %v", err)
	}

	tests := []struct {
		name        string
		requestHost string
		requestTLS  bool
		expectedURL string
		description string
	}{
		{
			name:        "DNS endpoint HTTP",
			requestHost: "dns.internal.jerkytreats.dev",
			requestTLS:  false,
			expectedURL: "http://dns.internal.jerkytreats.dev/docs/openapi.yaml",
			description: "Should use DNS endpoint for HTTP requests",
		},
		{
			name:        "DNS endpoint HTTPS",
			requestHost: "dns.internal.jerkytreats.dev",
			requestTLS:  true,
			expectedURL: "https://dns.internal.jerkytreats.dev/docs/openapi.yaml",
			description: "Should use DNS endpoint for HTTPS requests",
		},
		{
			name:        "DNS endpoint with custom port HTTP",
			requestHost: "dns.internal.jerkytreats.dev:8080",
			requestTLS:  false,
			expectedURL: "http://dns.internal.jerkytreats.dev:8080/docs/openapi.yaml",
			description: "Should preserve custom port in DNS endpoint HTTP",
		},
		{
			name:        "DNS endpoint with custom port HTTPS",
			requestHost: "dns.internal.jerkytreats.dev:8443",
			requestTLS:  true,
			expectedURL: "https://dns.internal.jerkytreats.dev:8443/docs/openapi.yaml",
			description: "Should preserve custom port in DNS endpoint HTTPS",
		},
		{
			name:        "Local development fallback",
			requestHost: "localhost:8080",
			requestTLS:  false,
			expectedURL: "http://localhost:8080/docs/openapi.yaml",
			description: "Should work with localhost for development",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/swagger", nil)
			req.Host = tt.requestHost
			
			if tt.requestTLS {
				req.TLS = &tls.ConnectionState{}
			}
			
			html := handler.generateSwaggerHTML(req)
			
			if !strings.Contains(html, tt.expectedURL) {
				t.Errorf("Expected HTML to contain URL '%s', but it didn't", tt.expectedURL)
				
				// Find and display the actual URL for debugging
				urlStart := strings.Index(html, "url: '") + 6
				if urlStart > 5 {
					urlEnd := strings.Index(html[urlStart:], "'")
					if urlEnd > 0 {
						actualURL := html[urlStart : urlStart+urlEnd]
						t.Errorf("Actual URL found: '%s'", actualURL)
					}
				}
			}
			
			// Verify no localhost fallback when using DNS endpoint
			if strings.Contains(tt.requestHost, "dns.internal.jerkytreats.dev") {
				if strings.Contains(html, "localhost") {
					t.Errorf("DNS endpoint should not contain localhost fallback in URL")
				}
			}
		})
	}
}

func TestGenerateSwaggerHTML_ProxyHeaders(t *testing.T) {
	handler, err := NewDocsHandler()
	if err != nil {
		t.Fatalf("NewDocsHandler() error = %v", err)
	}

	tests := []struct {
		name           string
		requestHost    string
		requestTLS     bool
		proxyHeaders   map[string]string
		expectedURL    string
		description    string
	}{
		{
			name:        "X-Forwarded-Proto HTTPS header",
			requestHost: "dns.internal.jerkytreats.dev",
			requestTLS:  false, // Direct TLS is false, but proxy header indicates HTTPS
			proxyHeaders: map[string]string{
				"X-Forwarded-Proto": "https",
			},
			expectedURL: "https://dns.internal.jerkytreats.dev/docs/openapi.yaml",
			description: "Should detect HTTPS from X-Forwarded-Proto header",
		},
		{
			name:        "X-Forwarded-Scheme HTTPS header",
			requestHost: "dns.internal.jerkytreats.dev",
			requestTLS:  false,
			proxyHeaders: map[string]string{
				"X-Forwarded-Scheme": "https",
			},
			expectedURL: "https://dns.internal.jerkytreats.dev/docs/openapi.yaml",
			description: "Should detect HTTPS from X-Forwarded-Scheme header",
		},
		{
			name:        "X-Forwarded-Ssl ON header",
			requestHost: "dns.internal.jerkytreats.dev",
			requestTLS:  false,
			proxyHeaders: map[string]string{
				"X-Forwarded-Ssl": "on",
			},
			expectedURL: "https://dns.internal.jerkytreats.dev/docs/openapi.yaml",
			description: "Should detect HTTPS from X-Forwarded-Ssl: on header",
		},
		{
			name:        "X-Forwarded-Ssl ON header case insensitive",
			requestHost: "dns.internal.jerkytreats.dev",
			requestTLS:  false,
			proxyHeaders: map[string]string{
				"X-Forwarded-Ssl": "ON",
			},
			expectedURL: "https://dns.internal.jerkytreats.dev/docs/openapi.yaml",
			description: "Should detect HTTPS from X-Forwarded-Ssl: ON header (case insensitive)",
		},
		{
			name:        "Multiple proxy headers with HTTPS",
			requestHost: "dns.internal.jerkytreats.dev:8443",
			requestTLS:  false,
			proxyHeaders: map[string]string{
				"X-Forwarded-Proto":  "https",
				"X-Forwarded-Scheme": "https",
				"X-Forwarded-Ssl":    "on",
			},
			expectedURL: "https://dns.internal.jerkytreats.dev:8443/docs/openapi.yaml",
			description: "Should work with multiple proxy headers indicating HTTPS",
		},
		{
			name:        "HTTP proxy headers",
			requestHost: "dns.internal.jerkytreats.dev",
			requestTLS:  false,
			proxyHeaders: map[string]string{
				"X-Forwarded-Proto": "http",
			},
			expectedURL: "http://dns.internal.jerkytreats.dev/docs/openapi.yaml",
			description: "Should use HTTP when proxy headers indicate HTTP",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/swagger", nil)
			req.Host = tt.requestHost
			
			if tt.requestTLS {
				req.TLS = &tls.ConnectionState{}
			}
			
			// Set proxy headers
			for header, value := range tt.proxyHeaders {
				req.Header.Set(header, value)
			}
			
			html := handler.generateSwaggerHTML(req)
			
			if !strings.Contains(html, tt.expectedURL) {
				t.Errorf("Expected HTML to contain URL '%s', but it didn't", tt.expectedURL)
				
				// Find and display the actual URL for debugging
				urlStart := strings.Index(html, "url: '") + 6
				if urlStart > 5 {
					urlEnd := strings.Index(html[urlStart:], "'")
					if urlEnd > 0 {
						actualURL := html[urlStart : urlStart+urlEnd]
						t.Errorf("Actual URL found: '%s'", actualURL)
					}
				}
			}
		})
	}
}

func TestGetThemeCSS(t *testing.T) {
	handler, err := NewDocsHandler()
	if err != nil {
		t.Fatalf("NewDocsHandler() error = %v", err)
	}

	// Test dark theme
	handler.swaggerConfig.Theme = "dark"
	darkCSS := handler.getThemeCSS()
	if darkCSS == "" {
		t.Error("Dark theme should return CSS")
	}
	if !strings.Contains(darkCSS, "background: #1f1f1f") {
		t.Error("Dark theme CSS should contain dark background")
	}

	// Test light theme (or any other theme)
	handler.swaggerConfig.Theme = "light"
	lightCSS := handler.getThemeCSS()
	if lightCSS != "" {
		t.Error("Non-dark theme should return empty CSS")
	}
}