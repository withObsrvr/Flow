package main

import (
	"context"
	"database/sql"
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

	_ "github.com/mattn/go-sqlite3" // Import SQLite driver
	"github.com/withObsrvr/Flow/pkg/schemaapi"
	"gopkg.in/yaml.v3"
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

// GenerateDynamicSchemas generates GraphQL schemas from database tables
func (sr *SchemaRegistry) GenerateDynamicSchemas(dbPath string) error {
	// Check if the database file exists
	_, err := os.Stat(dbPath)
	if os.IsNotExist(err) {
		log.Printf("Database file %s does not exist yet, will check again later", dbPath)

		// Start a goroutine to periodically check for the database
		go sr.watchForDatabase(dbPath)
		return nil
	} else if err != nil {
		return fmt.Errorf("failed to check database file: %w", err)
	}

	// Connect to the SQLite database
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	defer db.Close()

	// Get a list of all tables
	rows, err := db.Query("SELECT name FROM sqlite_master WHERE type='table'")
	if err != nil {
		return fmt.Errorf("failed to query tables: %w", err)
	}
	defer rows.Close()

	// Process each table
	for rows.Next() {
		var tableName string
		if err := rows.Scan(&tableName); err != nil {
			return fmt.Errorf("failed to scan table name: %w", err)
		}

		// Skip internal SQLite tables
		if strings.HasPrefix(tableName, "sqlite_") {
			continue
		}

		log.Printf("Generating schema for table: %s", tableName)

		// Get table schema
		schemaRows, err := db.Query(fmt.Sprintf("PRAGMA table_info(%s)", tableName))
		if err != nil {
			return fmt.Errorf("failed to get schema for table %s: %w", tableName, err)
		}

		// Generate GraphQL type
		var typeBuilder strings.Builder
		typeBuilder.WriteString(fmt.Sprintf("type %s {\n", pascalCase(tableName)))

		// Track fields for queries
		var fields []string
		var primaryKey string

		// Process each column
		for schemaRows.Next() {
			var cid int
			var name, typeName string
			var notNull, pk int
			var dfltValue interface{}

			if err := schemaRows.Scan(&cid, &name, &typeName, &notNull, &dfltValue, &pk); err != nil {
				schemaRows.Close()
				return fmt.Errorf("failed to scan column info: %w", err)
			}

			// Convert SQLite type to GraphQL type
			gqlType := "String"
			switch strings.ToUpper(typeName) {
			case "INTEGER", "NUMERIC", "REAL":
				gqlType = "Int"
			case "BOOLEAN":
				gqlType = "Boolean"
			}

			// Add non-null if required
			if notNull == 1 {
				gqlType += "!"
			}

			// Add field to type definition
			typeBuilder.WriteString(fmt.Sprintf("  %s: %s\n", camelCase(name), gqlType))

			// Track field for queries
			fields = append(fields, name)

			// Track primary key
			if pk == 1 {
				primaryKey = name
			}
		}
		schemaRows.Close()

		typeBuilder.WriteString("}\n\n")

		// Register the type schema
		sr.mutex.Lock()
		sr.schemas[tableName] = typeBuilder.String()
		sr.mutex.Unlock()

		// Generate queries if we have a primary key
		if primaryKey != "" {
			var queryBuilder strings.Builder

			// Single item query
			queryBuilder.WriteString(fmt.Sprintf("%s(%s: %s!): %s\n",
				camelCase(tableName),
				camelCase(primaryKey),
				sqlTypeToGraphQL(db, tableName, primaryKey),
				pascalCase(tableName)))

			// List query
			queryBuilder.WriteString(fmt.Sprintf("%s(first: Int, after: String): [%s!]!\n",
				pluralize(camelCase(tableName)),
				pascalCase(tableName)))

			sr.mutex.Lock()
			sr.queries[tableName] = queryBuilder.String()
			sr.mutex.Unlock()
		}
	}

	return nil
}

// watchForDatabase periodically checks for the database file and generates schemas when it exists
func (sr *SchemaRegistry) watchForDatabase(dbPath string) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// Check if the database file exists now
			_, err := os.Stat(dbPath)
			if os.IsNotExist(err) {
				log.Printf("Database file %s still does not exist, waiting...", dbPath)
				continue
			} else if err != nil {
				log.Printf("Error checking database file: %v", err)
				continue
			}

			// Database exists, try to generate schemas
			log.Printf("Database file %s now exists, generating schemas", dbPath)
			if err := sr.GenerateDynamicSchemas(dbPath); err != nil {
				log.Printf("Error generating schemas: %v", err)
				continue
			}

			// Success, stop watching
			log.Printf("Successfully generated schemas from database %s", dbPath)
			return
		}
	}
}

// Helper functions for schema generation
func pascalCase(s string) string {
	// Convert snake_case to PascalCase
	parts := strings.Split(s, "_")
	for i := range parts {
		if len(parts[i]) > 0 {
			parts[i] = strings.ToUpper(parts[i][:1]) + parts[i][1:]
		}
	}
	return strings.Join(parts, "")
}

func camelCase(s string) string {
	// Convert snake_case to camelCase
	pascal := pascalCase(s)
	if len(pascal) > 0 {
		return strings.ToLower(pascal[:1]) + pascal[1:]
	}
	return ""
}

func pluralize(s string) string {
	// Simple pluralization
	if strings.HasSuffix(s, "y") {
		return s[:len(s)-1] + "ies"
	}
	return s + "s"
}

func sqlTypeToGraphQL(db *sql.DB, table, column string) string {
	// Get column type
	var typeName string
	row := db.QueryRow(fmt.Sprintf("SELECT type FROM pragma_table_info('%s') WHERE name='%s'", table, column))
	if err := row.Scan(&typeName); err != nil {
		return "String"
	}

	// Convert SQLite type to GraphQL type
	switch strings.ToUpper(typeName) {
	case "INTEGER", "NUMERIC", "REAL":
		return "Int"
	case "BOOLEAN":
		return "Boolean"
	default:
		return "String"
	}
}

// Add this new method to extract the database path from pipeline config
func findDatabasePathInConfig(pipelineConfigFile string) (string, error) {
	// Read the pipeline configuration file
	data, err := os.ReadFile(pipelineConfigFile)
	if err != nil {
		return "", fmt.Errorf("failed to read pipeline config file: %w", err)
	}

	// Define a struct to parse the YAML configuration
	type PipelineConfig struct {
		Pipelines map[string]struct {
			Source struct {
				Type   string                 `yaml:"type"`
				Config map[string]interface{} `yaml:"config"`
			} `yaml:"source"`
			Processors []struct {
				Type   string                 `yaml:"type"`
				Config map[string]interface{} `yaml:"config"`
			} `yaml:"processors"`
			Consumers []struct {
				Type   string                 `yaml:"type"`
				Config map[string]interface{} `yaml:"config"`
			} `yaml:"consumers"`
		} `yaml:"pipelines"`
	}

	// Parse the YAML configuration
	var config PipelineConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return "", fmt.Errorf("failed to parse pipeline config: %w", err)
	}

	// Look for a SQLite consumer in any pipeline
	for pipelineName, pipeline := range config.Pipelines {
		for _, consumer := range pipeline.Consumers {
			// Check if this is a SQLite consumer
			if strings.Contains(strings.ToLower(consumer.Type), "sqlite") {
				log.Printf("Found SQLite consumer in pipeline %s", pipelineName)

				// Get the database path from the consumer config
				if dbPath, ok := consumer.Config["db_path"].(string); ok {
					log.Printf("Using database path from config: %s", dbPath)
					return dbPath, nil
				}
			}
		}
	}

	// Default to flow_data.db if no SQLite consumer found
	return "flow_data.db", nil
}

func main() {
	// Parse command line flags
	port := "8081"
	dbPath := ""
	pipelineConfigFile := ""

	if len(os.Args) > 1 {
		port = os.Args[1]
	}

	if len(os.Args) > 2 {
		// The second argument can be either a direct DB path or a pipeline config file
		arg := os.Args[2]

		// Check if the argument is a YAML file (likely a pipeline config)
		if strings.HasSuffix(arg, ".yaml") || strings.HasSuffix(arg, ".yml") {
			pipelineConfigFile = arg
			log.Printf("Using pipeline config file: %s", pipelineConfigFile)

			// Extract the database path from the pipeline config
			extractedPath, err := findDatabasePathInConfig(pipelineConfigFile)
			if err != nil {
				log.Printf("Warning: Failed to extract database path from config: %v", err)
				log.Printf("Using default database path: flow_data.db")
				dbPath = "flow_data.db"
			} else {
				dbPath = extractedPath
			}
		} else {
			// Assume it's a direct database path
			dbPath = arg
		}
	}

	// Create and start the schema registry
	registry := NewSchemaRegistry(port)

	// Generate dynamic schemas if database path is provided
	if dbPath != "" {
		log.Printf("Generating dynamic schemas from database: %s", dbPath)
		if err := registry.GenerateDynamicSchemas(dbPath); err != nil {
			log.Printf("Warning: Failed to generate dynamic schemas: %v", err)
		}
	}

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
