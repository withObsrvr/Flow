// main.go
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"plugin"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"gopkg.in/yaml.v2"

	"github.com/withObsrvr/Flow/internal/metrics"
	"github.com/withObsrvr/pluginapi"
)

// Global counters for basic metrics
var (
	globalMessageCount int64
	globalErrorCount   int64
)

// ------------------
// Pipeline Config Types
// ------------------
type PipelineConfig struct {
	Pipelines map[string]PipelineDefinition `yaml:"pipelines"`
}

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

func LoadPipelineConfig(filename string) (*PipelineConfig, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	var cfg PipelineConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	// Add debug logging
	log.Printf("Loaded pipeline config: %+v", cfg.Pipelines)

	return &cfg, nil
}

// ------------------
// Plugin Registry and Manager
// ------------------
type PluginRegistry struct {
	Sources    map[string]pluginapi.Source
	Processors map[string]pluginapi.Processor
	Consumers  map[string]pluginapi.Consumer
}

func NewPluginRegistry() *PluginRegistry {
	return &PluginRegistry{
		Sources:    make(map[string]pluginapi.Source),
		Processors: make(map[string]pluginapi.Processor),
		Consumers:  make(map[string]pluginapi.Consumer),
	}
}

func (pr *PluginRegistry) Register(p pluginapi.Plugin) error {
	switch p.Type() {
	case pluginapi.SourcePlugin:
		src, ok := p.(pluginapi.Source)
		if !ok {
			return fmt.Errorf("plugin %s does not implement Source", p.Name())
		}
		pr.Sources[p.Name()] = src
	case pluginapi.ProcessorPlugin:
		proc, ok := p.(pluginapi.Processor)
		if !ok {
			return fmt.Errorf("plugin %s does not implement Processor", p.Name())
		}
		pr.Processors[p.Name()] = proc
	case pluginapi.ConsumerPlugin:
		cons, ok := p.(pluginapi.Consumer)
		if !ok {
			return fmt.Errorf("plugin %s does not implement Consumer", p.Name())
		}
		pr.Consumers[p.Name()] = cons
	default:
		return fmt.Errorf("unknown plugin type for plugin %s", p.Name())
	}
	return nil
}

type PluginManager struct {
	Registry *PluginRegistry
}

func NewPluginManager() *PluginManager {
	return &PluginManager{
		Registry: NewPluginRegistry(),
	}
}

func (pm *PluginManager) LoadPlugins(dir string, config map[string]interface{}) error {
	log.Printf("Loading plugins with global config: %+v", config)
	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || filepath.Ext(path) != ".so" {
			return nil
		}
		log.Printf("Loading plugin from %s", path)
		p, err := plugin.Open(path)
		if err != nil {
			return fmt.Errorf("failed to open plugin %s: %w", path, err)
		}
		newSymbol, err := p.Lookup("New")
		if err != nil {
			return fmt.Errorf("plugin %s does not export New: %w", path, err)
		}
		newFunc, ok := newSymbol.(func() pluginapi.Plugin)
		if !ok {
			return fmt.Errorf("plugin %s New symbol has wrong type", path)
		}
		instance := newFunc()
		if err := pm.Registry.Register(instance); err != nil {
			return fmt.Errorf("failed to register plugin %s: %w", instance.Name(), err)
		}
		log.Printf("Plugin %s v%s loaded successfully", instance.Name(), instance.Version())
		return nil
	})
}

// ------------------
// Pipeline and Core Engine Types
// ------------------
type Pipeline struct {
	Name       string
	Source     pluginapi.Source
	Processors []pluginapi.Processor
	Consumers  []pluginapi.Consumer
}

// BuildPipeline creates a Pipeline from its configuration.
// It merges the global plugin config with the pipeline-specific config.
func BuildPipeline(name string, def PipelineDefinition, reg *PluginRegistry) (*Pipeline, error) {
	// Use only the source-specific config
	sourceConfig := def.Source.Config

	src, ok := reg.Sources[def.Source.Type]
	if !ok {
		return nil, fmt.Errorf("source plugin %s not found", def.Source.Type)
	}
	if err := src.Initialize(sourceConfig); err != nil {
		return nil, fmt.Errorf("failed to initialize source %s: %w", def.Source.Type, err)
	}

	var procs []pluginapi.Processor
	var cons []pluginapi.Consumer

	// Connect processors in a chain
	for i, pDef := range def.Processors {
		log.Printf("Initializing processor %d: %s", i, pDef.Type)

		proc, ok := reg.Processors[pDef.Type]
		if !ok {
			return nil, fmt.Errorf("processor plugin %s not found", pDef.Type)
		}
		if err := proc.Initialize(pDef.Config); err != nil {
			return nil, fmt.Errorf("failed to initialize processor %s: %w", pDef.Type, err)
		}

		// First processor gets subscribed to the source
		if i == 0 {
			log.Printf("Subscribing first processor %s to source", proc.Name())
			src.Subscribe(proc)
		} else {
			// Chain processors together
			prevProc := procs[i-1]
			log.Printf("Attempting to chain processor %s to previous processor %s", proc.Name(), prevProc.Name())

			registry, ok := prevProc.(pluginapi.ConsumerRegistry)
			if !ok {
				return nil, fmt.Errorf("processor %s does not implement ConsumerRegistry", prevProc.Name())
			}

			consumer, ok := proc.(pluginapi.Consumer)
			if !ok {
				return nil, fmt.Errorf("processor %s does not implement Consumer", proc.Name())
			}

			log.Printf("Successfully chaining processor %s to %s", proc.Name(), prevProc.Name())
			registry.RegisterConsumer(consumer)
		}

		procs = append(procs, proc)
	}

	// Only register consumers with the last processor
	if len(procs) > 0 {
		lastProcessor := procs[len(procs)-1]
		log.Printf("Attempting to register consumers with last processor: %s", lastProcessor.Name())

		registry, ok := lastProcessor.(pluginapi.ConsumerRegistry)
		if !ok {
			return nil, fmt.Errorf("last processor %s does not implement ConsumerRegistry", lastProcessor.Name())
		}

		for _, cDef := range def.Consumers {
			consPlugin, ok := reg.Consumers[cDef.Type]
			if !ok {
				return nil, fmt.Errorf("consumer plugin %s not found", cDef.Type)
			}
			if err := consPlugin.Initialize(cDef.Config); err != nil {
				return nil, fmt.Errorf("failed to initialize consumer %s: %w", cDef.Type, err)
			}
			log.Printf("Registering consumer %s with last processor", consPlugin.Name())
			registry.RegisterConsumer(consPlugin)
			cons = append(cons, consPlugin)
		}
	}

	// Create pipeline
	pipeline := &Pipeline{
		Name:       name,
		Source:     src,
		Processors: procs,
		Consumers:  cons,
	}

	return pipeline, nil
}

// CoreEngine holds pipelines and provides a production interface.
type CoreEngine struct {
	PluginMgr      *PluginManager
	Pipelines      map[string]*Pipeline
	InstanceConfig InstanceConfig
}

// NewCoreEngine creates the engine by loading plugins and pipelines.
func NewCoreEngine(pluginsDir, pipelineConfigFile string) (*CoreEngine, error) {
	// Load pipeline configuration first
	pCfg, err := LoadPipelineConfig(pipelineConfigFile)
	if err != nil {
		return nil, err
	}

	// Create plugin manager and load plugins with empty config
	pm := NewPluginManager()
	if err := pm.LoadPlugins(pluginsDir, nil); err != nil {
		return nil, err
	}

	pipelines := make(map[string]*Pipeline)
	for name, def := range pCfg.Pipelines {
		p, err := BuildPipeline(name, def, pm.Registry)
		if err != nil {
			return nil, fmt.Errorf("error building pipeline %s: %w", name, err)
		}
		pipelines[name] = p
		log.Printf("Pipeline %s built successfully", name)
	}

	return &CoreEngine{
		PluginMgr: pm,
		Pipelines: pipelines,
	}, nil
}

// ProcessMessage routes a message through the specified pipeline.
func (ce *CoreEngine) ProcessMessage(ctx context.Context, pipelineName string, msg pluginapi.Message) error {
	pl, ok := ce.Pipelines[pipelineName]
	if !ok {
		metrics.ProcessingErrors.WithLabelValues(pipelineName, "pipeline", "not_found").Inc()
		return fmt.Errorf("pipeline %s not found", pipelineName)
	}

	// Increment source metric
	metrics.MessagesProcessed.WithLabelValues(
		ce.InstanceConfig.TenantID,
		ce.InstanceConfig.InstanceID,
		pipelineName,
		pl.Source.Name(),
	).Inc()

	// Process through processors with better error context
	for _, proc := range pl.Processors {
		start := time.Now()
		if err := proc.Process(ctx, msg); err != nil {
			metrics.ProcessingErrors.WithLabelValues(
				ce.InstanceConfig.TenantID,
				ce.InstanceConfig.InstanceID,
				pipelineName,
				proc.Name(),
				"process_error",
			).Inc()
			return fmt.Errorf("processor %s failed: %w", proc.Name(), err)
		}
		metrics.ProcessingDuration.WithLabelValues(
			ce.InstanceConfig.TenantID,
			ce.InstanceConfig.InstanceID,
			pipelineName,
			proc.Name(),
		).Observe(time.Since(start).Seconds())
		metrics.MessagesProcessed.WithLabelValues(
			ce.InstanceConfig.TenantID,
			ce.InstanceConfig.InstanceID,
			pipelineName,
			proc.Name(),
		).Inc()
	}

	// Process through consumers
	for _, cons := range pl.Consumers {
		start := time.Now()
		if err := cons.Process(ctx, msg); err != nil {
			metrics.ProcessingErrors.WithLabelValues(pipelineName, cons.Name(), "consume_error").Inc()
			return fmt.Errorf("consumer %s failed: %w", cons.Name(), err)
		}
		metrics.ProcessingDuration.WithLabelValues(pipelineName, cons.Name()).Observe(time.Since(start).Seconds())
		metrics.MessagesConsumed.WithLabelValues(pipelineName, cons.Name()).Inc()
	}

	return nil
}

// Shutdown stops sources and closes consumers.
func (ce *CoreEngine) Shutdown(ctx context.Context) error {
	log.Println("Initiating graceful shutdown...")

	// Report shutdown status
	if ce.InstanceConfig.CallbackURL != "" {
		status := map[string]interface{}{
			"instance_id": ce.InstanceConfig.InstanceID,
			"tenant_id":   ce.InstanceConfig.TenantID,
			"status":      "shutting_down",
			"timestamp":   time.Now().UTC().Format(time.RFC3339),
		}
		sendHeartbeat(ce.InstanceConfig, status)
	}

	var errs []error

	for name, pl := range ce.Pipelines {
		log.Printf("Shutting down pipeline: %s", name)

		// Stop source with timeout
		_, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		if err := pl.Source.Stop(); err != nil {
			log.Printf("Error stopping source %s: %v", pl.Source.Name(), err)
			errs = append(errs, fmt.Errorf("source %s: %w", pl.Source.Name(), err))
		}

		// Close consumers with timeout
		for _, cons := range pl.Consumers {
			if err := cons.Close(); err != nil {
				log.Printf("Error closing consumer %s: %v", cons.Name(), err)
				errs = append(errs, fmt.Errorf("consumer %s: %w", cons.Name(), err))
			}
		}
	}

	log.Println("Shutdown complete")
	if len(errs) > 0 {
		return fmt.Errorf("shutdown errors: %v", errs)
	}
	return nil
}

func verifyPipeline(p *Pipeline) {
	log.Printf("Verifying pipeline configuration:")
	log.Printf("Source: %s", p.Source.Name())

	for i, proc := range p.Processors {
		log.Printf("Processor %d: %s", i, proc.Name())
		if _, ok := proc.(pluginapi.ConsumerRegistry); ok {
			log.Printf("  Processor implements ConsumerRegistry")
		}
	}

	for i, cons := range p.Consumers {
		log.Printf("Consumer %d: %s", i, cons.Name())
	}
}

// Add to main.go
type InstanceConfig struct {
	InstanceID      string `json:"instance_id"`
	TenantID        string `json:"tenant_id"`
	APIKey          string `json:"api_key"`
	CallbackURL     string `json:"callback_url"`
	HeartbeatURL    string `json:"heartbeat_url"`
	HeartbeatPeriod string `json:"heartbeat_period"`
}

func startHeartbeat(ctx context.Context, config InstanceConfig) {
	ticker := time.NewTicker(30 * time.Second) // Or parse from config.HeartbeatPeriod
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			status := map[string]interface{}{
				"instance_id": config.InstanceID,
				"tenant_id":   config.TenantID,
				"status":      "running",
				"timestamp":   time.Now().UTC().Format(time.RFC3339),
				"metrics": map[string]int64{
					"messages_processed": atomic.LoadInt64(&globalMessageCount),
					"errors":             atomic.LoadInt64(&globalErrorCount),
				},
			}

			if err := sendHeartbeat(config, status); err != nil {
				log.Printf("Failed to send heartbeat: %v", err)
			}
		case <-ctx.Done():
			// Send final status before exiting
			sendHeartbeat(config, map[string]interface{}{
				"instance_id": config.InstanceID,
				"tenant_id":   config.TenantID,
				"status":      "stopped",
				"timestamp":   time.Now().UTC().Format(time.RFC3339),
			})
			return
		}
	}
}

func sendHeartbeat(config InstanceConfig, status map[string]interface{}) error {
	// Skip if no callback URL configured
	if config.HeartbeatURL == "" {
		return nil
	}

	jsonData, err := json.Marshal(status)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", config.HeartbeatURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+config.APIKey)

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("heartbeat failed with status: %d", resp.StatusCode)
	}

	return nil
}

// Add to CoreEngine
type ResourceLimits struct {
	MaxMemoryMB       int64
	MaxCPUPercent     int
	MaxMessagesPerSec int
	MaxTotalMessages  int64
}

// func (ce *CoreEngine) enforceResourceLimits(limits ResourceLimits) {
// 	// Set up monitoring and enforcement
// 	go func() {
// 		ticker := time.NewTicker(5 * time.Second)
// 		defer ticker.Stop()

// 		for range ticker.C {
// 			// Check memory usage
// 			var m runtime.MemStats
// 			runtime.ReadMemStats(&m)

// 			memoryMB := m.Alloc / 1024 / 1024
// 			if limits.MaxMemoryMB > 0 && memoryMB > uint64(limits.MaxMemoryMB) {
// 				log.Printf("WARNING: Memory usage exceeds limit: %d MB", memoryMB)
// 				// Implement throttling or backpressure
// 			}

// 			// Check message rate and implement throttling if needed
// 		}
// 	}()
// }

func (ce *CoreEngine) startConfigWatcher(ctx context.Context) {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// Check for configuration updates
			updated := false
			// err := nil

			if updated {
				log.Println("Configuration updated, reloading pipelines...")
				// Implement hot reload of pipelines
			}
		case <-ctx.Done():
			return
		}
	}
}

func main() {
	// Add new command-line flags
	instanceID := flag.String("instance-id", "", "Unique ID for this pipeline instance")
	tenantID := flag.String("tenant-id", "", "ID of the tenant who owns this pipeline")
	apiKey := flag.String("api-key", "", "API key for authenticating with Obsrvr platform")
	callbackURL := flag.String("callback-url", "", "URL to send status updates to")
	pluginsDir := flag.String("plugins", "./plugins", "Directory containing plugin .so files")
	pipelineConfigFile := flag.String("pipeline", "pipeline_example.yaml", "Path to the pipeline configuration YAML file")
	flag.Parse()

	// Validate required parameters
	if *instanceID == "" || *tenantID == "" || *apiKey == "" {
		log.Fatalf("instance-id, tenant-id, and api-key are required")
	}

	// Initialize instance config
	instanceConfig := InstanceConfig{
		InstanceID:  *instanceID,
		TenantID:    *tenantID,
		APIKey:      *apiKey,
		CallbackURL: *callbackURL,
	}

	// Start Prometheus metrics endpoint first
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

	// Create the core engine without global plugin config
	engine, err := NewCoreEngine(*pluginsDir, *pipelineConfigFile)
	if err != nil {
		log.Fatalf("Error initializing core engine: %v", err)
	}
	engine.InstanceConfig = instanceConfig
	// Start config watcher
	go engine.startConfigWatcher(context.Background())
	defer engine.Shutdown(context.Background())

	// Create a context with cancellation for managing pipeline lifecycles
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start heartbeat monitoring
	go startHeartbeat(ctx, instanceConfig)

	// Start each pipeline's source
	var wg sync.WaitGroup
	for name, pipeline := range engine.Pipelines {
		wg.Add(1)
		go func(name string, p *Pipeline) {
			defer wg.Done()
			log.Printf("Starting pipeline: %s", name)

			// Test metric
			metrics.MessagesProcessed.WithLabelValues(
				engine.InstanceConfig.TenantID,
				engine.InstanceConfig.InstanceID,
				name,
				"startup",
			).Inc()

			// Start the source
			if err := p.Source.Start(ctx); err != nil {
				metrics.ProcessingErrors.WithLabelValues(name, "source", "startup_error").Inc()
				log.Printf("Error in pipeline %s: %v", name, err)
				cancel() // Cancel other pipelines on error
				return
			}
		}(name, pipeline)
	}

	log.Println("Core engine initialized and running in production mode.")

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Wait for either interrupt or pipeline completion
	select {
	case sig := <-sigChan:
		log.Printf("Received signal %v, shutting down...", sig)
		cancel() // Cancel all pipelines
	case <-ctx.Done():
		log.Println("Context cancelled, shutting down...")
	}

	// Wait for all pipelines to complete
	wg.Wait()

	// After creating the engine
	for name, pipeline := range engine.Pipelines {
		log.Printf("Verifying pipeline: %s", name)
		verifyPipeline(pipeline)
	}
}
