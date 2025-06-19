package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jerkytreats/dns/internal/api/handler"
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
)

func init() {
	config.RegisterRequiredKey(AppVersionKey)
	config.RegisterRequiredKey(ServerHostKey)
	config.RegisterRequiredKey(ServerPortKey)
	config.RegisterRequiredKey(ServerReadTimeoutKey)
	config.RegisterRequiredKey(ServerWriteTimeoutKey)
	config.RegisterRequiredKey(ServerIdleTimeoutKey)
}

func main() {
	// Initialize logger
	logging.Info("Application starting...")
	defer logging.Sync()

	// Load configuration
	// This is now handled by the config package's init functions
	// We can directly use the config getters.

	// Initialize CoreDNS manager from config
	configPath := config.GetString("dns.coredns.config_path")
	zonesPath := config.GetString("dns.coredns.zones_path")
	reloadCmd := config.GetStringSlice("dns.coredns.reload_command")
	manager := coredns.NewManager(configPath, zonesPath, reloadCmd)

	// Create record handler
	recordHandler := handler.NewRecordHandler(manager)

	// Create server
	server := &http.Server{
		Addr:         fmt.Sprintf("%s:%d", config.GetString(ServerHostKey), config.GetInt(ServerPortKey)),
		ReadTimeout:  config.GetDuration(ServerReadTimeoutKey),
		WriteTimeout: config.GetDuration(ServerWriteTimeoutKey),
		IdleTimeout:  config.GetDuration(ServerIdleTimeoutKey),
	}

	// Setup routes
	mux := http.NewServeMux()
	mux.HandleFunc("/health", healthCheckHandler)
	mux.HandleFunc("/add-record", recordHandler.AddRecord)
	server.Handler = mux

	// Start server in a goroutine
	go func() {
		logging.Info("Starting server at %s:%d",
			config.GetString(ServerHostKey),
			config.GetInt(ServerPortKey))
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
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

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}
