package main

import (
	"encoding/json"
	"log"
	"reflect"
	"unsafe"
)

// Define JSON structures for events and results
type Event struct {
	LedgerSequence uint32          `json:"ledger_sequence"`
	Data           json.RawMessage `json:"data"`
}

type Result struct {
	Output string `json:"output"`
	Error  string `json:"error,omitempty"`
}

// Global configuration for the processor
var config map[string]interface{}

// Memory management functions for WebAssembly
//
//export alloc
func alloc(size uint32) *byte {
	buf := make([]byte, size)
	return &buf[0]
}

//export dealloc
func dealloc(ptr *byte, size uint32) {
	// Go's GC will handle memory deallocation
}

// Initialize processor with config
//
//export initProcessor
func initProcessor(configPtr *byte, configLen uint32) uint32 {
	// Convert the config data to Go string/slice
	configData := ptrToBytes(configPtr, configLen)

	// Parse the JSON config
	err := json.Unmarshal(configData, &config)
	if err != nil {
		log.Printf("Failed to parse config: %v", err)
		return 1 // Error
	}

	log.Printf("Initialized WebAssembly processor with config: %v", config)
	return 0 // Success
}

// Process an event
//
//export process
func process(eventPtr *byte, eventLen uint32) uint32 {
	// Convert the event data to Go string/slice
	eventData := ptrToBytes(eventPtr, eventLen)

	// Parse the event JSON
	var event Event
	err := json.Unmarshal(eventData, &event)
	if err != nil {
		log.Printf("Failed to parse event: %v", err)
		return 1 // Error
	}

	log.Printf("Processing event with ledger sequence %d", event.LedgerSequence)

	// Extract the event data
	var eventObj map[string]interface{}
	err = json.Unmarshal(event.Data, &eventObj)
	if err != nil {
		log.Printf("Failed to parse event data: %v", err)
		return 1 // Error
	}

	// Process the event (this is where you'd implement your custom logic)
	// For example, let's add a new field to the event data
	eventObj["processed_by"] = "wasm-example"
	eventObj["config_value"] = config["example_value"]

	// Marshal the modified data back to JSON
	modifiedData, err := json.Marshal(eventObj)
	if err != nil {
		log.Printf("Failed to marshal modified data: %v", err)
		return 1 // Error
	}

	// Create a result object
	result := Result{
		Output: string(modifiedData),
		Error:  "",
	}

	// Marshal the result to JSON
	resultJSON, err := json.Marshal(result)
	if err != nil {
		log.Printf("Failed to marshal result: %v", err)
		return 1 // Error
	}

	// Copy the result to the shared WASM memory
	// In a real implementation, you would use a callback or
	// another mechanism to return the result
	// This example just logs the result
	log.Printf("Result: %s", string(resultJSON))

	return 0 // Success
}

// Return metadata about this processor
//
//export metadata
func metadata() uint32 {
	meta := map[string]string{
		"name":        "wasm-example",
		"version":     "0.1.0",
		"description": "Example WebAssembly processor",
	}

	metaJSON, err := json.Marshal(meta)
	if err != nil {
		log.Printf("Failed to marshal metadata: %v", err)
		return 1 // Error
	}

	log.Printf("Metadata: %s", string(metaJSON))
	return 0 // Success
}

// Helper function to convert WebAssembly memory pointer to Go byte slice
func ptrToBytes(ptr *byte, length uint32) []byte {
	return (*[1 << 30]byte)(unsafe.Pointer(ptr))[:length:length]
}

// Helper function to convert Go byte slice to WebAssembly memory pointer
func bytesToPtr(data []byte) (*byte, uint32) {
	header := (*reflect.SliceHeader)(unsafe.Pointer(&data))
	return (*byte)(unsafe.Pointer(header.Data)), uint32(len(data))
}

func main() {
	// This main function is required for Go build, but not used when compiled to WebAssembly
}
