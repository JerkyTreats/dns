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
	// Global managers for health checks and TLS integration
	bootstrapManager *bootstrap.Manager
	configManager    *coredns.ConfigManager
	certManager      *certificate.Manager
	dnsManager       *coredns.Manager
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

	// Step 1: Initialize Dynamic CoreDNS Configuration Manager
	logging.Info("Initializing dynamic CoreDNS configuration management...")
	configPath := config.GetString("dns.coredns.config_path")
	templatePath := config.GetString(coredns.DNSTemplatePathKey)
	if templatePath == "" {
		// Default template path if not configured
		templatePath = "configs/coredns/Corefile.template"
	}
	zonesPath := config.GetString("dns.coredns.zones_path")
	domain := config.GetString(coredns.DNSDomainKey)

	configManager = coredns.NewConfigManager(configPath, templatePath, domain, zonesPath)

	// Step 2: Generate minimal initial Corefile (no TLS, no domain-specific routes)
	logging.Info("Generating initial CoreDNS configuration...")
	if err := configManager.GenerateCorefile(); err != nil {
		logging.Error("Failed to generate initial CoreDNS configuration: %v", err)
		os.Exit(1)
	}

	// Step 3: Initialize CoreDNS manager with ConfigManager integration
	reloadCmd := config.GetStringSlice("dns.coredns.reload_command")
	dnsManager = coredns.NewManager(configPath, zonesPath, reloadCmd, domain)
	dnsManager.SetConfigManager(configManager)

	// Step 4: Wait for CoreDNS to be ready (managed by docker-compose)
	logging.Info("Waiting for CoreDNS to be ready...")
	time.Sleep(5 * time.Second) // Allow docker-compose to start CoreDNS

	// Step 5: Initialize Certificate Manager (if TLS enabled)
	var tlsEnabled = config.GetBool(ServerTLSEnabledKey)
	var certificateReady = false

	if config.GetBool(certificate.CertRenewalEnabledKey) {
		logging.Info("Initializing certificate manager with CoreDNS integration...")
		var err error
		certManager, err = certificate.NewManager()
		if err != nil {
			logging.Error("Failed to create certificate manager: %v", err)
			os.Exit(1)
		}

		// Integrate certificate manager with ConfigManager for TLS enablement
		certManager.SetCoreDNSManager(configManager)
	}

	if config.GetBool(BootstrapEnabledKey) {
		logging.Info("Bootstrap is enabled, initializing dynamic zone bootstrap...")

		// Validate required bootstrap configuration
		apiKey := config.GetString(TailscaleAPIKeyKey)
		tailnet := config.GetString(TailscaleTailnetKey)

		// Create Tailscale client
		baseURL := config.GetString(config.TailscaleBaseURLKey)
		tailscaleClient, err := tailscale.NewClient(apiKey, tailnet, baseURL)
		if err != nil {
			logging.Error("Failed to create Tailscale client: %v", err)
			os.Exit(1)
		}

		// Create bootstrap manager using the ConfigManager-integrated DNS manager
		bootstrapManager, err = bootstrap.NewManager(dnsManager, tailscaleClient)
		if err != nil {
			logging.Error("Failed to create bootstrap manager: %v", err)
			os.Exit(1)
		}

		// Step 7: Add internal domain configuration via ConfigManager
		logging.Info("Adding internal domain to dynamic configuration...")
		if err := configManager.AddDomain(domain, nil); err != nil {
			logging.Error("Failed to add internal domain: %v", err)
			os.Exit(1)
		}

		// Initialize the internal zone and bootstrap devices
		if err := bootstrapManager.EnsureInternalZone(); err != nil {
			logging.Error("Failed to bootstrap internal zone: %v", err)
			logging.Warn("Bootstrap failed, continuing without dynamic zone bootstrap")
			bootstrapManager = nil
		} else {
			logging.Info("Dynamic zone bootstrap completed successfully")
		}
	} else {
		logging.Info("Bootstrap is disabled")
	}

	// Step 8: Check if TLS certificates already exist
	if tlsEnabled {
		certFile := config.GetString(ServerTLSCertFileKey)
		keyFile := config.GetString(ServerTLSKeyFileKey)

		// Check if certificate files exist and are valid
		if _, err := os.Stat(certFile); err == nil {
			if _, err := os.Stat(keyFile); err == nil {
				// Certificate files exist, try to load them to verify they're valid
				if _, err := tls.LoadX509KeyPair(certFile, keyFile); err == nil {
					certificateReady = true
					logging.Info("Valid TLS certificates found, enabling TLS configuration...")

					// Enable TLS in CoreDNS configuration
					if err := configManager.EnableTLS(domain, certFile, keyFile); err != nil {
						logging.Warn("Failed to enable TLS in CoreDNS configuration: %v", err)
					} else {
						logging.Info("TLS enabled in CoreDNS configuration")
					}
				} else {
					logging.Warn("Certificate files exist but are invalid: %v", err)
				}
			}
		}

		// Step 9: Start certificate obtainment if certificates not ready (async)
		if !certificateReady && certManager != nil {
			logging.Info("Starting certificate obtainment process...")
			go func() {
				certDomain := config.GetString(certificate.CertDomainKey)
				if err := certManager.ObtainCertificate(certDomain); err != nil {
					logging.Error("Failed to obtain certificate: %v", err)
				} else {
					logging.Info("Certificate obtained successfully")
				}

				// Start renewal loop
				if config.GetBool(certificate.CertRenewalEnabledKey) {
					logging.Info("Starting certificate renewal loop...")
					certManager.StartRenewalLoop(certDomain)
				}
			}()
		}
	}

	// Create record handler
	recordHandler := handler.NewRecordHandler(dnsManager)

	// Setup routes
	mux := http.NewServeMux()
	mux.HandleFunc("/health", healthCheckHandler)
	mux.HandleFunc("/add-record", recordHandler.AddRecord)

	// Step 10: Create and start API server
	var server *http.Server

	// Start with HTTP server if certificates are not ready, or if TLS is disabled
	if !tlsEnabled || !certificateReady {
		server = &http.Server{
			Addr:         fmt.Sprintf("%s:%d", config.GetString(ServerHostKey), config.GetInt(ServerPortKey)),
			ReadTimeout:  config.GetDuration(ServerReadTimeoutKey),
			WriteTimeout: config.GetDuration(ServerWriteTimeoutKey),
			IdleTimeout:  config.GetDuration(ServerIdleTimeoutKey),
			Handler:      mux,
		}
	} else {
		// Start with TLS server if certificates are ready
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
	}

	// Start server in a goroutine
	serverStarted := make(chan bool, 1)
	go func() {
		logging.Info("Starting server...")
		var err error
		if tlsEnabled && certificateReady {
			certFile := config.GetString(ServerTLSCertFileKey)
			keyFile := config.GetString(ServerTLSKeyFileKey)
			logging.Info("Server starting with TLS on port %d", config.GetInt(ServerTLSPortKey))
			serverStarted <- true
			err = server.ListenAndServeTLS(certFile, keyFile)
		} else {
			logging.Info("Server starting without TLS on port %d", config.GetInt(ServerPortKey))
			serverStarted <- true
			err = server.ListenAndServe()
		}

		if err != nil && err != http.ErrServerClosed {
			logging.Error("Failed to start server: %v", err)
			os.Exit(1)
		}
	}()

	// Wait for server to start
	<-serverStarted
	logging.Info("Server started successfully")

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
