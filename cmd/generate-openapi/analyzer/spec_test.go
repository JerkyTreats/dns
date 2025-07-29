package analyzer

import (
	"reflect"
	"strings"
	"testing"

	"github.com/jerkytreats/dns/internal/api/types"
	"gopkg.in/yaml.v3"
)

func TestBuildOpenAPISpec(t *testing.T) {
	gen := NewGenerator()
	
	// Mock some routes
	gen.routes = []types.RouteInfo{
		{
			Method:       "GET",
			Path:         "/health",
			RequestType:  nil,
			ResponseType: reflect.TypeOf(map[string]interface{}{}),
			Module:       "health",
			Summary:      "Health check endpoint",
		},
		{
			Method:       "POST",
			Path:         "/users",
			RequestType:  reflect.TypeOf(TestRequest{}),
			ResponseType: reflect.TypeOf(TestResponse{}),
			Module:       "users",
			Summary:      "Create a new user",
		},
	}
	
	// Generate schemas for the routes
	err := gen.generateSchemas()
	if err != nil {
		t.Fatalf("generateSchemas() error = %v", err)
	}
	
	// Add standard schemas
	gen.addStandardSchemas()
	
	spec := gen.buildOpenAPISpec()
	
	// Verify it's valid YAML
	var parsed map[string]interface{}
	err = yaml.Unmarshal([]byte(spec), &parsed)
	if err != nil {
		t.Fatalf("Generated spec is not valid YAML: %v", err)
	}
	
	// Check required OpenAPI fields
	if parsed["openapi"] != "3.0.3" {
		t.Error("OpenAPI version should be 3.0.3")
	}
	
	info, ok := parsed["info"].(map[string]interface{})
	if !ok {
		t.Fatal("Info section should be present")
	}
	
	if info["title"] != "DNS Manager API" {
		t.Error("Title should be 'DNS Manager API'")
	}
	
	// Check paths
	paths, ok := parsed["paths"].(map[string]interface{})
	if !ok {
		t.Fatal("Paths section should be present")
	}
	
	if len(paths) != 2 {
		t.Errorf("Expected 2 paths, got %d", len(paths))
	}
	
	// Check that both paths are present
	if _, exists := paths["/health"]; !exists {
		t.Error("Path '/health' should be present")
	}
	
	if _, exists := paths["/users"]; !exists {
		t.Error("Path '/users' should be present")
	}
}

func TestGenerateOperationID(t *testing.T) {
	gen := NewGenerator()
	
	tests := []struct {
		name     string
		route    types.RouteInfo
		expected string
	}{
		{
			name: "GET health",
			route: types.RouteInfo{
				Method: "GET",
				Path:   "/health",
			},
			expected: "gethealth",
		},
		{
			name: "POST with create in path",
			route: types.RouteInfo{
				Method: "POST",
				Path:   "/add-record",
			},
			expected: "createaddRecord",
		},
		{
			name: "GET with list in path",
			route: types.RouteInfo{
				Method: "GET",
				Path:   "/list-users",
			},
			expected: "listlistUsers",
		},
		{
			name: "PUT operation",
			route: types.RouteInfo{
				Method: "PUT",
				Path:   "/update-user",
			},
			expected: "updateupdateUser",
		},
		{
			name: "DELETE operation",
			route: types.RouteInfo{
				Method: "DELETE",
				Path:   "/remove-item",
			},
			expected: "deleteremoveItem",
		},
		{
			name: "Complex path with hyphens",
			route: types.RouteInfo{
				Method: "POST",
				Path:   "/api/v1/user-management/create-admin",
			},
			expected: "createapiV1UserManagementCreateAdmin",
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := gen.generateOperationID(tt.route)
			if result != tt.expected {
				t.Errorf("generateOperationID() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

func TestBuildRequestBody(t *testing.T) {
	gen := NewGenerator()
	
	route := types.RouteInfo{
		Method:       "POST",
		Path:         "/test",
		RequestType:  reflect.TypeOf(TestRequest{}),
		ResponseType: reflect.TypeOf(TestResponse{}),
		Module:       "test",
		Summary:      "Test endpoint",
	}
	
	// Generate schema for the request type
	schema, err := gen.generateTypeSchema(route.RequestType)
	if err != nil {
		t.Fatalf("generateTypeSchema() error = %v", err)
	}
	gen.typeSchemas[gen.getTypeName(route.RequestType)] = schema
	
	requestBody := gen.buildRequestBody(route)
	
	if requestBody == nil {
		t.Fatal("buildRequestBody() should not return nil")
	}
	
	if !requestBody.Required {
		t.Error("Request body should be required")
	}
	
	if requestBody.Description == "" {
		t.Error("Request body should have a description")
	}
	
	content := requestBody.Content
	if content == nil {
		t.Fatal("Request body should have content")
	}
	
	jsonContent, exists := content["application/json"]
	if !exists {
		t.Error("Request body should have application/json content")
	}
	
	expectedRef := "#/components/schemas/TestRequest"
	if jsonContent.Schema.Ref != expectedRef {
		t.Errorf("Expected schema ref %s, got %s", expectedRef, jsonContent.Schema.Ref)
	}
}

func TestBuildResponses(t *testing.T) {
	gen := NewGenerator()
	
	route := types.RouteInfo{
		Method:       "POST",
		Path:         "/test",
		RequestType:  reflect.TypeOf(TestRequest{}),
		ResponseType: reflect.TypeOf(TestResponse{}),
		Module:       "test",
		Summary:      "Test endpoint",
	}
	
	// Generate schema for the response type
	schema, err := gen.generateTypeSchema(route.ResponseType)
	if err != nil {
		t.Fatalf("generateTypeSchema() error = %v", err)
	}
	gen.typeSchemas[gen.getTypeName(route.ResponseType)] = schema
	
	responses := gen.buildResponses(route)
	
	if responses == nil {
		t.Fatal("buildResponses() should not return nil")
	}
	
	// Check success response
	successResponse, exists := responses["200"]
	if !exists {
		t.Error("Should have 200 success response")
	}
	
	if successResponse.Description == "" {
		t.Error("Success response should have description")
	}
	
	// Check error responses
	errorCodes := []string{"400", "500", "422"}
	for _, code := range errorCodes {
		if _, exists := responses[code]; !exists {
			t.Errorf("Should have %s error response", code)
		}
	}
	
	// Verify error responses reference ErrorResponse schema
	badRequestResponse := responses["400"]
	jsonContent := badRequestResponse.Content["application/json"]
	expectedErrorRef := "#/components/schemas/ErrorResponse"
	if jsonContent.Schema.Ref != expectedErrorRef {
		t.Errorf("Error response should reference ErrorResponse schema, got %s", jsonContent.Schema.Ref)
	}
}

func TestBuildResponses_GET_NoUnprocessableEntity(t *testing.T) {
	gen := NewGenerator()
	
	route := types.RouteInfo{
		Method:       "GET",
		Path:         "/test",
		RequestType:  nil,
		ResponseType: reflect.TypeOf(TestResponse{}),
		Module:       "test",
		Summary:      "Get test data",
	}
	
	responses := gen.buildResponses(route)
	
	// GET requests should not have 422 Unprocessable Entity
	if _, exists := responses["422"]; exists {
		t.Error("GET requests should not have 422 Unprocessable Entity response")
	}
	
	// But should still have other error responses
	if _, exists := responses["400"]; !exists {
		t.Error("Should still have 400 Bad Request response")
	}
	
	if _, exists := responses["500"]; !exists {
		t.Error("Should still have 500 Internal Server Error response")
	}
}

func TestBuildOperation(t *testing.T) {
	gen := NewGenerator()
	
	route := types.RouteInfo{
		Method:       "POST",
		Path:         "/test",
		RequestType:  reflect.TypeOf(TestRequest{}),
		ResponseType: reflect.TypeOf(TestResponse{}),
		Module:       "test",
		Summary:      "Test endpoint for creating stuff",
	}
	
	// Generate schemas
	reqSchema, err := gen.generateTypeSchema(route.RequestType)
	if err != nil {
		t.Fatalf("generateTypeSchema() error = %v", err)
	}
	gen.typeSchemas[gen.getTypeName(route.RequestType)] = reqSchema
	
	respSchema, err := gen.generateTypeSchema(route.ResponseType)
	if err != nil {
		t.Fatalf("generateTypeSchema() error = %v", err)
	}
	gen.typeSchemas[gen.getTypeName(route.ResponseType)] = respSchema
	
	operation := gen.buildOperation(route)
	
	if operation == nil {
		t.Fatal("buildOperation() should not return nil")
	}
	
	// Check tags
	if len(operation.Tags) != 1 || operation.Tags[0] != "test" {
		t.Errorf("Expected tags [test], got %v", operation.Tags)
	}
	
	// Check summary
	if operation.Summary != route.Summary {
		t.Errorf("Expected summary '%s', got '%s'", route.Summary, operation.Summary)
	}
	
	// Check operation ID
	if operation.OperationID == "" {
		t.Error("Operation should have an operation ID")
	}
	
	// Check request body (should exist for POST)
	if operation.RequestBody == nil {
		t.Error("POST operation should have request body")
	}
	
	// Check responses
	if operation.Responses == nil || len(operation.Responses) == 0 {
		t.Error("Operation should have responses")
	}
}

func TestBuildOperation_GET_NoRequestBody(t *testing.T) {
	gen := NewGenerator()
	
	route := types.RouteInfo{
		Method:       "GET",
		Path:         "/test",
		RequestType:  nil,
		ResponseType: reflect.TypeOf(TestResponse{}),
		Module:       "test",
		Summary:      "Get test data",
	}
	
	operation := gen.buildOperation(route)
	
	// GET operations should not have request body
	if operation.RequestBody != nil {
		t.Error("GET operation should not have request body")
	}
}

func TestBuildPaths(t *testing.T) {
	gen := NewGenerator()
	
	gen.routes = []types.RouteInfo{
		{
			Method:       "GET",
			Path:         "/users",
			RequestType:  nil,
			ResponseType: reflect.TypeOf([]TestResponse{}),
			Module:       "users",
			Summary:      "List users",
		},
		{
			Method:       "POST",
			Path:         "/users",
			RequestType:  reflect.TypeOf(TestRequest{}),
			ResponseType: reflect.TypeOf(TestResponse{}),
			Module:       "users",
			Summary:      "Create user",
		},
		{
			Method:       "GET",
			Path:         "/health",
			RequestType:  nil,
			ResponseType: reflect.TypeOf(map[string]interface{}{}),
			Module:       "health",
			Summary:      "Health check",
		},
	}
	
	paths := gen.buildPaths()
	
	if len(paths) != 2 {
		t.Errorf("Expected 2 paths, got %d", len(paths))
	}
	
	// Check /users path has both GET and POST
	usersPath, exists := paths["/users"]
	if !exists {
		t.Fatal("Path '/users' should exist")
	}
	
	if usersPath.Get == nil {
		t.Error("'/users' path should have GET operation")
	}
	
	if usersPath.Post == nil {
		t.Error("'/users' path should have POST operation")
	}
	
	// Check /health path has only GET
	healthPath, exists := paths["/health"]
	if !exists {
		t.Fatal("Path '/health' should exist")
	}
	
	if healthPath.Get == nil {
		t.Error("'/health' path should have GET operation")
	}
	
	if healthPath.Post != nil {
		t.Error("'/health' path should not have POST operation")
	}
}

func TestSpecGenerationHeader(t *testing.T) {
	gen := NewGenerator()
	
	// Mock minimal routes to avoid "no routes" error
	gen.routes = []types.RouteInfo{
		{
			Method:       "GET",
			Path:         "/test",
			RequestType:  nil,
			ResponseType: reflect.TypeOf(map[string]interface{}{}),
			Module:       "test",
			Summary:      "Test",
		},
	}
	
	// Generate minimal schemas
	gen.typeSchemas["map[string]interface {}"] = map[string]interface{}{
		"type": "object",
		"additionalProperties": true,
	}
	gen.addStandardSchemas()
	
	spec := gen.buildOpenAPISpec()
	
	// Check that spec starts with generation header
	if !strings.HasPrefix(spec, "# Auto-generated OpenAPI specification") {
		t.Error("Spec should start with auto-generation header")
	}
	
	if !strings.Contains(spec, "# DO NOT EDIT MANUALLY") {
		t.Error("Spec should contain manual edit warning")
	}
	
	if !strings.Contains(spec, "# Auto-generated OpenAPI specification") {
		t.Error("Spec should contain auto-generated header")
	}
}