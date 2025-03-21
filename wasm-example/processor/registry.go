package processor

import (
	"context"
	"fmt"
	"os"
	"plugin"
	"sync"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

// Registry manages processor types
type Registry struct {
	processors  map[string]Processor
	mu          sync.RWMutex
	wasmRuntime wazero.Runtime
}

// NewRegistry creates a new registry
func NewRegistry() *Registry {
	// Initialize the WebAssembly runtime
	ctx := context.Background()
	runtime := wazero.NewRuntime(ctx)

	// Initialize WASI
	wasi_snapshot_preview1.MustInstantiate(ctx, runtime)

	return &Registry{
		processors:  make(map[string]Processor),
		wasmRuntime: runtime,
	}
}

// LoadGoPlugin loads a Go plugin and registers its processor
func (r *Registry) LoadGoPlugin(path string, name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Skip if already loaded
	if _, exists := r.processors[name]; exists {
		return nil
	}

	// Load the plugin
	plug, err := plugin.Open(path)
	if err != nil {
		return fmt.Errorf("failed to open plugin %s: %w", path, err)
	}

	// Look up the "NewProcessor" symbol
	sym, err := plug.Lookup("NewProcessor")
	if err != nil {
		return fmt.Errorf("plugin %s does not export 'NewProcessor' symbol: %w", path, err)
	}

	// Check if the symbol is a function that returns a Processor
	newProcessorFunc, ok := sym.(func() interface{})
	if !ok {
		return fmt.Errorf("plugin %s's 'NewProcessor' is not func() interface{}", path)
	}

	// Create a new processor instance
	instance := newProcessorFunc()

	// Check if the instance implements the Processor interface
	processor, ok := instance.(Processor)
	if !ok {
		return fmt.Errorf("plugin %s's 'NewProcessor' does not return a Processor", path)
	}

	// Register the processor
	r.processors[name] = &GoPluginAdapter{
		Processor: processor,
		Name:      name,
	}

	return nil
}

// LoadWasmModule loads a WebAssembly module and registers its processor
func (r *Registry) LoadWasmModule(ctx context.Context, path string, name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Skip if already loaded
	if _, exists := r.processors[name]; exists {
		return nil
	}

	// Read the WASM module
	wasmBytes, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read WASM file %s: %w", path, err)
	}

	// Compile the module
	module, err := r.wasmRuntime.CompileModule(ctx, wasmBytes)
	if err != nil {
		return fmt.Errorf("failed to compile WASM module %s: %w", path, err)
	}

	// Create a new processor instance
	processor := &WasmProcessor{
		Path:    path,
		Name:    name,
		Module:  module,
		Runtime: r.wasmRuntime,
	}

	// Register the processor
	r.processors[name] = processor

	return nil
}

// GetProcessor returns a processor by name
func (r *Registry) GetProcessor(name string) (Processor, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	processor, exists := r.processors[name]
	if !exists {
		return nil, fmt.Errorf("processor %s not found", name)
	}

	return processor, nil
}

// GoPluginAdapter adapts a Go plugin to the Processor interface
type GoPluginAdapter struct {
	Processor Processor
	Name      string
}

// Process processes an event
func (a *GoPluginAdapter) Process(ctx context.Context, event Event) (Result, error) {
	return a.Processor.Process(ctx, event)
}

// Init initializes the processor
func (a *GoPluginAdapter) Init(config map[string]interface{}) error {
	return a.Processor.Init(config)
}

// Metadata returns metadata about the processor
func (a *GoPluginAdapter) Metadata() map[string]string {
	md := a.Processor.Metadata()
	md["name"] = a.Name
	return md
}

// WasmProcessor implements the Processor interface for WebAssembly modules
type WasmProcessor struct {
	Path     string
	Name     string
	Module   wazero.CompiledModule
	Runtime  wazero.Runtime
	Config   map[string]interface{}
	instance api.Module
}

// Process processes an event using the WASM module
func (p *WasmProcessor) Process(ctx context.Context, event Event) (Result, error) {
	// Create an instance if it doesn't exist
	if p.instance == nil {
		moduleConfig := wazero.NewModuleConfig().
			WithStdout(os.Stdout).
			WithStderr(os.Stderr)

		instance, err := p.Runtime.InstantiateModule(ctx, p.Module, moduleConfig)
		if err != nil {
			return Result{}, fmt.Errorf("failed to instantiate WASM module: %w", err)
		}
		p.instance = instance
	}

	// Get memory
	mem := p.instance.Memory()
	if mem == nil {
		return Result{}, fmt.Errorf("WASM module has no memory")
	}

	// Convert event to JSON (simplified for this example)
	eventJSON := fmt.Sprintf(`{"ledger_sequence":%d,"data":%s}`, event.LedgerSequence, string(event.Data))
	eventData := []byte(eventJSON)

	// Allocate memory for event data
	allocFn := p.instance.ExportedFunction("alloc")
	if allocFn == nil {
		return Result{}, fmt.Errorf("WASM module does not export 'alloc' function")
	}

	allocResults, err := allocFn.Call(ctx, uint64(len(eventData)))
	if err != nil {
		return Result{}, fmt.Errorf("failed to allocate memory in WASM: %w", err)
	}

	// Copy event data to WASM memory
	dataPtr := uint32(allocResults[0])
	if !mem.Write(dataPtr, eventData) {
		return Result{}, fmt.Errorf("failed to write to WASM memory")
	}

	// Call process function
	processFn := p.instance.ExportedFunction("process")
	if processFn == nil {
		return Result{}, fmt.Errorf("WASM module does not export 'process' function")
	}

	results, err := processFn.Call(ctx, uint64(dataPtr), uint64(len(eventData)))
	if err != nil {
		return Result{}, fmt.Errorf("failed to call process function: %w", err)
	}

	// Check status code
	statusCode := uint32(results[0])
	if statusCode != 0 {
		return Result{}, fmt.Errorf("WASM process function returned error code %d", statusCode)
	}

	// In a real implementation, you would read the results from memory
	// Here, we're just returning the input data for simplicity
	return Result{
		Output: event.Data,
		Error:  "",
	}, nil
}

// Init initializes the processor with configuration
func (p *WasmProcessor) Init(config map[string]interface{}) error {
	p.Config = config

	// Create an instance with the config
	ctx := context.Background()
	moduleConfig := wazero.NewModuleConfig().
		WithStdout(os.Stdout).
		WithStderr(os.Stderr)

	// Set environment variables for configuration (optional)
	for key, value := range config {
		moduleConfig = moduleConfig.WithEnv(key, fmt.Sprintf("%v", value))
	}

	// Instantiate the module
	instance, err := p.Runtime.InstantiateModule(ctx, p.Module, moduleConfig)
	if err != nil {
		return fmt.Errorf("failed to instantiate WASM module: %w", err)
	}
	p.instance = instance

	// Convert config to JSON (simplified)
	configJSON := fmt.Sprintf(`{"example_value":"%v"}`, config["example_value"])
	configData := []byte(configJSON)

	// Get memory
	mem := p.instance.Memory()
	if mem == nil {
		return fmt.Errorf("WASM module has no memory")
	}

	// Allocate memory for config data
	allocFn := p.instance.ExportedFunction("alloc")
	if allocFn == nil {
		return fmt.Errorf("WASM module does not export 'alloc' function")
	}

	allocResults, err := allocFn.Call(ctx, uint64(len(configData)))
	if err != nil {
		return fmt.Errorf("failed to allocate memory in WASM: %w", err)
	}

	// Copy config data to WASM memory
	dataPtr := uint32(allocResults[0])
	if !mem.Write(dataPtr, configData) {
		return fmt.Errorf("failed to write to WASM memory")
	}

	// Call init function
	initFn := p.instance.ExportedFunction("initProcessor")
	if initFn == nil {
		return fmt.Errorf("WASM module does not export 'initProcessor' function")
	}

	results, err := initFn.Call(ctx, uint64(dataPtr), uint64(len(configData)))
	if err != nil {
		return fmt.Errorf("failed to call init function: %w", err)
	}

	// Check status code
	statusCode := uint32(results[0])
	if statusCode != 0 {
		return fmt.Errorf("WASM init function returned error code %d", statusCode)
	}

	return nil
}

// Metadata returns metadata about the processor
func (p *WasmProcessor) Metadata() map[string]string {
	return map[string]string{
		"name":    p.Name,
		"type":    "wasm",
		"path":    p.Path,
		"version": "1.0.0", // Hardcoded for simplicity
	}
}
