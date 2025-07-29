package analyzer

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jerkytreats/dns/internal/api/types"
)

// Test types for schema generation
type TestRequest struct {
	Name        string `json:"name"`
	Count       int    `json:"count"`
	Enabled     bool   `json:"enabled"`
	OptionalVal *string `json:"optional_val,omitempty"`
}

type TestResponse struct {
	ID        int       `json:"id"`
	Message   string    `json:"message"`
	Timestamp time.Time `json:"timestamp"`
	Data      []string  `json:"data"`
}

type NestedStruct struct {
	Inner InnerStruct `json:"inner"`
	Items []string    `json:"items"`
}

type InnerStruct struct {
	Value string `json:"value"`
	Meta  map[string]interface{} `json:"meta"`
}

func TestNewGenerator(t *testing.T) {
	gen := NewGenerator()
	
	if gen == nil {
		t.Fatal("NewGenerator() returned nil")
	}
	
	if gen.fileSet == nil {
		t.Error("Generator fileSet should not be nil")
	}
	
	if gen.typeSchemas == nil {
		t.Error("Generator typeSchemas should not be nil")
	}
	
	if len(gen.routes) != 0 {
		t.Error("Generator routes should be empty initially")
	}
}

func TestGenerateTypeSchema_BasicTypes(t *testing.T) {
	gen := NewGenerator()
	
	tests := []struct {
		name         string
		inputType    reflect.Type
		expectedType string
	}{
		{"string", reflect.TypeOf(""), "string"},
		{"int", reflect.TypeOf(0), "integer"},
		{"int64", reflect.TypeOf(int64(0)), "integer"},
		{"float64", reflect.TypeOf(0.0), "number"},
		{"bool", reflect.TypeOf(true), "boolean"},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			schema, err := gen.generateTypeSchema(tt.inputType)
			if err != nil {
				t.Fatalf("generateTypeSchema() error = %v", err)
			}
			
			if schema["type"] != tt.expectedType {
				t.Errorf("Expected type %s, got %v", tt.expectedType, schema["type"])
			}
		})
	}
}

func TestGenerateTypeSchema_TimeType(t *testing.T) {
	gen := NewGenerator()
	
	schema, err := gen.generateTypeSchema(reflect.TypeOf(time.Time{}))
	if err != nil {
		t.Fatalf("generateTypeSchema() error = %v", err)
	}
	
	if schema["type"] != "string" {
		t.Errorf("Expected time.Time to have type 'string', got %v", schema["type"])
	}
	
	if schema["format"] != "date-time" {
		t.Errorf("Expected time.Time to have format 'date-time', got %v", schema["format"])
	}
}

func TestGenerateTypeSchema_Struct(t *testing.T) {
	gen := NewGenerator()
	
	schema, err := gen.generateTypeSchema(reflect.TypeOf(TestRequest{}))
	if err != nil {
		t.Fatalf("generateTypeSchema() error = %v", err)
	}
	
	if schema["type"] != "object" {
		t.Errorf("Expected struct to have type 'object', got %v", schema["type"])
	}
	
	properties, ok := schema["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected properties to be a map")
	}
	
	// Check required fields
	required, ok := schema["required"].([]string)
	if !ok {
		t.Fatal("Expected required to be a string slice")
	}
	
	expectedRequired := []string{"name", "count", "enabled"}
	if len(required) != len(expectedRequired) {
		t.Errorf("Expected %d required fields, got %d", len(expectedRequired), len(required))
	}
	
	// Check field types
	nameField, ok := properties["name"].(map[string]interface{})
	if !ok || nameField["type"] != "string" {
		t.Error("Expected 'name' field to be string type")
	}
	
	countField, ok := properties["count"].(map[string]interface{})
	if !ok || countField["type"] != "integer" {
		t.Error("Expected 'count' field to be integer type")
	}
	
	enabledField, ok := properties["enabled"].(map[string]interface{})
	if !ok || enabledField["type"] != "boolean" {
		t.Error("Expected 'enabled' field to be boolean type")
	}
}

func TestGenerateTypeSchema_Array(t *testing.T) {
	gen := NewGenerator()
	
	schema, err := gen.generateTypeSchema(reflect.TypeOf([]string{}))
	if err != nil {
		t.Fatalf("generateTypeSchema() error = %v", err)
	}
	
	if schema["type"] != "array" {
		t.Errorf("Expected array to have type 'array', got %v", schema["type"])
	}
	
	items, ok := schema["items"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected items to be a map")
	}
	
	if items["type"] != "string" {
		t.Errorf("Expected array items to be string type, got %v", items["type"])
	}
}

func TestGenerateTypeSchema_Map(t *testing.T) {
	gen := NewGenerator()
	
	schema, err := gen.generateTypeSchema(reflect.TypeOf(map[string]interface{}{}))
	if err != nil {
		t.Fatalf("generateTypeSchema() error = %v", err)
	}
	
	if schema["type"] != "object" {
		t.Errorf("Expected map to have type 'object', got %v", schema["type"])
	}
	
	if schema["additionalProperties"] != true {
		t.Error("Expected map to have additionalProperties: true")
	}
}

func TestGenerateTypeSchema_Pointer(t *testing.T) {
	gen := NewGenerator()
	
	// Test pointer to string
	schema, err := gen.generateTypeSchema(reflect.TypeOf((*string)(nil)))
	if err != nil {
		t.Fatalf("generateTypeSchema() error = %v", err)
	}
	
	if schema["type"] != "string" {
		t.Errorf("Expected pointer to string to have type 'string', got %v", schema["type"])
	}
}

func TestGetTypeName(t *testing.T) {
	gen := NewGenerator()
	
	tests := []struct {
		name     string
		input    reflect.Type
		expected string
	}{
		{"simple struct", reflect.TypeOf(TestRequest{}), "TestRequest"},
		{"string slice", reflect.TypeOf([]string{}), "stringArray"},
		{"struct slice", reflect.TypeOf([]TestResponse{}), "TestResponseArray"},
		{"basic type", reflect.TypeOf(""), "string"},
		{"nested type", reflect.TypeOf(NestedStruct{}), "NestedStruct"},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := gen.getTypeName(tt.input)
			if result != tt.expected {
				t.Errorf("getTypeName() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

func TestAddStandardSchemas(t *testing.T) {
	gen := NewGenerator()
	
	gen.addStandardSchemas()
	
	errorResponse, exists := gen.typeSchemas["ErrorResponse"]
	if !exists {
		t.Fatal("ErrorResponse schema should be added")
	}
	
	errorMap, ok := errorResponse.(map[string]interface{})
	if !ok {
		t.Fatal("ErrorResponse should be a map")
	}
	
	if errorMap["type"] != "object" {
		t.Error("ErrorResponse should be an object type")
	}
	
	properties, ok := errorMap["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("ErrorResponse should have properties")
	}
	
	expectedFields := []string{"error", "message", "status"}
	for _, field := range expectedFields {
		if _, exists := properties[field]; !exists {
			t.Errorf("ErrorResponse should have field '%s'", field)
		}
	}
}

func TestGenerateSchemas(t *testing.T) {
	gen := NewGenerator()
	
	// Mock some routes with different types
	gen.routes = []types.RouteInfo{
		{
			Method:       "POST",
			Path:         "/test",
			RequestType:  reflect.TypeOf(TestRequest{}),
			ResponseType: reflect.TypeOf(TestResponse{}),
			Module:       "test",
		},
		{
			Method:       "GET",
			Path:         "/nested",
			RequestType:  nil,
			ResponseType: reflect.TypeOf(NestedStruct{}),
			Module:       "test",
		},
	}
	
	err := gen.generateSchemas()
	if err != nil {
		t.Fatalf("generateSchemas() error = %v", err)
	}
	
	// Check that schemas were generated for all types
	expectedSchemas := []string{"TestRequest", "TestResponse", "NestedStruct"}
	for _, schemaName := range expectedSchemas {
		if _, exists := gen.typeSchemas[schemaName]; !exists {
			t.Errorf("Schema '%s' should have been generated", schemaName)
		}
	}
}

func TestGetDiscoveredRoutes(t *testing.T) {
	gen := NewGenerator()
	
	// Initially should be empty
	routes := gen.GetDiscoveredRoutes()
	if len(routes) != 0 {
		t.Error("GetDiscoveredRoutes() should return empty slice initially")
	}
	
	// Add some routes
	testRoutes := []types.RouteInfo{
		{Method: "GET", Path: "/test1", Module: "test"},
		{Method: "POST", Path: "/test2", Module: "test"},
	}
	
	gen.routes = testRoutes
	
	discoveredRoutes := gen.GetDiscoveredRoutes()
	if len(discoveredRoutes) != 2 {
		t.Errorf("Expected 2 discovered routes, got %d", len(discoveredRoutes))
	}
	
	// Verify the routes match
	for i, route := range discoveredRoutes {
		if route.Method != testRoutes[i].Method {
			t.Errorf("Route %d method mismatch: expected %s, got %s", i, testRoutes[i].Method, route.Method)
		}
		if route.Path != testRoutes[i].Path {
			t.Errorf("Route %d path mismatch: expected %s, got %s", i, testRoutes[i].Path, route.Path)
		}
	}
}

func TestGenerateSpec_EmptyRegistry(t *testing.T) {
	gen := NewGenerator()
	
	// Clear the global registry to simulate no routes
	types.ClearRegistry()
	
	_, err := gen.GenerateSpec()
	if err == nil {
		t.Error("GenerateSpec() should return error when no routes are discovered")
	}
	
	if !strings.Contains(err.Error(), "no routes discovered") {
		t.Errorf("Error should mention no routes discovered, got: %v", err)
	}
}

func TestCircularReferenceHandling(t *testing.T) {
	gen := NewGenerator()
	
	// Create a type that references itself (circular reference)
	type CircularStruct struct {
		Name string         `json:"name"`
		Self *CircularStruct `json:"self,omitempty"`
	}
	
	schema, err := gen.generateTypeSchema(reflect.TypeOf(CircularStruct{}))
	if err != nil {
		t.Fatalf("generateTypeSchema() should handle circular references, error = %v", err)
	}
	
	if schema["type"] != "object" {
		t.Error("Circular struct should still be an object type")
	}
	
	// The function should complete without infinite recursion
	properties, ok := schema["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected properties to be a map")
	}
	
	if _, exists := properties["name"]; !exists {
		t.Error("Should still have 'name' property")
	}
	
	if _, exists := properties["self"]; !exists {
		t.Error("Should still have 'self' property")
	}
}

func TestJSONTagParsing(t *testing.T) {
	gen := NewGenerator()
	
	type TaggedStruct struct {
		IncludedField    string  `json:"included_field"`
		OmitEmptyField   *string `json:"omit_empty,omitempty"`
		ExcludedField    string  `json:"-"`
		RequiredField    string  `json:"required_field"`
		NoTagField       string  // No json tag
	}
	
	schema, err := gen.generateTypeSchema(reflect.TypeOf(TaggedStruct{}))
	if err != nil {
		t.Fatalf("generateTypeSchema() error = %v", err)
	}
	
	properties, ok := schema["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected properties to be a map")
	}
	
	// Check that excluded field is not present
	if _, exists := properties["ExcludedField"]; exists {
		t.Error("Field with json:\"-\" should be excluded")
	}
	
	// Check that included fields are present with correct names
	if _, exists := properties["included_field"]; !exists {
		t.Error("Field with json tag should be present with tag name")
	}
	
	if _, exists := properties["omit_empty"]; !exists {
		t.Error("Field with omitempty should still be present")
	}
	
	// Check required fields
	required, ok := schema["required"].([]string)
	if !ok {
		t.Fatal("Expected required to be a string slice")
	}
	
	// Fields without omitempty should be required
	requiredFields := make(map[string]bool)
	for _, field := range required {
		requiredFields[field] = true
	}
	
	if !requiredFields["included_field"] {
		t.Error("Field without omitempty should be required")
	}
	
	if !requiredFields["required_field"] {
		t.Error("Field without omitempty should be required")
	}
	
	if requiredFields["omit_empty"] {
		t.Error("Field with omitempty should not be required")
	}
}