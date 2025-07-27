package record

import (
	"github.com/jerkytreats/dns/internal/dns/coredns"
	"github.com/jerkytreats/dns/internal/proxy"
	"github.com/jerkytreats/dns/internal/tailscale"
	"github.com/stretchr/testify/mock"
)

// MockDNSManager is a mock implementation of DNSManagerInterface
type MockDNSManager struct {
	mock.Mock
}

func (m *MockDNSManager) AddRecord(serviceName, name, ip string) error {
	args := m.Called(serviceName, name, ip)
	return args.Error(0)
}

func (m *MockDNSManager) RemoveRecord(serviceName, name string) error {
	args := m.Called(serviceName, name)
	return args.Error(0)
}

func (m *MockDNSManager) ListRecords(serviceName string) ([]coredns.Record, error) {
	args := m.Called(serviceName)
	return args.Get(0).([]coredns.Record), args.Error(1)
}

// MockProxyManager is a mock implementation of ProxyManagerInterface
type MockProxyManager struct {
	mock.Mock
}

func (m *MockProxyManager) AddRule(rule *proxy.ProxyRule) error {
	args := m.Called(rule)
	return args.Error(0)
}

func (m *MockProxyManager) RemoveRule(hostname string) error {
	args := m.Called(hostname)
	return args.Error(0)
}

func (m *MockProxyManager) ListRules() []*proxy.ProxyRule {
	args := m.Called()
	return args.Get(0).([]*proxy.ProxyRule)
}

func (m *MockProxyManager) IsEnabled() bool {
	args := m.Called()
	return args.Bool(0)
}

// MockTailscaleClient is a mock implementation of TailscaleClientInterface
type MockTailscaleClient struct {
	mock.Mock
}

func (m *MockTailscaleClient) GetCurrentDeviceIPByName(deviceName string) (string, error) {
	args := m.Called(deviceName)
	return args.String(0), args.Error(1)
}

func (m *MockTailscaleClient) GetCurrentDeviceIP() (string, error) {
	args := m.Called()
	return args.String(0), args.Error(1)
}

func (m *MockTailscaleClient) GetTailscaleIPFromSourceIP(sourceIP string) (string, error) {
	args := m.Called(sourceIP)
	return args.String(0), args.Error(1)
}

func (m *MockTailscaleClient) ListDevices() ([]tailscale.Device, error) {
	args := m.Called()
	return args.Get(0).([]tailscale.Device), args.Error(1)
}

// MockGenerator is a mock implementation of Generator
type MockGenerator struct {
	mock.Mock
}

func (m *MockGenerator) GenerateRecords() ([]Record, error) {
	args := m.Called()
	return args.Get(0).([]Record), args.Error(1)
}

// MockValidator is a mock implementation of Validator
type MockValidator struct {
	mock.Mock
}

func (m *MockValidator) ValidateCreateRequest(req *CreateRecordRequest) error {
	args := m.Called(req)
	return args.Error(0)
}
