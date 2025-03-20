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

// WASMPluginLoader loads WebAssembly plugins (.wasm files)
type WASMPluginLoader struct{}

// CanLoad returns true if the file has a .wasm extension
func (l *WASMPluginLoader) CanLoad(path string) bool {
	return filepath.Ext(path) == ".wasm"
}

// LoadPlugin loads a WASM plugin from the given path
func (l *WASMPluginLoader) LoadPlugin(path string) (pluginapi.Plugin, error) {
	log.Printf("Loading WASM plugin from %s", path)

	// Read the WASM file
	wasmBytes, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read WASM file %s: %w", path, err)
	}

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
