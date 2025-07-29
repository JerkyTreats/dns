package certificate

import (
	"errors"
	"testing"
	"time"

	"github.com/jerkytreats/dns/internal/dns/record"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// MockRecordService implements the record service interface for testing
type MockRecordService struct {
	mock.Mock
}

func (m *MockRecordService) ListRecords() ([]record.Record, error) {
	args := m.Called()
	return args.Get(0).([]record.Record), args.Error(1)
}

func TestNewDNSRecordAdapter(t *testing.T) {
	mockService := &MockRecordService{}
	adapter := NewDNSRecordAdapter(mockService)

	assert.NotNil(t, adapter)
	assert.Equal(t, mockService, adapter.recordService)
}

func TestDNSRecordAdapter_ListRecords_Success(t *testing.T) {
	tests := []struct {
		name            string
		inputRecords    []record.Record
		expectedRecords []DNSRecord
	}{
		{
			name: "Converts multiple records successfully",
			inputRecords: []record.Record{
				{
					Name: "api",
					Type: "A",
					IP:   "100.64.1.5",
					CreatedAt: time.Now(),
					UpdatedAt: time.Now(),
				},
				{
					Name: "dns",
					Type: "A",
					IP:   "100.64.1.6",
					CreatedAt: time.Now(),
					UpdatedAt: time.Now(),
				},
				{
					Name: "web",
					Type: "CNAME",
					IP:   "100.64.1.7",
					CreatedAt: time.Now(),
					UpdatedAt: time.Now(),
				},
			},
			expectedRecords: []DNSRecord{
				{Name: "api", Type: "A"},
				{Name: "dns", Type: "A"},
				{Name: "web", Type: "CNAME"},
			},
		},
		{
			name:            "Handles empty records list",
			inputRecords:    []record.Record{},
			expectedRecords: []DNSRecord{},
		},
		{
			name: "Handles records with empty names",
			inputRecords: []record.Record{
				{
					Name: "",
					Type: "A",
					IP:   "100.64.1.5",
				},
				{
					Name: "valid",
					Type: "A",
					IP:   "100.64.1.6",
				},
			},
			expectedRecords: []DNSRecord{
				{Name: "", Type: "A"},
				{Name: "valid", Type: "A"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockService := &MockRecordService{}
			mockService.On("ListRecords").Return(tt.inputRecords, nil)

			adapter := NewDNSRecordAdapter(mockService)
			result, err := adapter.ListRecords()

			require.NoError(t, err)
			assert.Equal(t, tt.expectedRecords, result)
			mockService.AssertExpectations(t)
		})
	}
}

func TestDNSRecordAdapter_ListRecords_Error(t *testing.T) {
	mockService := &MockRecordService{}
	expectedError := errors.New("failed to list records from service")
	mockService.On("ListRecords").Return([]record.Record{}, expectedError)

	adapter := NewDNSRecordAdapter(mockService)
	result, err := adapter.ListRecords()

	assert.Error(t, err)
	assert.Equal(t, expectedError, err)
	assert.Nil(t, result)
	mockService.AssertExpectations(t)
}

func TestDNSRecordAdapter_ListRecords_OnlyConvertsNameAndType(t *testing.T) {
	// Test that only Name and Type are converted, other fields are ignored
	inputRecord := record.Record{
		Name: "test",
		Type: "A",
		IP:   "100.64.1.5",
		ProxyRule: &record.ProxyRule{
			Enabled:    true,
			TargetIP:   "100.64.1.10",
			TargetPort: 8080,
			Protocol:   "http",
			Hostname:   "test.example.com",
			CreatedAt:  time.Now(),
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	mockService := &MockRecordService{}
	mockService.On("ListRecords").Return([]record.Record{inputRecord}, nil)

	adapter := NewDNSRecordAdapter(mockService)
	result, err := adapter.ListRecords()

	require.NoError(t, err)
	require.Len(t, result, 1)
	
	// Verify only Name and Type are set
	dnsRecord := result[0]
	assert.Equal(t, "test", dnsRecord.Name)
	assert.Equal(t, "A", dnsRecord.Type)
	mockService.AssertExpectations(t)
}