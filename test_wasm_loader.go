package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/withObsrvr/Flow/internal/pluginmanager"
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
	content, err := os.ReadFile(wasmPath)
	if err != nil {
		log.Fatalf("Error reading WASM file: %v", err)
	}

	if len(content) > 100 {
		log.Printf("First 100 bytes of the file: %v", content[:100])
	} else {
		log.Printf("File content: %v", content)
	}

	// Create a new WASM plugin loader
	loader := &pluginmanager.WASMPluginLoader{}

	// Check if the loader can load this file
	if !loader.CanLoad(wasmPath) {
		log.Fatalf("Loader reports it cannot load file: %s", wasmPath)
	}

	// Try to load the plugin
	log.Printf("Attempting to load WASM plugin from: %s", wasmPath)
	plugin, err := loader.LoadPlugin(wasmPath)
	if err != nil {
		log.Fatalf("Failed to load WASM plugin: %v", err)
	}

	// Print plugin information
	log.Printf("Successfully loaded WASM plugin!")
	log.Printf("  Name:    %s", plugin.Name())
	log.Printf("  Version: %s", plugin.Version())
	log.Printf("  Type:    %v", plugin.Type())

	// Initialize the plugin with empty config
	err = plugin.Initialize(map[string]interface{}{
		"network_passphrase": "Public Global Stellar Network ; September 2015",
	})
	if err != nil {
		log.Fatalf("Failed to initialize plugin: %v", err)
	}

	fmt.Println("WASM plugin test completed successfully!")
}
