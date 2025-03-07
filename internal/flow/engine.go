package flow

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"plugin"
	"time"

	"github.com/withObsrvr/Flow/pkg/schemaapi"
	"github.com/withObsrvr/pluginapi"
	"gopkg.in/yaml.v2"
)

// InstanceConfig holds configuration for a Flow instance
type InstanceConfig struct {
	InstanceID      string `json:"instance_id"`
	TenantID        string `json:"tenant_id"`
	APIKey          string `json:"api_key"`
	CallbackURL     string `json:"callback_url"`
	HeartbeatURL    string `json:"heartbeat_url"`
	HeartbeatPeriod string `json:"heartbeat_period"`
}

// PipelineConfig holds the configuration for all pipelines
type PipelineConfig struct {
	Pipelines map[string]PipelineDefinition `yaml:"pipelines"`
}

// PipelineDefinition defines a single pipeline
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

// LoadPipelineConfig loads pipeline configuration from a file
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

// PluginRegistry holds references to all loaded plugins
type PluginRegistry struct {
	Sources    map[string]pluginapi.Source
	Processors map[string]pluginapi.Processor
	Consumers  map[string]pluginapi.Consumer
}

// NewPluginRegistry creates a new plugin registry
func NewPluginRegistry() *PluginRegistry {
	return &PluginRegistry{
		Sources:    make(map[string]pluginapi.Source),
		Processors: make(map[string]pluginapi.Processor),
		Consumers:  make(map[string]pluginapi.Consumer),
	}
}

// Register adds a plugin to the registry
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

// PluginManager manages loading and initializing plugins
type PluginManager struct {
	Registry *PluginRegistry
}

// NewPluginManager creates a new plugin manager
func NewPluginManager() *PluginManager {
	return &PluginManager{
		Registry: NewPluginRegistry(),
	}
}

// LoadPlugins loads plugins from a directory
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

// Pipeline represents a configured data processing pipeline
type Pipeline struct {
	Name       string
	Source     pluginapi.Source
	Processors []pluginapi.Processor
	Consumers  []pluginapi.Consumer
}

// BuildPipeline creates a Pipeline from its configuration
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

// CoreEngine holds pipelines and provides a production interface
type CoreEngine struct {
	PluginMgr      *PluginManager
	Pipelines      map[string]*Pipeline
	InstanceConfig InstanceConfig
	ctx            context.Context
	cancel         context.CancelFunc
}

// NewCoreEngine creates the engine by loading plugins and pipelines
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

	// Create context for the engine
	ctx, cancel := context.WithCancel(context.Background())

	engine := &CoreEngine{
		PluginMgr: pm,
		Pipelines: pipelines,
		ctx:       ctx,
		cancel:    cancel,
	}

	// Start all sources
	for name, pipeline := range pipelines {
		log.Printf("Starting source for pipeline: %s", name)
		if err := pipeline.Source.Start(ctx); err != nil {
			cancel() // Cancel context on error
			return nil, fmt.Errorf("error starting source for pipeline %s: %w", name, err)
		}
	}

	return engine, nil
}

// ProcessMessage routes a message through the specified pipeline
func (ce *CoreEngine) ProcessMessage(ctx context.Context, pipelineName string, msg pluginapi.Message) error {
	pl, ok := ce.Pipelines[pipelineName]
	if !ok {
		return fmt.Errorf("pipeline %s not found", pipelineName)
	}

	// Process through processors
	for _, proc := range pl.Processors {
		if err := proc.Process(ctx, msg); err != nil {
			return fmt.Errorf("processor %s failed: %w", proc.Name(), err)
		}
	}

	// Process through consumers
	for _, cons := range pl.Consumers {
		if err := cons.Process(ctx, msg); err != nil {
			return fmt.Errorf("consumer %s failed: %w", cons.Name(), err)
		}
	}

	return nil
}

// Shutdown stops sources and closes consumers
func (ce *CoreEngine) Shutdown(ctx context.Context) error {
	log.Println("Initiating graceful shutdown...")

	// Cancel the engine context
	if ce.cancel != nil {
		ce.cancel()
	}

	var errs []error

	for name, pl := range ce.Pipelines {
		log.Printf("Shutting down pipeline: %s", name)

		if err := pl.Source.Stop(); err != nil {
			log.Printf("Error stopping source %s: %v", pl.Source.Name(), err)
			errs = append(errs, fmt.Errorf("source %s: %w", pl.Source.Name(), err))
		}

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

// StartConfigWatcher starts a goroutine that watches for configuration changes
func (ce *CoreEngine) StartConfigWatcher(ctx context.Context) {
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

// RegisterSchemas registers plugin schemas with the schema registry
func RegisterSchemas(engine *CoreEngine, schemaRegistryURL string) error {
	log.Printf("Registering schemas with registry at %s", schemaRegistryURL)

	// Collect schemas from plugins that implement SchemaProvider
	for name, pipeline := range engine.Pipelines {
		log.Printf("Checking pipeline %s for schema providers", name)

		// Check source
		if schemaProvider, ok := pipeline.Source.(schemaapi.SchemaProvider); ok {
			if err := registerPluginSchema(pipeline.Source.Name(), schemaProvider, schemaRegistryURL); err != nil {
				return err
			}
		}

		// Check processors
		for _, proc := range pipeline.Processors {
			if schemaProvider, ok := proc.(schemaapi.SchemaProvider); ok {
				if err := registerPluginSchema(proc.Name(), schemaProvider, schemaRegistryURL); err != nil {
					return err
				}
			}
		}

		// Check consumers
		for _, cons := range pipeline.Consumers {
			if schemaProvider, ok := cons.(schemaapi.SchemaProvider); ok {
				if err := registerPluginSchema(cons.Name(), schemaProvider, schemaRegistryURL); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

// registerPluginSchema registers a single plugin's schema with the registry
func registerPluginSchema(pluginName string, provider schemaapi.SchemaProvider, registryURL string) error {
	log.Printf("Registering schema for plugin: %s", pluginName)

	registration := schemaapi.SchemaRegistration{
		PluginName: pluginName,
		Schema:     provider.GetSchemaDefinition(),
		Queries:    provider.GetQueryDefinitions(),
	}

	data, err := json.Marshal(registration)
	if err != nil {
		return err
	}

	resp, err := http.Post(registryURL+"/register", "application/json", bytes.NewBuffer(data))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("schema registration failed with status: %d", resp.StatusCode)
	}

	log.Printf("Successfully registered schema for plugin: %s", pluginName)
	return nil
}
