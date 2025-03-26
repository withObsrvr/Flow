// Package main is a native Go plugin that wraps a WASM processor.
package main

import (
	"context"
	"log"
	"time"

	"github.com/withObsrvr/pluginapi"
)

// LatestLedgerProcessor is a wrapper for the WASM processor
type LatestLedgerProcessor struct {
	name       string
	version    string
	config     map[string]interface{}
	consumers  []pluginapi.Consumer
	processors []pluginapi.Processor
}

// New creates a new LatestLedgerProcessor
// This must be exported for the Go plugin system to work
func New() pluginapi.Plugin {
	log.Println("Creating latest ledger processor wrapper")
	return &LatestLedgerProcessor{
		name:       "flow/processor/latest-ledger",
		version:    "1.0.0",
		consumers:  make([]pluginapi.Consumer, 0),
		processors: make([]pluginapi.Processor, 0),
	}
}

// Name returns the name of the plugin
func (p *LatestLedgerProcessor) Name() string {
	return p.name
}

// Version returns the version of the plugin
func (p *LatestLedgerProcessor) Version() string {
	return p.version
}

// Type returns the type of the plugin
func (p *LatestLedgerProcessor) Type() pluginapi.PluginType {
	return pluginapi.ProcessorPlugin
}

// Initialize sets up the processor with configuration
func (p *LatestLedgerProcessor) Initialize(config map[string]interface{}) error {
	p.config = config
	log.Printf("Initialized Latest Ledger Processor wrapper with config: %v", config)
	return nil
}

// Process handles incoming messages
func (p *LatestLedgerProcessor) Process(ctx context.Context, msg pluginapi.Message) error {
	// Log the message
	log.Printf("Latest Ledger Processor processing message with %d bytes", len(msg.Payload.([]byte)))

	// Create a new message to pass to downstream consumers
	outMsg := pluginapi.Message{
		Payload: msg.Payload,
		Metadata: map[string]interface{}{
			"processed_by": "flow/processor/latest-ledger",
			"timestamp":    time.Now().Format(time.RFC3339),
		},
		Timestamp: time.Now(),
	}

	// Add any existing metadata
	for k, v := range msg.Metadata {
		outMsg.Metadata[k] = v
	}

	// Forward the message to all consumers
	for _, consumer := range p.consumers {
		if err := consumer.Process(ctx, outMsg); err != nil {
			log.Printf("Error forwarding message to consumer %s: %v", consumer.Name(), err)
		}
	}

	// Forward the message to all processors
	for _, processor := range p.processors {
		if err := processor.Process(ctx, outMsg); err != nil {
			log.Printf("Error forwarding message to processor %s: %v", processor.Name(), err)
		}
	}

	return nil
}

// RegisterConsumer registers a downstream consumer
func (p *LatestLedgerProcessor) RegisterConsumer(consumer pluginapi.Consumer) {
	log.Printf("Registered consumer: %s", consumer.Name())
	p.consumers = append(p.consumers, consumer)
}

// Subscribe allows a processor to subscribe to this one
func (p *LatestLedgerProcessor) Subscribe(processor pluginapi.Processor) {
	log.Printf("Processor %s subscribed", processor.Name())
	p.processors = append(p.processors, processor)
}

func main() {
	// This function is not used for plugins
}
