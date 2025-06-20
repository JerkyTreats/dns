# Cert Manager Clean-up Brief

# DNS Code Cleanup Plan

## Overview
This document outlines the required cleanup changes for all non-test files in the `internal/` directory to comply with the .agent/README.md style guide.

## Phase Breakdown

| Phase | File | Required Changes |
|-------|------|------------------|
| **1. Duplication Analysis** | `internal/config/config.go` | ✅ No duplication found - well-architected singleton pattern |
| | `internal/logging/logging.go` | ✅ No duplication found |
| | `internal/api/handler/record.go` | • Remove duplicate validation logic - `serviceNameRegex` also exists in `coredns/manager.go`<br>• Remove unused struct fields in `AddRecordRequest` and `AddRecordResponse` |
| | `internal/certificate/manager.go` | ✅ No significant duplication found |
| | `internal/dns/coredns/manager.go` | • `serviceNameRegex` duplicated from `api/handler/record.go` - consolidate to shared location |
| | `internal/dns/coredns/provider.go` | ✅ No duplication found |
| **2. Unnecessary Validation** | `internal/config/config.go` | ✅ All validations are necessary |
| | `internal/logging/logging.go` | ✅ All validations are necessary |
| | `internal/api/handler/record.go` | • Remove unused `serviceNameRegex` - validation handled by CoreDNS manager<br>• Simplify error handling - remove `sendError` function that's not used |
| | `internal/certificate/manager.go` | • Remove redundant file existence check in `ObtainCertificate` - handled by the certificate logic<br>• Simplify user registration check |
| | `internal/dns/coredns/manager.go` | • Remove duplicate service name validation - already handled in API layer<br>• Remove unnecessary file existence check in `AddRecord` - create if not exists |
| | `internal/dns/coredns/provider.go` | ✅ All validations are necessary |
| **3. Deprecated Code** | `internal/config/config.go` | • Comment on line 5 references "PHITE" instead of "DNS" - update package description<br>• Remove global `MissingKeys` variable (exported) - not following Go conventions |
| | `internal/logging/logging.go` | • Comment on line 1-2 references "PHITE" instead of "DNS" - update package description |
| | `internal/api/handler/record.go` | • Unused struct definitions: `AddRecordRequest`, `AddRecordResponse`, `ErrorResponse` |
| | `internal/certificate/manager.go` | • Direct usage of `viper.GetViper()` on line 138 - violates style guide rule about using internal/config<br>• Import of `"github.com/spf13/viper"` should be removed |
| | `internal/dns/coredns/manager.go` | • Hard-coded test-specific logic in `Reload()` method (lines 173-185) should be extracted<br>• Hard-coded sleep duration `time.Sleep(5 * time.Second)` |
| | `internal/dns/coredns/provider.go` | ✅ No deprecated code found |
| **4. Comments & Logging** | `internal/config/config.go` | **Comments:**<br>• Remove verbose comments on lines 58-60, 68-70, 79-81 (functional options descriptions)<br>• Simplify comment on line 127 "for parse errors or other errors, log and return viper with defaults"<br>**Logging:**<br>• Add INFO logging at start of `InitConfig()` function<br>• Add ERROR logging for configuration load failures in `loadConfig()` |
| | `internal/logging/logging.go` | **Comments:**<br>✅ Comments are minimal and necessary<br>**Logging:**<br>✅ This is the logging package itself |
| | `internal/api/handler/record.go` | **Comments:**<br>✅ Comments are minimal and appropriate<br>**Logging:**<br>• Add INFO logging at start of `AddRecord()` method<br>• Current ERROR logging for failures is appropriate |
| | `internal/certificate/manager.go` | **Comments:**<br>• Remove verbose comment lines 83-84 "Set custom CA directory URL if provided"<br>• Remove verbose comment lines 106-107 "Check if certificate already exists"<br>• Remove verbose comment lines 112-113 "Register user if not already registered"<br>• Remove verbose comment lines 124-125 "Obtain the certificate"<br>**Logging:**<br>• Add INFO logging at start of `NewManager()` function<br>• Add INFO logging at start of `StartRenewalLoop()` function<br>• Current ERROR/INFO logging in methods is appropriate |
| | `internal/dns/coredns/manager.go` | **Comments:**<br>• Remove verbose comment on lines 56-57 "Validate service name"<br>• Remove verbose comment on lines 76-77 "Ensure zones directory exists"<br>• Remove verbose comment on lines 81-82 "Write zone file"<br>• Remove verbose comment on lines 85-86 "Update CoreDNS configuration"<br>• Remove verbose comment on lines 90-91 "Reload CoreDNS"<br>• Remove verbose comment on lines 101-102 "Read current config"<br>• Remove verbose comment on lines 105-106 "Add zone configuration"<br>• Remove verbose comment on lines 113-114 "Append new zone config"<br>• Remove verbose comment on lines 117-118 "Write updated config"<br>• Remove verbose comments on lines 132-133, 145-146, 150-151, 156-157<br>• Remove verbose comment on lines 170-171 "Special handling for integration tests"<br>**Logging:**<br>• Add INFO logging at start of `NewManager()` function<br>• Add INFO logging at start of `AddZone()` function<br>• Add INFO logging at start of `RemoveZone()` function<br>• Add ERROR logging for failed operations in `updateConfig()` and `removeFromConfig()` |
| | `internal/dns/coredns/provider.go` | **Comments:**<br>• Remove verbose comment on lines 21-22 "The fqdn from GetRecord has a trailing dot..."<br>**Logging:**<br>• Add INFO logging at start of `Present()` method<br>• Add INFO logging at start of `CleanUp()` method<br>• Add ERROR logging for file operation failures |

## Implementation Priority

1. **High Priority**: Phase 3 (Deprecated Code) - Remove viper usage and PHITE references
2. **Medium Priority**: Phase 1 (Duplication) - Consolidate regex patterns and remove unused code
3. **Medium Priority**: Phase 4 (Comments & Logging) - Standardize according to style guide
4. **Low Priority**: Phase 2 (Unnecessary Validation) - Simplify validation logic

## Post-Cleanup Verification

After implementing all phases:
- [ ] Run all unit tests to ensure functionality is preserved
- [ ] Run integration tests to verify end-to-end functionality
- [ ] Verify no third-party configuration packages are used outside of internal/config
- [ ] Confirm all major processing steps have INFO level logging
- [ ] Confirm all error conditions have ERROR level logging
