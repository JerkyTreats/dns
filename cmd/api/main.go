package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jerkytreats/dns/internal/api/handler"
	"github.com/jerkytreats/dns/internal/certificate"
	"github.com/jerkytreats/dns/internal/config"
	"github.com/jerkytreats/dns/internal/dns/bootstrap"
	"github.com/jerkytreats/dns/internal/dns/coredns"
	"github.com/jerkytreats/dns/internal/healthcheck"
	"github.com/jerkytreats/dns/internal/logging"
	"github.com/jerkytreats/dns/internal/tailscale"
)

const (
	// App
	AppVersionKey = "app.version"

	// Server
	ServerHostKey         = "server.host"
	ServerPortKey         = "server.port"
	ServerReadTimeoutKey  = "server.read_timeout"
	ServerWriteTimeoutKey = "server.write_timeout"
	ServerIdleTimeoutKey  = "server.idle_timeout"
	ServerTLSEnabledKey   = "server.tls.enabled"
	ServerTLSPortKey      = "server.tls.port"
	ServerTLSCertFileKey  = "server.tls.cert_file"
	ServerTLSKeyFileKey   = "server.tls.key_file"

	// Bootstrap
	BootstrapEnabledKey = "dns.internal.enabled"
)

var (
	// Global managers for health checks and TLS integration
	bootstrapManager *bootstrap.Manager
	certManager      *certificate.Manager
	dnsManager       *coredns.Manager
	dnsChecker       healthcheck.Checker
)

func init() {
	config.RegisterRequiredKey(AppVersionKey)
	config.RegisterRequiredKey(ServerHostKey)
	config.RegisterRequiredKey(ServerPortKey)
	config.RegisterRequiredKey(ServerReadTimeoutKey)
	config.RegisterRequiredKey(ServerWriteTimeoutKey)
	config.RegisterRequiredKey(ServerIdleTimeoutKey)
	config.RegisterRequiredKey(ServerTLSEnabledKey)
	config.RegisterRequiredKey(ServerTLSPortKey)
	config.RegisterRequiredKey(ServerTLSCertFileKey)
	config.RegisterRequiredKey(ServerTLSKeyFileKey)
}

func main() {
	configFile := flag.String("config", "", "Path to the configuration file")
	flag.Parse()

	if err := config.FirstTimeInit(configFile); err != nil {
		logging.Error("Failed to initialize configuration: %v", err)
		os.Exit(1)
	}

	logging.Info("Application starting...")
	defer logging.Sync()

	dnsManager = newCoreDNSManager()

	// Ensure a Corefile exists; if not, write a minimal one from the template so
	// the CoreDNS container can start successfully on first deploy.
	ensureCorefileExists(dnsManager)

	// Potentially create internal zone before waiting for health so a usable
	// Corefile is already written when bootstrap is enabled.
	bootstrapManager = maybeBootstrap(dnsManager)

	// Now actively wait for CoreDNS to become healthy.
	dnsServer := "127.0.0.1:53"
	if healthcheck.IsDockerEnvironment() {
		dnsServer = "coredns:53"
	}
	dnsChecker = healthcheck.NewDNSHealthChecker(dnsServer, 10*time.Second, 10, 2*time.Second)

	logging.Info("Waiting for CoreDNS to report healthy status...")
	if !dnsChecker.WaitHealthy() {
		logging.Error("CoreDNS did not become healthy within the expected time")
		os.Exit(1)
	}
	logging.Info("CoreDNS is healthy")

	tlsEnabled := config.GetBool(ServerTLSEnabledKey)

	// Channel that will be closed once a certificate has been successfully obtained.
	var certReadyCh chan struct{}

	if tlsEnabled && config.GetBool(certificate.CertRenewalEnabledKey) {
		certReadyCh = make(chan struct{})

		// Launch certificate obtainment in background so the HTTP server can
		// start immediately.
		go func() {
			logging.Info("Initializing certificate manager with CoreDNS integration...")

			var err error
			certManager, err = certificate.NewManager()
			if err != nil {
				logging.Error("Failed to create certificate manager: %v", err)
				return
			}

			// Integrate certificate manager with ConfigManager for TLS enablement
			certManager.SetCoreDNSManager(dnsManager)

			domain := config.GetString(certificate.CertDomainKey)
			if domain == "" {
				logging.Warn("certificate.domain not configured – skipping certificate obtainment")
				return
			}

			if err := certManager.ObtainCertificate(domain); err != nil {
				logging.Error("Certificate obtain failed: %v", err)
				return
			}

			// Signal the main routine that the certificate is ready.
			close(certReadyCh)

			// Start background renewal loop if enabled
			if config.GetBool(certificate.CertRenewalEnabledKey) {
				go certManager.StartRenewalLoop(domain)
			}
		}()
	}

	// Create record handler
	recordHandler := handler.NewRecordHandler(dnsManager)

	// Setup routes
	mux := http.NewServeMux()
	mux.HandleFunc("/health", healthCheckHandler)
	mux.HandleFunc("/add-record", recordHandler.AddRecord)

	// Step 10: Create and start initial HTTP server (always plain HTTP).
	var server *http.Server

	server = &http.Server{
		Addr:         fmt.Sprintf("%s:%d", config.GetString(ServerHostKey), config.GetInt(ServerPortKey)),
		ReadTimeout:  config.GetDuration(ServerReadTimeoutKey),
		WriteTimeout: config.GetDuration(ServerWriteTimeoutKey),
		IdleTimeout:  config.GetDuration(ServerIdleTimeoutKey),
		Handler:      mux,
	}

	// Start server in a goroutine
	serverStarted := make(chan bool, 1)
	go func() {
		logging.Info("Starting server...")
		var err error
		logging.Info("Server starting without TLS on port %d", config.GetInt(ServerPortKey))
		serverStarted <- true
		err = server.ListenAndServe()

		if err != nil && err != http.ErrServerClosed {
			logging.Error("Failed to start server: %v", err)
			os.Exit(1)
		}
	}()

	// Wait for server to start
	<-serverStarted
	logging.Info("Server started successfully")

	// If TLS is enabled and we have a certificate goroutine, switch to HTTPS
	if tlsEnabled && certReadyCh != nil {
		go func() {
			<-certReadyCh

			logging.Info("Certificate obtained – switching server to HTTPS")

			// Gracefully stop the HTTP server
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			_ = server.Shutdown(ctx)
			cancel()

			// Build HTTPS server with same handler
			httpsSrv := &http.Server{
				Addr:         fmt.Sprintf("%s:%d", config.GetString(ServerHostKey), config.GetInt(ServerTLSPortKey)),
				ReadTimeout:  config.GetDuration(ServerReadTimeoutKey),
				WriteTimeout: config.GetDuration(ServerWriteTimeoutKey),
				IdleTimeout:  config.GetDuration(ServerIdleTimeoutKey),
				Handler:      mux,
				TLSConfig: &tls.Config{
					MinVersion: tls.VersionTLS12,
				},
			}

			// Update the server pointer so shutdown logic hits the right instance later.
			server = httpsSrv

			certFile := config.GetString(ServerTLSCertFileKey)
			keyFile := config.GetString(ServerTLSKeyFileKey)
			logging.Info("HTTPS server now listening on port %d", config.GetInt(ServerTLSPortKey))
			if err := httpsSrv.ListenAndServeTLS(certFile, keyFile); err != nil && err != http.ErrServerClosed {
				logging.Error("HTTPS server error: %v", err)
			}
		}()
	}

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	// Graceful shutdown
	logging.Info("Shutting down server...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		logging.Error("Server forced to shutdown: %v", err)
	}

	logging.Info("Server exited properly")
}

func healthCheckHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	components := map[string]interface{}{
		"api": map[string]string{
			"status":  "healthy",
			"message": "API is running",
		},
	}

	// Dynamic CoreDNS health status
	if dnsChecker != nil {
		if ok, latency, err := dnsChecker.CheckOnce(); ok {
			components["coredns"] = map[string]string{
				"status":  "healthy",
				"message": fmt.Sprintf("CoreDNS responded in %v", latency),
			}
		} else {
			msg := "CoreDNS probe failed"
			if err != nil {
				msg = err.Error()
			}
			components["coredns"] = map[string]string{
				"status":  "error",
				"message": msg,
			}
		}
	} else {
		components["coredns"] = map[string]string{
			"status":  "unknown",
			"message": "DNS checker not initialized",
		}
	}

	// Add bootstrap status if enabled
	if config.GetBool(BootstrapEnabledKey) {
		if bootstrapManager != nil && bootstrapManager.IsZoneBootstrapped() {
			components["bootstrap"] = map[string]string{
				"status":  "healthy",
				"message": "Dynamic zone bootstrap is active",
			}
		} else if bootstrapManager != nil {
			components["bootstrap"] = map[string]string{
				"status":  "warning",
				"message": "Bootstrap manager created but zone not bootstrapped",
			}
		} else {
			components["bootstrap"] = map[string]string{
				"status":  "error",
				"message": "Bootstrap enabled but manager failed to initialize",
			}
		}
	} else {
		components["bootstrap"] = map[string]string{
			"status":  "disabled",
			"message": "Dynamic zone bootstrap is disabled",
		}
	}

	response := map[string]interface{}{
		"status":     "healthy",
		"version":    config.GetString(AppVersionKey),
		"components": components,
	}

	if config.GetBool(ServerTLSEnabledKey) {
		certPath := config.GetString(ServerTLSCertFileKey)
		if certInfo, err := certificate.GetCertificateInfo(certPath); err == nil {
			response["certificate"] = map[string]interface{}{
				"subject":    certInfo.Subject.CommonName,
				"issuer":     certInfo.Issuer.CommonName,
				"not_before": certInfo.NotBefore,
				"not_after":  certInfo.NotAfter,
				"expires_in": time.Until(certInfo.NotAfter).String(),
			}
		} else {
			response["certificate"] = map[string]interface{}{
				"status":  "error",
				"message": "Failed to get certificate info",
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// newCoreDNSManager constructs and returns a configured CoreDNS manager.
func newCoreDNSManager() *coredns.Manager {
	configPath := config.GetString(coredns.DNSConfigPathKey)
	templatePath := config.GetString(coredns.DNSTemplatePathKey)
	zonesPath := config.GetString(coredns.DNSZonesPathKey)
	reloadCmd := config.GetStringSlice(coredns.DNSReloadCommandKey)
	domain := config.GetString(coredns.DNSDomainKey)

	dnsMgr := coredns.NewManager(configPath, templatePath, zonesPath, reloadCmd, domain)

	// Add base domain so zone files can refer to it.
	baseDomain := config.GetString(coredns.DNSDomainKey)
	_ = dnsMgr.AddDomain(baseDomain, nil)

	return dnsMgr
}

// maybeBootstrap initialises dynamic zone bootstrap if it is enabled in config.
// It returns the *bootstrap.Manager instance or nil if bootstrap is disabled or fails.
func maybeBootstrap(dnsMgr *coredns.Manager) *bootstrap.Manager {
	if !config.GetBool(BootstrapEnabledKey) {
		logging.Info("Bootstrap is disabled")
		return nil
	}

	logging.Info("Bootstrap is enabled, initializing dynamic zone bootstrap...")

	tailscaleClient, err := tailscale.NewClient()
	if err != nil {
		logging.Error("Failed to create Tailscale client: %v", err)
		return nil
	}

	bm, err := bootstrap.NewManager(dnsMgr, tailscaleClient)
	if err != nil {
		logging.Error("Failed to create bootstrap manager: %v", err)
		return nil
	}

	if err := bm.EnsureInternalZone(); err != nil {
		logging.Error("Failed to bootstrap internal zone: %v", err)
		logging.Warn("Bootstrap failed, continuing without dynamic zone bootstrap")
		return nil
	}

	logging.Info("Dynamic zone bootstrap completed successfully")
	return bm
}

func ensureCorefileExists(dnsMgr *coredns.Manager) {
	configPath := config.GetString(coredns.DNSConfigPathKey)
	if _, err := os.Stat(configPath); err == nil {
		return // already exists
	}

	templatePath := config.GetString(coredns.DNSTemplatePathKey)
	if templatePath == "" {
		templatePath = "configs/coredns/Corefile.template"
	}
	tmpl, err := os.ReadFile(templatePath)
	if err != nil {
		logging.Warn("Failed to read CoreDNS template to seed Corefile: %v", err)
		return
	}

	if err := os.WriteFile(configPath, tmpl, 0644); err != nil {
		logging.Warn("Failed to write initial Corefile: %v", err)
		return
	}
	logging.Info("Seeded initial Corefile at %s", configPath)
}
