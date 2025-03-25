package pluginmanager

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"path/filepath"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
	"github.com/withObsrvr/pluginapi"
)

// Helper functions for string encoding/decoding
func encodeString(s string) uint64 {
	// For now, just return a placeholder
	// In a real implementation, we would need to properly encode the string
	// and ensure memory management, but this is a simplification
	return uint64(len(s))
}

func decodeString(val uint64) string {
	// In a real implementation, we would read from WASM memory
	// This is a simplification that will be replaced with proper implementation
	return ""
}

// WASMProcessorLoader loads WebAssembly processor plugins that use string-based exports
// rather than the memory pointer exports used by the standard WASMPluginLoader.
type WASMProcessorLoader struct{}

// CanLoad returns true if the file has a .wasm extension and appears to be a processor WASM module
func (l *WASMProcessorLoader) CanLoad(path string) bool {
	if filepath.Ext(path) != ".wasm" {
		return false
	}

	// Only handle WASM files that are processors
	return filepath.Base(path) == "flow-processor-latest-ledger.wasm" ||
		filepath.Base(path) == "flow-processor-effects.wasm" ||
		filepath.Base(path) == "flow-processor-contract-events.wasm" ||
		filepath.Base(path) == "flow-processor-contract-invocations.wasm"
}

// LoadPlugin loads a WASM plugin from the given path
func (l *WASMProcessorLoader) LoadPlugin(path string) (pluginapi.Plugin, error) {
	log.Printf("Loading WASM processor plugin from %s", path)

	// Read the WASM file
	wasmBytes, err := ioutil.ReadFile(path)
	if err != nil {
		log.Printf("Failed to read WASM file %s: %v", path, err)
		return nil, fmt.Errorf("failed to read WASM file %s: %w", path, err)
	}
	log.Printf("Read WASM file successfully, size: %d bytes", len(wasmBytes))

	// Create a context
	ctx := context.Background()

	// Create a new WebAssembly Runtime
	runtime := wazero.NewRuntime(ctx)

	// Add WASI support to the runtime
	wasi_snapshot_preview1.MustInstantiate(ctx, runtime)

	// Compile the WASM module
	module, err := runtime.CompileModule(ctx, wasmBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to compile WASM module %s: %w", path, err)
	}

	// Get module name and exit if none
	modName := filepath.Base(path)
	if module.Name() != "" {
		modName = module.Name()
	}

	// Create a configuration for the module
	config := wazero.NewModuleConfig().
		WithName(modName).
		WithStdout(log.Writer()).
		WithStderr(log.Writer())

	// Instantiate the module
	instance, err := runtime.InstantiateModule(ctx, module, config)
	if err != nil {
		return nil, fmt.Errorf("failed to instantiate WASM module %s: %w", path, err)
	}

	// Look up the functions we need for this type of processor
	nameFunc := instance.ExportedFunction("name")
	versionFunc := instance.ExportedFunction("version")
	initializeFunc := instance.ExportedFunction("initialize")
	processLedgerFunc := instance.ExportedFunction("processLedger")
	getSchemaDefFunc := instance.ExportedFunction("getSchemaDefinition")
	getQueryDefsFunc := instance.ExportedFunction("getQueryDefinitions")

	// Check that all required functions are present
	if nameFunc == nil || versionFunc == nil || initializeFunc == nil || processLedgerFunc == nil {
		return nil, fmt.Errorf("WASM module %s is missing required exports", path)
	}

	// Call the name function to get the plugin name
	nameResult, err := nameFunc.Call(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to call name function: %w", err)
	}
	name := decodeString(nameResult[0])

	// Call the version function to get the plugin version
	versionResult, err := versionFunc.Call(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to call version function: %w", err)
	}
	version := decodeString(versionResult[0])

	// Create the WASM processor plugin
	return &WASMProcessorPlugin{
		name:              name,
		version:           version,
		pluginType:        pluginapi.ProcessorPlugin,
		ctx:               ctx,
		runtime:           runtime,
		module:            instance,
		initializeFunc:    initializeFunc,
		processLedgerFunc: processLedgerFunc,
		getSchemaDefFunc:  getSchemaDefFunc,
		getQueryDefsFunc:  getQueryDefsFunc,
		initialized:       false,
	}, nil
}

// WASMProcessorPlugin implements the Plugin/Processor interfaces for string-based WASM exports
type WASMProcessorPlugin struct {
	name              string
	version           string
	pluginType        pluginapi.PluginType
	ctx               context.Context
	runtime           wazero.Runtime
	module            api.Module
	initializeFunc    api.Function
	processLedgerFunc api.Function
	getSchemaDefFunc  api.Function
	getQueryDefsFunc  api.Function
	initialized       bool
	config            map[string]interface{}
	consumers         []pluginapi.Consumer  // Downstream consumers
	processors        []pluginapi.Processor // Downstream processors
}

// Name implements the Plugin interface
func (p *WASMProcessorPlugin) Name() string {
	return p.name
}

// Version implements the Plugin interface
func (p *WASMProcessorPlugin) Version() string {
	return p.version
}

// Type implements the Plugin interface
func (p *WASMProcessorPlugin) Type() pluginapi.PluginType {
	return p.pluginType
}

// Initialize implements the Plugin interface
func (p *WASMProcessorPlugin) Initialize(config map[string]interface{}) error {
	if p.initialized {
		return nil
	}

	p.config = config
	p.consumers = make([]pluginapi.Consumer, 0)
	p.processors = make([]pluginapi.Processor, 0)

	// Convert the config to JSON
	configJSON, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal config to JSON: %w", err)
	}

	// Call the initialize function with the JSON string
	// This expects the initialize function to have the signature: initialize(configJSON string) int32
	result, err := p.initializeFunc.Call(p.ctx, encodeString(string(configJSON)))
	if err != nil {
		return fmt.Errorf("failed to initialize plugin: %w", err)
	}

	// Check the result (0 = success)
	if result[0] != 0 {
		return fmt.Errorf("plugin initialization returned error code: %d", result[0])
	}

	p.initialized = true
	return nil
}

// Process implements the Processor interface
func (p *WASMProcessorPlugin) Process(ctx context.Context, msg pluginapi.Message) error {
	if !p.initialized {
		return fmt.Errorf("plugin not initialized")
	}

	// Convert the message payload to JSON
	payloadJSON, err := json.Marshal(msg.Payload)
	if err != nil {
		return fmt.Errorf("failed to marshal message payload to JSON: %w", err)
	}

	// Call the process function with the JSON string
	// This expects the processLedger function to have the signature: processLedger(ledgerJSON string) string
	result, err := p.processLedgerFunc.Call(p.ctx, encodeString(string(payloadJSON)))
	if err != nil {
		return fmt.Errorf("failed to process message: %w", err)
	}

	resultJSON := decodeString(result[0])

	// Parse the result JSON if needed
	var resultData interface{}
	if err := json.Unmarshal([]byte(resultJSON), &resultData); err != nil {
		return fmt.Errorf("failed to parse result JSON: %w", err)
	}

	// Create forward message
	forwardMsg := pluginapi.Message{
		Payload:   resultData,
		Timestamp: msg.Timestamp,
		Metadata: map[string]interface{}{
			"source":    p.name,
			"data_type": "latest_ledger",
		},
	}

	// Forward to consumers
	for _, consumer := range p.consumers {
		if err := consumer.Process(ctx, forwardMsg); err != nil {
			log.Printf("Error in consumer %s: %v", consumer.Name(), err)
		}
	}

	// Forward to processors
	for _, proc := range p.processors {
		if err := proc.Process(ctx, forwardMsg); err != nil {
			log.Printf("Error in processor %s: %v", proc.Name(), err)
		}
	}

	return nil
}

// RegisterConsumer registers a downstream consumer
func (p *WASMProcessorPlugin) RegisterConsumer(consumer pluginapi.Consumer) {
	log.Printf("WASMProcessorPlugin: Registering consumer %s", consumer.Name())
	p.consumers = append(p.consumers, consumer)
}

// Subscribe registers a downstream processor
func (p *WASMProcessorPlugin) Subscribe(proc pluginapi.Processor) {
	log.Printf("WASMProcessorPlugin: Registering processor %s", proc.Name())
	p.processors = append(p.processors, proc)
}

// GetSchemaDefinition returns GraphQL type definitions for this plugin
func (p *WASMProcessorPlugin) GetSchemaDefinition() string {
	if p.getSchemaDefFunc == nil {
		return ""
	}

	result, err := p.getSchemaDefFunc.Call(p.ctx)
	if err != nil {
		log.Printf("Failed to get schema definition: %v", err)
		return ""
	}

	return decodeString(result[0])
}

// GetQueryDefinitions returns GraphQL query definitions for this plugin
func (p *WASMProcessorPlugin) GetQueryDefinitions() string {
	if p.getQueryDefsFunc == nil {
		return ""
	}

	result, err := p.getQueryDefsFunc.Call(p.ctx)
	if err != nil {
		log.Printf("Failed to get query definitions: %v", err)
		return ""
	}

	return decodeString(result[0])
}

// Close implements the Plugin interface
func (p *WASMProcessorPlugin) Close() error {
	if !p.initialized {
		return nil
	}

	// Close the runtime
	err := p.runtime.Close(p.ctx)
	if err != nil {
		return fmt.Errorf("failed to close runtime: %w", err)
	}

	p.initialized = false
	return nil
}

// Register the processor-focused WASM loader
func init() {
	log.Printf("Registering WASMProcessorLoader for processor-style WASM modules")
	RegisterLoader(&WASMProcessorLoader{})
}
