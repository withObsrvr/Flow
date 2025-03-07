package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/graphql-go/graphql"
	"github.com/graphql-go/handler"
)

// GraphQLAPI represents the GraphQL API service
type GraphQLAPI struct {
	schemaRegistryURL string
	httpServer        *http.Server
	schema            *graphql.Schema
}

// NewGraphQLAPI creates a new GraphQL API service
func NewGraphQLAPI(port, schemaRegistryURL string) *GraphQLAPI {
	api := &GraphQLAPI{
		schemaRegistryURL: schemaRegistryURL,
	}

	// Set up HTTP handlers
	mux := http.NewServeMux()
	mux.HandleFunc("/health", api.handleHealth)

	// The GraphQL handler will be set up after fetching the schema

	api.httpServer = &http.Server{
		Addr:    ":" + port,
		Handler: mux,
	}

	return api
}

// Start begins the GraphQL API service
func (api *GraphQLAPI) Start() error {
	// Fetch the schema from the registry
	schema, err := api.fetchSchema()
	if err != nil {
		return err
	}

	// Parse the schema
	schemaConfig := graphql.SchemaConfig{
		Query: graphql.NewObject(graphql.ObjectConfig{
			Name: "Query",
			Fields: graphql.Fields{
				// Default fields can be added here
				"_version": &graphql.Field{
					Type: graphql.String,
					Resolve: func(p graphql.ResolveParams) (interface{}, error) {
						return "1.0.0", nil
					},
				},
			},
		}),
	}

	// Add schema from registry if available
	if schema != "" {
		log.Printf("Received schema from registry: %s", schema)
		// In a real implementation, you would parse the schema string
		// and add it to the schemaConfig
	}

	parsedSchema, err := graphql.NewSchema(schemaConfig)
	if err != nil {
		return err
	}

	api.schema = &parsedSchema

	// Set up GraphQL handler
	h := handler.New(&handler.Config{
		Schema:   &parsedSchema,
		Pretty:   true,
		GraphiQL: true,
	})

	// Add GraphQL handler to mux
	api.httpServer.Handler.(*http.ServeMux).Handle("/graphql", h)

	log.Printf("Starting GraphQL API on %s", api.httpServer.Addr)
	return api.httpServer.ListenAndServe()
}

// Stop gracefully shuts down the GraphQL API service
func (api *GraphQLAPI) Stop() error {
	log.Println("Shutting down GraphQL API...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return api.httpServer.Shutdown(ctx)
}

// fetchSchema retrieves the schema from the schema registry
func (api *GraphQLAPI) fetchSchema() (string, error) {
	resp, err := http.Get(api.schemaRegistryURL + "/schema")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to fetch schema, status: %d", resp.StatusCode)
	}

	schema, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return string(schema), nil
}

// handleHealth provides a health check endpoint
func (api *GraphQLAPI) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "healthy",
		"time":   time.Now().UTC().Format(time.RFC3339),
	})
}

func main() {
	// Parse command line arguments
	port := "8080"
	schemaRegistryURL := "http://localhost:8081"

	if len(os.Args) > 1 {
		port = os.Args[1]
	}

	if len(os.Args) > 2 {
		schemaRegistryURL = os.Args[2]
	}

	// Create and start the GraphQL API
	api := NewGraphQLAPI(port, schemaRegistryURL)

	// Handle graceful shutdown
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	go func() {
		if err := api.Start(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Error starting GraphQL API: %v", err)
		}
	}()

	// Wait for shutdown signal
	<-stop

	// Gracefully shutdown
	if err := api.Stop(); err != nil {
		log.Fatalf("Error shutting down GraphQL API: %v", err)
	}

	log.Println("GraphQL API stopped")
}
