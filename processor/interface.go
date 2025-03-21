package processor

import (
	"context"
)

// Event represents data to be processed
type Event struct {
	LedgerSequence uint64
	Data           []byte
	// Add other fields as needed
}

// Result represents the outcome of processing
type Result struct {
	Output []byte
	Error  error
}

// Processor is the interface that all processors must implement
type Processor interface {
	// Process handles an event and returns a result
	Process(ctx context.Context, event Event) (Result, error)

	// Init initializes the processor with configuration
	Init(config map[string]interface{}) error

	// Metadata returns information about this processor
	Metadata() map[string]string
}
