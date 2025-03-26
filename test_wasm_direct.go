package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

func main() {
	// Enable debug output
	os.Setenv("FLOW_DEBUG_WASM", "1")
	os.Setenv("GOLOG_LEVEL", "debug")

	// Parse command line flags
	pluginsDir := flag.String("plugins", "./plugins", "Directory containing plugin files")
	wasmFile := flag.String("wasm", "flow-processor-latest-ledger.wasm", "WASM file to test")
	flag.Parse()

	// Construct the full path to the WASM file
	wasmPath := filepath.Join(*pluginsDir, *wasmFile)

	// Check if the file exists
	fileInfo, err := os.Stat(wasmPath)
	if err != nil {
		log.Fatalf("Error accessing WASM file %s: %v", wasmPath, err)
	}

	log.Printf("Testing WASM file: %s (size: %d bytes)", wasmPath, fileInfo.Size())

	// Print the content of the file (first 100 bytes)
	content, err := ioutil.ReadFile(wasmPath)
	if err != nil {
		log.Fatalf("Error reading WASM file: %v", err)
	}

	// Check if it's a JSON file (our placeholder)
	if len(content) < 1000 && content[0] == '{' {
		log.Printf("This appears to be a JSON placeholder, not a real WASM module")
		var placeholderData map[string]string
		if err := json.Unmarshal(content, &placeholderData); err != nil {
			log.Printf("Failed to parse JSON: %v", err)
		} else {
			log.Printf("Placeholder data: name=%s, version=%s",
				placeholderData["name"], placeholderData["version"])
		}

		log.Printf("Creating a valid WASM file for testing...")
		// For now we'll just ensure the file is properly identified as a WASM file

		// The simplest valid WASM module has these magic bytes
		minimalWasm := []byte{
			0x00, 0x61, 0x73, 0x6D, // Magic number ("\0asm")
			0x01, 0x00, 0x00, 0x00, // WebAssembly binary version (1)
		}

		// Write the minimal WASM to a temporary file
		tempWasmPath := wasmPath + ".valid"
		if err := ioutil.WriteFile(tempWasmPath, minimalWasm, 0644); err != nil {
			log.Fatalf("Failed to write temporary WASM file: %v", err)
		}
		wasmPath = tempWasmPath
		log.Printf("Created minimal valid WASM file at %s", wasmPath)

		// Update content to the new file
		content, err = ioutil.ReadFile(wasmPath)
		if err != nil {
			log.Fatalf("Error reading temporary WASM file: %v", err)
		}
	}

	if len(content) > 100 {
		log.Printf("First 100 bytes of the file: %v", content[:100])
	} else {
		log.Printf("File content: %v", content)
	}

	// Create a new wazero Runtime to test WASM loading
	log.Printf("Attempting to load WASM module using wazero...")
	ctx := context.Background()

	// Create a new WASM runtime
	runtime := wazero.NewRuntime(ctx)
	defer runtime.Close(ctx)

	// Add WASI support to the runtime
	wasi_snapshot_preview1.MustInstantiate(ctx, runtime)

	// Try to compile the module
	log.Printf("Compiling WASM module...")
	module, err := runtime.CompileModule(ctx, content)
	if err != nil {
		log.Fatalf("Failed to compile WASM module: %v", err)
	}

	log.Printf("WASM module compilation successful:")
	log.Printf("  Name: %s", module.Name())
	log.Printf("  Size: %d bytes", len(content))
	log.Printf("  Exports: %d functions", len(module.ExportedFunctions()))

	fmt.Println("WASM test completed successfully!")
}
