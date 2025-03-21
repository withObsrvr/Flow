# Flow Pipeline System with WebAssembly Support

This enhanced version of the Flow pipeline system supports both traditional Go plugins and WebAssembly modules as processors, allowing for more flexible and portable deployment options.

## Overview

The Flow pipeline system processes events from sources through a series of processors and sends the results to consumers. The system is configured using YAML files and supports two types of processors:

1. **Go Plugins** - Traditional Go plugins loaded dynamically at runtime
2. **WebAssembly Modules** - WASM modules for cross-platform compatibility and enhanced security

## Features

- Configuration-based pipeline definition in YAML
- Support for both Go plugins and WebAssembly processors
- Common processor interface for consistent event handling
- Dynamic loading of processors at runtime
- Flexible configuration options for processors

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
│   ├── go/                 # Go plugin processors
│   └── wasm/               # WebAssembly processors
│       ├── example.go      # Example WebAssembly processor
│       └── Makefile        # For building WebAssembly modules
├── example_main.go         # Example application using the pipeline system
└── example_pipeline.yaml   # Example pipeline configuration
```

## Getting Started

### Prerequisites

- Go 1.16 or later
- TinyGo or standard Go with WASM support for building WebAssembly modules

### Building WebAssembly Processors

1. Navigate to the WebAssembly processor directory:
   ```
   cd processors/wasm
   ```

2. Build the WebAssembly module:
   ```
   make
   ```

This will create a `.wasm` file that can be used in your pipeline configuration.

### Running the Example

1. Update the `example_pipeline.yaml` file with your specific configuration
2. Run the example application:
   ```
   go run example_main.go
   ```

## Pipeline Configuration

A pipeline is defined in YAML format. Here's an example configuration:

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
          
      # WebAssembly processor
      - type: data/enrichment
        runtime: wasm
        path: processors/wasm/example.wasm
        config:
          example_value: "test-value"

    consumers:
      - type: sqlite
        config:
          db_path: "/tmp/flow-latest-ledger.db"
          table_name: "latest_ledger_transactions"
```

### Processor Types

There are two ways to specify a WebAssembly processor:

1. Using the `runtime` field:
   ```yaml
   - type: data/enrichment
     runtime: wasm
     path: processors/wasm/example.wasm
     config:
       example_value: "test-value"
   ```

2. Using the `wasm/` prefix in the type:
   ```yaml
   - type: wasm/filter
     path: processors/wasm/filter.wasm
     config:
       filter_field: "type"
       filter_value: "payment"
   ```

## Creating Custom Processors

### Go Plugin Processor

1. Create a new Go plugin that implements the processor interface
2. Build it as a plugin with `go build -buildmode=plugin`
3. Place the `.so` file in your plugins directory

### WebAssembly Processor

1. Create a Go file with exported WebAssembly functions:
   - `alloc` for memory allocation
   - `dealloc` for memory deallocation
   - `initProcessor` for initializing with config
   - `process` for processing events
   - `metadata` for returning processor information

2. Build it to WebAssembly:
   ```
   GOOS=js GOARCH=wasm go build -o myprocessor.wasm myprocessor.go
   ```

3. Reference it in your pipeline configuration

## License

MIT
