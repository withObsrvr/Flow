# WASM Plugin Sample for Flow

This is a sample WebAssembly (WASM) plugin for the Flow project. It demonstrates how to create a simple consumer plugin that can be loaded using the WASM runtime.

## Prerequisites

- TinyGo (required for building WASM plugins)
- Go 1.20 or higher

## Building the Plugin

To build the plugin, use TinyGo with the WASI target:

```bash
tinygo build -o wasm-sample.wasm -target=wasi ./main.go
```

Alternatively, if you're using the Nix development environment provided with Flow, you can use the `buildWasmPlugin` function in the flake.nix file.

## Plugin Structure

This sample plugin implements the basic interface required for Flow plugins in WebAssembly:

- Memory management functions: `alloc` and `free`
- Plugin interface functions:
  - `name`: Returns the plugin name
  - `version`: Returns the plugin version
  - `type_`: Returns the plugin type (2 for ConsumerPlugin)
  - `initialize`: Initializes the plugin with configuration
  - `process`: Processes incoming messages
  - `close`: Cleans up resources when the plugin is closed

## Usage in Flow

To use this plugin with Flow, add it to your pipeline configuration:

```yaml
pipeline:
  processors:
    - name: wasm-sample
      type: consumer
      plugin: ./examples/wasm-plugin-sample/wasm-sample.wasm
      config:
        # Custom configuration for your plugin
        key: value
```

## Understanding the WASM Plugin Interface

The WASM plugin interface uses exported functions and memory management to communicate between the host (Flow) and the plugin:

1. The host initializes the plugin by calling the `initialize` function with a JSON configuration.
2. Messages are passed to the plugin via the `process` function, which receives memory pointers.
3. The plugin reads data from memory, processes it, and returns a success/error code.

This approach allows for safe cross-language plugin development while maintaining performance. 