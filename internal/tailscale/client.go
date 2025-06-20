// Package tailscale provides integration with the Tailscale API for device discovery and IP resolution.
package tailscale

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/jerkytreats/dns/internal/logging"
)

const (
	// Default Tailscale API base URL
	DefaultAPIBaseURL = "https://api.tailscale.com"

	// API endpoints
	devicesEndpoint = "/api/v2/tailnet/%s/devices"

	// HTTP client timeout
	defaultTimeout = 30 * time.Second
)

// Client represents a Tailscale API client
type Client struct {
	apiKey  string
	tailnet string
	baseURL string
	client  *http.Client
}

// Device represents a Tailscale device from the API
type Device struct {
	Name      string   `json:"name"`
	Hostname  string   `json:"hostname"`
	Addresses []string `json:"addresses"`
	Online    bool     `json:"online"`
	ID        string   `json:"id"`
}

// DevicesResponse represents the API response for listing devices
type DevicesResponse struct {
	Devices []Device `json:"devices"`
}

// NewClient creates a new Tailscale API client
func NewClient(apiKey, tailnet string) *Client {
	return NewClientWithBaseURL(apiKey, tailnet, DefaultAPIBaseURL)
}

// NewClientWithBaseURL creates a new Tailscale API client with a custom base URL (for testing)
func NewClientWithBaseURL(apiKey, tailnet, baseURL string) *Client {
	return &Client{
		apiKey:  apiKey,
		tailnet: tailnet,
		baseURL: baseURL,
		client: &http.Client{
			Timeout: defaultTimeout,
		},
	}
}

// ListDevices retrieves all devices from the Tailscale tailnet
func (c *Client) ListDevices() ([]Device, error) {
	url := fmt.Sprintf("%s%s", c.baseURL, fmt.Sprintf(devicesEndpoint, c.tailnet))

	logging.Debug("Fetching devices from Tailscale API: %s", url)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Accept", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API request failed with status %d", resp.StatusCode)
	}

	var devicesResp DevicesResponse
	if err := json.NewDecoder(resp.Body).Decode(&devicesResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	logging.Debug("Retrieved %d devices from Tailscale", len(devicesResp.Devices))
	return devicesResp.Devices, nil
}

// GetDevice retrieves a specific device by name or hostname
func (c *Client) GetDevice(nameOrHostname string) (*Device, error) {
	devices, err := c.ListDevices()
	if err != nil {
		return nil, fmt.Errorf("failed to list devices: %w", err)
	}

	// Try exact match first
	for _, device := range devices {
		if device.Name == nameOrHostname || device.Hostname == nameOrHostname {
			return &device, nil
		}
	}

	// Try hostname without domain suffix
	for _, device := range devices {
		hostname := strings.Split(device.Hostname, ".")[0]
		if hostname == nameOrHostname {
			return &device, nil
		}
	}

	return nil, fmt.Errorf("device '%s' not found", nameOrHostname)
}

// GetDeviceIP retrieves the Tailscale IP address for a device
func (c *Client) GetDeviceIP(deviceName string) (string, error) {
	device, err := c.GetDevice(deviceName)
	if err != nil {
		return "", err
	}

	if !device.Online {
		return "", fmt.Errorf("device '%s' is offline", deviceName)
	}

	// Find the Tailscale IP (typically 100.x.x.x range)
	for _, addr := range device.Addresses {
		if strings.HasPrefix(addr, "100.") {
			logging.Debug("Resolved device '%s' to IP %s", deviceName, addr)
			return addr, nil
		}
	}

	return "", fmt.Errorf("no Tailscale IP found for device '%s'", deviceName)
}

// IsDeviceOnline checks if a device is currently online
func (c *Client) IsDeviceOnline(deviceName string) (bool, error) {
	device, err := c.GetDevice(deviceName)
	if err != nil {
		return false, err
	}

	return device.Online, nil
}

// ValidateConnection tests the connection to the Tailscale API
func (c *Client) ValidateConnection() error {
	_, err := c.ListDevices()
	if err != nil {
		return fmt.Errorf("failed to connect to Tailscale API: %w", err)
	}
	return nil
}
