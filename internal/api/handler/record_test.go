package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/jerkytreats/dns/internal/dns/coredns"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func setupTestHandler(t *testing.T) *RecordHandler {
	tempDir, err := os.MkdirTemp("", "coredns-test-*")
	require.NoError(t, err)

	configPath := filepath.Join(tempDir, "Corefile")
	zonesPath := filepath.Join(tempDir, "zones")
	// Create initial Corefile
	initialConfig := `. {
    errors
    log
}`
	err = os.WriteFile(configPath, []byte(initialConfig), 0644)
	require.NoError(t, err)

	logger, _ := zap.NewDevelopment()
	manager := coredns.NewManager(logger, configPath, zonesPath, []string{"echo", "reload"})
	return NewRecordHandler(logger, manager)
}

func TestAddRecordHandler(t *testing.T) {
	handler := setupTestHandler(t)

	tests := []struct {
		name           string
		method         string
		requestBody    interface{}
		expectedStatus int
		expectedBody   map[string]interface{}
	}{
		{
			name:   "Valid request",
			method: http.MethodPost,
			requestBody: AddRecordRequest{
				ServiceName: "test-service",
			},
			expectedStatus: http.StatusOK,
			expectedBody: map[string]interface{}{
				"status":  "success",
				"message": "Record added successfully",
				"data": map[string]interface{}{
					"hostname": "test-service.internal.jerkytreats.dev",
				},
			},
		},
		{
			name:   "Invalid method",
			method: http.MethodGet,
			requestBody: AddRecordRequest{
				ServiceName: "test-service",
			},
			expectedStatus: http.StatusMethodNotAllowed,
			expectedBody:   nil,
		},
		{
			name:   "Invalid service name format",
			method: http.MethodPost,
			requestBody: AddRecordRequest{
				ServiceName: "test_service", // contains underscore
			},
			expectedStatus: http.StatusBadRequest,
			expectedBody: map[string]interface{}{
				"status":     "error",
				"message":    "Invalid service name format",
				"error_code": "INVALID_SERVICE_NAME",
			},
		},
		{
			name:   "Empty service name",
			method: http.MethodPost,
			requestBody: AddRecordRequest{
				ServiceName: "",
			},
			expectedStatus: http.StatusBadRequest,
			expectedBody: map[string]interface{}{
				"status":     "error",
				"message":    "Invalid service name format",
				"error_code": "INVALID_SERVICE_NAME",
			},
		},
		{
			name:           "Invalid JSON",
			method:         http.MethodPost,
			requestBody:    "invalid json",
			expectedStatus: http.StatusBadRequest,
			expectedBody: map[string]interface{}{
				"status":     "error",
				"message":    "Invalid request body",
				"error_code": "INVALID_REQUEST",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var body []byte
			if tt.requestBody != nil {
				if strBody, ok := tt.requestBody.(string); ok {
					body = []byte(strBody)
				} else {
					body, _ = json.Marshal(tt.requestBody)
				}
			}

			req := httptest.NewRequest(tt.method, "/add-record", bytes.NewBuffer(body))
			w := httptest.NewRecorder()

			handler.AddRecord(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if tt.expectedBody != nil {
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedBody, response)
			}
		})
	}
}

func TestSendError(t *testing.T) {
	tests := []struct {
		name           string
		message        string
		errorCode      string
		statusCode     int
		expectedStatus int
		expectedBody   map[string]interface{}
	}{
		{
			name:           "Basic error",
			message:        "Test error",
			errorCode:      "TEST_ERROR",
			statusCode:     http.StatusBadRequest,
			expectedStatus: http.StatusBadRequest,
			expectedBody: map[string]interface{}{
				"status":     "error",
				"message":    "Test error",
				"error_code": "TEST_ERROR",
			},
		},
		{
			name:           "Error without code",
			message:        "Test error",
			errorCode:      "",
			statusCode:     http.StatusInternalServerError,
			expectedStatus: http.StatusInternalServerError,
			expectedBody: map[string]interface{}{
				"status":  "error",
				"message": "Test error",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			sendError(w, tt.message, tt.errorCode, tt.statusCode)

			assert.Equal(t, tt.expectedStatus, w.Code)

			var response map[string]interface{}
			err := json.Unmarshal(w.Body.Bytes(), &response)
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedBody, response)
		})
	}
}
