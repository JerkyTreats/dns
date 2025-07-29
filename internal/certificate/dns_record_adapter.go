package certificate

import (
	"github.com/jerkytreats/dns/internal/dns/record"
	"github.com/jerkytreats/dns/internal/logging"
)

// DNSRecordAdapter adapts the record service interface for certificate SAN validation
type DNSRecordAdapter struct {
	recordService interface {
		ListRecords() ([]record.Record, error)
	}
}

// NewDNSRecordAdapter creates a new adapter for the record service
func NewDNSRecordAdapter(recordService interface {
	ListRecords() ([]record.Record, error)
}) *DNSRecordAdapter {
	return &DNSRecordAdapter{
		recordService: recordService,
	}
}

// ListRecords adapts the record service's ListRecords method to return DNSRecord objects
func (adapter *DNSRecordAdapter) ListRecords() ([]DNSRecord, error) {
	logging.Debug("Fetching DNS records for SAN validation")
	
	records, err := adapter.recordService.ListRecords()
	if err != nil {
		return nil, err
	}

	dnsRecords := make([]DNSRecord, len(records))
	for i, rec := range records {
		dnsRecords[i] = DNSRecord{
			Name: rec.Name,
			Type: rec.Type,
		}
	}

	logging.Debug("Converted %d records for SAN validation", len(dnsRecords))
	return dnsRecords, nil
}