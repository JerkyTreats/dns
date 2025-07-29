package handler

import (
	"reflect"

	"github.com/jerkytreats/dns/internal/api/types"
	"github.com/jerkytreats/dns/internal/tailscale"
)

func init() {
	// Register ListDevices endpoint
	types.RegisterRoute(types.RouteInfo{
		Method:       "GET",
		Path:         "/list-devices",
		Handler:      nil, // Will be set during handler initialization
		RequestType:  nil, // GET request has no body
		ResponseType: reflect.TypeOf([]tailscale.PersistedDevice{}),
		Module:       "tailscale",
		Summary:      "List all Tailscale devices with their metadata",
	})

	// Register AnnotateDevice endpoint
	types.RegisterRoute(types.RouteInfo{
		Method:       "POST",
		Path:         "/annotate-device",
		Handler:      nil, // Will be set during handler initialization
		RequestType:  reflect.TypeOf(tailscale.AnnotationRequest{}),
		ResponseType: reflect.TypeOf(map[string]interface{}{}),
		Module:       "tailscale",
		Summary:      "Update annotatable device properties",
	})

	// Register GetStorageInfo endpoint
	types.RegisterRoute(types.RouteInfo{
		Method:       "GET",
		Path:         "/device-storage-info",
		Handler:      nil, // Will be set during handler initialization
		RequestType:  nil, // GET request has no body
		ResponseType: reflect.TypeOf(map[string]interface{}{}),
		Module:       "tailscale",
		Summary:      "Get device storage information for debugging",
	})
}