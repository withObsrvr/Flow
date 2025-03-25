package pluginmanager

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
	"github.com/withObsrvr/pluginapi"
)

// WASMPlugin implements the Plugin interface for WASM plugins
type WASMPlugin struct {
	name           string
	version        string
	pluginType     pluginapi.PluginType
	ctx            context.Context
	runtime        wazero.Runtime
	module         api.Module
	initializeFunc api.Function
	processFunc    api.Function
	closeFunc      api.Function
	allocFunc      api.Function
	freeFunc       api.Function
	initialized    bool
	config         map[string]interface{}
}

// Make sure WASMPlugin implements the Plugin interfaces
var _ pluginapi.Plugin = (*WASMPlugin)(nil)
var _ pluginapi.Source = (*WASMPlugin)(nil)
var _ pluginapi.Processor = (*WASMPlugin)(nil)
var _ pluginapi.Consumer = (*WASMPlugin)(nil)

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

// Make sure WASMProcessorPlugin implements the required interfaces
var _ pluginapi.Plugin = (*WASMProcessorPlugin)(nil)
var _ pluginapi.Processor = (*WASMProcessorPlugin)(nil)

// WASMPluginLoader loads WebAssembly plugins (.wasm files)
type WASMPluginLoader struct{}

// CanLoad returns true if the file has a .wasm extension
func (l *WASMPluginLoader) CanLoad(path string) bool {
	isWasm := filepath.Ext(path) == ".wasm"
	log.Printf("WASM Loader checking file: %s, is WASM: %v", path, isWasm)

	// If FLOW_DEBUG_WASM is set, print more info
	if _, exists := os.LookupEnv("FLOW_DEBUG_WASM"); exists {
		fileInfo, err := os.Stat(path)
		if err != nil {
			log.Printf("DEBUG WASM: Error accessing file %s: %v", path, err)
		} else {
			log.Printf("DEBUG WASM: File %s exists, size: %d bytes, mode: %s",
				path, fileInfo.Size(), fileInfo.Mode().String())
		}
	}

	return isWasm
}

// LoadPlugin loads a WASM plugin from the given path
func (l *WASMPluginLoader) LoadPlugin(path string) (pluginapi.Plugin, error) {
	log.Printf("Loading WASM plugin from %s", path)

	// Check if this is one of our special processor WASM modules
	isProcessorWASM := filepath.Base(path) == "flow-processor-latest-ledger.wasm" ||
		filepath.Base(path) == "flow-processor-effects.wasm" ||
		filepath.Base(path) == "flow-processor-contract-events.wasm" ||
		filepath.Base(path) == "flow-processor-contract-invocations.wasm"

	if isProcessorWASM {
		return l.loadProcessorWASM(path)
	}

	// If it's not a processor WASM, continue with the standard WASM loading
	return l.loadStandardWASM(path)
}

// loadStandardWASM loads a standard WASM plugin that uses memory pointers
func (l *WASMPluginLoader) loadStandardWASM(path string) (pluginapi.Plugin, error) {
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

	// Look up the functions we need
	nameFunc := instance.ExportedFunction("name")
	versionFunc := instance.ExportedFunction("version")
	typeFunc := instance.ExportedFunction("type")
	initializeFunc := instance.ExportedFunction("initialize")
	processFunc := instance.ExportedFunction("process")
	closeFunc := instance.ExportedFunction("close")
	allocFunc := instance.ExportedFunction("alloc")
	freeFunc := instance.ExportedFunction("free")

	// Check that all required functions are present
	if nameFunc == nil || versionFunc == nil || typeFunc == nil ||
		initializeFunc == nil || processFunc == nil || closeFunc == nil ||
		allocFunc == nil || freeFunc == nil {
		return nil, fmt.Errorf("WASM module %s is missing required exports", path)
	}

	// Call the name function to get the plugin name
	nameResult, err := nameFunc.Call(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to call name function: %w", err)
	}

	namePtr, nameLen := nameResult[0], nameResult[1]
	nameBuf, ok := instance.Memory().Read(uint32(namePtr), uint32(nameLen))
	if !ok {
		return nil, fmt.Errorf("failed to read name from memory")
	}
	name := string(nameBuf)

	// Call the version function to get the plugin version
	versionResult, err := versionFunc.Call(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to call version function: %w", err)
	}

	versionPtr, versionLen := versionResult[0], versionResult[1]
	versionBuf, ok := instance.Memory().Read(uint32(versionPtr), uint32(versionLen))
	if !ok {
		return nil, fmt.Errorf("failed to read version from memory")
	}
	version := string(versionBuf)

	// Call the type function to get the plugin type
	typeResult, err := typeFunc.Call(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to call type function: %w", err)
	}

	pluginType := pluginapi.PluginType(typeResult[0])

	// Create the WASM plugin
	return &WASMPlugin{
		name:           name,
		version:        version,
		pluginType:     pluginType,
		ctx:            ctx,
		runtime:        runtime,
		module:         instance,
		initializeFunc: initializeFunc,
		processFunc:    processFunc,
		closeFunc:      closeFunc,
		allocFunc:      allocFunc,
		freeFunc:       freeFunc,
		initialized:    false,
	}, nil
}

// loadProcessorWASM loads a processor WASM module that uses string-based exports
func (l *WASMPluginLoader) loadProcessorWASM(path string) (pluginapi.Plugin, error) {
	log.Printf("Loading processor-style WASM module from %s", path)

	// Debug environment check
	if _, exists := os.LookupEnv("FLOW_DEBUG_WASM"); exists {
		log.Printf("DEBUG WASM: Attempting to load processor WASM from %s", path)
	}

	// Read the WASM file
	wasmBytes, err := ioutil.ReadFile(path)
	if err != nil {
		log.Printf("Failed to read WASM file %s: %v", path, err)
		return nil, fmt.Errorf("failed to read WASM file %s: %w", path, err)
	}
	log.Printf("Read processor WASM file successfully, size: %d bytes", len(wasmBytes))

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

	// Look up the functions we need for processor-style WASM modules
	nameFunc := instance.ExportedFunction("name")
	versionFunc := instance.ExportedFunction("version")
	initializeFunc := instance.ExportedFunction("initialize")
	processLedgerFunc := instance.ExportedFunction("processLedger")
	getSchemaDefFunc := instance.ExportedFunction("getSchemaDefinition")
	getQueryDefsFunc := instance.ExportedFunction("getQueryDefinitions")

	// Check that all required functions are present
	if nameFunc == nil || versionFunc == nil || initializeFunc == nil || processLedgerFunc == nil {
		return nil, fmt.Errorf("processor WASM module %s is missing required exports", path)
	}

	// Call the name function to get the plugin name
	result, err := nameFunc.Call(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to call name function: %w", err)
	}
	// For processor WASM modules, the name function is expected to return a string directly
	name := string(result[0])

	// If the name is empty or doesn't look right, default to the path basename
	if name == "" || name == "0" {
		log.Printf("Warning: name function didn't return a proper value, using default name")
		// Extract the processor name from the filename
		baseName := filepath.Base(path)
		// Remove extension
		name = strings.TrimSuffix(baseName, filepath.Ext(baseName))
		// Remove the flow-processor- prefix if it exists
		name = strings.TrimPrefix(name, "flow-processor-")
		// Add the flow processor prefix to match expected format
		name = "flow/processor/" + name
	}

	// Call the version function to get the plugin version
	result, err = versionFunc.Call(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to call version function: %w", err)
	}
	version := string(result[0])
	if version == "" || version == "0" {
		version = "1.0.0" // Default version
	}

	log.Printf("Loaded processor WASM module %s: name=%s, version=%s", path, name, version)

	// Create the WASM processor plugin
	return &WASMProcessorPlugin{
		name:              name,
		version:           version,
		pluginType:        pluginapi.ProcessorPlugin, // Always a processor
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

// Name implements the Plugin interface
func (p *WASMPlugin) Name() string {
	return p.name
}

// Version implements the Plugin interface
func (p *WASMPlugin) Version() string {
	return p.version
}

// Type implements the Plugin interface
func (p *WASMPlugin) Type() pluginapi.PluginType {
	return p.pluginType
}

// Initialize implements the Plugin interface
func (p *WASMPlugin) Initialize(config map[string]interface{}) error {
	if p.initialized {
		return nil
	}

	p.config = config

	// Convert the config to JSON
	configJSON, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal config to JSON: %w", err)
	}

	// Allocate memory for the config
	allocResult, err := p.allocFunc.Call(p.ctx, uint64(len(configJSON)))
	if err != nil {
		return fmt.Errorf("failed to allocate memory for config: %w", err)
	}

	configPtr := allocResult[0]

	// Write the config to memory
	ok := p.module.Memory().Write(uint32(configPtr), configJSON)
	if !ok {
		return fmt.Errorf("failed to write config to memory")
	}

	// Call the initialize function
	_, err = p.initializeFunc.Call(p.ctx, configPtr, uint64(len(configJSON)))
	if err != nil {
		return fmt.Errorf("failed to initialize plugin: %w", err)
	}

	// Free the memory
	_, err = p.freeFunc.Call(p.ctx, configPtr, uint64(len(configJSON)))
	if err != nil {
		return fmt.Errorf("failed to free memory: %w", err)
	}

	p.initialized = true
	return nil
}

// Process implements the Processor and Consumer interfaces
func (p *WASMPlugin) Process(ctx context.Context, msg pluginapi.Message) error {
	if !p.initialized {
		return fmt.Errorf("plugin not initialized")
	}

	// Convert the message to JSON
	msgJSON, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message to JSON: %w", err)
	}

	// Allocate memory for the message
	allocResult, err := p.allocFunc.Call(p.ctx, uint64(len(msgJSON)))
	if err != nil {
		return fmt.Errorf("failed to allocate memory for message: %w", err)
	}

	msgPtr := allocResult[0]

	// Write the message to memory
	ok := p.module.Memory().Write(uint32(msgPtr), msgJSON)
	if !ok {
		return fmt.Errorf("failed to write message to memory")
	}

	// Call the process function
	_, err = p.processFunc.Call(p.ctx, msgPtr, uint64(len(msgJSON)))
	if err != nil {
		return fmt.Errorf("failed to process message: %w", err)
	}

	// Free the memory
	_, err = p.freeFunc.Call(p.ctx, msgPtr, uint64(len(msgJSON)))
	if err != nil {
		return fmt.Errorf("failed to free memory: %w", err)
	}

	return nil
}

// Subscribe implements the Source interface
func (p *WASMPlugin) Subscribe(processor pluginapi.Processor) {
	// This is a placeholder - we can't directly call the Subscribe function in WASM
	// In a real implementation, we would need to handle this differently
	log.Printf("Subscribe called on WASM plugin %s, this is not fully implemented yet", p.name)
}

// Start implements the Source interface
func (p *WASMPlugin) Start(ctx context.Context) error {
	// This is a placeholder - we can't directly call the Start function in WASM
	// In a real implementation, we would need to handle this differently
	log.Printf("Start called on WASM plugin %s, this is not fully implemented yet", p.name)
	return nil
}

// Stop implements the Source interface
func (p *WASMPlugin) Stop() error {
	// This is a placeholder - we can't directly call the Stop function in WASM
	// In a real implementation, we would need to handle this differently
	log.Printf("Stop called on WASM plugin %s, this is not fully implemented yet", p.name)
	return nil
}

// RegisterConsumer implements the Processor interface
func (p *WASMPlugin) RegisterConsumer(consumer pluginapi.Consumer) {
	// This is a placeholder - we can't directly call the RegisterConsumer function in WASM
	// In a real implementation, we would need to handle this differently
	log.Printf("RegisterConsumer called on WASM plugin %s, this is not fully implemented yet", p.name)
}

// Close implements the Plugin interface
func (p *WASMPlugin) Close() error {
	if !p.initialized {
		return nil
	}

	// Call the close function
	_, err := p.closeFunc.Call(p.ctx)
	if err != nil {
		return fmt.Errorf("failed to close plugin: %w", err)
	}

	// Close the runtime
	err = p.runtime.Close(p.ctx)
	if err != nil {
		return fmt.Errorf("failed to close runtime: %w", err)
	}

	p.initialized = false
	return nil
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
	configStr := string(configJSON)
	log.Printf("Initializing processor WASM with config: %s", configStr)

	// Different WASM modules might have different function signatures
	// Handle both string and []byte parameters
	var result []uint64
	var callErr error

	// Try to pass the string directly
	result, callErr = p.initializeFunc.Call(p.ctx, uint64(len(configStr)))
	if callErr != nil {
		// If that fails, try passing the config as a string parameter
		// Create a special array in memory for the string
		memory := p.module.Memory()
		configPtr := uint32(1024) // Just use a fixed address for now
		if memory != nil {
			if !memory.Write(configPtr, configJSON) {
				return fmt.Errorf("failed to write config to memory")
			}
			result, callErr = p.initializeFunc.Call(p.ctx, uint64(configPtr), uint64(len(configJSON)))
		}

		if callErr != nil {
			return fmt.Errorf("failed to initialize plugin (all methods): %w", callErr)
		}
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
	payloadStr := string(payloadJSON)
	log.Printf("Processing message with processor WASM: %s", payloadStr[:min(100, len(payloadStr))]) // Log just the first 100 chars

	// Different WASM modules might have different function signatures
	// Handle both string and []byte parameters
	var result []uint64
	var callErr error

	// Try to pass the string directly
	result, callErr = p.processLedgerFunc.Call(p.ctx, uint64(len(payloadStr)))
	if callErr != nil {
		// If that fails, try passing the payload as a string parameter
		// Create a special array in memory for the string
		memory := p.module.Memory()
		payloadPtr := uint32(1024) // Just use a fixed address for now
		if memory != nil {
			if !memory.Write(payloadPtr, payloadJSON) {
				return fmt.Errorf("failed to write payload to memory")
			}
			result, callErr = p.processLedgerFunc.Call(p.ctx, uint64(payloadPtr), uint64(len(payloadJSON)))
		}

		if callErr != nil {
			return fmt.Errorf("failed to process message (all methods): %w", callErr)
		}
	}

	// Get the result as a string
	var resultJSON string
	if len(result) > 0 {
		// Try to read from memory if the result is a pointer
		if result[0] > 0 && p.module.Memory() != nil {
			// Try to read a string from memory at the pointer
			resultBuf, ok := p.module.Memory().Read(uint32(result[0]), 1024) // Read up to 1KB
			if ok && len(resultBuf) > 0 {
				// Find null terminator if any
				nullIdx := 0
				for i, b := range resultBuf {
					if b == 0 {
						nullIdx = i
						break
					}
				}
				if nullIdx > 0 {
					resultJSON = string(resultBuf[:nullIdx])
				} else {
					resultJSON = string(resultBuf)
				}
			}
		}

		if resultJSON == "" {
			// If we couldn't read from memory, just convert the result to a string
			resultJSON = fmt.Sprintf("%v", result[0])
		}
	}

	// Parse the result JSON if needed
	var resultData interface{}
	if err := json.Unmarshal([]byte(resultJSON), &resultData); err != nil {
		log.Printf("Warning: Failed to parse result JSON: %v", err)
		resultData = resultJSON // Just use the string if parsing fails
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

	return string(result[0])
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

	return string(result[0])
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

// min returns the smaller of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
