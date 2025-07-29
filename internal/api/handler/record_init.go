package handler

import (
	"reflect"

	"github.com/jerkytreats/dns/internal/api/types"
	"github.com/jerkytreats/dns/internal/dns/record"
)

func init() {
	// Register AddRecord endpoint
	types.RegisterRoute(types.RouteInfo{
		Method:       "POST",
		Path:         "/add-record",
		Handler:      nil, // Will be set during handler initialization
		RequestType:  reflect.TypeOf(record.CreateRecordRequest{}),
		ResponseType: reflect.TypeOf(record.Record{}),
		Module:       "dns",
		Summary:      "Create a new DNS record with optional reverse proxy configuration",
	})

	// Register ListRecords endpoint
	types.RegisterRoute(types.RouteInfo{
		Method:       "GET",
		Path:         "/list-records",
		Handler:      nil, // Will be set during handler initialization
		RequestType:  nil, // GET request has no body
		ResponseType: reflect.TypeOf([]record.Record{}),
		Module:       "dns",
		Summary:      "List all DNS records with proxy information",
	})
}