// Package tailscale provides integration with the Tailscale API for device discovery and IP resolution.
package tailscale

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/jerkytreats/dns/internal/config"
	"github.com/jerkytreats/dns/internal/logging"
)

const (
	// Default Tailscale API base URL
	DefaultAPIBaseURL = "https://api.tailscale.com"

	// API endpoints
	devicesEndpoint = "/api/v2/tailnet/%s/devices"

	// HTTP client timeout
	defaultTimeout = 30 * time.Second

	TailscaleAPIKeyKey     = "tailscale.api_key"
	TailscaleTailnetKey    = "tailscale.tailnet"
	TailscaleDeviceNameKey = "tailscale.device_name"
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

func NewClient() (*Client, error) {
	apiKey := config.GetString(TailscaleAPIKeyKey)
	tailnet := config.GetString(TailscaleTailnetKey)

	if apiKey == "" || apiKey == "${TAILSCALE_API_KEY}" {
		errMsg := "tailscale.api_key is not configured or environment variable is not set"
		logging.Error(errMsg)
		return nil, fmt.Errorf("%s", errMsg)
	}
	if tailnet == "" || tailnet == "${TAILSCALE_TAILNET}" {
		errMsg := "tailscale.tailnet is not configured or environment variable is not set"
		logging.Error(errMsg)
		return nil, fmt.Errorf("%s", errMsg)
	}

	baseURL := config.GetString(config.TailscaleBaseURLKey)
	if baseURL == "" {
		baseURL = DefaultAPIBaseURL
	}

	client := &Client{
		apiKey:  apiKey,
		tailnet: tailnet,
		baseURL: baseURL,
		client: &http.Client{
			Timeout: defaultTimeout,
		},
	}

	if err := client.ValidateConnection(); err != nil {
		logging.Error("Bootstrap configuration validation failed: %v", err)
		return nil, err
	}

	return client, nil
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

// GetCurrentDeviceIPByName retrieves the Tailscale IP address for a specific device name
// This is useful when running in containers where hostname doesn't match Tailscale device name
func (c *Client) GetCurrentDeviceIPByName(deviceName string) (string, error) {
	devices, err := c.ListDevices()
	if err != nil {
		return "", fmt.Errorf("failed to list devices to find device '%s': %w", deviceName, err)
	}

	logging.Debug("Looking for device with name: %s", deviceName)

	// Try to find the device by matching name or hostname
	for _, device := range devices {
		// Try exact name match first
		if device.Name == deviceName {
			if !device.Online {
				return "", fmt.Errorf("device '%s' is offline", deviceName)
			}

			// Find the Tailscale IP (typically 100.x.x.x range)
			for _, addr := range device.Addresses {
				if strings.HasPrefix(addr, "100.") {
					logging.Info("Found device '%s' Tailscale IP: %s", deviceName, addr)
					return addr, nil
				}
			}
			return "", fmt.Errorf("no Tailscale IP found for device '%s'", deviceName)
		}

		// Try hostname match
		if device.Hostname == deviceName {
			if !device.Online {
				return "", fmt.Errorf("device '%s' is offline", deviceName)
			}

			// Find the Tailscale IP (typically 100.x.x.x range)
			for _, addr := range device.Addresses {
				if strings.HasPrefix(addr, "100.") {
					logging.Info("Found device '%s' Tailscale IP: %s (matched by hostname)", deviceName, addr)
					return addr, nil
				}
			}
			return "", fmt.Errorf("no Tailscale IP found for device '%s'", deviceName)
		}

		// Try hostname without domain suffix match
		deviceShortName := strings.Split(device.Hostname, ".")[0]
		if deviceShortName == deviceName {
			if !device.Online {
				return "", fmt.Errorf("device '%s' is offline", deviceName)
			}

			// Find the Tailscale IP (typically 100.x.x.x range)
			for _, addr := range device.Addresses {
				if strings.HasPrefix(addr, "100.") {
					logging.Info("Found device '%s' Tailscale IP: %s (matched by short hostname)", deviceName, addr)
					return addr, nil
				}
			}
			return "", fmt.Errorf("no Tailscale IP found for device '%s'", deviceName)
		}
	}

	// If device not found, list available devices for debugging
	var availableDevices []string
	for _, device := range devices {
		status := "offline"
		if device.Online {
			status = "online"
		}
		availableDevices = append(availableDevices, fmt.Sprintf("%s (%s, %s)", device.Name, device.Hostname, status))
	}

	return "", fmt.Errorf("device '%s' not found in Tailscale network. Available devices: %s",
		deviceName, strings.Join(availableDevices, ", "))
}

// GetCurrentDeviceIP retrieves the Tailscale IP address of the current device
// It identifies the current device by matching the system hostname
func (c *Client) GetCurrentDeviceIP() (string, error) {
	devices, err := c.ListDevices()
	if err != nil {
		return "", fmt.Errorf("failed to list devices to find current device: %w", err)
	}

	// Get the current system hostname
	hostname, err := os.Hostname()
	if err != nil {
		return "", fmt.Errorf("failed to get system hostname: %w", err)
	}

	logging.Debug("Looking for current device with hostname: %s", hostname)

	// Try to find the current device by matching hostname
	for _, device := range devices {
		// Try exact hostname match first
		if device.Hostname == hostname {
			if !device.Online {
				return "", fmt.Errorf("current device '%s' is offline", hostname)
			}

			// Find the Tailscale IP (typically 100.x.x.x range)
			for _, addr := range device.Addresses {
				if strings.HasPrefix(addr, "100.") {
					logging.Info("Found current device Tailscale IP: %s", addr)
					return addr, nil
				}
			}
			return "", fmt.Errorf("no Tailscale IP found for current device '%s'", hostname)
		}

		// Try hostname without domain suffix match
		deviceShortName := strings.Split(device.Hostname, ".")[0]
		systemShortName := strings.Split(hostname, ".")[0]
		if deviceShortName == systemShortName {
			if !device.Online {
				return "", fmt.Errorf("current device '%s' is offline", hostname)
			}

			// Find the Tailscale IP (typically 100.x.x.x range)
			for _, addr := range device.Addresses {
				if strings.HasPrefix(addr, "100.") {
					logging.Info("Found current device Tailscale IP: %s (matched by short hostname)", addr)
					return addr, nil
				}
			}
			return "", fmt.Errorf("no Tailscale IP found for current device '%s'", hostname)
		}
	}

	// If device not found, list available devices for debugging
	var availableDevices []string
	for _, device := range devices {
		status := "offline"
		if device.Online {
			status = "online"
		}
		availableDevices = append(availableDevices, fmt.Sprintf("%s (%s, %s)", device.Name, device.Hostname, status))
	}

	return "", fmt.Errorf("current device with hostname '%s' not found in Tailscale network. Available devices: %s",
		hostname, strings.Join(availableDevices, ", "))
}
