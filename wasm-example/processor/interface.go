package processor

import (
	"context"
)

// Event represents data to be processed
type Event struct {
	LedgerSequence uint64 `json:"ledger_sequence"`
	Data           []byte `json:"data"`
}

// Result represents the outcome of processing
type Result struct {
	Output []byte `json:"output"`
	Error  string `json:"error,omitempty"`
}

// Processor defines the interface for both Go plugin and WebAssembly processors
type Processor interface {
	// Process processes an event and returns a result
	Process(ctx context.Context, event Event) (Result, error)

	// Init initializes the processor with configuration
	Init(config map[string]interface{}) error

	// Metadata returns information about the processor
	Metadata() map[string]string
}
