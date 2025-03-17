package pluginmanager

import (
	"fmt"
	"log"
	"os"
	"github.com/withObsrvr/pluginapi"
	"path/filepath"
)

// PluginManager handles loading and initializing plugins
type PluginManager struct {
	Registry *PluginRegistry
	loaders  []PluginLoader
}

// NewPluginManager creates a new plugin manager
func NewPluginManager() *PluginManager {
	return &PluginManager{
		Registry: NewPluginRegistry(),
		loaders:  getRegisteredLoaders(),
	}
}

// LoadPlugins loads all plugins from the specified directory
func (pm *PluginManager) LoadPlugins(dir string, config map[string]interface{}) error {
	log.Printf("Loading plugins from directory: %s", dir)
	// Make sure the directory exists
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return fmt.Errorf("plugin directory %s does not exist", dir)
	}

	// Walk through the directory and load all plugin files
	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		// Find a loader that can handle this file
		for _, loader := range pm.loaders {
			if loader.CanLoad(path) {
				log.Printf("Loading plugin from %s", path)
				return pm.loadPluginWithLoader(path, loader, config)
			}
		}

		// No loader found for this file, skip it
		log.Printf("Skipping %s: no loader available for this file type", path)
		return nil
	})
}

// loadPluginWithLoader loads a plugin using the specified loader
func (pm *PluginManager) loadPluginWithLoader(path string, loader PluginLoader, config map[string]interface{}) error {
	// Load the plugin
	instance, err := loader.LoadPlugin(path)
	if err != nil {
		return fmt.Errorf("failed to load plugin %s: %w", path, err)
	}

	// Initialize the plugin with the provided config
	pluginConfig, _ := config[instance.Name()].(map[string]interface{})
	if err := instance.Initialize(pluginConfig); err != nil {
		return fmt.Errorf("failed to initialize plugin %s: %w", instance.Name(), err)
	}

	// Register the plugin
	if err := pm.Registry.Register(instance); err != nil {
		return fmt.Errorf("failed to register plugin %s: %w", instance.Name(), err)
	}

	log.Printf("Plugin %s v%s loaded successfully", instance.Name(), instance.Version())
	return nil
}

// RegisterLoader adds a new plugin loader to the manager
func (pm *PluginManager) RegisterLoader(loader PluginLoader) {
	pm.loaders = append(pm.loaders, loader)
}
