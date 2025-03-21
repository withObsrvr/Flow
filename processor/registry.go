package processor

import (
	"context"
	"errors"
	"fmt"
	"os"
	"plugin"
	"sync"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

// Registry manages all processor types
type Registry struct {
	mu          sync.RWMutex
	processors  map[string]Processor
	wasmRuntime wazero.Runtime
}

// NewRegistry creates a new processor registry
func NewRegistry(ctx context.Context) *Registry {
	runtime := wazero.NewRuntime(ctx)

	// Initialize WASI
	wasi_snapshot_preview1.MustInstantiate(ctx, runtime)

	return &Registry{
		processors:  make(map[string]Processor),
		wasmRuntime: runtime,
	}
}

// LoadGoPlugin loads a Go plugin by path
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

// LoadWasmModule loads a WebAssembly module by path
func (r *Registry) LoadWasmModule(ctx context.Context, path string, name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Skip if already loaded
	if _, exists := r.processors[name]; exists {
		return nil
	}

	// Read the WebAssembly module
	wasmBytes, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read WASM file %s: %w", path, err)
	}

	// Compile the WebAssembly module
	compiled, err := r.wasmRuntime.CompileModule(ctx, wasmBytes)
	if err != nil {
		return fmt.Errorf("failed to compile WASM module %s: %w", path, err)
	}

	// Create the processor
	processor := &WasmProcessor{
		name:     name,
		path:     path,
		runtime:  r.wasmRuntime,
		compiled: compiled,
	}

	// Initialize the processor
	if err := processor.init(ctx); err != nil {
		return fmt.Errorf("failed to initialize WASM processor %s: %w", name, err)
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
	name     string
	path     string
	runtime  wazero.Runtime
	compiled wazero.CompiledModule
	instance api.Module
	config   map[string]interface{}
}

// init initializes the WASM processor
func (p *WasmProcessor) init(ctx context.Context) error {
	// Instantiate the module
	instance, err := p.runtime.InstantiateModule(ctx, p.compiled, wazero.NewModuleConfig())
	if err != nil {
		return fmt.Errorf("failed to instantiate WASM module: %w", err)
	}

	p.instance = instance
	return nil
}

// Process processes an event using the WASM module
func (p *WasmProcessor) Process(ctx context.Context, event Event) (Result, error) {
	if p.instance == nil {
		return Result{}, errors.New("WASM module not instantiated")
	}

	// Get the process function
	processFunc := p.instance.ExportedFunction("process")
	if processFunc == nil {
		return Result{}, errors.New("WASM module doesn't export 'process' function")
	}

	// Allocate memory for the event data
	allocFunc := p.instance.ExportedFunction("alloc")
	if allocFunc == nil {
		return Result{}, errors.New("WASM module doesn't export 'alloc' function")
	}

	allocResults, err := allocFunc.Call(ctx, uint64(len(event.Data)))
	if err != nil {
		return Result{}, err
	}
	dataPtr := uint32(allocResults[0])

	// Copy the data to WASM memory
	mem := p.instance.Memory()
	if !mem.Write(dataPtr, event.Data) {
		return Result{}, errors.New("failed to write to WASM memory")
	}

	// Call the process function
	results, err := processFunc.Call(ctx, uint64(event.LedgerSequence), uint64(dataPtr), uint64(len(event.Data)))
	if err != nil {
		return Result{}, err
	}

	// Get result status
	statusCode := uint32(results[0])
	if statusCode != 0 {
		return Result{}, fmt.Errorf("WASM processor returned error code: %d", statusCode)
	}

	// Logic to retrieve result data would go here
	// This depends on your WASM module API design
	// For now, we'll return the original data as a placeholder

	return Result{
		Output: event.Data,
		Error:  nil,
	}, nil
}

// Init initializes the processor with configuration
func (p *WasmProcessor) Init(config map[string]interface{}) error {
	p.config = config

	// Create an instance of the module
	moduleConfig := wazero.NewModuleConfig().
		WithStdout(os.Stdout).
		WithStderr(os.Stderr)

	// Add config as environment variables
	for k, v := range config {
		moduleConfig = moduleConfig.WithEnv(k, fmt.Sprintf("%v", v))
	}

	var err error
	p.instance, err = p.runtime.InstantiateModule(context.Background(), p.compiled, moduleConfig)

	// Call init function if it exists
	if err == nil {
		initFunc := p.instance.ExportedFunction("init")
		if initFunc != nil {
			_, err = initFunc.Call(context.Background())
		}
	}

	return err
}

// Metadata returns metadata about the processor
func (p *WasmProcessor) Metadata() map[string]string {
	return map[string]string{
		"name":    p.name,
		"type":    "wasm",
		"path":    p.path,
		"version": "1.0.0", // Hardcoded for simplicity
	}
}
