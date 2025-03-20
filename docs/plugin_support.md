# Flow Plugin Support

The Flow system supports plugins in two formats:
1. Native Go plugins (`.so` files)
2. WebAssembly (WASM) plugins (`.wasm` files)

This document explains how to create and use both types of plugins.

## Plugin Types

Flow supports three types of plugins:

1. **Source Plugins**: Generate data for the pipeline (e.g., from a blockchain, API, or file)
2. **Processor Plugins**: Transform or filter data from sources
3. **Consumer Plugins**: Store or forward data to external systems

## Native Go Plugins

Native Go plugins are compiled as shared libraries (`.so` files) and loaded dynamically at runtime.

### Building Native Plugins

To build a native Go plugin:

```bash
go build -buildmode=plugin -o my-plugin.so
```

### Plugin Interface

Native plugins must implement the `pluginapi.Plugin` interface and export a `New()` function:

```go
package main

import (
	"github.com/withObsrvr/pluginapi"
)

// MyPlugin implements the Plugin interface
type MyPlugin struct {
	// plugin fields
}

// New creates a new instance of the plugin
func New() pluginapi.Plugin {
	return &MyPlugin{}
}

// Name returns the name of the plugin
func (p *MyPlugin) Name() string {
	return "my-plugin"
}

// Version returns the version of the plugin
func (p *MyPlugin) Version() string {
	return "1.0.0"
}

// Type returns the type of the plugin
func (p *MyPlugin) Type() pluginapi.PluginType {
	return pluginapi.ProcessorPlugin
}

// Initialize sets up the plugin
func (p *MyPlugin) Initialize(config map[string]interface{}) error {
	// implementation
}

// Other methods required by the specific plugin type...
```

## WebAssembly (WASM) Plugins

WASM plugins provide better cross-platform compatibility and security isolation compared to native plugins.

### Building WASM Plugins

To build a WASM plugin, you need [TinyGo](https://tinygo.org/):

```bash
tinygo build -o my-plugin.wasm -target=wasi ./main.go
```

### WASM Plugin Interface

WASM plugins need to export specific functions for interoperability:

```go
package main

import (
	"encoding/json"
	"fmt"
	"unsafe"
)

//export name
func name() (uint64, uint64) {
	// Return pointer and length of plugin name
}

//export version
func version() (uint64, uint64) {
	// Return pointer and length of plugin version
}

//export type
func type_() uint64 {
	// Return plugin type (1=Source, 2=Consumer, 3=Processor)
}

//export initialize
func initialize(configPtr uint64, configLen uint64) uint64 {
	// Initialize plugin with the config
}

//export process
func process(msgPtr uint64, msgLen uint64) uint64 {
	// Process a message
}

//export close
func close() uint64 {
	// Clean up resources
}

//export alloc
func alloc(size uint64) uint64 {
	// Allocate memory
}

//export free
func free(ptr uint64, size uint64) {
	// Free memory
}

func main() {}
```

See the [example WASM plugin](/examples/wasm-plugin-sample) for a complete implementation.

## Using Plugins in Flow

### Pipeline Configuration

Specify plugins in your pipeline configuration:

```yaml
pipeline:
  source:
    type: "flow/source/my-source"
    config:
      # Source configuration
  
  processors:
    - type: "flow/processor/my-processor"
      config:
        # Processor configuration
  
  consumers:
    - type: "flow/consumer/my-consumer"
      config:
        # Consumer configuration
```

### Plugin Loading

Flow loads plugins from the `plugins` directory by default. The system automatically detects whether a plugin is a native Go plugin (`.so`) or a WASM plugin (`.wasm`) based on the file extension.

## Creating New Plugins

1. Create a new repository for your plugin
2. Implement the required plugin interface
3. Build the plugin as either a native `.so` file or a `.wasm` file
4. Place the plugin in the `plugins` directory
5. Add the plugin to your pipeline configuration

## Testing Plugins

Use the provided test scripts to verify your plugin works correctly:

- For native plugins: `scripts/test_native_plugin.sh`
- For WASM plugins: `scripts/test_wasm_plugin.sh`

## Best Practices

1. **Plugin Naming**: Use a consistent naming scheme: `flow-[type]-[name]`
2. **Configuration**: Make your plugin configurable and document all options
3. **Error Handling**: Handle errors gracefully and provide meaningful error messages
4. **Memory Management**: Be careful with memory allocation, especially in WASM plugins
5. **Documentation**: Document your plugin's functionality and configuration options 