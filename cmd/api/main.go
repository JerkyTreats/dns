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
	TailscaleAPIKeyKey  = "tailscale.api_key"
	TailscaleTailnetKey = "tailscale.tailnet"
)

var (
	// Global bootstrap manager for health checks
	bootstrapManager *bootstrap.Manager
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

	// Bootstrap keys are only required if bootstrap is enabled
	// We'll check these conditionally in main()
}

func main() {
	// Command-line flag for config file
	configFile := flag.String("config", "", "Path to the configuration file")
	flag.Parse()

	// Initialize configuration
	if *configFile != "" {
		if err := config.InitConfig(config.WithConfigPath(*configFile)); err != nil {
			fmt.Printf("Failed to initialize configuration: %v\n", err)
			os.Exit(1)
		}
	} else {
		if err := config.InitConfig(); err != nil {
			fmt.Printf("Failed to initialize configuration: %v\n", err)
			os.Exit(1)
		}
	}

	// Check required configuration keys after initialization
	if err := config.CheckRequiredKeys(); err != nil {
		fmt.Printf("Configuration validation failed: %v\n", err)
		os.Exit(1)
	}

	// Initialize logger
	logging.Info("Application starting...")
	defer logging.Sync()

	// Initialize CoreDNS manager from config
	configPath := config.GetString("dns.coredns.config_path")
	zonesPath := config.GetString("dns.coredns.zones_path")
	reloadCmd := config.GetStringSlice("dns.coredns.reload_command")
	domain := config.GetString(coredns.DNSDomainKey)
	manager := coredns.NewManager(configPath, zonesPath, reloadCmd, domain)

	// Initialize bootstrap if enabled
	if config.GetBool(BootstrapEnabledKey) {
		logging.Info("Bootstrap is enabled, initializing dynamic zone bootstrap...")

		// Validate required bootstrap configuration
		apiKey := config.GetString(TailscaleAPIKeyKey)
		tailnet := config.GetString(TailscaleTailnetKey)

		if apiKey == "" || apiKey == "${TAILSCALE_API_KEY}" {
			logging.Error("Bootstrap enabled but tailscale.api_key is not configured or environment variable is not set")
			os.Exit(1)
		}
		if tailnet == "" || tailnet == "${TAILSCALE_TAILNET}" {
			logging.Error("Bootstrap enabled but tailscale.tailnet is not configured or environment variable is not set")
			os.Exit(1)
		}

		// Validate bootstrap configuration
		if err := config.ValidateBootstrapConfig(); err != nil {
			logging.Error("Bootstrap configuration validation failed: %v", err)
			os.Exit(1)
		}

		// Create Tailscale client
		baseURL := config.GetString(config.TailscaleBaseURLKey)
		var tailscaleClient *tailscale.Client
		if baseURL != "" {
			tailscaleClient = tailscale.NewClientWithBaseURL(apiKey, tailnet, baseURL)
		} else {
			tailscaleClient = tailscale.NewClient(apiKey, tailnet)
		}

		// Get bootstrap configuration
		bootstrapConfig := config.GetBootstrapConfig()

		// Create bootstrap manager
		bootstrapManager = bootstrap.NewManager(manager, tailscaleClient, bootstrapConfig)

		// Validate Tailscale connection and configuration
		if err := bootstrapManager.ValidateConfiguration(); err != nil {
			logging.Error("Bootstrap validation failed: %v", err)
			os.Exit(1)
		}

		// Initialize the internal zone and bootstrap devices
		if err := bootstrapManager.EnsureInternalZone(); err != nil {
			logging.Error("Failed to bootstrap internal zone: %v", err)
			// Decide whether this should be fatal or continue
			// For now, we'll continue with a warning to avoid breaking existing deployments
			logging.Warn("Bootstrap failed, continuing without dynamic zone bootstrap")
			bootstrapManager = nil
		} else {
			logging.Info("Dynamic zone bootstrap completed successfully")
		}
	} else {
		logging.Info("Bootstrap is disabled")
	}

	// Certificate management
	if config.GetBool(certificate.CertRenewalEnabledKey) {
		go func() {
			certManager, err := certificate.NewManager()
			if err != nil {
				logging.Error("Failed to create certificate manager: %v", err)
				return
			}

			domain := config.GetString(certificate.CertDomainKey)
			if err := certManager.ObtainCertificate(domain); err != nil {
				logging.Error("Failed to obtain certificate: %v", err)
				return
			}

			certManager.StartRenewalLoop(domain)
		}()
	}

	// Create record handler
	recordHandler := handler.NewRecordHandler(manager)

	// Setup routes
	mux := http.NewServeMux()
	mux.HandleFunc("/health", healthCheckHandler)
	mux.HandleFunc("/add-record", recordHandler.AddRecord)

	// Create server
	var server *http.Server

	if config.GetBool(ServerTLSEnabledKey) {
		server = &http.Server{
			Addr:         fmt.Sprintf("%s:%d", config.GetString(ServerHostKey), config.GetInt(ServerTLSPortKey)),
			ReadTimeout:  config.GetDuration(ServerReadTimeoutKey),
			WriteTimeout: config.GetDuration(ServerWriteTimeoutKey),
			IdleTimeout:  config.GetDuration(ServerIdleTimeoutKey),
			Handler:      mux,
			TLSConfig: &tls.Config{
				MinVersion: tls.VersionTLS12,
			},
		}
	} else {
		server = &http.Server{
			Addr:         fmt.Sprintf("%s:%d", config.GetString(ServerHostKey), config.GetInt(ServerPortKey)),
			ReadTimeout:  config.GetDuration(ServerReadTimeoutKey),
			WriteTimeout: config.GetDuration(ServerWriteTimeoutKey),
			IdleTimeout:  config.GetDuration(ServerIdleTimeoutKey),
			Handler:      mux,
		}
	}

	// Start server in a goroutine
	go func() {
		logging.Info("Starting server...")
		var err error
		if config.GetBool(ServerTLSEnabledKey) {
			certFile := config.GetString(ServerTLSCertFileKey)
			keyFile := config.GetString(ServerTLSKeyFileKey)
			logging.Info("Server will start with TLS on port %d", config.GetInt(ServerTLSPortKey))
			err = server.ListenAndServeTLS(certFile, keyFile)
		} else {
			logging.Info("Server will start without TLS on port %d", config.GetInt(ServerPortKey))
			err = server.ListenAndServe()
		}

		if err != nil && err != http.ErrServerClosed {
			logging.Error("Failed to start server: %v", err)
			os.Exit(1)
		}
	}()

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
		"coredns": map[string]string{
			"status":  "healthy",
			"message": "CoreDNS is running",
		},
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
