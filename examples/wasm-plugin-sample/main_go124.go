// Go 1.24-compatible WASM plugin sample
package main

import (
	"encoding/json"
	"fmt"
	"syscall/js"
)

// Global variables to store plugin information
var (
	pluginName    = "flow/consumer/wasm-sample"
	pluginVersion = "1.0.0"
	pluginType    = 2 // ConsumerPlugin
	config        map[string]interface{}
	initialized   bool
)

// Register JavaScript functions
func registerCallbacks() {
	js.Global().Set("name", js.FuncOf(jsName))
	js.Global().Set("version", js.FuncOf(jsVersion))
	js.Global().Set("type", js.FuncOf(jsType))
	js.Global().Set("initialize", js.FuncOf(jsInitialize))
	js.Global().Set("process", js.FuncOf(jsProcess))
	js.Global().Set("close", js.FuncOf(jsClose))

	// Keep the program running
	c := make(chan struct{}, 0)
	<-c
}

// JavaScript-callable functions

func jsName(_ js.Value, _ []js.Value) interface{} {
	return pluginName
}

func jsVersion(_ js.Value, _ []js.Value) interface{} {
	return pluginVersion
}

func jsType(_ js.Value, _ []js.Value) interface{} {
	return pluginType
}

func jsInitialize(_ js.Value, args []js.Value) interface{} {
	if initialized {
		return 0 // Success
	}

	if len(args) < 1 {
		fmt.Println("Missing config argument")
		return 1 // Error
	}

	// Parse config from JSON
	configJSON := args[0].String()
	err := json.Unmarshal([]byte(configJSON), &config)
	if err != nil {
		fmt.Printf("Error parsing config: %v\n", err)
		return 1 // Error
	}

	fmt.Printf("Initialized WASM plugin %s with config: %v\n", pluginName, config)
	initialized = true
	return 0 // Success
}

func jsProcess(_ js.Value, args []js.Value) interface{} {
	if !initialized {
		fmt.Println("Plugin not initialized")
		return 1 // Error
	}

	if len(args) < 1 {
		fmt.Println("Missing message argument")
		return 1 // Error
	}

	// Parse message from JSON
	msgJSON := args[0].String()
	var message map[string]interface{}
	err := json.Unmarshal([]byte(msgJSON), &message)
	if err != nil {
		fmt.Printf("Error parsing message: %v\n", err)
		return 1 // Error
	}

	fmt.Printf("WASM plugin %s received message: %v\n", pluginName, message)
	return 0 // Success
}

func jsClose(_ js.Value, _ []js.Value) interface{} {
	if !initialized {
		return 0 // Success
	}

	fmt.Printf("Closing WASM plugin %s\n", pluginName)
	initialized = false
	return 0 // Success
}

func main() {
	fmt.Println("WASM plugin loaded (Go 1.24+ compatible)")
	registerCallbacks()
}
