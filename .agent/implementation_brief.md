# Integration Test Suite Implementation Brief

This document outlines the plan for creating an integration test suite for the DNS management application.

## Overview

We will create a new Docker Compose file, `docker-compose.test.yml`, specifically for the integration test suite. This environment will consist of:

- The `api` service.
- A `coredns-test` service, a CoreDNS instance configured for the test environment.
- A `pebble` service, a small ACME server for testing certificate management.

This approach isolates the test environment from the development/production environment, preventing any conflicts.

## Configuration

A new configuration file, `configs/config.test.yaml`, will be created for the test environment. This file will:

- Use a different domain, `test.jerkytreats.dev`, to avoid conflicts with `internal.jerkytreats.dev`.
- Configure the certificate manager to use the `pebble` ACME server.
- Adjust other settings as necessary for the test environment.

The `config.go` will be updated to allow setting the config type to `yaml`.

## API Tests

Go-based integration tests will be developed to verify the functionality of the API endpoints. These tests will:

- Use the `net/http/httptest` package to make live requests to the `api` service running in the test environment.
- Test the `AddRecord` endpoint by sending a POST request with a new record and verifying the response.
- Test other endpoints as they are developed.

## DNS Management Tests

Integration tests for the `coredns.Manager` will ensure that DNS zones and records are managed correctly. These tests will:

- Verify that the `coredns.Manager` correctly creates and updates CoreDNS configuration and zone files within the `coredns-test` container.
- Use a Go DNS client library to send DNS queries to the `coredns-test` instance to confirm that records are being served as expected.
- Test both adding and removing records and zones.

## Certificate Management Tests

The `certificate.Manager` will be tested against the `pebble` ACME server. These tests will:

- Verify that the `dns-01` challenge is correctly performed. This involves checking that the `DNSProvider` creates the necessary TXT records.
- Confirm that a certificate is successfully obtained from `pebble`.
- Test the certificate renewal logic by setting a short expiry time and verifying that the `certificate.Manager` renews the certificate.

## Test Runner

A shell script, `scripts/run-integration-tests.sh`, will be created to automate the execution of the integration tests. The script will perform the following steps:

1.  Start the test environment using `docker-compose -f docker-compose.test.yml up -d`.
2.  Wait for the services to be healthy.
3.  Run the Go integration tests using `go test ./...`.
4.  Stop and clean up the test environment using `docker-compose -f docker-compose.test.yml down`.

## Implementation Details & Refactoring

To support the test suite, some refactoring of the existing code is required:

1.  **Hardcoded Domain**: The `internal.jerkytreats.dev` domain is hardcoded in `internal/dns/coredns/manager.go`. This will be refactored to be configurable via the `config.yaml` file.
2.  **`AddRecord` Implementation**: The `AddRecord` method in `internal/dns/coredns/manager.go` is currently a stub. This will be fully implemented to write records to the appropriate zone file and reload CoreDNS.
3.  **Config Type**: The `config.go` loader will be updated to support `yaml` files.
