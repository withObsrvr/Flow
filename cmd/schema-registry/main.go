package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/withObsrvr/Flow/pkg/schemaapi"
)

// SchemaRegistry stores and manages GraphQL schema components
type SchemaRegistry struct {
	schemas    map[string]string
	queries    map[string]string
	mutex      sync.RWMutex
	httpServer *http.Server
}

// NewSchemaRegistry creates a new schema registry service
func NewSchemaRegistry(port string) *SchemaRegistry {
	registry := &SchemaRegistry{
		schemas: make(map[string]string),
		queries: make(map[string]string),
	}

	// Set up HTTP handlers
	mux := http.NewServeMux()
	mux.HandleFunc("/register", registry.handleRegister)
	mux.HandleFunc("/schema", registry.handleGetSchema)
	mux.HandleFunc("/health", registry.handleHealth)

	registry.httpServer = &http.Server{
		Addr:    ":" + port,
		Handler: mux,
	}

	return registry
}

// Start begins the schema registry service
func (sr *SchemaRegistry) Start() error {
	log.Printf("Starting Schema Registry on %s", sr.httpServer.Addr)
	return sr.httpServer.ListenAndServe()
}

// Stop gracefully shuts down the schema registry service
func (sr *SchemaRegistry) Stop() error {
	log.Println("Shutting down Schema Registry...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return sr.httpServer.Shutdown(ctx)
}

// handleRegister processes schema registration requests
func (sr *SchemaRegistry) handleRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var registration schemaapi.SchemaRegistration

	if err := json.NewDecoder(r.Body).Decode(&registration); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	sr.mutex.Lock()
	sr.schemas[registration.PluginName] = registration.Schema
	sr.queries[registration.PluginName] = registration.Queries
	sr.mutex.Unlock()

	log.Printf("Registered schema for plugin: %s", registration.PluginName)

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "success"})
}

// handleGetSchema returns the complete GraphQL schema
func (sr *SchemaRegistry) handleGetSchema(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	sr.mutex.RLock()
	defer sr.mutex.RUnlock()

	// Compose the full schema
	var schemaBuilder strings.Builder
	var queryBuilder strings.Builder

	for _, schema := range sr.schemas {
		schemaBuilder.WriteString(schema)
		schemaBuilder.WriteString("\n")
	}

	for _, query := range sr.queries {
		queryBuilder.WriteString(query)
		queryBuilder.WriteString("\n")
	}

	fullSchema := fmt.Sprintf(`
%s

type Query {
%s
}
`, schemaBuilder.String(), queryBuilder.String())

	w.Header().Set("Content-Type", "application/graphql")
	w.Write([]byte(fullSchema))
}

// handleHealth provides a health check endpoint
func (sr *SchemaRegistry) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "healthy",
		"time":   time.Now().UTC().Format(time.RFC3339),
	})
}

func main() {
	// Parse command line flags
	port := "8081"
	if len(os.Args) > 1 {
		port = os.Args[1]
	}

	// Create and start the schema registry
	registry := NewSchemaRegistry(port)

	// Handle graceful shutdown
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	go func() {
		if err := registry.Start(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Error starting schema registry: %v", err)
		}
	}()

	// Wait for shutdown signal
	<-stop

	// Gracefully shutdown
	if err := registry.Stop(); err != nil {
		log.Fatalf("Error shutting down schema registry: %v", err)
	}

	log.Println("Schema Registry stopped")
}
