# Flow Pipeline with WebAssembly Support Example

This example demonstrates how to use both traditional Go plugins and WebAssembly modules as processors in a pipeline system.

## Overview

The Flow pipeline system processes events from sources through a series of processors and sends the results to consumers. This example shows how to support both processor types:

1. **Go Plugins** - Traditional Go plugins loaded dynamically at runtime
2. **WebAssembly Modules** - WASM modules for cross-platform compatibility and enhanced security

## Directory Structure

```
.
├── pipeline/               # Pipeline configuration and execution
│   ├── config.go           # Configuration parsing and loading
│   └── executor.go         # Pipeline execution engine
├── processor/              # Processor interfaces and registry
│   ├── interface.go        # Common processor interface
│   └── registry.go         # Registry for loading and retrieving processors
├── processors/             # Actual processor implementations
│   └── wasm/               # WebAssembly processors
│       ├── example.go      # Example WebAssembly processor
│       └── Makefile        # For building WebAssembly modules
├── main.go                 # Main application
└── example_pipeline.yaml   # Example pipeline configuration
```

## Building and Running

### Prerequisites

- Go 1.16 or later
- Standard Go or TinyGo with WebAssembly support

### Building the WebAssembly Processor

1. Navigate to the WebAssembly processor directory:
   ```
   cd processors/wasm
   ```

2. Build the WebAssembly module using the Makefile:
   ```
   make
   ```

### Running the Example

1. Build and run the main application:
   ```
   go run main.go
   ```

## Pipeline Configuration

Pipelines are defined in YAML format. Here's an example configuration:

```yaml
pipelines:
  latest_ledger:
    source:
      type: stellar/latest_ledger
      config:
        horizon_url: "https://horizon.stellar.org"
        poll_interval: 5

    processors:
      # Traditional Go plugin processor
      - type: stellar/transaction_extractor
        config:
          include_failed: false
          
      # WebAssembly processor using runtime specification
      - type: data/enrichment
        runtime: wasm
        path: processors/wasm/example.wasm
        config:
          example_value: "test-value"
          
      # WebAssembly processor using type prefix
      - type: wasm/filter
        path: processors/wasm/filter.wasm
        config:
          filter_field: "type"
          filter_value: "payment"

    consumers:
      - type: sqlite
        config:
          db_path: "/tmp/flow-latest-ledger.db"
          table_name: "latest_ledger_transactions"
```

## Creating Custom Processors

### WebAssembly Processor

1. Create a Go file with exported WebAssembly functions:
   - `alloc` for memory allocation
   - `dealloc` for memory deallocation
   - `initProcessor` for initializing with config
   - `process` for processing events
   - `metadata` for returning processor information

2. Build it to WebAssembly:
   ```bash
   GOOS=js GOARCH=wasm go build -o myprocessor.wasm myprocessor.go
   ```

3. Reference it in your pipeline configuration.

## How It Works

1. The pipeline configuration is loaded from a YAML file.
2. The processor registry manages both Go plugins and WebAssembly modules.
3. When loading processors, the system determines the type (Go plugin or WebAssembly) based on the configuration.
4. Processors are initialized with their configuration.
5. The pipeline executes, sending events through each processor in sequence.
6. Results are sent to consumers.

## Benefits of WebAssembly Support

- **Cross-platform compatibility**: WebAssembly modules can be run on any platform.
- **Enhanced security**: WebAssembly runs in a sandboxed environment.
- **Language flexibility**: Processors can be written in any language that compiles to WebAssembly.
- **Smaller deployment size**: WebAssembly modules are typically smaller than equivalent Go plugins.
- **Isolation**: WebAssembly modules run in isolation, reducing the risk of one processor affecting others.

## Limitations

- **Performance**: WebAssembly may be slower than native Go code for some operations.
- **Memory management**: WebAssembly requires more explicit memory management.
- **Feature access**: WebAssembly has limited access to system resources compared to Go plugins.

## License

MIT 