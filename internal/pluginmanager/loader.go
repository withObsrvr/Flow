package pluginmanager

import (
	"fmt"
	"log"
	"path/filepath"
	"plugin"
	"sync"

	"github.com/withObsrvr/pluginapi"
)

// PluginLoader is an interface for loading plugins from files
type PluginLoader interface {
	// CanLoad returns true if this loader can load the given file
	CanLoad(path string) bool

	// LoadPlugin loads a plugin from the given path
	LoadPlugin(path string) (pluginapi.Plugin, error)
}

// Global registry of plugin loaders
var (
	loadersMutex sync.RWMutex
	loaders      []PluginLoader
)

// RegisterLoader adds a plugin loader to the global registry
func RegisterLoader(loader PluginLoader) {
	loadersMutex.Lock()
	defer loadersMutex.Unlock()
	loaders = append(loaders, loader)
	log.Printf("Registered plugin loader: %T", loader)
}

// NativePluginLoader loads native Go plugins (.so files)
type NativePluginLoader struct{}

// CanLoad returns true if the file has a .so extension
func (l *NativePluginLoader) CanLoad(path string) bool {
	return filepath.Ext(path) == ".so"
}

// LoadPlugin loads a native Go plugin from the given path
func (l *NativePluginLoader) LoadPlugin(path string) (pluginapi.Plugin, error) {
	// Use the Go plugin package to load the .so file
	p, err := plugin.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open plugin %s: %w", path, err)
	}

	// Look up the New symbol
	newSymbol, err := p.Lookup("New")
	if err != nil {
		return nil, fmt.Errorf("plugin %s does not export New: %w", path, err)
	}

	// Check that the New symbol is a function that returns a pluginapi.Plugin
	newFunc, ok := newSymbol.(func() pluginapi.Plugin)
	if !ok {
		return nil, fmt.Errorf("plugin %s New symbol has wrong type", path)
	}

	// Create an instance of the plugin
	return newFunc(), nil
}

// getRegisteredLoaders returns all registered plugin loaders
func getRegisteredLoaders() []PluginLoader {
	// Register built-in loaders if this is the first time
	if len(loaders) == 0 {
		RegisterLoader(&NativePluginLoader{})
		RegisterLoader(&WASMPluginLoader{})
	}

	loadersMutex.RLock()
	defer loadersMutex.RUnlock()

	// Return a copy of the loaders slice to prevent concurrent modification
	result := make([]PluginLoader, len(loaders))
	copy(result, loaders)
	return result
}

// LoadPluginFromFile iterates over registered loaders and tries to load a plugin from the given file
func LoadPluginFromFile(path string) (pluginapi.Plugin, error) {
	for _, loader := range getRegisteredLoaders() {
		if loader.CanLoad(path) {
			return loader.LoadPlugin(path)
		}
	}
	return nil, fmt.Errorf("no suitable plugin loader found for file: %s", path)
}
