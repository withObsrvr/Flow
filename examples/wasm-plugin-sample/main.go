package main

import (
	"encoding/json"
	"fmt"
	"reflect"
	"unsafe"
)

// Global variables to store plugin information
var (
	pluginName    = "flow/consumer/wasm-sample"
	pluginVersion = "1.0.0"
	pluginType    = 2 // ConsumerPlugin
	config        map[string]interface{}
	initialized   bool
)

// Memory management functions
//
//export alloc
func alloc(size uint64) uint64 {
	buf := make([]byte, size)
	return uint64(uintptr(unsafe.Pointer(&buf[0])))
}

//export free
func free(ptr uint64, size uint64) {
	// In Go with WASM, memory is handled by the Go runtime
	// This is a no-op function for compatibility
}

// Plugin interface functions

//export name
func name() (uint64, uint64) {
	buf := []byte(pluginName)
	ptr := &buf[0]
	unsafePtr := uintptr(unsafe.Pointer(ptr))
	return uint64(unsafePtr), uint64(len(buf))
}

//export version
func version() (uint64, uint64) {
	buf := []byte(pluginVersion)
	ptr := &buf[0]
	unsafePtr := uintptr(unsafe.Pointer(ptr))
	return uint64(unsafePtr), uint64(len(buf))
}

//export type
func type_() uint64 {
	return uint64(pluginType)
}

//export initialize
func initialize(configPtr uint64, configLen uint64) uint64 {
	if initialized {
		return 0 // Success
	}

	// Read the config from memory
	configBytes := readMemory(configPtr, configLen)

	// Parse the config
	err := json.Unmarshal(configBytes, &config)
	if err != nil {
		fmt.Printf("Error parsing config: %v\n", err)
		return 1 // Error
	}

	// Print out the config for debugging
	fmt.Printf("Initialized WASM plugin %s with config: %v\n", pluginName, config)

	initialized = true
	return 0 // Success
}

//export process
func process(msgPtr uint64, msgLen uint64) uint64 {
	if !initialized {
		fmt.Println("Plugin not initialized")
		return 1 // Error
	}

	// Read the message from memory
	msgBytes := readMemory(msgPtr, msgLen)

	// Parse the message
	var message map[string]interface{}
	err := json.Unmarshal(msgBytes, &message)
	if err != nil {
		fmt.Printf("Error parsing message: %v\n", err)
		return 1 // Error
	}

	// Print the message for demonstration purposes
	fmt.Printf("WASM plugin %s received message: %v\n", pluginName, message)

	return 0 // Success
}

//export close
func close() uint64 {
	if !initialized {
		return 0 // Success
	}

	fmt.Printf("Closing WASM plugin %s\n", pluginName)

	initialized = false
	return 0 // Success
}

// Helper function to read memory
func readMemory(ptr uint64, size uint64) []byte {
	// Convert the pointer to a slice
	var buf []byte
	sh := (*reflect.SliceHeader)(unsafe.Pointer(&buf))
	sh.Data = uintptr(ptr)
	sh.Len = int(size)
	sh.Cap = int(size)

	// Make a copy of the data to avoid issues with GC
	result := make([]byte, size)
	copy(result, buf)

	return result
}

func main() {
	// This function is required by TinyGo, but is not used when the module is loaded by Wazero
	fmt.Println("WASM plugin loaded")
}
