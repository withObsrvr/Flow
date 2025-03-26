# Latest Ledger Processor WASM Template

This is a template for building a WebAssembly (WASM) module for the "latest-ledger" processor in the Flow data indexer system.

## Structure

- `main.go` - The main source file containing the required exports for the WASM module
- `build.sh` - A script to compile the Go code to WASM

## Required Exports

This template implements all the required exports expected by the Flow application:

1. `name()` - Returns the processor name as "flow/processor/latest-ledger"
2. `version()` - Returns the processor version
3. `initialize(configJSON string)` - Initializes the processor with configuration
4. `processLedger(ledgerJSON string)` - Processes a ledger and returns the result

## Building the WASM Module

1. Make sure you have Go 1.21+ installed (required for WASIP1 support)
2. Run the build script:

```bash
chmod +x build.sh
./build.sh
```

This will create `flow-processor-latest-ledger.wasm` in the current directory.

## Using the WASM Module

1. Copy the WASM file to your Flow plugins directory:

```bash
cp flow-processor-latest-ledger.wasm ~/projects/obsrvr/flow/plugins/
```

2. Configure your pipeline to use the WASM module:

```yaml
processors:
  - type: "flow/processor/latest-ledger"
    plugin: "flow-processor-latest-ledger.wasm"
    config:
      network_passphrase: "Public Global Stellar Network ; September 2015"
```

3. Run the Flow application with the plugins directory specified:

```bash
./bin/flow -pipeline examples/pipelines/pipeline_latest_ledger_wasm.yaml -plugins=./plugins
```

## Customizing

To customize this processor for your specific needs:

1. Modify the `Config` struct to include any additional configuration options
2. Enhance the `processLedger` function to implement your specific processing logic
3. Update the result structure to match your expected output format

## Debugging

To enable detailed WASM debugging, set these environment variables:

```bash
export FLOW_DEBUG_WASM=1
export GOLOG_LEVEL=debug
``` 