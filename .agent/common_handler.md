# Feature Brief: Common Handler Registration

## Objective

Implement a `handler.go` file that serves as a single registration point for all HTTP handlers in the application. This will allow `main.go` to initialize handlers without needing to know about individual handler implementations.

## Implementation Plan

| Task | Status | Files to Modify |
|------|--------|-----------------|
| Create `handler.go` with a function to register all handlers | ✅ Done | `internal/api/handler/handler.go` |
| Update `main.go` to use the new handler registration function | ✅ Done | `cmd/api/main.go` |
| Update record handler if necessary | ✅ Done | `internal/api/handler/record.go` |
| Add logging and configuration as per guidelines | ✅ Done | `internal/api/handler/handler.go`, `cmd/api/main.go` |
| Verify all existing handlers are registered and functional | ✅ Done | N/A |
| Add tests for new handler registration process | ✅ Done | `internal/api/handler/handler_test.go` |

## Benefits

- Simplifies the process of adding new handlers.
- Reduces coupling between `main.go` and individual handlers.
- Centralizes handler registration, making the codebase easier to maintain and extend.

---

# Certificate Process Refactoring

## Current Issues

The certificate management process currently has several architectural problems:

1. **Mixed Responsibilities**: `main.go` contains certificate orchestration logic that should belong to the certificate domain
2. **Tight Coupling**: Main directly manages certificate manager lifecycle, retry logic, and error handling
3. **Global Dependencies**: Uses global `certManager` variable accessed from main
4. **Domain Logic Leak**: Certificate-specific logic (retry intervals, process flow) lives outside the certificate package

## Current Flow Problems

```
main.go owns:
├── certReadyCh channel creation
├── Background goroutine with retry loop
├── runCertificateProcess orchestration
├── Certificate manager lifecycle
├── TLS transition logic
```

## Recommended Refactoring

### New Architecture

Move certificate process ownership to `internal/certificate` package:

```go
// ProcessManager handles complete certificate lifecycle
type ProcessManager struct {
    manager     *Manager
    dnsManager  interface{ EnableTLS(domain, certPath, keyPath string) error }
    domain      string
}

// NewProcessManager - factory with dependency injection
func NewProcessManager(dnsManager interface{ EnableTLS(...) error }) (*ProcessManager, error)

// StartWithRetry - non-blocking process with built-in retry logic
func (pm *ProcessManager) StartWithRetry(retryInterval time.Duration) <-chan struct{}
```

### Simplified Main.go

```go
if tlsEnabled && config.GetBool(certificate.CertRenewalEnabledKey) {
    certProcess, err := certificate.NewProcessManager(dnsManager)
    if err != nil {
        logging.Error("Failed to create certificate process: %v", err)
        os.Exit(1)
    }

    certReadyCh := certProcess.StartWithRetry(30 * time.Second)
    // TLS transition logic uses certReadyCh
}
```

## Implementation Plan

| Task | Status | Files to Modify |
|------|--------|-----------------|
| Create ProcessManager struct in certificate package | ✅ Done | `internal/certificate/manager.go` |
| Move runCertificateProcess logic to ProcessManager | ✅ Done | `internal/certificate/manager.go` |
| Add configuration handling within certificate package | ✅ Done | `internal/certificate/manager.go` |
| Update main.go to use simplified ProcessManager API | ✅ Done | `cmd/api/main.go` |
| Remove global certManager variable | ✅ Done | `cmd/api/main.go` |
| Add ProcessManager tests | 🚧 TODO | `internal/certificate/manager_test.go` |

## Benefits

✅ **Domain Ownership** - Certificate package owns all certificate logic
✅ **Clear Interfaces** - ProcessManager provides clean API for main.go
✅ **Dependency Injection** - DNS manager injected, no tight coupling
✅ **Error Propagation** - Errors bubble up through return values
✅ **Non-blocking Maintained** - Channel-based communication preserved
✅ **Testable** - ProcessManager can be easily unit tested
✅ **Single Responsibility** - Main.go focuses on coordination, certificate package handles certificates

## Clean Dependency Chain

```
main.go
├── imports: internal/certificate
├── calls: certificate.NewProcessManager(dnsManager)

internal/certificate/manager.go
├── imports: internal/config, internal/logging, internal/dns/coredns
├── owns: certificate lifecycle, retry logic, channel management
```

## Completed Refactoring Summary

✅ **ProcessManager Implementation**: Created new `ProcessManager` struct with proper dependency injection
✅ **Main.go Simplification**: Reduced certificate logic from ~40 lines to ~7 lines in main.go
✅ **Global Variable Elimination**: Removed global `certManager` variable
✅ **Domain Encapsulation**: All certificate logic now owned by certificate package
✅ **Non-blocking Preserved**: Maintained channel-based communication for TLS transition
✅ **Error Handling**: Proper error propagation through return values

The refactoring successfully achieved the goal of moving certificate process ownership to the certificate domain while maintaining all existing functionality and improving the overall architecture.
