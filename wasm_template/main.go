package main

import (
	"encoding/json"
	"fmt"
)

// Configuration for the latest ledger processor
type Config struct {
	NetworkPassphrase string `json:"network_passphrase"`
	// Add other configuration options here
}

// Global configuration - will be set by initialize
var processorConfig Config

//export name
func _name() string {
	return "flow/processor/latest-ledger"
}

//export version
func _version() string {
	return "1.0.0"
}

//export initialize
func _initialize(configJSON string) int32 {
	// Parse the configuration JSON
	err := json.Unmarshal([]byte(configJSON), &processorConfig)
	if err != nil {
		fmt.Printf("Error parsing config: %v\n", err)
		return 1 // Error
	}

	fmt.Printf("Initialized latest-ledger processor with network: %s\n",
		processorConfig.NetworkPassphrase)
	return 0 // Success
}

//export processLedger
func _processLedger(ledgerJSON string) string {
	// Parse the ledger data
	var ledgerData map[string]interface{}
	err := json.Unmarshal([]byte(ledgerJSON), &ledgerData)
	if err != nil {
		return fmt.Sprintf(`{"error":"Failed to parse ledger data: %v"}`, err)
	}

	// Extract ledger sequence if available
	var ledgerSeq string
	if seq, ok := ledgerData["sequence"]; ok {
		ledgerSeq = fmt.Sprintf("%v", seq)
	} else {
		ledgerSeq = "unknown"
	}

	// Process the ledger data
	// This is where your actual processing logic would go
	fmt.Printf("Processing ledger %s with network %s\n",
		ledgerSeq, processorConfig.NetworkPassphrase)

	// Example result - this would be your actual latest ledger data
	result := map[string]interface{}{
		"processor":       "latest-ledger",
		"ledger_sequence": ledgerSeq,
		"network":         processorConfig.NetworkPassphrase,
		"timestamp":       ledgerData["close_time"],
		"status":          "success",
	}

	// Convert result to JSON
	resultJSON, err := json.Marshal(result)
	if err != nil {
		return `{"error":"Failed to generate result"}`
	}

	return string(resultJSON)
}

// Required for Go WASM modules - doesn't need any implementation
func main() {}
