package main

import (
	"context"
	"crypto/tls"
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
	"github.com/jerkytreats/dns/internal/dns/coredns"
	"github.com/jerkytreats/dns/internal/firewall"
	"github.com/jerkytreats/dns/internal/healthcheck"
	"github.com/jerkytreats/dns/internal/logging"
	"github.com/jerkytreats/dns/internal/persistence"
	"github.com/jerkytreats/dns/internal/tailscale"
	"github.com/jerkytreats/dns/internal/tailscale/sync"
)

const (
	AppVersionKey = "app.version"

	ServerHostKey         = "server.host"
	ServerPortKey         = "server.port"
	ServerReadTimeoutKey  = "server.read_timeout"
	ServerWriteTimeoutKey = "server.write_timeout"
	ServerIdleTimeoutKey  = "server.idle_timeout"
	ServerTLSEnabledKey   = "server.tls.enabled"
	ServerTLSPortKey      = "server.tls.port"
	ServerTLSCertFileKey  = "server.tls.cert_file"
	ServerTLSKeyFileKey   = "server.tls.key_file"

	SyncEnabledKey = "dns.internal.enabled"
)

var (
	syncManager     *sync.Manager
	dnsManager      *coredns.Manager
	dnsChecker      healthcheck.Checker
	firewallManager *firewall.Manager
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

	var tailscaleClient *tailscale.Client
	var currentDeviceIP string

	logging.Info("Initializing Tailscale client for NS record configuration...")
	client, err := tailscale.NewClient()
	if err != nil {
		logging.Error("Failed to initialize Tailscale client: %v", err)
		os.Exit(1)
	}

	tailscaleClient = client

	deviceName := config.GetString(tailscale.TailscaleDeviceNameKey)
	if deviceName != "" {
		logging.Info("Using configured Tailscale device name: %s", deviceName)
		currentDeviceIP, err = tailscaleClient.GetCurrentDeviceIPByName(deviceName)
	} else {
		logging.Info("No device name configured, attempting hostname-based detection...")
		currentDeviceIP, err = tailscaleClient.GetCurrentDeviceIP()
	}

	if err != nil {
		logging.Error("Failed to get current device Tailscale IP: %v", err)
		os.Exit(1)
	}

	logging.Info("Tailscale client initialized successfully, device IP: %s", currentDeviceIP)

	// Initialize firewall manager
	logging.Info("Initializing firewall manager...")
	firewallManager, err = firewall.NewManager()
	if err != nil {
		logging.Error("Failed to initialize firewall manager: %v", err)
		os.Exit(1)
	}

	// Setup firewall rules for Tailscale CIDR protection
	logging.Info("Setting up firewall rules for Tailscale CIDR...")
	if err := firewallManager.EnsureFirewallRules(); err != nil {
		logging.Error("Failed to setup firewall rules: %v", err)
		logging.Warn("Continuing without firewall management...")
	} else {
		logging.Info("Firewall rules configured successfully")
	}

	dnsManager = newCoreDNSManager(currentDeviceIP)

	domain := config.GetString(coredns.DNSDomainKey)
	if domain != "" {
		if err := dnsManager.AddZone(domain); err != nil {
			logging.Warn("Failed to ensure zone file exists for domain %s: %v", domain, err)
		}

		// Add DNS record for the API endpoint (dns.{domain})
		if err := dnsManager.AddRecord(domain, "dns", currentDeviceIP); err != nil {
			logging.Warn("Failed to add API endpoint record dns.%s -> %s: %v", domain, currentDeviceIP, err)
		} else {
			logging.Info("Added API endpoint record: dns.%s -> %s", domain, currentDeviceIP)
		}
	}

	dnsServer := "coredns:53"
	dnsChecker = healthcheck.NewDNSHealthChecker(dnsServer, 10*time.Second, 10, 2*time.Second)

	if err := healthcheck.TestBasicConnectivity(dnsServer, 5*time.Second); err != nil {
		logging.Error("%v", err)
		os.Exit(1)
	}

	maxAttempts := 15
	retryDelay := 3 * time.Second

	if err := healthcheck.WaitForHealthyWithDiagnostics(dnsChecker, maxAttempts, retryDelay); err != nil {
		logging.Error("%v", err)
		os.Exit(1)
	}

	tlsEnabled := config.GetBool(ServerTLSEnabledKey)
	var certReadyCh <-chan struct{}

	if tlsEnabled && config.GetBool(certificate.CertRenewalEnabledKey) {
		logging.Info("Starting certificate process in background...")

		certProcess, err := certificate.NewProcessManager(dnsManager)
		if err != nil {
			logging.Error("Failed to create certificate process: %v", err)
			os.Exit(1)
		}

		certReadyCh = certProcess.StartWithRetry(30 * time.Second)
	}

	logging.Info("Starting sync process in background...")
	go func() {
		if sm := maybeSync(dnsManager, tailscaleClient); sm != nil {
			syncManager = sm
			logging.Info("Background sync process completed successfully")
		} else {
			logging.Info("Sync disabled or failed, continuing without dynamic zone sync")
		}
	}()

	// Initialize handler registry with all handlers
	handlerRegistry, err := handler.NewHandlerRegistry(dnsManager, dnsChecker, syncManager)
	if err != nil {
		logging.Error("Failed to initialize handler registry: %v", err)
		os.Exit(1)
	}

	mux := http.NewServeMux()

	// Register all application handlers through the registry
	handlerRegistry.RegisterHandlers(mux)

	server := &http.Server{
		Addr:         fmt.Sprintf("%s:%d", config.GetString(ServerHostKey), config.GetInt(ServerPortKey)),
		ReadTimeout:  config.GetDuration(ServerReadTimeoutKey),
		WriteTimeout: config.GetDuration(ServerWriteTimeoutKey),
		IdleTimeout:  config.GetDuration(ServerIdleTimeoutKey),
		Handler:      mux,
	}

	serverStarted := make(chan bool, 1)
	go func() {
		logging.Info("Starting HTTP server on port %d", config.GetInt(ServerPortKey))
		serverStarted <- true
		err := server.ListenAndServe()

		if err != nil && err != http.ErrServerClosed {
			logging.Error("Failed to start server: %v", err)
			os.Exit(1)
		}
	}()

	<-serverStarted
	logging.Info("HTTP server started successfully")

	if tlsEnabled && certReadyCh != nil {
		logging.Info("TLS is enabled, setting up dual HTTP/HTTPS servers...")
		go func() {
			logging.Info("Waiting for certificate ready signal...")
			<-certReadyCh
			logging.Info("Certificate ready signal received, starting HTTPS server...")

			tlsPort := config.GetInt(ServerTLSPortKey)
			certFile := config.GetString(ServerTLSCertFileKey)
			keyFile := config.GetString(ServerTLSKeyFileKey)

			logging.Info("Starting HTTPS server on port %d with cert: %s, key: %s", tlsPort, certFile, keyFile)

			httpsSrv := &http.Server{
				Addr:         fmt.Sprintf("%s:%d", config.GetString(ServerHostKey), tlsPort),
				ReadTimeout:  config.GetDuration(ServerReadTimeoutKey),
				WriteTimeout: config.GetDuration(ServerWriteTimeoutKey),
				IdleTimeout:  config.GetDuration(ServerIdleTimeoutKey),
				Handler:      mux,
				TLSConfig: &tls.Config{
					MinVersion: tls.VersionTLS12,
				},
			}

			logging.Info("HTTPS server now listening on port %d", tlsPort)
			if err := httpsSrv.ListenAndServeTLS(certFile, keyFile); err != nil && err != http.ErrServerClosed {
				logging.Error("HTTPS server error: %v", err)
			} else {
				logging.Info("HTTPS server started successfully")
			}
		}()
	} else {
		logging.Info("TLS disabled or no certificate channel - continuing with HTTP only")
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logging.Info("Shutting down server...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		logging.Error("Server forced to shutdown: %v", err)
	}

	logging.Info("Server exited properly")
}

// newCoreDNSManager constructs and returns a configured CoreDNS manager.
func newCoreDNSManager(currentDeviceIP string) *coredns.Manager {

	dnsMgr := coredns.NewManager(currentDeviceIP)

	baseDomain := config.GetString(coredns.DNSDomainKey)
	_ = dnsMgr.AddDomain(baseDomain, nil)

	return dnsMgr
}

// maybeSync initialises dynamic zone sync if it is enabled in config.
// It returns the *sync.Manager instance or nil if sync is disabled or fails.
func maybeSync(dnsMgr *coredns.Manager, tailscaleClient *tailscale.Client) *sync.Manager {
	if !config.GetBool(SyncEnabledKey) {
		logging.Info("Sync is disabled")
		return nil
	}

	logging.Info("Sync is enabled, initializing dynamic zone sync...")

	if tailscaleClient == nil {
		logging.Info("No Tailscale client provided, attempting to create one for sync...")
		var err error
		tailscaleClient, err = tailscale.NewClient()
		if err != nil {
			logging.Error("Failed to create Tailscale client for sync: %v", err)
			return nil
		}
	}

	// Initialize device persistence storage
	logging.Info("Initializing device persistence storage...")
	deviceStorage := persistence.NewFileStorage()

	sm, err := sync.NewManager(dnsMgr, tailscaleClient, deviceStorage)
	if err != nil {
		logging.Error("Failed to create sync manager: %v", err)
		return nil
	}

	if err := sm.EnsureInternalZone(); err != nil {
		logging.Error("Failed to sync internal zone: %v", err)
		logging.Warn("Sync failed, continuing without dynamic zone sync")
		return nil
	}

	syncConfig := config.GetSyncConfig()
	if syncConfig.Polling.Enabled {
		sm.StartPolling(syncConfig.Polling.Interval)
	}

	logging.Info("Dynamic zone sync completed successfully")
	return sm
}
