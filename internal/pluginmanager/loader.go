package pluginmanager

import (
	"fmt"
	"path/filepath"
	"plugin"

	"github.com/withObsrvr/pluginapi"
)

// PluginLoader is an interface for loading plugins from files
type PluginLoader interface {
	// CanLoad returns true if this loader can load the given file
	CanLoad(path string) bool

	// LoadPlugin loads a plugin from the given path
	LoadPlugin(path string) (pluginapi.Plugin, error)
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
	return []PluginLoader{
		&NativePluginLoader{},
		&WASMPluginLoader{},
	}
}
