package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/withObsrvr/Flow/internal/flow"
)

func main() {
	// Add command-line flags
	instanceID := flag.String("instance-id", "", "Unique ID for this pipeline instance")
	tenantID := flag.String("tenant-id", "", "ID of the tenant who owns this pipeline")
	apiKey := flag.String("api-key", "", "API key for authenticating with Obsrvr platform")
	callbackURL := flag.String("callback-url", "", "URL to send status updates to")
	pluginsDir := flag.String("plugins", "./plugins", "Directory containing plugin .so files")
	pipelineConfigFile := flag.String("pipeline", "pipeline_example.yaml", "Path to the pipeline configuration YAML file")
	schemaRegistryURL := flag.String("schema-registry", "http://localhost:8081", "URL of the schema registry service")
	flag.Parse()

	// Validate required parameters
	if *instanceID == "" || *tenantID == "" || *apiKey == "" {
		log.Fatalf("instance-id, tenant-id, and api-key are required")
	}

	// Initialize instance config
	instanceConfig := flow.InstanceConfig{
		InstanceID:  *instanceID,
		TenantID:    *tenantID,
		APIKey:      *apiKey,
		CallbackURL: *callbackURL,
	}

	// Start Prometheus metrics endpoint
	metricsAddr := ":2112"
	go func() {
		log.Printf("Starting metrics server on %s", metricsAddr)
		http.Handle("/metrics", promhttp.Handler())
		if err := http.ListenAndServe(metricsAddr, nil); err != nil {
			log.Printf("Error starting metrics server: %v", err)
		}
	}()

	// Give the metrics server a moment to start
	time.Sleep(time.Second)

	// Create the core engine
	engine, err := flow.NewCoreEngine(*pluginsDir, *pipelineConfigFile)
	if err != nil {
		log.Fatalf("Error initializing core engine: %v", err)
	}
	engine.InstanceConfig = instanceConfig

	// Register schemas with the schema registry if URL is provided
	if *schemaRegistryURL != "" {
		if err := flow.RegisterSchemas(engine, *schemaRegistryURL); err != nil {
			log.Printf("Warning: Failed to register schemas with registry: %v", err)
		}
	}

	// Set up signal handling for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start config watcher
	go engine.StartConfigWatcher(ctx)

	// Handle OS signals for graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Wait for termination signal
	<-sigCh
	log.Println("Received shutdown signal")

	// Perform graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := engine.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("Error during shutdown: %v", err)
	}

	log.Println("Shutdown complete")
}
