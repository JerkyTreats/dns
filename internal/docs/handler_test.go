package docs

import (
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

	html := handler.generateSwaggerHTML()

	requiredElements := []string{
		"<!DOCTYPE html>",
		"DNS Manager API Documentation",
		"swagger-ui-dist",
		"SwaggerUIBundle",
		"/docs/openapi.yaml",
		"swagger-ui",
	}

	for _, element := range requiredElements {
		if !strings.Contains(html, element) {
			t.Errorf("Generated HTML should contain '%s'", element)
		}
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