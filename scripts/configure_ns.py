#!/usr/bin/env python3
"""
Automatically configure Tailscale NS device.
This script detects the current Tailscale device and configures it in the config file.
Uses only built-in Python modules for maximum compatibility.
"""

import sys
import json
import socket
import subprocess
import re
import urllib.request
import urllib.parse
import urllib.error
from pathlib import Path


def log_info(message):
    """Print an info message."""
    print(f"\033[0;32m[INFO]\033[0m {message}")


def log_warn(message):
    """Print a warning message."""
    print(f"\033[1;33m[WARN]\033[0m {message}")


def log_error(message):
    """Print an error message."""
    print(f"\033[0;31m[ERROR]\033[0m {message}")


def get_tailscale_ip():
    """Get the Tailscale IP address of this machine."""
    try:
        # Try Linux ip command
        result = subprocess.run(['ip', 'addr', 'show'], capture_output=True, text=True, timeout=10)
        if result.returncode == 0:
            match = re.search(r'100\.(\d+)\.(\d+)\.(\d+)', result.stdout)
            if match:
                return match.group(0)
    except (subprocess.SubprocessError, FileNotFoundError):
        pass

    try:
        # Try macOS/BSD ifconfig command
        result = subprocess.run(['ifconfig'], capture_output=True, text=True, timeout=10)
        if result.returncode == 0:
            match = re.search(r'100\.(\d+)\.(\d+)\.(\d+)', result.stdout)
            if match:
                return match.group(0)
    except (subprocess.SubprocessError, FileNotFoundError):
        pass

    return None


def get_hostname():
    """Get the current hostname."""
    try:
        return socket.gethostname()
    except Exception:
        return None


def fetch_tailscale_devices(api_key, tailnet):
    """Fetch devices from Tailscale API using built-in urllib."""
    try:
        url = f"https://api.tailscale.com/api/v2/tailnet/{tailnet}/devices"
        request = urllib.request.Request(url)
        request.add_header("Authorization", f"Bearer {api_key}")
        request.add_header("User-Agent", "dns-manager-setup/1.0")

        with urllib.request.urlopen(request, timeout=30) as response:
            if response.status != 200:
                log_error(f"HTTP error {response.status}: {response.reason}")
                return None

            data = response.read().decode('utf-8')
            return json.loads(data)
    except urllib.error.HTTPError as e:
        log_error(f"HTTP error fetching Tailscale devices: {e.code} {e.reason}")
        return None
    except urllib.error.URLError as e:
        log_error(f"Network error fetching Tailscale devices: {e.reason}")
        return None
    except json.JSONDecodeError as e:
        log_error(f"Failed to parse JSON response: {e}")
        return None
    except Exception as e:
        log_error(f"Failed to fetch Tailscale devices: {e}")
        return None


def detect_current_device(devices_data, tailscale_ip, hostname):
    """Detect the current device from the Tailscale devices list."""
    if not devices_data or 'devices' not in devices_data:
        return None

    devices = devices_data['devices']

    # Try to match by Tailscale IP first
    if tailscale_ip:
        log_info(f"Found Tailscale IP on this machine: {tailscale_ip}")
        for device in devices:
            if 'addresses' in device and tailscale_ip in device['addresses']:
                log_info(f"Matched device by IP: {device['name']}")
                return device['name']

    # Fallback: try to match by hostname
    if hostname:
        log_info(f"Trying to match by hostname: {hostname}")
        for device in devices:
            device_hostname = device.get('hostname', '')
            # Try exact match or match without domain
            if (device_hostname == hostname or
                device_hostname.split('.')[0] == hostname or
                device.get('name', '') == hostname):
                log_info(f"Matched device by hostname: {device['name']}")
                return device['name']

    return None


def update_config_with_device(config_file, device_name):
    """Update the config file with the detected device name."""
    try:
        with open(config_file, 'r') as f:
            content = f.read()

        # Replace the commented device_name line with the actual device name
        updated_content = content.replace(
            '# device_name: "your-device-name"',
            f'device_name: "{device_name}"'
        )

        with open(config_file, 'w') as f:
            f.write(updated_content)

        log_info(f"Updated config with device name: {device_name}")
        return True
    except Exception as e:
        log_error(f"Failed to update config file: {e}")
        return False


def extract_config_values(config_file):
    """Extract API key and tailnet from config file using simple text parsing."""
    try:
        with open(config_file, 'r') as f:
            content = f.read()

        # Extract API key
        api_key_match = re.search(r'api_key:\s*["\']([^"\']+)["\']', content)
        tailnet_match = re.search(r'tailnet:\s*["\']([^"\']+)["\']', content)

        if not api_key_match:
            log_error("Could not find api_key in config file")
            return None, None

        if not tailnet_match:
            log_error("Could not find tailnet in config file")
            return None, None

        return api_key_match.group(1), tailnet_match.group(1)
    except Exception as e:
        log_error(f"Failed to read config file: {e}")
        return None, None


def main():
    """Main function."""
    config_file = Path("configs/config.yaml")

    if not config_file.exists():
        log_error("Configuration file configs/config.yaml not found!")
        sys.exit(1)

        log_info("Configuring Tailscale NS device...")

    # Extract config values
    api_key, tailnet = extract_config_values(config_file)
    if not api_key or not tailnet:
        sys.exit(1)

    # Detect current device
    tailscale_ip = get_tailscale_ip()
    hostname = get_hostname()

    if not tailscale_ip and not hostname:
        log_error("Could not determine Tailscale IP or hostname")
        sys.exit(1)

    # Fetch devices from API
    devices_data = fetch_tailscale_devices(api_key, tailnet)
    if not devices_data:
        sys.exit(1)

    # Detect current device
    device_name = detect_current_device(devices_data, tailscale_ip, hostname)

    if not device_name:
        log_error("Could not auto-detect current Tailscale device")
        log_error("Available devices:")
        for device in devices_data.get('devices', []):
            status = "online" if device.get('online', False) else "offline"
            hostname_info = device.get('hostname', 'unknown')
            log_error(f"  - {device.get('name', 'unknown')} ({hostname_info}) - {status}")
        sys.exit(1)

    # Update config file
    if update_config_with_device(config_file, device_name):
        log_info(f"Successfully configured NS device: {device_name}")
    else:
        sys.exit(1)


if __name__ == '__main__':
    main()
