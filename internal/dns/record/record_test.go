package record

import (
	"testing"
	"time"

	"github.com/jerkytreats/dns/internal/proxy"
	"github.com/stretchr/testify/assert"
)

func TestNewRecord(t *testing.T) {
	// Arrange
	name := "testrecord"
	recordType := "A"
	ip := "100.64.0.1"

	// Act
	record := NewRecord(name, recordType, ip)

	// Assert
	assert.NotNil(t, record)
	assert.Equal(t, name, record.Name)
	assert.Equal(t, recordType, record.Type)
	assert.Equal(t, ip, record.IP)
	assert.Nil(t, record.ProxyRule)
	assert.False(t, record.CreatedAt.IsZero())
	assert.False(t, record.UpdatedAt.IsZero())
	assert.WithinDuration(t, time.Now(), record.CreatedAt, 2*time.Second)
	assert.WithinDuration(t, time.Now(), record.UpdatedAt, 2*time.Second)
}

func TestAddProxyRule(t *testing.T) {
	// Arrange
	record := NewRecord("testrecord", "A", "100.64.0.1")
	targetIP := "100.64.0.2"
	targetPort := 8080
	protocol := "http"
	hostname := "testrecord.internal.example.com"

	// Act
	record.AddProxyRule(targetIP, targetPort, protocol, hostname)

	// Assert
	assert.NotNil(t, record.ProxyRule)
	assert.Equal(t, targetIP, record.ProxyRule.TargetIP)
	assert.Equal(t, targetPort, record.ProxyRule.TargetPort)
	assert.Equal(t, protocol, record.ProxyRule.Protocol)
	assert.Equal(t, hostname, record.ProxyRule.Hostname)
	assert.True(t, record.ProxyRule.Enabled)
	assert.False(t, record.ProxyRule.CreatedAt.IsZero())
	assert.WithinDuration(t, time.Now(), record.UpdatedAt, 2*time.Second)
}

func TestHasProxyRule(t *testing.T) {
	// Test with no proxy rule
	record := NewRecord("testrecord", "A", "100.64.0.1")
	assert.False(t, record.HasProxyRule())

	// Test with proxy rule
	record.AddProxyRule("100.64.0.2", 8080, "http", "testrecord.internal.example.com")
	assert.True(t, record.HasProxyRule())
}

func TestIsProxyEnabled(t *testing.T) {
	// Test with no proxy rule
	record := NewRecord("testrecord", "A", "100.64.0.1")
	assert.False(t, record.IsProxyEnabled())

	// Test with enabled proxy rule
	record.AddProxyRule("100.64.0.2", 8080, "http", "testrecord.internal.example.com")
	assert.True(t, record.IsProxyEnabled())

	// Test with disabled proxy rule
	record.ProxyRule.Enabled = false
	assert.False(t, record.IsProxyEnabled())
}

func TestUpdate(t *testing.T) {
	// Arrange
	record := NewRecord("testrecord", "A", "100.64.0.1")
	originalUpdatedAt := record.UpdatedAt

	// Wait a small amount of time to ensure timestamp changes
	time.Sleep(10 * time.Millisecond)

	// Act
	record.Update()

	// Assert
	assert.True(t, record.UpdatedAt.After(originalUpdatedAt))
	assert.WithinDuration(t, time.Now(), record.UpdatedAt, 2*time.Second)
}

func TestToProxyRule(t *testing.T) {
	t.Run("with proxy rule", func(t *testing.T) {
		// Arrange
		record := NewRecord("testrecord", "A", "100.64.0.1")
		record.AddProxyRule("100.64.0.2", 8080, "http", "testrecord.internal.example.com")

		// Act
		proxyRule, err := record.ToProxyRule()

		// Assert
		assert.NoError(t, err)
		assert.NotNil(t, proxyRule)
		assert.Equal(t, record.ProxyRule.Hostname, proxyRule.Hostname)
		assert.Equal(t, record.ProxyRule.TargetIP, proxyRule.TargetIP)
		assert.Equal(t, record.ProxyRule.TargetPort, proxyRule.TargetPort)
		assert.Equal(t, record.ProxyRule.Protocol, proxyRule.Protocol)
		assert.Equal(t, record.ProxyRule.Enabled, proxyRule.Enabled)
		assert.Equal(t, record.ProxyRule.CreatedAt, proxyRule.CreatedAt)
	})

	t.Run("without proxy rule", func(t *testing.T) {
		// Arrange
		record := NewRecord("testrecord", "A", "100.64.0.1")

		// Act
		proxyRule, err := record.ToProxyRule()

		// Assert
		assert.Error(t, err)
		assert.Nil(t, proxyRule)
		assert.Contains(t, err.Error(), "record has no proxy rule")
	})
}

func TestFromProxyRule(t *testing.T) {
	t.Run("with valid proxy rule", func(t *testing.T) {
		// Arrange
		createdAt := time.Now().Add(-1 * time.Hour)
		proxyRule := &proxy.ProxyRule{
			Hostname:   "testrecord.internal.example.com",
			TargetIP:   "100.64.0.2",
			TargetPort: 8080,
			Protocol:   "http",
			Enabled:    true,
			CreatedAt:  createdAt,
		}

		// Act
		result := FromProxyRule(proxyRule)

		// Assert
		assert.NotNil(t, result)
		assert.Equal(t, proxyRule.Hostname, result.Hostname)
		assert.Equal(t, proxyRule.TargetIP, result.TargetIP)
		assert.Equal(t, proxyRule.TargetPort, result.TargetPort)
		assert.Equal(t, proxyRule.Protocol, result.Protocol)
		assert.Equal(t, proxyRule.Enabled, result.Enabled)
		assert.Equal(t, proxyRule.CreatedAt, result.CreatedAt)
	})

	t.Run("with nil proxy rule", func(t *testing.T) {
		// Act
		result := FromProxyRule(nil)

		// Assert
		assert.Nil(t, result)
	})
}
