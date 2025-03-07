package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/graphql-go/graphql"
	"github.com/graphql-go/handler"
	_ "github.com/mattn/go-sqlite3"
	"gopkg.in/yaml.v2"
)

// PipelineConfig represents the structure of a pipeline configuration file
type PipelineConfig struct {
	Pipelines map[string]PipelineDefinition `yaml:"pipelines"`
}

// PipelineDefinition represents a single pipeline definition
type PipelineDefinition struct {
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
}

// GraphQLAPI represents the GraphQL API service
type GraphQLAPI struct {
	schemaRegistryURL  string
	httpServer         *http.Server
	schema             *graphql.Schema
	db                 *sql.DB
	pipelineConfigFile string
	mutex              *sync.Mutex
}

// NewGraphQLAPI creates a new GraphQL API service
func NewGraphQLAPI(port, schemaRegistryURL, pipelineConfigFile string) *GraphQLAPI {
	// Create a new HTTP server
	mux := http.NewServeMux()
	server := &http.Server{
		Addr:    ":" + port,
		Handler: mux,
	}

	// Add a health check endpoint
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	return &GraphQLAPI{
		schemaRegistryURL:  schemaRegistryURL,
		httpServer:         server,
		pipelineConfigFile: pipelineConfigFile,
		mutex:              &sync.Mutex{},
	}
}

// Start begins the GraphQL API service
func (api *GraphQLAPI) Start() error {
	// Find the SQLite database path from the pipeline configuration
	dbPath, err := api.findSQLiteConsumer()
	if err != nil {
		log.Printf("Warning: Failed to find SQLite consumer in pipeline config: %v", err)
		log.Printf("Using default database path: flow_data_soroswap_2.db")
		dbPath = "flow_data_soroswap_2.db"
	}

	// Connect to the SQLite database
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	api.db = db

	// Test the connection
	if err := db.Ping(); err != nil {
		return fmt.Errorf("failed to ping database: %w", err)
	}
	log.Printf("Connected to SQLite database: %s", dbPath)

	// Log database tables for debugging
	rows, err := api.db.Query("SELECT name FROM sqlite_master WHERE type='table'")
	if err != nil {
		log.Printf("Warning: Failed to query tables: %v", err)
	} else {
		defer rows.Close()
		log.Printf("Tables in database %s:", dbPath)
		for rows.Next() {
			var tableName string
			if err := rows.Scan(&tableName); err != nil {
				log.Printf("Error scanning table name: %v", err)
				continue
			}
			log.Printf("  Table: %s", tableName)

			// Count rows in the table
			count, err := api.countRows(tableName)
			if err != nil {
				log.Printf("    Error counting rows: %v", err)
			} else {
				log.Printf("    Row count: %d", count)
			}
		}
	}

	// Try to fetch schema from registry
	schemaStr, err := api.fetchSchema()
	if err != nil {
		log.Printf("Warning: Failed to fetch schema from registry: %v", err)
		log.Printf("Will build schema from database structure")
		schemaStr = ""
	} else {
		log.Printf("Successfully fetched schema from registry")
	}

	// Build the schema
	schema, err := api.buildSchema(schemaStr)
	if err != nil {
		return fmt.Errorf("failed to build schema: %w", err)
	}

	// Update the API's schema
	api.mutex.Lock()
	api.schema = schema
	api.mutex.Unlock()

	// Create the GraphQL handler
	h := handler.New(&handler.Config{
		Schema:   schema,
		Pretty:   true,
		GraphiQL: true,
	})

	// Add the GraphQL endpoint
	api.httpServer.Handler.(*http.ServeMux).Handle("/graphql", h)

	// Start the HTTP server
	log.Printf("Starting GraphQL API on %s", api.httpServer.Addr)
	return api.httpServer.ListenAndServe()
}

// Stop gracefully stops the GraphQL API service
func (api *GraphQLAPI) Stop() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := api.httpServer.Shutdown(ctx); err != nil {
		return err
	}

	if api.db != nil {
		if err := api.db.Close(); err != nil {
			return err
		}
	}

	return nil
}

// findSQLiteConsumer finds the SQLite consumer in the pipeline configuration
func (api *GraphQLAPI) findSQLiteConsumer() (string, error) {
	// Check if a database path is provided via environment variable
	if dbPath := os.Getenv("GRAPHQL_API_DB_PATH"); dbPath != "" {
		log.Printf("Using database path from environment variable: %s", dbPath)
		return dbPath, nil
	}

	// Read the pipeline configuration file
	data, err := os.ReadFile(api.pipelineConfigFile)
	if err != nil {
		return "", fmt.Errorf("failed to read pipeline config file: %w", err)
	}

	// Parse the YAML configuration
	var config PipelineConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return "", fmt.Errorf("failed to parse pipeline config: %w", err)
	}

	// Look for a SQLite consumer in any pipeline
	for pipelineName, pipeline := range config.Pipelines {
		for _, consumer := range pipeline.Consumers {
			consumerType := strings.ToLower(consumer.Type)
			// Check for "sqlite" or "soroswap-sqlite" in the consumer type
			if strings.Contains(consumerType, "sqlite") {
				// Found a SQLite consumer
				dbPath, ok := consumer.Config["db_path"].(string)
				if !ok {
					return "", fmt.Errorf("sqlite consumer in pipeline %s has no db_path", pipelineName)
				}
				log.Printf("Found SQLite consumer: %s with db_path: %s", consumer.Type, dbPath)
				return dbPath, nil
			}
		}
	}

	// If no SQLite consumer is found, check if a database path was provided as a command-line argument
	if len(os.Args) > 3 && strings.HasSuffix(os.Args[3], ".db") {
		dbPath := os.Args[3]
		log.Printf("Using database path from command line: %s", dbPath)
		return dbPath, nil
	}

	// As a last resort, try a hardcoded path
	if _, err := os.Stat("flow_data_soroswap_2.db"); err == nil {
		log.Printf("Using existing database: flow_data_soroswap_2.db")
		return "flow_data_soroswap_2.db", nil
	}

	return "", fmt.Errorf("no sqlite consumer found in any pipeline")
}

// fetchSchema retrieves the schema from the schema registry
func (api *GraphQLAPI) fetchSchema() (string, error) {
	// Try to connect to the schema registry with retries
	var resp *http.Response
	var err error

	maxRetries := 3
	retryDelay := 2 * time.Second

	for i := 0; i < maxRetries; i++ {
		resp, err = http.Get(api.schemaRegistryURL + "/schema")
		if err == nil && resp.StatusCode == http.StatusOK {
			break
		}

		if err != nil {
			log.Printf("Attempt %d: Failed to connect to schema registry: %v", i+1, err)
		} else {
			log.Printf("Attempt %d: Schema registry returned status: %d", i+1, resp.StatusCode)
			resp.Body.Close()
		}

		if i < maxRetries-1 {
			log.Printf("Retrying in %v...", retryDelay)
			time.Sleep(retryDelay)
		}
	}

	if err != nil {
		return "", fmt.Errorf("failed to connect to schema registry after %d attempts: %w", maxRetries, err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("schema registry returned non-OK status: %d", resp.StatusCode)
	}

	defer resp.Body.Close()
	schema, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read schema response: %w", err)
	}

	return string(schema), nil
}

// buildSchema builds a GraphQL schema from the registry schema or database structure
func (api *GraphQLAPI) buildSchema(registrySchema string) (*graphql.Schema, error) {
	// Create a map to hold all the fields for the root query
	fields := graphql.Fields{}

	// Add a simple health check query
	fields["health"] = &graphql.Field{
		Type: graphql.String,
		Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			return "OK", nil
		},
	}

	// If we have a schema from the registry, try to use it
	if registrySchema != "" {
		// TODO: Parse and use the registry schema
		// For now, we'll just build from the database
		log.Printf("Registry schema support not yet implemented, building from database")
	}

	// Build schema from database structure
	if err := api.buildSchemaFromDatabase(fields); err != nil {
		return nil, fmt.Errorf("failed to build schema from database: %w", err)
	}

	// Create the schema
	schemaConfig := graphql.SchemaConfig{
		Query: graphql.NewObject(graphql.ObjectConfig{
			Name:   "Query",
			Fields: fields,
		}),
	}

	schema, err := graphql.NewSchema(schemaConfig)
	if err != nil {
		return nil, err
	}

	return &schema, nil
}

// buildSchemaFromDatabase builds a GraphQL schema from the database structure
func (api *GraphQLAPI) buildSchemaFromDatabase(fields graphql.Fields) error {
	// Get all tables in the database
	rows, err := api.db.Query("SELECT name FROM sqlite_master WHERE type='table'")
	if err != nil {
		return fmt.Errorf("failed to query tables: %w", err)
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var tableName string
		if err := rows.Scan(&tableName); err != nil {
			return fmt.Errorf("failed to scan table name: %w", err)
		}
		tables = append(tables, tableName)
	}

	// Process each table
	for _, tableName := range tables {
		// Skip internal tables
		if tableName == "sqlite_sequence" || tableName == "flow_metadata" {
			continue
		}

		log.Printf("Building schema for table: %s", tableName)

		// Get table schema
		schemaRows, err := api.db.Query(fmt.Sprintf("PRAGMA table_info(%s)", tableName))
		if err != nil {
			log.Printf("Error getting schema for table %s: %v", tableName, err)
			continue
		}

		// Create fields for the type
		typeFields := graphql.Fields{}
		var columns []string
		var columnTypes []string

		for schemaRows.Next() {
			var cid int
			var name, typeName string
			var notNull, pk int
			var defaultValue interface{}

			if err := schemaRows.Scan(&cid, &name, &typeName, &notNull, &defaultValue, &pk); err != nil {
				log.Printf("Error scanning column info: %v", err)
				continue
			}

			columns = append(columns, name)
			columnTypes = append(columnTypes, typeName)

			// Map SQL type to GraphQL type
			var fieldType graphql.Type
			switch strings.ToUpper(typeName) {
			case "INTEGER", "INT", "SMALLINT", "MEDIUMINT", "BIGINT":
				fieldType = graphql.Int
			case "REAL", "FLOAT", "DOUBLE", "NUMERIC", "DECIMAL":
				fieldType = graphql.Float
			case "TEXT", "VARCHAR", "CHAR", "CLOB":
				fieldType = graphql.String
			case "BOOLEAN":
				fieldType = graphql.Boolean
			default:
				fieldType = graphql.String // Default to string for unknown types
			}

			// Make non-nullable if required
			if notNull == 1 && pk == 0 { // Primary keys can be auto-increment, so they might appear null initially
				fieldType = graphql.NewNonNull(fieldType)
			}

			typeFields[name] = &graphql.Field{
				Type: fieldType,
			}
		}
		schemaRows.Close()

		// Create the type
		typeName := api.pascalCase(api.singularize(tableName))
		objType := graphql.NewObject(graphql.ObjectConfig{
			Name:   typeName,
			Fields: typeFields,
		})

		// Create query for getting a single item by ID
		singleQueryName := api.camelCase(api.singularize(tableName))
		idField := api.findIdField(columns)
		if idField != "" {
			fields[singleQueryName] = &graphql.Field{
				Type: objType,
				Args: graphql.FieldConfigArgument{
					idField: &graphql.ArgumentConfig{
						Type: graphql.NewNonNull(graphql.String),
					},
				},
				Resolve: api.createSingleItemResolver(tableName, columns, idField),
			}
		}

		// Create query for getting a list of items
		listQueryName := api.camelCase(api.pluralize(tableName))
		fields[listQueryName] = &graphql.Field{
			Type: graphql.NewList(objType),
			Args: graphql.FieldConfigArgument{
				"first": &graphql.ArgumentConfig{
					Type:         graphql.Int,
					DefaultValue: 10,
					Description:  "Number of items to return",
				},
				"after": &graphql.ArgumentConfig{
					Type:        graphql.String,
					Description: "Cursor for pagination",
				},
			},
			Resolve: api.createListResolver(tableName, columns, idField),
		}
	}

	return nil
}

// createSingleItemResolver creates a resolver for a single item query
func (api *GraphQLAPI) createSingleItemResolver(tableName string, columns []string, idField string) graphql.FieldResolveFn {
	return func(p graphql.ResolveParams) (interface{}, error) {
		id, ok := p.Args[idField].(string)
		if !ok {
			return nil, fmt.Errorf("invalid ID argument")
		}

		log.Printf("Querying %s with %s=%s", tableName, idField, id)

		query := fmt.Sprintf("SELECT * FROM %s WHERE %s = ?", tableName, idField)
		row := api.db.QueryRow(query, id)

		// Create a map to hold the result
		result := make(map[string]interface{})
		scanArgs := make([]interface{}, len(columns))
		scanValues := make([]interface{}, len(columns))
		for i := range columns {
			scanValues[i] = new(interface{})
			scanArgs[i] = scanValues[i]
		}

		if err := row.Scan(scanArgs...); err != nil {
			if err == sql.ErrNoRows {
				return nil, nil
			}
			return nil, fmt.Errorf("error scanning row: %w", err)
		}

		// Populate the result map
		for i, col := range columns {
			val := *(scanValues[i].(*interface{}))
			if val == nil {
				result[col] = nil
				continue
			}

			// Handle different types
			switch v := val.(type) {
			case []byte:
				// Try to convert to string
				result[col] = string(v)
			default:
				result[col] = v
			}
		}

		return result, nil
	}
}

// createListResolver creates a resolver for a list query
func (api *GraphQLAPI) createListResolver(tableName string, columns []string, idField string) graphql.FieldResolveFn {
	return func(p graphql.ResolveParams) (interface{}, error) {
		limit, _ := p.Args["first"].(int)
		if limit <= 0 {
			limit = 10
		}

		after, _ := p.Args["after"].(string)

		log.Printf("Querying %s with limit=%d, after=%s", tableName, limit, after)

		query := fmt.Sprintf("SELECT * FROM %s", tableName)
		args := []interface{}{}

		if after != "" && idField != "" {
			query += fmt.Sprintf(" WHERE %s > ?", idField)
			args = append(args, after)
		}

		if idField != "" {
			query += fmt.Sprintf(" ORDER BY %s", idField)
		}

		query += " LIMIT ?"
		args = append(args, limit)

		rows, err := api.db.Query(query, args...)
		if err != nil {
			return nil, fmt.Errorf("error querying database: %w", err)
		}
		defer rows.Close()

		var results []interface{}
		for rows.Next() {
			// Create a map to hold the result
			result := make(map[string]interface{})
			scanArgs := make([]interface{}, len(columns))
			scanValues := make([]interface{}, len(columns))
			for i := range columns {
				scanValues[i] = new(interface{})
				scanArgs[i] = scanValues[i]
			}

			if err := rows.Scan(scanArgs...); err != nil {
				return nil, fmt.Errorf("error scanning row: %w", err)
			}

			// Populate the result map
			for i, col := range columns {
				val := *(scanValues[i].(*interface{}))
				if val == nil {
					result[col] = nil
					continue
				}

				// Handle different types
				switch v := val.(type) {
				case []byte:
					// Try to convert to string
					result[col] = string(v)
				default:
					result[col] = v
				}
			}

			results = append(results, result)
		}

		return results, nil
	}
}

// Helper functions

// countRows counts the number of rows in a table
func (api *GraphQLAPI) countRows(tableName string) (int, error) {
	row := api.db.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM %s", tableName))
	var count int
	err := row.Scan(&count)
	return count, err
}

// findIdField finds the ID field in a list of columns
func (api *GraphQLAPI) findIdField(columns []string) string {
	// Common ID field names
	idFields := []string{"id", "account_id", "sequence", "hash"}

	for _, field := range idFields {
		for _, col := range columns {
			if strings.EqualFold(col, field) {
				return col
			}
		}
	}

	return ""
}

// singularize converts a plural word to singular
func (api *GraphQLAPI) singularize(s string) string {
	if strings.HasSuffix(s, "ies") {
		return s[:len(s)-3] + "y"
	}
	if strings.HasSuffix(s, "s") && !strings.HasSuffix(s, "ss") {
		return s[:len(s)-1]
	}
	return s
}

// pluralize converts a singular word to plural
func (api *GraphQLAPI) pluralize(s string) string {
	if strings.HasSuffix(s, "y") {
		return s[:len(s)-1] + "ies"
	}
	if !strings.HasSuffix(s, "s") {
		return s + "s"
	}
	return s
}

// pascalCase converts a string to PascalCase
func (api *GraphQLAPI) pascalCase(s string) string {
	words := strings.FieldsFunc(s, func(r rune) bool {
		return r == '_' || r == '-' || r == ' '
	})

	for i, word := range words {
		if len(word) > 0 {
			words[i] = strings.ToUpper(word[:1]) + word[1:]
		}
	}

	return strings.Join(words, "")
}

// camelCase converts a string to camelCase
func (api *GraphQLAPI) camelCase(s string) string {
	pascal := api.pascalCase(s)
	if len(pascal) > 0 {
		return strings.ToLower(pascal[:1]) + pascal[1:]
	}
	return ""
}

func main() {
	// Parse command line arguments
	port := flag.String("port", "8080", "Port to listen on")
	schemaRegistryURL := flag.String("schema-registry", "http://localhost:8081", "URL of the schema registry")
	pipelineConfigFile := flag.String("pipeline-config", "", "Path to the pipeline configuration file")
	dbPath := flag.String("db-path", "", "Path to the SQLite database file")
	flag.Parse()

	// Check for positional arguments if flags are not provided
	args := flag.Args()
	if *port == "8080" && len(args) > 0 {
		*port = args[0]
	}
	if *schemaRegistryURL == "http://localhost:8081" && len(args) > 1 {
		*schemaRegistryURL = args[1]
	}
	if *pipelineConfigFile == "" && len(args) > 2 {
		*pipelineConfigFile = args[2]
	}
	if *dbPath == "" && len(args) > 3 {
		*dbPath = args[3]
	}

	// Check if pipeline config file is provided
	if *pipelineConfigFile == "" {
		log.Fatal("Pipeline configuration file is required")
	}

	// Log the arguments for debugging
	log.Printf("Starting GraphQL API with port=%s, schema-registry=%s, pipeline-config=%s, db-path=%s",
		*port, *schemaRegistryURL, *pipelineConfigFile, *dbPath)

	// Create the GraphQL API
	api := NewGraphQLAPI(*port, *schemaRegistryURL, *pipelineConfigFile)

	// If a database path is provided, set it as an environment variable for the findSQLiteConsumer method
	if *dbPath != "" {
		os.Setenv("GRAPHQL_API_DB_PATH", *dbPath)
	}

	// Set up signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start the API in a goroutine
	errChan := make(chan error, 1)
	go func() {
		errChan <- api.Start()
	}()

	// Wait for signal or error
	select {
	case err := <-errChan:
		if err != nil && err != http.ErrServerClosed {
			log.Fatalf("Error starting GraphQL API: %v", err)
		}
	case sig := <-sigChan:
		log.Printf("Received signal: %v", sig)
		if err := api.Stop(); err != nil {
			log.Fatalf("Error stopping GraphQL API: %v", err)
		}
		log.Println("GraphQL API stopped")
	}
}
