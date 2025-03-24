package flow

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

// APIServer represents the management API server
type APIServer struct {
	port         int
	engine       *CoreEngine
	server       *http.Server
	authEnabled  bool
	username     string
	password     string
	pipelines    map[string]*PipelineStatus
	pipelineLock sync.RWMutex
	startTime    time.Time // Track when the server was started
}

// PipelineStatus represents the current status of a pipeline
type PipelineStatus struct {
	ID            string                 `json:"id"`
	State         string                 `json:"state"` // running, stopped, error
	CurrentLedger int64                  `json:"current_ledger,omitempty"`
	StartTime     time.Time              `json:"start_time,omitempty"`
	LastError     string                 `json:"last_error,omitempty"`
	Config        map[string]interface{} `json:"config,omitempty"`
}

// APIResponse defines the standard response format for API endpoints
type APIResponse struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
}

// NewAPIServer creates a new API server instance
func NewAPIServer(engine *CoreEngine, port int, authEnabled bool, username, password string) *APIServer {
	return &APIServer{
		port:        port,
		engine:      engine,
		authEnabled: authEnabled,
		username:    username,
		password:    password,
		pipelines:   make(map[string]*PipelineStatus),
		startTime:   time.Now(), // Initialize start time
	}
}

// Start initializes and starts the API server
func (a *APIServer) Start(ctx context.Context) error {
	log.Printf("DEBUG: Inside APIServer.Start method, port: %d", a.port)
	mux := http.NewServeMux()

	// Register API endpoints
	mux.HandleFunc("/api/pipelines", a.authMiddleware(a.handleListPipelines))
	mux.HandleFunc("/api/pipelines/start", a.authMiddleware(a.handleStartPipeline))
	mux.HandleFunc("/api/pipelines/stop", a.authMiddleware(a.handleStopPipeline))
	mux.HandleFunc("/api/metrics", a.authMiddleware(a.handleGetMetrics))
	mux.HandleFunc("/api/health", a.handleHealthCheck)
	log.Printf("DEBUG: Registered API endpoints")

	// Create HTTP server
	a.server = &http.Server{
		Addr:    fmt.Sprintf(":%d", a.port),
		Handler: mux,
	}
	log.Printf("DEBUG: Created HTTP server with address %s", a.server.Addr)

	// Set start time
	a.startTime = time.Now()

	// Initialize pipeline status map
	a.initPipelineStatus()
	log.Printf("DEBUG: Initialized pipeline status map")

	// Start the server in a separate goroutine
	go func() {
		log.Printf("Starting API server on port %d", a.port)
		if err := a.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("API server error: %v", err)
		}
	}()
	log.Printf("DEBUG: API server goroutine started")

	return nil
}

// Shutdown stops the API server
func (a *APIServer) Shutdown(ctx context.Context) error {
	if a.server != nil {
		log.Println("Shutting down API server")
		return a.server.Shutdown(ctx)
	}
	return nil
}

// authMiddleware checks for basic authentication if enabled
func (a *APIServer) authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !a.authEnabled {
			next(w, r)
			return
		}

		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			a.respondWithError(w, http.StatusUnauthorized, "Authorization required")
			return
		}

		if !strings.HasPrefix(authHeader, "Basic ") {
			a.respondWithError(w, http.StatusUnauthorized, "Invalid authentication method")
			return
		}

		payload, err := base64.StdEncoding.DecodeString(authHeader[6:])
		if err != nil {
			a.respondWithError(w, http.StatusUnauthorized, "Invalid authorization header")
			return
		}

		pair := strings.SplitN(string(payload), ":", 2)
		if len(pair) != 2 || pair[0] != a.username || pair[1] != a.password {
			a.respondWithError(w, http.StatusUnauthorized, "Invalid credentials")
			return
		}

		next(w, r)
	}
}

// initPipelineStatus initializes the pipeline status map
func (a *APIServer) initPipelineStatus() {
	a.pipelineLock.Lock()
	defer a.pipelineLock.Unlock()

	for name := range a.engine.Pipelines {
		a.pipelines[name] = &PipelineStatus{
			ID:    name,
			State: "stopped",
		}
	}
}

// handleListPipelines returns the list of all pipelines and their status
func (a *APIServer) handleListPipelines(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		a.respondWithError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	a.pipelineLock.RLock()
	pipelineList := make([]*PipelineStatus, 0, len(a.pipelines))
	for _, status := range a.pipelines {
		pipelineList = append(pipelineList, status)
	}
	a.pipelineLock.RUnlock()

	a.respondWithJSON(w, http.StatusOK, APIResponse{
		Success: true,
		Data:    pipelineList,
	})
}

// StartPipelineRequest defines the structure for pipeline start requests
type StartPipelineRequest struct {
	PipelineID string                 `json:"pipeline_id"`
	Overrides  map[string]interface{} `json:"overrides,omitempty"`
}

// handleStartPipeline starts a pipeline with optional overrides
func (a *APIServer) handleStartPipeline(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		a.respondWithError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	var req StartPipelineRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		a.respondWithError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.PipelineID == "" {
		a.respondWithError(w, http.StatusBadRequest, "Pipeline ID is required")
		return
	}

	_, exists := a.engine.Pipelines[req.PipelineID]
	if !exists {
		a.respondWithError(w, http.StatusNotFound, "Pipeline not found")
		return
	}

	// Call CoreEngine's StartPipeline method
	if err := a.engine.StartPipeline(req.PipelineID, req.Overrides); err != nil {
		a.respondWithError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to start pipeline: %v", err))
		return
	}

	a.respondWithJSON(w, http.StatusOK, APIResponse{
		Success: true,
		Data:    fmt.Sprintf("Pipeline %s started successfully", req.PipelineID),
	})
}

// handleStopPipeline stops a running pipeline
func (a *APIServer) handleStopPipeline(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		a.respondWithError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	var req struct {
		PipelineID string `json:"pipeline_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		a.respondWithError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.PipelineID == "" {
		a.respondWithError(w, http.StatusBadRequest, "Pipeline ID is required")
		return
	}

	_, exists := a.engine.Pipelines[req.PipelineID]
	if !exists {
		a.respondWithError(w, http.StatusNotFound, "Pipeline not found")
		return
	}

	// Call CoreEngine's StopPipeline method
	if err := a.engine.StopPipeline(req.PipelineID); err != nil {
		a.respondWithError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to stop pipeline: %v", err))
		return
	}

	a.respondWithJSON(w, http.StatusOK, APIResponse{
		Success: true,
		Data:    fmt.Sprintf("Pipeline %s stopped successfully", req.PipelineID),
	})
}

// handleGetMetrics returns system-wide or pipeline-specific metrics
func (a *APIServer) handleGetMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		a.respondWithError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	// Get pipeline ID from query parameters if provided
	pipelineID := r.URL.Query().Get("pipeline_id")

	// Create metrics response using data from metrics package
	metricsData := map[string]interface{}{
		"system": map[string]interface{}{
			"uptime_seconds":    time.Since(a.startTime).Seconds(),
			"memory_usage_mb":   calculateMemoryUsage(),
			"cpu_usage_percent": calculateCPUUsage(),
		},
	}

	if pipelineID != "" {
		// Get pipeline-specific metrics if a pipeline ID was provided
		if state, currentLedger, startTime, lastError, err := a.engine.GetPipelineStatus(pipelineID); err == nil {
			pipelineMetrics := map[string]interface{}{
				"id":             pipelineID,
				"state":          state,
				"current_ledger": currentLedger,
			}

			if !startTime.IsZero() {
				pipelineMetrics["start_time"] = startTime.Format(time.RFC3339)
				pipelineMetrics["uptime_seconds"] = time.Since(startTime).Seconds()
			}

			if lastError != "" {
				pipelineMetrics["last_error"] = lastError
			}

			metricsData["pipeline"] = pipelineMetrics
		} else {
			a.respondWithError(w, http.StatusNotFound, fmt.Sprintf("Pipeline %s not found", pipelineID))
			return
		}
	}

	a.respondWithJSON(w, http.StatusOK, APIResponse{
		Success: true,
		Data:    metricsData,
	})
}

// calculateMemoryUsage is a placeholder for memory usage calculation
func calculateMemoryUsage() float64 {
	// This would be implemented with actual memory usage calculation
	// For now, returning a placeholder value
	return 256.0
}

// calculateCPUUsage is a placeholder for CPU usage calculation
func calculateCPUUsage() float64 {
	// This would be implemented with actual CPU usage calculation
	// For now, returning a placeholder value
	return 15.5
}

// handleHealthCheck provides a simple health check endpoint
func (a *APIServer) handleHealthCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		a.respondWithError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	a.respondWithJSON(w, http.StatusOK, APIResponse{
		Success: true,
		Data: map[string]interface{}{
			"status":  "healthy",
			"version": "1.0.0", // Replace with actual version
			"uptime":  "1h",    // Placeholder
		},
	})
}

// UpdatePipelineStatus updates the status of a pipeline
func (a *APIServer) UpdatePipelineStatus(pipelineID string, state string, currentLedger int64, lastError string) {
	a.pipelineLock.Lock()
	defer a.pipelineLock.Unlock()

	status, exists := a.pipelines[pipelineID]
	if !exists {
		status = &PipelineStatus{
			ID: pipelineID,
		}
		a.pipelines[pipelineID] = status
	}

	status.State = state
	if currentLedger > 0 {
		status.CurrentLedger = currentLedger
	}
	if lastError != "" {
		status.LastError = lastError
	}
}

// respondWithJSON sends a JSON response to the client
func (a *APIServer) respondWithJSON(w http.ResponseWriter, status int, payload interface{}) {
	response, err := json.Marshal(payload)
	if err != nil {
		log.Printf("Error marshaling JSON response: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	w.Write(response)
}

// respondWithError sends an error response to the client
func (a *APIServer) respondWithError(w http.ResponseWriter, status int, message string) {
	a.respondWithJSON(w, status, APIResponse{
		Success: false,
		Error:   message,
	})
}
