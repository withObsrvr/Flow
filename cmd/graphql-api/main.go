package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
	"github.com/graphql-go/graphql"
	"github.com/graphql-go/graphql/language/ast"
	"github.com/graphql-go/graphql/language/parser"
	"github.com/graphql-go/handler"
	_ "github.com/mattn/go-sqlite3"
	"gopkg.in/yaml.v2"
)

// ErrorCode represents a specific error type
type ErrorCode string

const (
	// Database error codes
	ErrDatabaseConnection ErrorCode = "DATABASE_CONNECTION_ERROR"
	ErrDatabaseQuery      ErrorCode = "DATABASE_QUERY_ERROR"

	// GraphQL error codes
	ErrSchemaBuilding ErrorCode = "SCHEMA_BUILDING_ERROR"
	ErrInvalidQuery   ErrorCode = "INVALID_QUERY"

	// Subscription error codes
	ErrSubscriptionFailed ErrorCode = "SUBSCRIPTION_FAILED"

	// Configuration error codes
	ErrInvalidConfig ErrorCode = "INVALID_CONFIGURATION"

	// General error codes
	ErrInternal   ErrorCode = "INTERNAL_ERROR"
	ErrNotFound   ErrorCode = "NOT_FOUND"
	ErrBadRequest ErrorCode = "BAD_REQUEST"
)

// AppError represents an application error with context
type AppError struct {
	Code    ErrorCode
	Message string
	Err     error
	Context map[string]interface{}
}

// Error implements the error interface
func (e *AppError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %s - %v", e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Unwrap returns the wrapped error
func (e *AppError) Unwrap() error {
	return e.Err
}

// NewError creates a new AppError
func NewError(code ErrorCode, message string, err error) *AppError {
	return &AppError{
		Code:    code,
		Message: message,
		Err:     err,
		Context: make(map[string]interface{}),
	}
}

// WithContext adds context to an AppError
func (e *AppError) WithContext(key string, value interface{}) *AppError {
	e.Context[key] = value
	return e
}

// ToGraphQLError converts an AppError to a GraphQL error response
func (e *AppError) ToGraphQLError() map[string]interface{} {
	extensions := map[string]interface{}{
		"code": e.Code,
	}

	// Add context if available
	if len(e.Context) > 0 {
		extensions["context"] = e.Context
	}

	return map[string]interface{}{
		"message":    e.Message,
		"extensions": extensions,
	}
}

// IsAppError checks if an error is an AppError
func IsAppError(err error) bool {
	var appErr *AppError
	return errors.As(err, &appErr)
}

// GetAppError extracts an AppError from an error
func GetAppError(err error) *AppError {
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr
	}
	return NewError(ErrInternal, "An unexpected error occurred", err)
}

// PubSub is a simple publish-subscribe system for GraphQL subscriptions
type PubSub struct {
	subscribers map[string]map[string]chan interface{}
	mutex       sync.RWMutex
}

// NewPubSub creates a new PubSub instance
func NewPubSub() *PubSub {
	return &PubSub{
		subscribers: make(map[string]map[string]chan interface{}),
	}
}

// Subscribe adds a subscriber for a topic
func (ps *PubSub) Subscribe(topic string, id string) chan interface{} {
	ps.mutex.Lock()
	defer ps.mutex.Unlock()

	if _, ok := ps.subscribers[topic]; !ok {
		ps.subscribers[topic] = make(map[string]chan interface{})
	}

	ch := make(chan interface{}, 1)
	ps.subscribers[topic][id] = ch
	return ch
}

// Unsubscribe removes a subscriber from a topic
func (ps *PubSub) Unsubscribe(topic string, id string) {
	ps.mutex.Lock()
	defer ps.mutex.Unlock()

	if _, ok := ps.subscribers[topic]; !ok {
		return
	}

	if ch, ok := ps.subscribers[topic][id]; ok {
		close(ch)
		delete(ps.subscribers[topic], id)
	}
}

// Publish sends a message to all subscribers of a topic
func (ps *PubSub) Publish(topic string, data interface{}) {
	ps.mutex.RLock()
	defer ps.mutex.RUnlock()

	if _, ok := ps.subscribers[topic]; !ok {
		return
	}

	for _, ch := range ps.subscribers[topic] {
		select {
		case ch <- data:
		default:
			// Channel is full, skip this message
		}
	}
}

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

// Metrics tracks various metrics for the GraphQL API
type Metrics struct {
	StartTime           time.Time
	TotalQueries        uint64
	TotalMutations      uint64
	TotalSubscriptions  uint64
	ActiveSubscriptions int32
	ErrorCount          uint64
	SlowQueries         uint64
	QueryTimes          []time.Duration // Last 100 query times
	mutex               sync.RWMutex
}

// NewMetrics creates a new Metrics instance
func NewMetrics() *Metrics {
	return &Metrics{
		StartTime:  time.Now(),
		QueryTimes: make([]time.Duration, 0, 100),
	}
}

// RecordQuery records a query execution
func (m *Metrics) RecordQuery(duration time.Duration) {
	atomic.AddUint64(&m.TotalQueries, 1)

	// Record slow queries (over 500ms)
	if duration > 500*time.Millisecond {
		atomic.AddUint64(&m.SlowQueries, 1)
	}

	// Record query time
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.QueryTimes = append(m.QueryTimes, duration)
	if len(m.QueryTimes) > 100 {
		m.QueryTimes = m.QueryTimes[1:]
	}
}

// RecordMutation records a mutation execution
func (m *Metrics) RecordMutation() {
	atomic.AddUint64(&m.TotalMutations, 1)
}

// RecordSubscription records a new subscription
func (m *Metrics) RecordSubscription() {
	atomic.AddUint64(&m.TotalSubscriptions, 1)
	atomic.AddInt32(&m.ActiveSubscriptions, 1)
}

// RecordUnsubscribe records an unsubscribe event
func (m *Metrics) RecordUnsubscribe() {
	atomic.AddInt32(&m.ActiveSubscriptions, -1)
}

// RecordError records an error
func (m *Metrics) RecordError() {
	atomic.AddUint64(&m.ErrorCount, 1)
}

// GetMetrics returns the current metrics
func (m *Metrics) GetMetrics() map[string]interface{} {
	uptime := time.Since(m.StartTime).Truncate(time.Second)

	// Calculate average query time
	var avgQueryTime time.Duration
	m.mutex.RLock()
	if len(m.QueryTimes) > 0 {
		var total time.Duration
		for _, t := range m.QueryTimes {
			total += t
		}
		avgQueryTime = total / time.Duration(len(m.QueryTimes))
	}
	m.mutex.RUnlock()

	// Get memory stats
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	return map[string]interface{}{
		"uptime":              uptime.String(),
		"totalQueries":        atomic.LoadUint64(&m.TotalQueries),
		"totalMutations":      atomic.LoadUint64(&m.TotalMutations),
		"totalSubscriptions":  atomic.LoadUint64(&m.TotalSubscriptions),
		"activeSubscriptions": atomic.LoadInt32(&m.ActiveSubscriptions),
		"errorCount":          atomic.LoadUint64(&m.ErrorCount),
		"slowQueries":         atomic.LoadUint64(&m.SlowQueries),
		"avgQueryTime":        avgQueryTime.String(),
		"memoryUsage": map[string]interface{}{
			"alloc":      memStats.Alloc,
			"totalAlloc": memStats.TotalAlloc,
			"sys":        memStats.Sys,
			"numGC":      memStats.NumGC,
		},
	}
}

// GraphQLAPI represents the GraphQL API service
type GraphQLAPI struct {
	schemaRegistryURL     string
	httpServer            *http.Server
	schema                *graphql.Schema
	db                    *sql.DB
	pipelineConfigFile    string
	mutex                 *sync.Mutex
	pubsub                *PubSub
	upgrader              websocket.Upgrader
	metrics               *Metrics
	schemaRefreshInterval time.Duration
	lastSchemaRefresh     time.Time
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

	// Create the WebSocket upgrader
	upgrader := websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin: func(r *http.Request) bool {
			return true // Allow all origins
		},
		Subprotocols: []string{"graphql-ws", "graphql-transport-ws"},
	}

	return &GraphQLAPI{
		schemaRegistryURL:     schemaRegistryURL,
		httpServer:            server,
		pipelineConfigFile:    pipelineConfigFile,
		mutex:                 &sync.Mutex{},
		pubsub:                NewPubSub(),
		upgrader:              upgrader,
		metrics:               NewMetrics(),
		schemaRefreshInterval: 5 * time.Minute, // Refresh schema every 5 minutes
		lastSchemaRefresh:     time.Now(),
	}
}

// Start begins the GraphQL API service
func (api *GraphQLAPI) Start() error {
	// Find the SQLite database path from the pipeline configuration
	dbPath, err := api.findSQLiteConsumer()
	if err != nil {
		log.Printf("Warning: Failed to find SQLite consumer in pipeline config: %v", err)

		// Check if this is a "not found" error
		if appErr, ok := err.(*AppError); ok && appErr.Code == ErrNotFound {
			log.Printf("No SQLite consumer found in pipeline config. GraphQL API will run without database access.")

			// Create a minimal schema with just the health endpoint
			queryType := graphql.NewObject(graphql.ObjectConfig{
				Name: "Query",
				Fields: graphql.Fields{
					"health": &graphql.Field{
						Type: graphql.String,
						Resolve: func(p graphql.ResolveParams) (interface{}, error) {
							return "OK", nil
						},
						Description: "Health check endpoint",
					},
				},
			})

			// Create a minimal subscription type
			subscriptionType := graphql.NewObject(graphql.ObjectConfig{
				Name: "Subscription",
				Fields: graphql.Fields{
					"healthUpdates": &graphql.Field{
						Type: graphql.String,
						Resolve: func(p graphql.ResolveParams) (interface{}, error) {
							return "OK", nil
						},
						Description: "Health check subscription",
					},
				},
			})

			// Create a minimal schema
			schemaConfig := graphql.SchemaConfig{
				Query:        queryType,
				Subscription: subscriptionType,
			}

			schema, err := graphql.NewSchema(schemaConfig)
			if err != nil {
				return fmt.Errorf("failed to create minimal schema: %w", err)
			}

			api.schema = &schema

			// Start the HTTP server without database access
			log.Printf("Starting GraphQL API on :%s", api.httpServer.Addr)
			return api.httpServer.ListenAndServe()
		}

		// For other errors, use the default database path
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

	// Initial schema build
	if err := api.refreshSchema(); err != nil {
		return fmt.Errorf("failed to build initial schema: %w", err)
	}

	// Create the GraphQL handler with the current schema
	h := handler.New(&handler.Config{
		Schema: func() *graphql.Schema {
			return api.getSchema()
		}(),
		Pretty:   true,
		GraphiQL: true,
	})

	// Add the GraphQL endpoint with metrics middleware
	api.httpServer.Handler.(*http.ServeMux).Handle("/graphql", api.metricsMiddleware(h))

	// Add WebSocket endpoint for subscriptions
	api.httpServer.Handler.(*http.ServeMux).HandleFunc("/subscriptions", api.handleWebSocket)

	// Add metrics endpoint
	api.httpServer.Handler.(*http.ServeMux).HandleFunc("/metrics", api.handleMetrics)

	// Start the schema refresh loop in a goroutine
	go api.refreshSchemaLoop()

	// Start the database change monitoring in a goroutine
	go api.monitorDatabaseChanges()

	// Start the HTTP server
	log.Printf("Starting GraphQL API on :%s", api.httpServer.Addr)
	return api.httpServer.ListenAndServe()
}

// refreshSchemaLoop periodically refreshes the GraphQL schema
func (api *GraphQLAPI) refreshSchemaLoop() {
	ticker := time.NewTicker(api.schemaRefreshInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := api.refreshSchema(); err != nil {
				log.Printf("Error refreshing schema: %v", err)
			}
		}
	}
}

// refreshSchema refreshes the GraphQL schema
func (api *GraphQLAPI) refreshSchema() error {
	log.Printf("Refreshing GraphQL schema...")

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
	api.lastSchemaRefresh = time.Now()
	api.mutex.Unlock()

	log.Printf("Schema refreshed successfully")
	return nil
}

// getSchema returns the current schema, refreshing it if necessary
func (api *GraphQLAPI) getSchema() *graphql.Schema {
	api.mutex.Lock()
	defer api.mutex.Unlock()

	// If the schema is nil or it's time to refresh, refresh it
	if api.schema == nil || time.Since(api.lastSchemaRefresh) > api.schemaRefreshInterval {
		api.mutex.Unlock()
		if err := api.refreshSchema(); err != nil {
			log.Printf("Error refreshing schema: %v", err)
		}
		api.mutex.Lock()
	}

	return api.schema
}

// metricsMiddleware wraps a handler with metrics recording
func (api *GraphQLAPI) metricsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Record start time
		startTime := time.Now()

		// Create a response recorder to capture the status code
		rr := &responseRecorder{
			ResponseWriter: w,
			statusCode:     http.StatusOK, // Default status code
		}

		// Process the request
		next.ServeHTTP(rr, r)

		// Record metrics
		duration := time.Since(startTime)

		// Parse the request to determine if it's a query or mutation
		if r.Method == http.MethodPost {
			var requestBody struct {
				OperationName string `json:"operationName"`
				Query         string `json:"query"`
			}

			// Try to decode the request body
			if r.Body != nil {
				bodyBytes, _ := io.ReadAll(r.Body)
				r.Body.Close()

				// Create a new reader with the same bytes for the next handler
				r.Body = io.NopCloser(strings.NewReader(string(bodyBytes)))

				// Parse the body
				_ = json.Unmarshal(bodyBytes, &requestBody)

				// Record the appropriate metric based on operation type
				if strings.Contains(requestBody.Query, "mutation") {
					api.metrics.RecordMutation()
				} else {
					api.metrics.RecordQuery(duration)
				}
			}
		}

		// Record errors
		if rr.statusCode >= 400 {
			api.metrics.RecordError()
		}
	})
}

// responseRecorder is a wrapper for http.ResponseWriter that records the status code
type responseRecorder struct {
	http.ResponseWriter
	statusCode int
}

// WriteHeader records the status code and calls the wrapped ResponseWriter's WriteHeader
func (rr *responseRecorder) WriteHeader(code int) {
	rr.statusCode = code
	rr.ResponseWriter.WriteHeader(code)
}

// handleWebSocket handles WebSocket connections for GraphQL subscriptions
func (api *GraphQLAPI) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	// Configure the upgrader to allow any origin
	upgrader := websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin: func(r *http.Request) bool {
			return true // Allow all origins
		},
		Subprotocols: []string{"graphql-ws", "graphql-transport-ws"},
	}

	// Upgrade the HTTP connection to a WebSocket connection
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Error upgrading to WebSocket: %v", err)
		return
	}
	defer conn.Close()

	// Generate a unique client ID
	clientID := fmt.Sprintf("client-%d", time.Now().UnixNano())
	log.Printf("WebSocket client connected: %s from %s", clientID, r.RemoteAddr)

	// Keep track of active subscriptions for this client
	subscriptions := make(map[string]chan interface{})
	defer func() {
		// Clean up subscriptions when the client disconnects
		for subID, ch := range subscriptions {
			log.Printf("Cleaning up subscription %s for client %s", subID, clientID)
			api.pubsub.Unsubscribe(subID, clientID+":"+subID)
			close(ch)
			delete(subscriptions, subID)
		}
		log.Printf("WebSocket client disconnected: %s", clientID)
	}()

	// Set up a ping handler to keep the connection alive
	conn.SetPingHandler(func(data string) error {
		log.Printf("Received ping from client %s", clientID)
		return conn.WriteControl(websocket.PongMessage, []byte(data), time.Now().Add(10*time.Second))
	})

	// Set up a pong handler to respond to server pings
	conn.SetPongHandler(func(data string) error {
		log.Printf("Received pong from client %s", clientID)
		return nil
	})

	// Start a goroutine to send periodic pings to keep the connection alive
	stopPing := make(chan struct{})
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if err := conn.WriteControl(websocket.PingMessage, []byte("keepalive"), time.Now().Add(10*time.Second)); err != nil {
					log.Printf("Error sending ping to client %s: %v", clientID, err)
					return
				}
				log.Printf("Sent ping to client %s", clientID)
			case <-stopPing:
				return
			}
		}
	}()
	defer close(stopPing)

	// Handle incoming messages
	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket error: %v", err)
			} else {
				log.Printf("WebSocket closed: %v", err)
			}
			break
		}

		// Log the received message for debugging
		log.Printf("Received WebSocket message from %s: %s", clientID, string(message))

		// Parse the message as JSON
		var request map[string]interface{}
		if err := json.Unmarshal(message, &request); err != nil {
			log.Printf("Error parsing WebSocket message: %v", err)
			sendErrorResponse(conn, "invalid_json", "Invalid JSON")
			continue
		}

		// Check if this is a GraphQL subscription request
		if typ, ok := request["type"].(string); ok {
			switch typ {
			case "connection_init":
				// Client is initializing the connection
				log.Printf("Client %s initialized connection", clientID)
				sendResponse(conn, "connection_ack", nil)

			case "start":
				// Client is starting a subscription
				id, ok := request["id"].(string)
				if !ok {
					sendErrorResponse(conn, "invalid_request", "Missing subscription ID")
					continue
				}

				payload, ok := request["payload"].(map[string]interface{})
				if !ok {
					sendErrorResponse(conn, "invalid_request", "Missing payload")
					continue
				}

				query, ok := payload["query"].(string)
				if !ok {
					sendErrorResponse(conn, "invalid_request", "Missing query")
					continue
				}

				variables, _ := payload["variables"].(map[string]interface{})

				log.Printf("Client %s starting subscription %s with query: %s", clientID, id, query)

				// Process the subscription request in a separate goroutine
				go api.handleSubscription(conn, clientID, id, query, variables, subscriptions)

			case "stop":
				// Client is stopping a subscription
				id, ok := request["id"].(string)
				if !ok {
					sendErrorResponse(conn, "invalid_request", "Missing subscription ID")
					continue
				}

				log.Printf("Client %s stopping subscription %s", clientID, id)

				// Unsubscribe from the topic
				if ch, ok := subscriptions[id]; ok {
					api.pubsub.Unsubscribe(id, clientID+":"+id)
					close(ch)
					delete(subscriptions, id)
				}

				sendResponse(conn, "complete", map[string]string{"id": id})

			default:
				log.Printf("Unknown message type from client %s: %s", clientID, typ)
				sendErrorResponse(conn, "unknown_type", fmt.Sprintf("Unknown message type: %s", typ))
			}
		} else {
			sendErrorResponse(conn, "invalid_request", "Missing message type")
		}
	}
}

// handleSubscription processes a GraphQL subscription request
func (api *GraphQLAPI) handleSubscription(
	conn *websocket.Conn,
	clientID string,
	subscriptionID string,
	query string,
	variables map[string]interface{},
	subscriptions map[string]chan interface{},
) {
	// Parse the GraphQL query
	document, err := parser.Parse(parser.ParseParams{
		Source: query,
	})
	if err != nil {
		sendErrorResponse(conn, "invalid_query", fmt.Sprintf("Invalid GraphQL query: %v", err))
		return
	}

	// Extract the operation name and subscription field
	var operationName string
	var fieldName string
	var args map[string]interface{}

	for _, definition := range document.Definitions {
		if operationDefinition, ok := definition.(*ast.OperationDefinition); ok {
			if operationDefinition.Operation == "subscription" {
				if operationDefinition.Name != nil {
					operationName = operationDefinition.Name.Value
				}

				// Get the subscription field name and arguments
				if len(operationDefinition.SelectionSet.Selections) > 0 {
					if field, ok := operationDefinition.SelectionSet.Selections[0].(*ast.Field); ok {
						fieldName = field.Name.Value

						// Extract arguments
						args = make(map[string]interface{})
						for _, arg := range field.Arguments {
							if arg.Value != nil {
								switch value := arg.Value.(type) {
								case *ast.StringValue:
									args[arg.Name.Value] = value.Value
								case *ast.IntValue:
									args[arg.Name.Value] = value.Value
								case *ast.FloatValue:
									args[arg.Name.Value] = value.Value
								case *ast.BooleanValue:
									args[arg.Name.Value] = value.Value
								case *ast.EnumValue:
									args[arg.Name.Value] = value.Value
								case *ast.ListValue:
									// Handle list values (simplified)
									listValues := []interface{}{}
									for _, item := range value.Values {
										if strItem, ok := item.(*ast.StringValue); ok {
											listValues = append(listValues, strItem.Value)
										}
									}
									args[arg.Name.Value] = listValues
								case *ast.ObjectValue:
									// Handle object values (simplified)
									objValues := map[string]interface{}{}
									for _, field := range value.Fields {
										if strField, ok := field.Value.(*ast.StringValue); ok {
											objValues[field.Name.Value] = strField.Value
										}
									}
									args[arg.Name.Value] = objValues
								}
							}
						}
					}
				}
				break
			}
		}
	}

	if fieldName == "" {
		sendErrorResponse(conn, "invalid_subscription", "No subscription field found")
		return
	}

	log.Printf("Subscription request: client=%s, id=%s, operation=%s, field=%s, args=%v",
		clientID, subscriptionID, operationName, fieldName, args)

	// Extract the ID argument if present
	var id interface{}
	if idArg, ok := args["id"]; ok && idArg != "" {
		id = idArg
	}

	// Construct the topic based on the field name and ID
	var topic string
	if id != nil && id != "" {
		// If an ID is provided, subscribe to changes for that specific entity
		topic = fmt.Sprintf("%s:%v:changed", fieldName, id)
		log.Printf("Subscribing to specific topic: %s", topic)
	} else {
		// Otherwise, subscribe to all changes for this entity type
		topic = fmt.Sprintf("%s:all:changed", fieldName)
		log.Printf("Subscribing to wildcard topic: %s", topic)
	}

	// Subscribe to the topic
	subKey := clientID + ":" + subscriptionID
	ch := api.pubsub.Subscribe(topic, subKey)
	subscriptions[subscriptionID] = ch

	// Send an initial confirmation that the subscription is active
	response := map[string]interface{}{
		"type": "next",
		"id":   subscriptionID,
		"payload": map[string]interface{}{
			"data": map[string]interface{}{
				fieldName: nil,
			},
		},
	}

	jsonResponse, err := json.Marshal(response)
	if err != nil {
		log.Printf("Error marshaling initial subscription response: %v", err)
	} else {
		if err := conn.WriteMessage(websocket.TextMessage, jsonResponse); err != nil {
			log.Printf("Error sending initial subscription response: %v", err)
			return
		}
	}

	// Listen for messages on this subscription
	for data := range ch {
		log.Printf("Received data on topic %s: %v", topic, data)

		// Add a type assertion for the data
		dataMap, ok := data.(map[string]interface{})
		if !ok {
			log.Printf("Error: received data is not a map[string]interface{}, got %T", data)
			continue
		}

		// Execute the GraphQL query with the data
		params := graphql.Params{
			Schema:         *api.getSchema(),
			RequestString:  query,
			VariableValues: variables,
			OperationName:  operationName,
			Context:        context.Background(),
			RootObject:     dataMap,
		}
		result := graphql.Do(params)

		if len(result.Errors) > 0 {
			log.Printf("GraphQL execution errors: %v", result.Errors)
		}

		// Send the result to the client
		response := map[string]interface{}{
			"type":    "next",
			"id":      subscriptionID,
			"payload": result,
		}

		jsonResponse, err := json.Marshal(response)
		if err != nil {
			log.Printf("Error marshaling subscription response: %v", err)
			continue
		}

		log.Printf("Sending subscription data to client %s: %s", clientID, string(jsonResponse))
		if err := conn.WriteMessage(websocket.TextMessage, jsonResponse); err != nil {
			log.Printf("Error sending subscription data: %v", err)
			return
		}
	}

	log.Printf("Subscription %s for client %s ended", subscriptionID, clientID)
}

// sendResponse sends a response to the WebSocket client
func sendResponse(conn *websocket.Conn, typ string, payload interface{}) {
	response := map[string]interface{}{
		"type": typ,
	}
	if payload != nil {
		response["payload"] = payload
	}

	jsonResponse, err := json.Marshal(response)
	if err != nil {
		log.Printf("Error marshaling response: %v", err)
		return
	}

	if err := conn.WriteMessage(websocket.TextMessage, jsonResponse); err != nil {
		log.Printf("Error sending response: %v", err)
	}
}

// sendErrorResponse sends an error response to the WebSocket client
func sendErrorResponse(conn *websocket.Conn, code string, message string) {
	response := map[string]interface{}{
		"type": "error",
		"payload": map[string]string{
			"code":    code,
			"message": message,
		},
	}

	jsonResponse, err := json.Marshal(response)
	if err != nil {
		log.Printf("Error marshaling error response: %v", err)
		return
	}

	if err := conn.WriteMessage(websocket.TextMessage, jsonResponse); err != nil {
		log.Printf("Error sending error response: %v", err)
	}
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
		return "", NewError(ErrInvalidConfig, "Failed to read pipeline config file", err).
			WithContext("file", api.pipelineConfigFile)
	}

	// Parse the YAML configuration
	var config PipelineConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return "", NewError(ErrInvalidConfig, "Failed to parse pipeline config", err).
			WithContext("file", api.pipelineConfigFile)
	}

	// Look for a SQLite consumer in any pipeline
	for pipelineName, pipeline := range config.Pipelines {
		for _, consumer := range pipeline.Consumers {
			// Check if this is a SQLite consumer (case-insensitive substring match)
			if strings.Contains(strings.ToLower(consumer.Type), "sqlite") {
				if dbPath, ok := consumer.Config["db_path"].(string); ok && dbPath != "" {
					log.Printf("Found SQLite consumer in pipeline %s with db_path: %s", pipelineName, dbPath)
					return dbPath, nil
				}
			}
		}
	}

	return "", NewError(ErrNotFound, "No SQLite consumer found in pipeline config", nil).
		WithContext("file", api.pipelineConfigFile)
}

// fetchSchema fetches the schema from the schema registry
func (api *GraphQLAPI) fetchSchema() (string, error) {
	// Set up HTTP client with timeout
	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	// Try to fetch the schema with retries
	var resp *http.Response
	var err error
	maxRetries := 3
	retryDelay := 1 * time.Second

	for i := 0; i < maxRetries; i++ {
		// Make the request
		resp, err = client.Get(api.schemaRegistryURL + "/schema")
		if err == nil && resp.StatusCode == http.StatusOK {
			break
		}

		if err != nil {
			log.Printf("Attempt %d: Error connecting to schema registry: %v", i+1, err)
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
		return "", NewError(ErrSchemaBuilding, "Failed to connect to schema registry", err).
			WithContext("url", api.schemaRegistryURL).
			WithContext("retries", maxRetries)
	}

	if resp.StatusCode != http.StatusOK {
		return "", NewError(ErrSchemaBuilding, "Schema registry returned non-OK status", nil).
			WithContext("url", api.schemaRegistryURL).
			WithContext("status", resp.StatusCode)
	}

	defer resp.Body.Close()
	schema, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", NewError(ErrSchemaBuilding, "Failed to read schema response", err).
			WithContext("url", api.schemaRegistryURL)
	}

	return string(schema), nil
}

// buildSchema builds a GraphQL schema from the registry schema
func (api *GraphQLAPI) buildSchema(registrySchema string) (*graphql.Schema, error) {
	// Create fields for query and subscription
	queryFields := graphql.Fields{
		"health": &graphql.Field{
			Type: graphql.String,
			Resolve: func(p graphql.ResolveParams) (interface{}, error) {
				return "OK", nil
			},
			Description: "Health check endpoint",
		},
	}

	subscriptionFields := graphql.Fields{}

	// Create the root query type
	queryType := graphql.NewObject(graphql.ObjectConfig{
		Name:   "Query",
		Fields: queryFields,
	})

	// Create the root subscription type
	subscriptionType := graphql.NewObject(graphql.ObjectConfig{
		Name:   "Subscription",
		Fields: subscriptionFields,
	})

	// Add fields from the database
	if err := api.buildSchemaFromDatabase(queryFields); err != nil {
		return nil, fmt.Errorf("failed to build schema from database: %w", err)
	}

	// Add subscription fields
	if err := api.buildSubscriptionFields(subscriptionFields); err != nil {
		return nil, fmt.Errorf("failed to build subscription fields: %w", err)
	}

	// Create the schema with both query and subscription types
	schemaConfig := graphql.SchemaConfig{
		Query:        queryType,
		Subscription: subscriptionType,
	}

	schema, err := graphql.NewSchema(schemaConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create schema: %w", err)
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
		var idField string
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

			// Identify the ID field (primary key)
			if pk == 1 {
				idField = name
			}

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

			// Add field with description
			typeFields[name] = &graphql.Field{
				Type:        fieldType,
				Description: fmt.Sprintf("The %s field from the %s table", name, tableName),
				// Add metadata about the field
				Args: graphql.FieldConfigArgument{},
				// Add resolver for nested objects if needed
			}
		}
		schemaRows.Close()

		// If no columns were found, skip this table
		if len(columns) == 0 {
			log.Printf("No columns found for table %s, skipping", tableName)
			continue
		}

		// If no ID field was found, use the first column
		if idField == "" {
			idField = columns[0]
			log.Printf("No primary key found for table %s, using first column %s as ID", tableName, idField)
		}

		// Create the object type with description
		// Add "DB" prefix to avoid conflicts with existing types
		typeName := "DB" + api.pascalCase(api.singularize(tableName))
		objectType := graphql.NewObject(graphql.ObjectConfig{
			Name:        typeName,
			Fields:      typeFields,
			Description: fmt.Sprintf("Represents a %s record from the database", api.singularize(tableName)),
		})

		// Create the edge type for connections
		edgeType := graphql.NewObject(graphql.ObjectConfig{
			Name: typeName + "Edge",
			Fields: graphql.Fields{
				"node": &graphql.Field{
					Type:        objectType,
					Description: fmt.Sprintf("The %s node", api.singularize(tableName)),
				},
				"cursor": &graphql.Field{
					Type:        graphql.String,
					Description: "A cursor for pagination",
				},
			},
			Description: fmt.Sprintf("An edge containing a %s node and its cursor", api.singularize(tableName)),
		})

		// Create the page info type if it doesn't exist yet
		var pageInfoType *graphql.Object
		if _, exists := fields["pageInfo"]; !exists {
			pageInfoType = graphql.NewObject(graphql.ObjectConfig{
				Name: "PageInfo",
				Fields: graphql.Fields{
					"hasNextPage": &graphql.Field{
						Type:        graphql.NewNonNull(graphql.Boolean),
						Description: "Indicates if there are more pages to fetch",
					},
					"endCursor": &graphql.Field{
						Type:        graphql.String,
						Description: "The cursor to continue pagination",
					},
				},
				Description: "Information about pagination in a connection",
			})
			fields["pageInfo"] = &graphql.Field{
				Type:        pageInfoType,
				Description: "Information about pagination in a connection",
			}
		}

		// Create the connection type
		connectionType := graphql.NewObject(graphql.ObjectConfig{
			Name: typeName + "Connection",
			Fields: graphql.Fields{
				"edges": &graphql.Field{
					Type:        graphql.NewList(edgeType),
					Description: fmt.Sprintf("A list of %s edges", api.singularize(tableName)),
				},
				"pageInfo": &graphql.Field{
					Type:        graphql.NewNonNull(pageInfoType),
					Description: "Information to aid in pagination",
				},
			},
			Description: fmt.Sprintf("A connection to a list of %s items", api.singularize(tableName)),
		})

		// Add query fields for this table
		fields[api.camelCase(api.singularize(tableName))] = &graphql.Field{
			Type:        objectType,
			Description: fmt.Sprintf("Get a single %s by ID", api.singularize(tableName)),
			Args: graphql.FieldConfigArgument{
				"id": &graphql.ArgumentConfig{
					Type:        graphql.NewNonNull(graphql.ID),
					Description: fmt.Sprintf("The ID of the %s", api.singularize(tableName)),
				},
			},
			Resolve: api.createSingleItemResolver(tableName, columns, idField),
		}

		fields[api.camelCase(api.pluralize(tableName))] = &graphql.Field{
			Type:        connectionType,
			Description: fmt.Sprintf("Get a list of %s", api.pluralize(tableName)),
			Args: graphql.FieldConfigArgument{
				"first": &graphql.ArgumentConfig{
					Type:        graphql.Int,
					Description: "Returns the first n elements from the list",
				},
				"after": &graphql.ArgumentConfig{
					Type:        graphql.String,
					Description: "Returns the elements that come after the specified cursor",
				},
				"last": &graphql.ArgumentConfig{
					Type:        graphql.Int,
					Description: "Returns the last n elements from the list",
				},
				"before": &graphql.ArgumentConfig{
					Type:        graphql.String,
					Description: "Returns the elements that come before the specified cursor",
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
		// Get pagination arguments
		first, _ := p.Args["first"].(int)
		if first <= 0 {
			first = 10 // Default limit
		}

		after, _ := p.Args["after"].(string)

		// Get filtering arguments
		filter, _ := p.Args["filter"].(map[string]interface{})

		// Get sorting arguments
		orderBy, _ := p.Args["orderBy"].(string)
		orderDir, _ := p.Args["orderDirection"].(string)
		if orderDir == "" {
			orderDir = "ASC" // Default sort direction
		}

		log.Printf("Querying %s with first=%d, after=%s, filter=%v, orderBy=%s, orderDir=%s",
			tableName, first, after, filter, orderBy, orderDir)

		// Build the query
		query := fmt.Sprintf("SELECT * FROM %s", tableName)
		args := []interface{}{}

		// Add WHERE clauses for filtering
		whereConditions := []string{}

		// Add after cursor condition if provided
		if after != "" && idField != "" {
			whereConditions = append(whereConditions, fmt.Sprintf("%s > ?", idField))
			args = append(args, after)
		}

		// Add filter conditions if provided
		if filter != nil {
			for field, value := range filter {
				// Skip if the field doesn't exist in the table
				if !contains(columns, field) {
					continue
				}

				// Handle different filter operations based on value type
				switch v := value.(type) {
				case map[string]interface{}:
					// Complex filter with operators
					for op, opValue := range v {
						switch op {
						case "eq":
							whereConditions = append(whereConditions, fmt.Sprintf("%s = ?", field))
							args = append(args, opValue)
						case "neq":
							whereConditions = append(whereConditions, fmt.Sprintf("%s != ?", field))
							args = append(args, opValue)
						case "gt":
							whereConditions = append(whereConditions, fmt.Sprintf("%s > ?", field))
							args = append(args, opValue)
						case "gte":
							whereConditions = append(whereConditions, fmt.Sprintf("%s >= ?", field))
							args = append(args, opValue)
						case "lt":
							whereConditions = append(whereConditions, fmt.Sprintf("%s < ?", field))
							args = append(args, opValue)
						case "lte":
							whereConditions = append(whereConditions, fmt.Sprintf("%s <= ?", field))
							args = append(args, opValue)
						case "like":
							whereConditions = append(whereConditions, fmt.Sprintf("%s LIKE ?", field))
							args = append(args, fmt.Sprintf("%%%s%%", opValue))
						case "in":
							// Handle IN operator with array of values
							if values, ok := opValue.([]interface{}); ok && len(values) > 0 {
								placeholders := make([]string, len(values))
								for i := range values {
									placeholders[i] = "?"
									args = append(args, values[i])
								}
								whereConditions = append(whereConditions, fmt.Sprintf("%s IN (%s)", field, strings.Join(placeholders, ",")))
							}
						}
					}
				default:
					// Simple equality filter
					whereConditions = append(whereConditions, fmt.Sprintf("%s = ?", field))
					args = append(args, v)
				}
			}
		}

		// Add WHERE clause to query if we have conditions
		if len(whereConditions) > 0 {
			query += " WHERE " + strings.Join(whereConditions, " AND ")
		}

		// Add ORDER BY clause
		if orderBy != "" && contains(columns, orderBy) {
			// Sanitize order direction
			if orderDir != "ASC" && orderDir != "DESC" {
				orderDir = "ASC"
			}
			query += fmt.Sprintf(" ORDER BY %s %s", orderBy, orderDir)
		} else if idField != "" {
			// Default sort by ID
			query += fmt.Sprintf(" ORDER BY %s ASC", idField)
		}

		// Request one more than needed to determine if there are more pages
		query += " LIMIT ?"
		args = append(args, first+1)

		rows, err := api.db.Query(query, args...)
		if err != nil {
			return nil, fmt.Errorf("error querying database: %w", err)
		}
		defer rows.Close()

		// Process the results
		var edges []map[string]interface{}
		var hasNextPage bool
		var lastCursor string
		count := 0

		for rows.Next() {
			// If we've reached our limit, just set hasNextPage and break
			if count >= first {
				hasNextPage = true
				break
			}

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

			// Get the cursor (ID) for this edge
			var cursor string
			if idVal, ok := result[idField]; ok && idVal != nil {
				cursor = fmt.Sprintf("%v", idVal)
				lastCursor = cursor
			}

			// Create the edge
			edge := map[string]interface{}{
				"node":   result,
				"cursor": cursor,
			}

			edges = append(edges, edge)
			count++
		}

		// Create the connection result
		connection := map[string]interface{}{
			"edges": edges,
			"pageInfo": map[string]interface{}{
				"hasNextPage": hasNextPage,
				"endCursor":   lastCursor,
			},
		}

		return connection, nil
	}
}

// contains checks if a string slice contains a specific string
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
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

// buildSubscriptionFields builds the subscription fields for the GraphQL schema
func (api *GraphQLAPI) buildSubscriptionFields(fields graphql.Fields) error {
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

		// Skip internal tables
		if tableName == "sqlite_sequence" || tableName == "flow_metadata" {
			continue
		}

		tables = append(tables, tableName)
	}

	// Process each table for subscriptions
	for _, tableName := range tables {
		// Get table schema
		schemaRows, err := api.db.Query(fmt.Sprintf("PRAGMA table_info(%s)", tableName))
		if err != nil {
			log.Printf("Error getting schema for table %s: %v", tableName, err)
			continue
		}

		var columns []string
		var idField string

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

			// Identify the ID field (primary key)
			if pk == 1 {
				idField = name
			}
		}
		schemaRows.Close()

		// If we found an ID field, create a subscription for this table
		if idField != "" {
			// Create the subscription field
			singularName := api.singularize(tableName)

			// Find the corresponding object type from the query fields
			objectType, err := api.getObjectTypeForTable(tableName)
			if err != nil {
				log.Printf("Warning: Could not create subscription for %s: %v", tableName, err)
				continue
			}

			// Add the subscription field
			fields[singularName+"Changed"] = &graphql.Field{
				Type: objectType,
				Args: graphql.FieldConfigArgument{
					"id": &graphql.ArgumentConfig{
						Type:        graphql.String,
						Description: "Optional account ID. If not provided, subscribes to all account changes.",
					},
				},
				// Implement the resolver to handle both regular queries and subscription events
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					// Get the ID from arguments (now optional)
					var id string
					if idArg, ok := p.Args["id"].(string); ok && idArg != "" {
						id = idArg
					}

					// If this is a subscription event (has Source data)
					if p.Source != nil {
						log.Printf("Processing subscription event with source type: %T, value: %v", p.Source, p.Source)

						// Try different formats that might be used in the payload
						if payload, ok := p.Source.(map[string]interface{}); ok {
							log.Printf("Payload keys: %v", getMapKeys(payload))

							// If we have an ID filter, check if this event is for the requested ID
							if id != "" {
								// Check if the payload has an ID that matches our filter
								if payloadID, ok := payload["id"]; ok && fmt.Sprintf("%v", payloadID) != id {
									// This event is for a different account, skip it
									return nil, fmt.Errorf("event is for a different account")
								}
							}

							// Option 1: Check if we have a data field
							if data, ok := payload["data"].(map[string]interface{}); ok {
								log.Printf("Found data in payload: %v", data)
								return data, nil
							}

							// Option 2: If no data field, maybe the payload itself is the data
							// Check if it has an ID field
							if _, ok := payload["id"]; ok {
								log.Printf("Using payload as data: %v", payload)
								return payload, nil
							}

							// Option 3: For GraphiQL direct queries, just return the row from the database
							if _, ok := payload["query"]; ok {
								log.Printf("GraphiQL direct query detected")
								if id != "" {
									return api.fetchRecordFromDatabase(tableName, idField, id, columns)
								} else {
									// For wildcard subscriptions in GraphiQL, return a placeholder
									return map[string]interface{}{
										"account_id":           "PLACEHOLDER",
										"balance":              "0",
										"sequence":             "0",
										"num_subentries":       0,
										"flags":                0,
										"last_modified_ledger": 0,
									}, nil
								}
							}

							// Option 4: Maybe the payload is just a wrapper and we need to return something
							// This is a fallback that might work in some cases
							if id != "" {
								return map[string]interface{}{
									"account_id":           id,
									"balance":              "0",
									"sequence":             "0",
									"num_subentries":       0,
									"flags":                0,
									"last_modified_ledger": 0,
								}, nil
							} else {
								return map[string]interface{}{
									"account_id":           "PLACEHOLDER",
									"balance":              "0",
									"sequence":             "0",
									"num_subentries":       0,
									"flags":                0,
									"last_modified_ledger": 0,
								}, nil
							}
						}

						// If we got here, we couldn't extract the data
						return nil, fmt.Errorf("could not extract data from event payload")
					}

					// If this is a regular query (no Source data), fetch from database
					if id == "" {
						// For wildcard queries, return a placeholder
						return map[string]interface{}{
							"account_id":           "PLACEHOLDER",
							"balance":              "0",
							"sequence":             "0",
							"num_subentries":       0,
							"flags":                0,
							"last_modified_ledger": 0,
						}, nil
					}

					log.Printf("Processing regular query for %s with ID: %s", tableName, id)
					return api.fetchRecordFromDatabase(tableName, idField, id, columns)
				},
				Description: fmt.Sprintf("Subscribe to changes to a %s. If no ID is provided, subscribes to all changes.", singularName),
			}
		}
	}

	return nil
}

// getObjectTypeForTable finds the GraphQL object type for a given table
func (api *GraphQLAPI) getObjectTypeForTable(tableName string) (*graphql.Object, error) {
	// Get table schema
	schemaRows, err := api.db.Query(fmt.Sprintf("PRAGMA table_info(%s)", tableName))
	if err != nil {
		return nil, fmt.Errorf("error getting schema for table %s: %w", tableName, err)
	}
	defer schemaRows.Close()

	// Create fields for the type
	typeFields := graphql.Fields{}

	for schemaRows.Next() {
		var cid int
		var name, typeName string
		var notNull, pk int
		var defaultValue interface{}

		if err := schemaRows.Scan(&cid, &name, &typeName, &notNull, &defaultValue, &pk); err != nil {
			return nil, fmt.Errorf("error scanning column info: %w", err)
		}

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

		typeFields[name] = &graphql.Field{
			Type: fieldType,
		}
	}

	// Create the object type
	return graphql.NewObject(graphql.ObjectConfig{
		Name:   api.pascalCase(api.singularize(tableName)),
		Fields: typeFields,
	}), nil
}

// monitorDatabaseChanges monitors the database for changes and publishes events to subscribers
func (api *GraphQLAPI) monitorDatabaseChanges() {
	log.Printf("Starting database change monitoring")

	// Get all tables in the database
	rows, err := api.db.Query("SELECT name FROM sqlite_master WHERE type='table'")
	if err != nil {
		log.Printf("Error querying tables for monitoring: %v", err)
		return
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var tableName string
		if err := rows.Scan(&tableName); err != nil {
			log.Printf("Error scanning table name: %v", err)
			continue
		}

		// Skip internal tables
		if tableName == "sqlite_sequence" || tableName == "flow_metadata" {
			continue
		}

		tables = append(tables, tableName)
	}

	// Create a map to store the last known row counts for each table
	lastCounts := make(map[string]int)

	// Initialize the last counts
	for _, tableName := range tables {
		count, err := api.countRows(tableName)
		if err != nil {
			log.Printf("Error counting rows in %s: %v", tableName, err)
			continue
		}
		lastCounts[tableName] = count
	}

	// Poll for changes every second
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			for _, tableName := range tables {
				// Get the current row count
				currentCount, err := api.countRows(tableName)
				if err != nil {
					log.Printf("Error counting rows in %s: %v", tableName, err)
					continue
				}

				// If the count has changed, something has been added or removed
				if currentCount != lastCounts[tableName] {
					log.Printf("Detected change in table %s: %d -> %d rows", tableName, lastCounts[tableName], currentCount)

					// Determine if rows were added or removed
					if currentCount > lastCounts[tableName] {
						// Rows were added - find the new rows
						api.handleRowsAdded(tableName, lastCounts[tableName], currentCount)
					} else {
						// Rows were removed - we can't easily determine which ones
						// Just notify that something changed
						api.pubsub.Publish(tableName+":changed", map[string]interface{}{
							"table":  tableName,
							"action": "removed",
						})
					}

					// Update the last known count
					lastCounts[tableName] = currentCount
				}
			}
		}
	}
}

// handleRowsAdded handles the case where rows were added to a table
func (api *GraphQLAPI) handleRowsAdded(tableName string, oldCount, newCount int) {
	// Get the schema for this table
	schemaRows, err := api.db.Query(fmt.Sprintf("PRAGMA table_info(%s)", tableName))
	if err != nil {
		log.Printf("Error getting schema for table %s: %v", tableName, err)
		return
	}
	defer schemaRows.Close()

	var columns []string
	var idField string

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

		// Identify the ID field (primary key)
		if pk == 1 {
			idField = name
		}
	}

	// If we couldn't find an ID field, use the first column
	if idField == "" && len(columns) > 0 {
		idField = columns[0]
	}

	// If we still don't have an ID field, we can't proceed
	if idField == "" {
		log.Printf("Could not find ID field for table %s", tableName)
		return
	}

	log.Printf("Processing %d new rows in table %s with ID field %s", newCount-oldCount, tableName, idField)

	// Query for the new rows
	// This is a simplistic approach - in a real system, you might want to use timestamps or sequence numbers
	query := fmt.Sprintf("SELECT * FROM %s ORDER BY %s DESC LIMIT ?", tableName, idField)
	rows, err := api.db.Query(query, newCount-oldCount)
	if err != nil {
		log.Printf("Error querying new rows: %v", err)
		return
	}
	defer rows.Close()

	// Process each new row
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
			log.Printf("Error scanning row: %v", err)
			continue
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

		// Get the ID value
		idValue, ok := result[idField]
		if !ok {
			log.Printf("ID field %s not found in result", idField)
			continue
		}

		// Create the singular name for the subscription field
		singularName := api.singularize(tableName)
		subscriptionField := singularName + "Changed"

		// Create a payload that matches the expected format for subscriptions
		payload := map[string]interface{}{
			"id":           idValue,
			"mutationType": "CREATED",
			"data":         result,
		}

		// For direct compatibility with GraphQL resolvers, also include the data at the top level
		for k, v := range result {
			payload[k] = v
		}

		// Log the payload for debugging
		payloadJSON, _ := json.Marshal(payload)
		log.Printf("Publishing to topics for %s with payload: %s", subscriptionField, string(payloadJSON))

		// Publish to the specific topic for this entity
		specificTopic := fmt.Sprintf("%s:%v:changed", subscriptionField, idValue)
		api.pubsub.Publish(specificTopic, payload)

		// Also publish to the wildcard topic for all entities of this type
		wildcardTopic := fmt.Sprintf("%s:all:changed", subscriptionField)
		api.pubsub.Publish(wildcardTopic, payload)
	}
}

// handleMetrics handles requests for metrics
func (api *GraphQLAPI) handleMetrics(w http.ResponseWriter, r *http.Request) {
	metrics := api.metrics.GetMetrics()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(metrics)
}

// getMapKeys returns the keys of a map as a slice
func getMapKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// fetchRecordFromDatabase fetches a record from the database by ID
func (api *GraphQLAPI) fetchRecordFromDatabase(tableName, idField, id string, columns []string) (interface{}, error) {
	// Query the database for this record
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
			return nil, fmt.Errorf("record not found")
		}
		return nil, fmt.Errorf("database error: %v", err)
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
