package handler

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jerkytreats/dns/internal/dns/coredns"
	"github.com/jerkytreats/dns/internal/dns/record"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRecordHandler_RemoveRecord_HTTPMethods(t *testing.T) {
	// Create a real service with real managers for this test
	// This tests the HTTP method validation
	dnsManager := coredns.NewManager("127.0.0.1")
	service := record.NewService(dnsManager, nil, nil)
	handler, err := NewRecordHandler(service, nil)
	require.NoError(t, err)

	testCases := []struct {
		method         string
		expectedStatus int
		description    string
	}{
		{http.MethodDelete, http.StatusInternalServerError, "DELETE should be allowed but may fail on service"}, // Will fail due to validation or DNS operations
		{http.MethodGet, http.StatusMethodNotAllowed, "GET should not be allowed"},
		{http.MethodPost, http.StatusMethodNotAllowed, "POST should not be allowed"},
		{http.MethodPut, http.StatusMethodNotAllowed, "PUT should not be allowed"},
		{http.MethodPatch, http.StatusMethodNotAllowed, "PATCH should not be allowed"},
	}

	for _, tc := range testCases {
		t.Run(tc.method+"_"+tc.description, func(t *testing.T) {
			reqBody := []byte(`{"service_name": "", "name": ""}`) // Invalid to trigger validation error
			httpReq := httptest.NewRequest(tc.method, "/remove-record", bytes.NewReader(reqBody))
			httpReq.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			// Act
			handler.RemoveRecord(w, httpReq)

			// Assert
			assert.Equal(t, tc.expectedStatus, w.Code, tc.description)
			
			if tc.method != http.MethodDelete {
				assert.Contains(t, w.Body.String(), "Method not allowed")
			}
		})
	}
}

func TestRecordHandler_RemoveRecord_RequestValidation(t *testing.T) {
	// Create a real service for testing request validation
	dnsManager := coredns.NewManager("127.0.0.1")
	service := record.NewService(dnsManager, nil, nil)
	handler, err := NewRecordHandler(service, nil)
	require.NoError(t, err)

	testCases := []struct {
		name           string
		requestBody    string
		expectedStatus int
		expectedError  string
	}{
		{
			name:           "invalid JSON",
			requestBody:    "invalid json",
			expectedStatus: http.StatusBadRequest,
			expectedError:  "Invalid request body",
		},
		{
			name:           "empty request body",
			requestBody:    "{}",
			expectedStatus: http.StatusInternalServerError,
			expectedError:  "Failed to remove record", // Will fail on validation
		},
		{
			name:           "missing service_name",
			requestBody:    `{"name": "testrecord"}`,
			expectedStatus: http.StatusInternalServerError,
			expectedError:  "Failed to remove record", // Will fail on validation
		},
		{
			name:           "missing name",
			requestBody:    `{"service_name": "internal"}`,
			expectedStatus: http.StatusInternalServerError,
			expectedError:  "Failed to remove record", // Will fail on validation
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			httpReq := httptest.NewRequest(http.MethodDelete, "/remove-record", bytes.NewReader([]byte(tc.requestBody)))
			httpReq.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			// Act
			handler.RemoveRecord(w, httpReq)

			// Assert
			assert.Equal(t, tc.expectedStatus, w.Code)
			assert.Contains(t, w.Body.String(), tc.expectedError)
		})
	}
}

func TestRecordHandler_RemoveRecord_WithCertificateManager(t *testing.T) {
	// Test that certificate manager integration doesn't break the handler
	dnsManager := coredns.NewManager("127.0.0.1")
	service := record.NewService(dnsManager, nil, nil)
	
	// Use existing MockCertificateManager
	mockCertManager := &MockCertificateManager{shouldSucceed: true}
	handler, err := NewRecordHandler(service, mockCertManager)
	require.NoError(t, err)

	// This test will succeed because DNS manager handles non-existent records gracefully
	reqBody := `{"service_name": "internal", "name": "testrecord"}`
	httpReq := httptest.NewRequest(http.MethodDelete, "/remove-record", bytes.NewReader([]byte(reqBody)))
	httpReq.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	// Act
	handler.RemoveRecord(w, httpReq)

	// Assert - should succeed even if record doesn't exist
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))
	assert.Contains(t, w.Body.String(), "Record removed successfully")

	// Verify handler has certificate manager
	assert.NotNil(t, handler.certificateManager)
}

func TestRecordHandler_RemoveRecord_NilCertificateManager(t *testing.T) {
	// Test that nil certificate manager doesn't break the handler
	dnsManager := coredns.NewManager("127.0.0.1")
	service := record.NewService(dnsManager, nil, nil)
	handler, err := NewRecordHandler(service, nil)
	require.NoError(t, err)

	reqBody := `{"service_name": "internal", "name": "testrecord"}`
	httpReq := httptest.NewRequest(http.MethodDelete, "/remove-record", bytes.NewReader([]byte(reqBody)))
	httpReq.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	// Act
	handler.RemoveRecord(w, httpReq)

	// Assert - should succeed even if record doesn't exist
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))
	assert.Contains(t, w.Body.String(), "Record removed successfully")

	// Verify handler has nil certificate manager
	assert.Nil(t, handler.certificateManager)
}

func TestRecordHandler_RemoveRecord_Integration(t *testing.T) {
	// Integration test that verifies the complete handler behavior
	// This test demonstrates that the handler properly integrates with the service layer
	
	dnsManager := coredns.NewManager("127.0.0.1")
	service := record.NewService(dnsManager, nil, nil)
	handler, err := NewRecordHandler(service, nil)
	require.NoError(t, err)

	t.Run("method validation", func(t *testing.T) {
		// Test that only DELETE is allowed
		methods := []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch}
		
		for _, method := range methods {
			httpReq := httptest.NewRequest(method, "/remove-record", bytes.NewReader([]byte(`{}`)))
			w := httptest.NewRecorder()
			
			handler.RemoveRecord(w, httpReq)
			
			assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
			assert.Contains(t, w.Body.String(), "Method not allowed")
		}
	})

	t.Run("json parsing", func(t *testing.T) {
		// Test that invalid JSON is handled properly
		httpReq := httptest.NewRequest(http.MethodDelete, "/remove-record", bytes.NewReader([]byte("invalid json")))
		httpReq.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		
		handler.RemoveRecord(w, httpReq)
		
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "Invalid request body")
	})

	t.Run("service integration", func(t *testing.T) {
		// Test that the handler properly calls the service layer
		reqBody := `{"service_name": "internal", "name": "testrecord"}`
		httpReq := httptest.NewRequest(http.MethodDelete, "/remove-record", bytes.NewReader([]byte(reqBody)))
		httpReq.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		
		handler.RemoveRecord(w, httpReq)
		
		// Should succeed because DNS manager handles non-existent records gracefully
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "application/json", w.Header().Get("Content-Type"))
		assert.Contains(t, w.Body.String(), "Record removed successfully")
	})
}