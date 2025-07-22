package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/jerkytreats/dns/internal/config"
	"github.com/jerkytreats/dns/internal/dns/coredns"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestHandler(t *testing.T) *RecordHandler {
	// Create temp directory for test
	tempDir, err := os.MkdirTemp("", "coredns-test-*")
	require.NoError(t, err)

	// Create template file
	templatePath := filepath.Join(tempDir, "Corefile.template")
	templateContent := `. {
    errors
    log
}
{{range .Domains}}
{{if .Enabled}}
# Configuration for {{.Domain}}
{{.Domain}}:{{.Port}} {
    file {{.ZoneFile}} {{.Domain}}
    errors
    log
}
{{end}}
{{end}}
`
	err = os.WriteFile(templatePath, []byte(templateContent), 0644)
	require.NoError(t, err)

	// Set up test configuration
	config.SetForTest("dns.coredns.config_path", filepath.Join(tempDir, "Corefile"))
	config.SetForTest("dns.coredns.template_path", templatePath)
	config.SetForTest("dns.coredns.zones_path", filepath.Join(tempDir, "zones"))
	config.SetForTest("dns.domain", "test.local")

	// Create zones directory
	err = os.MkdirAll(filepath.Join(tempDir, "zones"), 0755)
	require.NoError(t, err)

	manager := coredns.NewManager("127.0.0.1")
	err = manager.AddZone("test-service")
	require.NoError(t, err)

	handler, err := NewRecordHandler(manager, nil)
	require.NoError(t, err)

	return handler
}

func TestAddRecordHandler(t *testing.T) {
	handler := setupTestHandler(t)

	tests := []struct {
		name           string
		method         string
		requestBody    interface{}
		expectedStatus int
		expectedBody   string
	}{
		{
			name:   "Valid request",
			method: http.MethodPost,
			requestBody: map[string]string{
				"service_name": "test-service",
				"name":         "test-record",
				"ip":           "192.168.1.10",
			},
			expectedStatus: http.StatusCreated,
			expectedBody:   "Record added successfully",
		},
		{
			name:   "Invalid method",
			method: http.MethodGet,
			requestBody: map[string]string{
				"service_name": "test-service",
				"name":         "test-record",
				"ip":           "192.168.1.10",
			},
			expectedStatus: http.StatusMethodNotAllowed,
			expectedBody:   "Method not allowed\n",
		},
		{
			name:           "Invalid JSON",
			method:         http.MethodPost,
			requestBody:    "invalid json",
			expectedStatus: http.StatusBadRequest,
			expectedBody:   "Invalid request body\n",
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
			if tt.expectedBody != "" {
				assert.Equal(t, tt.expectedBody, w.Body.String())
			}
		})
	}
}

func TestListRecordsHandler(t *testing.T) {
	handler := setupTestHandler(t)

	// Add some test records
	manager := coredns.NewManager("127.0.0.1")

	// Set up test configuration
	tempDir, err := os.MkdirTemp("", "coredns-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	config.SetForTest("dns.coredns.config_path", filepath.Join(tempDir, "Corefile"))
	config.SetForTest("dns.coredns.zones_path", filepath.Join(tempDir, "zones"))
	config.SetForTest("dns.domain", "test.local")

	err = os.MkdirAll(filepath.Join(tempDir, "zones"), 0755)
	require.NoError(t, err)

	err = manager.AddZone("test-service")
	require.NoError(t, err)
	err = manager.AddRecord("test-service", "device1", "100.64.1.1")
	require.NoError(t, err)
	err = manager.AddRecord("test-service", "device2", "100.64.1.2")
	require.NoError(t, err)

	// Create handler with the manager that has records
	handler, err = NewRecordHandler(manager, nil)
	require.NoError(t, err)

	tests := []struct {
		name           string
		method         string
		expectedStatus int
		expectedCount  int
	}{
		{
			name:           "Valid GET request",
			method:         http.MethodGet,
			expectedStatus: http.StatusOK,
			expectedCount:  3, // 2 added records + 1 NS record
		},
		{
			name:           "Invalid method",
			method:         http.MethodPost,
			expectedStatus: http.StatusMethodNotAllowed,
			expectedCount:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, "/list-records", nil)
			w := httptest.NewRecorder()

			handler.ListRecords(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if tt.expectedStatus == http.StatusOK {
				assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

				var records []coredns.Record
				err := json.Unmarshal(w.Body.Bytes(), &records)
				require.NoError(t, err)
				assert.Len(t, records, tt.expectedCount)

				if tt.expectedCount > 0 {
					// Verify the structure of returned records
					for _, record := range records {
						assert.NotEmpty(t, record.Name)
						assert.Equal(t, "A", record.Type)
						assert.NotEmpty(t, record.IP)
					}

					// Verify specific records exist
					recordMap := make(map[string]string)
					for _, record := range records {
						recordMap[record.Name] = record.IP
					}
					assert.Equal(t, "100.64.1.1", recordMap["device1"])
					assert.Equal(t, "100.64.1.2", recordMap["device2"])
					assert.Equal(t, "127.0.0.1", recordMap["ns"]) // NS record automatically created
				}
			}
		})
	}
}

func TestListRecordsHandler_EmptyZone(t *testing.T) {
	handler := setupTestHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/list-records", nil)
	w := httptest.NewRecorder()

	handler.ListRecords(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var records []coredns.Record
	err := json.Unmarshal(w.Body.Bytes(), &records)
	require.NoError(t, err)
	assert.Len(t, records, 1) // Should have 1 NS record
	assert.Equal(t, "ns", records[0].Name)
	assert.Equal(t, "A", records[0].Type)
	assert.Equal(t, "127.0.0.1", records[0].IP)
}
