# Flow - Data Processing Pipeline Framework

Flow is a plugin-based data processing pipeline framework that allows you to ingest, process, and output data through a configurable pipeline.

## Repository Structure

This repository is organized as a monorepo containing multiple components:

```
/flow-project
  /cmd                  # Command-line applications
    /flow              # Main Flow engine
    /schema-registry   # Schema Registry service
    /graphql-api       # GraphQL API service
  
  /internal             # Internal packages
    /flow              # Core Flow engine code
    /metrics           # Metrics collection
    /pluginmanager     # Plugin loading and management
  
  /pkg                  # Public packages
    /pluginapi         # Plugin API interfaces
    /schemaapi         # Schema API interfaces
    /common            # Shared utilities
  
  /plugins              # Plugin .so and .wasm files
  
  /scripts              # Utility scripts
    run_local.sh       # Script to run all components locally
    test_wasm_plugin.sh # Script to test WASM plugin functionality
  
  /examples             # Example configurations
    /pipelines         # Example pipeline configurations
    /wasm-plugin-sample # Sample WASM plugin implementation
```

## Components

### Flow Engine

The core Flow engine loads plugins and executes data processing pipelines based on configuration.

### Schema Registry

The Schema Registry service collects GraphQL schema definitions from plugins and composes them into a complete schema.

### GraphQL API

The GraphQL API service provides a query interface to access data processed by Flow pipelines.

## Plugin Support

Flow supports two types of plugins:

1. **Native Go Plugins** (.so files): Traditional Go plugins compiled as shared libraries.
2. **WebAssembly (WASM) Plugins** (.wasm files): Portable plugins that run in a sandboxed environment with improved security and cross-platform compatibility.

See [Plugin Support Documentation](docs/plugin_support.md) for details on creating and using both types of plugins.

## Running Locally

To run all components locally, use the provided script:

```bash
./scripts/run_local.sh --pipeline your_pipeline.yaml
```

Options:
- `--pipeline`: Path to pipeline configuration file (default: pipeline_example.yaml)
- `--plugins`: Directory containing plugin .so files (default: ./plugins)
- `--instance-id`: Unique ID for this instance (default: local-dev)
- `--tenant-id`: Tenant ID (default: local)
- `--api-key`: API key (default: local-dev-key)
- `--schema-port`: Port for Schema Registry (default: 8081)
- `--api-port`: Port for GraphQL API (default: 8080)

## Creating Plugins

Plugins can be created in two formats:

### Native Go Plugins

```bash
go build -buildmode=plugin -o myplugin.so
```

### WASM Plugins

```bash
tinygo build -o myplugin.wasm -target=wasi ./main.go
```

A plugin can be a:
- **Source**: Fetches data from an external system
- **Processor**: Transforms data
- **Consumer**: Outputs data to an external system

Plugins can also implement the `SchemaProvider` interface to contribute to the GraphQL schema.

## Pipeline Configuration

Pipelines are defined in YAML files:

```yaml
pipelines:
  MyPipeline:
    source:
      type: "MySource"
      config:
        # Source configuration
    processors:
      - type: "MyProcessor"
        plugin: "./plugins/my-processor.so"  # or ".wasm" for WASM plugins
        config:
          # Processor configuration
    consumers:
      - type: "MyConsumer"
        plugin: "./plugins/my-consumer.wasm"  # WASM consumer example
        config:
          # Consumer configuration
```

## GraphQL API

The GraphQL API provides a query interface to access data processed by Flow pipelines. It dynamically generates its schema based on the plugins used in your pipelines.

Access the GraphQL playground at: http://localhost:8080/graphql

## Development

To build all components:

```bash
go build -o bin/flow cmd/flow/main.go
go build -o bin/schema-registry cmd/schema-registry/main.go
go build -o bin/graphql-api cmd/graphql-api/main.go
```

## License

[License information]
