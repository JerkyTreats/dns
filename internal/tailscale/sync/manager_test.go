// Package sync provides dynamic zone synchronization using Tailscale device discovery.
package sync

import (
	"errors"
	"testing"

	"github.com/jerkytreats/dns/internal/config"
	"github.com/jerkytreats/dns/internal/tailscale"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// MockCorednsManager is a mock type for the coredns.Manager type
type MockCorednsManager struct {
	mock.Mock
}

func (m *MockCorednsManager) AddZone(serviceName string) error {
	args := m.Called(serviceName)
	return args.Error(0)
}

func (m *MockCorednsManager) AddRecord(serviceName, name, ip string) error {
	args := m.Called(serviceName, name, ip)
	return args.Error(0)
}

func (m *MockCorednsManager) DropRecord(serviceName, name, ip string) error {
	args := m.Called(serviceName, name, ip)
	return args.Error(0)
}

func (m *MockCorednsManager) Reload() error {
	args := m.Called()
	return args.Error(0)
}

// MockTailscaleClient is a mock type for the tailscale.Client type
type MockTailscaleClient struct {
	mock.Mock
}

func (m *MockTailscaleClient) ListDevices() ([]tailscale.Device, error) {
	args := m.Called()
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]tailscale.Device), args.Error(1)
}

func TestNewManager(t *testing.T) {
	// Because NewManager gets config globally, we need to set it for the test.
	config.ResetForTest()
	config.SetForTest("dns.internal.origin", "internal.jerkytreats.dev")

	mockCoredns := &MockCorednsManager{}
	mockTailscale := &MockTailscaleClient{}

	manager, err := NewManager(mockCoredns, mockTailscale)
	require.NoError(t, err)
	require.NotNil(t, manager)

	assert.Equal(t, mockCoredns, manager.corednsManager)
	assert.Equal(t, mockTailscale, manager.tailscaleClient)
	assert.NotNil(t, manager.ipCache)
}

func TestSyncDevices(t *testing.T) {
	config.ResetForTest()
	config.SetForTest("dns.internal.origin", "internal.jerkytreats.dev")

	t.Run("successful sync with new and updated devices", func(t *testing.T) {
		mockCoredns := &MockCorednsManager{}
		mockTailscale := &MockTailscaleClient{}

		manager, _ := NewManager(mockCoredns, mockTailscale)
		require.NotNil(t, manager)

		// Pre-cache an existing device to test the update path
		manager.cacheIP("device-b", "100.0.0.2-old")

		devices := []tailscale.Device{
			{Hostname: "device-a", Addresses: []string{"100.0.0.1", "fe80::1"}}, // New device
			{Hostname: "device-b", Addresses: []string{"100.0.0.2"}},            // IP updated
			{Hostname: "device-c", Addresses: []string{}},                       // No IP, should be skipped
			{Hostname: "device-d", Addresses: []string{"192.168.1.10"}},         // No tailscale IP, should be skipped
			{Hostname: "device-e", Addresses: []string{"100.0.0.5"}},            // Add should fail
		}

		mockTailscale.On("ListDevices").Return(devices, nil).Once()
		mockCoredns.On("AddRecord", "internal", "device-a", "100.0.0.1").Return(nil).Once()
		mockCoredns.On("DropRecord", "internal", "device-b", "100.0.0.2-old").Return(nil).Once()
		mockCoredns.On("AddRecord", "internal", "device-b", "100.0.0.2").Return(nil).Once()
		mockCoredns.On("AddRecord", "internal", "device-e", "100.0.0.5").Return(errors.New("add failed")).Once()
		mockCoredns.On("Reload").Return(nil).Once()

		result, err := manager.syncDevices("internal")
		require.NoError(t, err)

		assert.False(t, result.Success) // Success should be false because one device failed
		assert.Equal(t, 5, result.TotalDevices)
		assert.Equal(t, 2, result.ResolvedDevices) // a, b
		assert.Equal(t, 2, result.SkippedDevices)  // c, d
		assert.Equal(t, 1, result.FailedDevices)   // e

		mockTailscale.AssertExpectations(t)
		mockCoredns.AssertExpectations(t)
	})
}
