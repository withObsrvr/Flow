#!/bin/bash
# Script to patch the Flow application to properly handle WASM plugins

echo "Preparing WASM plugin integration patch"
echo "======================================"

# Set directories
FLOW_DIR=$(pwd)
PROCESSOR_DIR="/home/tmosleyiii/projects/obsrvr/flow-processor-latestledger"

# Verify the flow-processor-latestledger.wasm exists
if [ ! -f "$PROCESSOR_DIR/flow-latest-ledger.wasm" ]; then
  echo "Error: WASM file not found at $PROCESSOR_DIR/flow-latest-ledger.wasm"
  echo "Make sure the WASM module is built first!"
  exit 1
fi

# Create a modified test directory
TEST_DIR=$(mktemp -d)
mkdir -p $TEST_DIR/plugins
echo "Created test directory: $TEST_DIR"

# Copy both the WASM plugin and any needed .so files
cp "$PROCESSOR_DIR/flow-latest-ledger.wasm" "$TEST_DIR/plugins/"
echo "Copied flow-latest-ledger.wasm to test plugins directory"

find plugins -name "*.so" -type f | while read so_file; do
  echo "Found shared library: $so_file"
  cp "$so_file" "$TEST_DIR/plugins/"
done

# List all files in the plugins directory
echo "Files in plugins directory:"
ls -la $TEST_DIR/plugins/

# Create a test pipeline that correctly specifies both type and plugin fields
cat > $TEST_DIR/test_pipeline.yaml << EOF
pipelines:
  LatestLedgerTest:
    source:
      type: "BufferedStorageSourceAdapter"
      config:
        bucket_name: "obsrvr-stellar-ledger-data-mainnet-data/landing/"
        network: "mainnet"
        num_workers: 10
        retry_limit: 3
        retry_wait: 5
        start_ledger: 56146570
        ledgers_per_file: 1
        files_per_partition: 64000
    processors:
      - type: "flow/processor/latest-ledger"
        plugin: "flow-latest-ledger.wasm"
        config:
          network_passphrase: "Public Global Stellar Network ; September 2015"
    consumers:
      - type: "SaveToZeroMQ"
        config:
          address: "tcp://127.0.0.1:5556"
EOF

echo "Created test pipeline configuration:"
cat $TEST_DIR/test_pipeline.yaml

# Create a modified version of the debug script that dumps more WASM information
cat > $TEST_DIR/wasm_test.go << EOF
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

// A simple Go program to verify WASM loading
func main() {
	// Set up a directory path from command line or use the current directory
	dir := "."
	if len(os.Args) > 1 {
		dir = os.Args[1]
	}

	// Check the plugin directory
	pluginsDir := filepath.Join(dir, "plugins")
	if _, err := os.Stat(pluginsDir); os.IsNotExist(err) {
		log.Fatalf("Plugins directory not found: %s", pluginsDir)
	}

	// List all WASM files
	wasmFiles, err := filepath.Glob(filepath.Join(pluginsDir, "*.wasm"))
	if err != nil {
		log.Fatalf("Error searching for WASM files: %v", err)
	}

	log.Printf("Found %d WASM files in %s", len(wasmFiles), pluginsDir)
	for i, file := range wasmFiles {
		log.Printf("%d. WASM file: %s", i+1, file)
		validateWasmFile(file)
	}
}

func validateWasmFile(path string) {
	log.Printf("Validating WASM file: %s", path)
	fileInfo, err := os.Stat(path)
	if err != nil {
		log.Printf("  Error: %v", err)
		return
	}

	log.Printf("  File size: %d bytes", fileInfo.Size())

	// Read the file
	wasmCode, err := os.ReadFile(path)
	if err != nil {
		log.Printf("  Error reading file: %v", err)
		return
	}

	// Create a context
	ctx := context.Background()

	// Create a new WebAssembly Runtime
	runtime := wazero.NewRuntime(ctx)
	defer runtime.Close(ctx)

	// Add WASI support to the runtime
	wasi_snapshot_preview1.MustInstantiate(ctx, runtime)

	// Try to compile the module
	compiledModule, err := runtime.CompileModule(ctx, wasmCode)
	if err != nil {
		log.Printf("  Error compiling module: %v", err)
		return
	}

	// List all exported functions
	log.Printf("  Module name: %s", compiledModule.Name())
	log.Printf("  Exported functions:")
	for _, funcName := range compiledModule.ExportedFunctionNames() {
		log.Printf("    - %s", funcName)
	}

	// Try to instantiate the module
	config := wazero.NewModuleConfig().
		WithStdout(os.Stdout).
		WithStderr(os.Stderr)

	module, err := runtime.InstantiateModule(ctx, compiledModule, config)
	if err != nil {
		log.Printf("  Error instantiating module: %v", err)
		return
	}
	defer module.Close(ctx)

	// Try to call the name function if it exists
	if nameFunc := module.ExportedFunction("name"); nameFunc != nil {
		results, err := nameFunc.Call(ctx)
		if err != nil {
			log.Printf("  Error calling name function: %v", err)
		} else {
			log.Printf("  name() returned: %v", results)
		}
	} else {
		log.Printf("  name function not found")
	}

	// Try to call the version function if it exists
	if versionFunc := module.ExportedFunction("version"); versionFunc != nil {
		results, err := versionFunc.Call(ctx)
		if err != nil {
			log.Printf("  Error calling version function: %v", err)
		} else {
			log.Printf("  version() returned: %v", results)
		}
	} else {
		log.Printf("  version function not found")
	}

	log.Printf("  Module validated successfully")
}
EOF

# Instructions for the user
echo ""
echo "==============================="
echo "INSTRUCTIONS"
echo "==============================="
echo "To verify the WASM module is correctly formed, run:"
echo "  cd $TEST_DIR && go run wasm_test.go ."
echo ""
echo "To build and run Flow with the WASM processor test pipeline, use:"
echo "  cp $TEST_DIR/plugins/* ./plugins/"
echo "  nix build"
echo ""
echo "Then run Flow with:"
echo "  ./result/bin/flow -pipeline '$TEST_DIR/test_pipeline.yaml' -instance-id=test -tenant-id=test -api-key=test -plugins=./plugins"
echo ""
echo "Test directory: $TEST_DIR" 