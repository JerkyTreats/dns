#!/usr/bin/env python3
"""
Automatically configure Tailscale NS device.
This script detects the current Tailscale device and configures it in the config file.
"""

import sys
import yaml
import requests
import socket
import subprocess
import re
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
    """Fetch devices from Tailscale API."""
    try:
        headers = {"Authorization": f"Bearer {api_key}"}
        url = f"https://api.tailscale.com/api/v2/tailnet/{tailnet}/devices"

        response = requests.get(url, headers=headers, timeout=30)
        response.raise_for_status()

        return response.json()
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


def load_config(config_file):
    """Load the YAML config file."""
    try:
        with open(config_file, 'r') as f:
            return yaml.safe_load(f)
    except Exception as e:
        log_error(f"Failed to load config file: {e}")
        return None


def main():
    """Main function."""
    config_file = Path("configs/config.yaml")

    if not config_file.exists():
        log_error("Configuration file configs/config.yaml not found!")
        sys.exit(1)

    log_info("Configuring Tailscale NS device...")

    # Load config to get API key and tailnet
    config = load_config(config_file)
    if not config:
        sys.exit(1)

    tailscale_config = config.get('tailscale', {})
    api_key = tailscale_config.get('api_key')
    tailnet = tailscale_config.get('tailnet')

    if not api_key or not tailnet:
        log_error("Tailscale API key or tailnet not found in config file!")
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
