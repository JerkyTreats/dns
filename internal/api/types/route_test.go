package types

import (
	"fmt"
	"net/http"
	"reflect"
	"sync"
	"testing"
)

// Mock handler for testing
func mockHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

// Mock struct for testing reflection
type MockRequest struct {
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
}

type MockResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

func TestRegisterRoute(t *testing.T) {
	// Clear registry before test
	ClearRegistry()
	
	route := RouteInfo{
		Method:       "GET",
		Path:         "/test",
		Handler:      mockHandler,
		RequestType:  nil,
		ResponseType: reflect.TypeOf(MockResponse{}),
		Module:       "test",
		Summary:      "Test endpoint",
	}

	RegisterRoute(route)

	routes := GetRegisteredRoutes()
	if len(routes) != 1 {
		t.Errorf("Expected 1 route, got %d", len(routes))
	}

	if routes[0].Path != "/test" {
		t.Errorf("Expected path '/test', got '%s'", routes[0].Path)
	}

	if routes[0].Method != "GET" {
		t.Errorf("Expected method 'GET', got '%s'", routes[0].Method)
	}

	if routes[0].Module != "test" {
		t.Errorf("Expected module 'test', got '%s'", routes[0].Module)
	}
}

func TestRegisterMultipleRoutes(t *testing.T) {
	// Clear registry before test
	ClearRegistry()

	routes := []RouteInfo{
		{
			Method:       "GET",
			Path:         "/users",
			Handler:      mockHandler,
			RequestType:  nil,
			ResponseType: reflect.TypeOf([]MockResponse{}),
			Module:       "users",
			Summary:      "List users",
		},
		{
			Method:       "POST",
			Path:         "/users",
			Handler:      mockHandler,
			RequestType:  reflect.TypeOf(MockRequest{}),
			ResponseType: reflect.TypeOf(MockResponse{}),
			Module:       "users",
			Summary:      "Create user",
		},
		{
			Method:       "GET",
			Path:         "/health",
			Handler:      mockHandler,
			RequestType:  nil,
			ResponseType: reflect.TypeOf(MockResponse{}),
			Module:       "health",
			Summary:      "Health check",
		},
	}

	for _, route := range routes {
		RegisterRoute(route)
	}

	registeredRoutes := GetRegisteredRoutes()
	if len(registeredRoutes) != 3 {
		t.Errorf("Expected 3 routes, got %d", len(registeredRoutes))
	}

	// Verify routes are registered correctly
	pathsFound := make(map[string]bool)
	for _, route := range registeredRoutes {
		pathsFound[route.Path] = true
	}

	expectedPaths := []string{"/users", "/health"}
	for _, path := range expectedPaths {
		if !pathsFound[path] {
			t.Errorf("Expected path '%s' not found in registered routes", path)
		}
	}
}

func TestGetRegisteredRoutes_ReturnsCopy(t *testing.T) {
	// Clear registry before test
	ClearRegistry()

	originalRoute := RouteInfo{
		Method:  "GET",
		Path:    "/original",
		Handler: mockHandler,
		Module:  "test",
		Summary: "Original route",
	}

	RegisterRoute(originalRoute)

	// Get routes and modify the returned slice
	routes := GetRegisteredRoutes()
	if len(routes) != 1 {
		t.Errorf("Expected 1 route, got %d", len(routes))
	}

	// Modify the returned slice
	routes[0].Path = "/modified"

	// Get routes again and verify original wasn't changed
	routesAgain := GetRegisteredRoutes()
	if routesAgain[0].Path != "/original" {
		t.Errorf("Expected original path '/original', got '%s'. Registry was not properly protected from modification", routesAgain[0].Path)
	}
}

func TestUpdateRouteRegistry(t *testing.T) {
	// Clear registry before test
	ClearRegistry()

	// Register initial route
	initialRoute := RouteInfo{
		Method:  "GET",
		Path:    "/initial",
		Handler: mockHandler,
		Module:  "test",
		Summary: "Initial route",
	}
	RegisterRoute(initialRoute)

	// Create new routes to replace the registry
	newRoutes := []RouteInfo{
		{
			Method:  "POST",
			Path:    "/new1",
			Handler: mockHandler,
			Module:  "test",
			Summary: "New route 1",
		},
		{
			Method:  "PUT",
			Path:    "/new2",
			Handler: mockHandler,
			Module:  "test",
			Summary: "New route 2",
		},
	}

	UpdateRouteRegistry(newRoutes)

	// Verify registry was completely replaced
	routes := GetRegisteredRoutes()
	if len(routes) != 2 {
		t.Errorf("Expected 2 routes after update, got %d", len(routes))
	}

	// Verify old route is gone
	for _, route := range routes {
		if route.Path == "/initial" {
			t.Error("Old route '/initial' should have been removed after registry update")
		}
	}

	// Verify new routes are present
	foundPaths := make(map[string]bool)
	for _, route := range routes {
		foundPaths[route.Path] = true
	}

	expectedNewPaths := []string{"/new1", "/new2"}
	for _, path := range expectedNewPaths {
		if !foundPaths[path] {
			t.Errorf("Expected new path '%s' not found after registry update", path)
		}
	}
}

func TestClearRegistry(t *testing.T) {
	// Register some routes
	routes := []RouteInfo{
		{Method: "GET", Path: "/test1", Handler: mockHandler, Module: "test"},
		{Method: "POST", Path: "/test2", Handler: mockHandler, Module: "test"},
	}

	for _, route := range routes {
		RegisterRoute(route)
	}

	// Verify routes are registered
	registeredRoutes := GetRegisteredRoutes()
	if len(registeredRoutes) == 0 {
		t.Error("Routes should be registered before clearing")
	}

	// Clear registry
	ClearRegistry()

	// Verify registry is empty
	clearedRoutes := GetRegisteredRoutes()
	if len(clearedRoutes) != 0 {
		t.Errorf("Expected 0 routes after clearing, got %d", len(clearedRoutes))
	}
}

func TestConcurrentAccess(t *testing.T) {
	// Clear registry before test
	ClearRegistry()

	const numGoroutines = 10
	const routesPerGoroutine = 5

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	// Concurrently register routes
	for i := 0; i < numGoroutines; i++ {
		go func(goroutineID int) {
			defer wg.Done()
			for j := 0; j < routesPerGoroutine; j++ {
				route := RouteInfo{
					Method:  "GET",
					Path:    fmt.Sprintf("/test-%d-%d", goroutineID, j),
					Handler: mockHandler,
					Module:  fmt.Sprintf("module-%d", goroutineID),
					Summary: fmt.Sprintf("Route %d-%d", goroutineID, j),
				}
				RegisterRoute(route)
			}
		}(i)
	}

	wg.Wait()

	// Verify all routes were registered
	routes := GetRegisteredRoutes()
	expectedCount := numGoroutines * routesPerGoroutine
	if len(routes) != expectedCount {
		t.Errorf("Expected %d routes after concurrent registration, got %d", expectedCount, len(routes))
	}

	// Verify no duplicate paths (would indicate race condition)
	pathCounts := make(map[string]int)
	for _, route := range routes {
		pathCounts[route.Path]++
	}

	for path, count := range pathCounts {
		if count != 1 {
			t.Errorf("Path '%s' appears %d times, expected 1 (possible race condition)", path, count)
		}
	}
}

func TestConcurrentReadWrite(t *testing.T) {
	// Clear registry before test
	ClearRegistry()

	const numReaders = 5
	const numWriters = 3
	const numOperations = 10

	var wg sync.WaitGroup
	wg.Add(numReaders + numWriters)

	// Start reader goroutines
	for i := 0; i < numReaders; i++ {
		go func(readerID int) {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				_ = GetRegisteredRoutes() // Just read, don't verify contents due to concurrent writes
			}
		}(i)
	}

	// Start writer goroutines
	for i := 0; i < numWriters; i++ {
		go func(writerID int) {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				route := RouteInfo{
					Method:  "GET",
					Path:    fmt.Sprintf("/writer-%d-%d", writerID, j),
					Handler: mockHandler,
					Module:  fmt.Sprintf("writer-%d", writerID),
					Summary: fmt.Sprintf("Writer %d route %d", writerID, j),
				}
				RegisterRoute(route)
			}
		}(i)
	}

	wg.Wait()

	// If we get here without a race condition, the test passes
	// Final verification that some routes were written
	routes := GetRegisteredRoutes()
	if len(routes) == 0 {
		t.Error("Expected some routes to be registered by writer goroutines")
	}
}

func TestRouteInfoFields(t *testing.T) {
	// Clear registry before test
	ClearRegistry()

	route := RouteInfo{
		Method:       "POST",
		Path:         "/complex",
		Handler:      mockHandler,
		RequestType:  reflect.TypeOf(MockRequest{}),
		ResponseType: reflect.TypeOf(MockResponse{}),
		Module:       "complex-module",
		Summary:      "A complex endpoint with request and response types",
	}

	RegisterRoute(route)

	routes := GetRegisteredRoutes()
	if len(routes) != 1 {
		t.Fatalf("Expected 1 route, got %d", len(routes))
	}

	registeredRoute := routes[0]

	// Verify all fields
	if registeredRoute.Method != "POST" {
		t.Errorf("Expected method 'POST', got '%s'", registeredRoute.Method)
	}

	if registeredRoute.Path != "/complex" {
		t.Errorf("Expected path '/complex', got '%s'", registeredRoute.Path)
	}

	if registeredRoute.Module != "complex-module" {
		t.Errorf("Expected module 'complex-module', got '%s'", registeredRoute.Module)
	}

	if registeredRoute.Summary != "A complex endpoint with request and response types" {
		t.Errorf("Expected specific summary, got '%s'", registeredRoute.Summary)
	}

	// Verify reflection types
	if registeredRoute.RequestType != reflect.TypeOf(MockRequest{}) {
		t.Errorf("RequestType mismatch. Expected %v, got %v", reflect.TypeOf(MockRequest{}), registeredRoute.RequestType)
	}

	if registeredRoute.ResponseType != reflect.TypeOf(MockResponse{}) {
		t.Errorf("ResponseType mismatch. Expected %v, got %v", reflect.TypeOf(MockResponse{}), registeredRoute.ResponseType)
	}

	// Verify handler is not nil (can't compare function pointers directly)
	if registeredRoute.Handler == nil {
		t.Error("Handler should not be nil")
	}
}

