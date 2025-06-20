package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jerkytreats/dns/internal/config"
	"github.com/stretchr/testify/assert"
)

func TestHealthCheckHandler(t *testing.T) {
	// Setup test configuration
	config.SetForTest("app.version", "1.0.0")
	t.Cleanup(config.ResetForTest)

	tests := []struct {
		name           string
		method         string
		expectedStatus int
		expectedBody   map[string]interface{}
	}{
		{
			name:           "Valid GET request",
			method:         http.MethodGet,
			expectedStatus: http.StatusOK,
			expectedBody: map[string]interface{}{
				"status":  "healthy",
				"version": "1.0.0",
				"components": map[string]interface{}{
					"api": map[string]interface{}{
						"status":  "healthy",
						"message": "API is running",
					},
					"coredns": map[string]interface{}{
						"status":  "healthy",
						"message": "CoreDNS is running",
					},
					"bootstrap": map[string]interface{}{
						"status":  "disabled",
						"message": "Dynamic zone bootstrap is disabled",
					},
				},
			},
		},
		{
			name:           "Invalid method",
			method:         http.MethodPost,
			expectedStatus: http.StatusMethodNotAllowed,
			expectedBody:   nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, "/health", nil)
			w := httptest.NewRecorder()

			healthCheckHandler(w, req)

			// Check status code
			assert.Equal(t, tt.expectedStatus, w.Code)

			// Check response body if expected
			if tt.expectedBody != nil {
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedBody, response)
			}
		})
	}
}
