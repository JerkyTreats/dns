package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
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
	"github.com/jerkytreats/dns/internal/logging"
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
	// Initialize logger
	logging.Info("Application starting...")
	defer logging.Sync()

	// Initialize CoreDNS manager from config
	configPath := config.GetString("dns.coredns.config_path")
	zonesPath := config.GetString("dns.coredns.zones_path")
	reloadCmd := config.GetStringSlice("dns.coredns.reload_command")
	domain := config.GetString(coredns.DNSDomainKey)
	manager := coredns.NewManager(configPath, zonesPath, reloadCmd, domain)

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

	response := map[string]interface{}{
		"status":  "healthy",
		"version": config.GetString(AppVersionKey),
		"components": map[string]interface{}{
			"api": map[string]string{
				"status":  "healthy",
				"message": "API is running",
			},
			"coredns": map[string]string{
				"status":  "healthy",
				"message": "CoreDNS is running",
			},
		},
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
