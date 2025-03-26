#!/bin/bash
set -e

# Clean up existing pluginapi if it exists
rm -rf pluginapi

# Create simplified pluginapi package
echo "Creating simplified pluginapi package..."
mkdir -p pluginapi
cat > pluginapi/pluginapi.go << 'EOF'
package pluginapi

import (
	"context"
	"time"
)

// PluginType defines the category of a plugin.
type PluginType int

const (
	SourcePlugin PluginType = iota
	ProcessorPlugin
	ConsumerPlugin
	BufferPlugin
)

// Plugin is the basic interface that every plugin must implement.
type Plugin interface {
	// Name returns the plugin's unique name.
	Name() string
	// Version returns the plugin's version.
	Version() string
	// Type returns the category of the plugin.
	Type() PluginType
	// Initialize configures the plugin with a map of settings.
	Initialize(config map[string]interface{}) error
}

// Source is an extension of Plugin that ingests data.
type Source interface {
	Plugin
	// Start begins data ingestion.
	Start(ctx context.Context) error
	// Stop halts ingestion.
	Stop() error
	// Subscribe allows downstream processors to be added.
	Subscribe(proc Processor)
}

// Processor is an extension of Plugin that transforms data.
type Processor interface {
	Plugin
	// Process transforms a message.
	Process(ctx context.Context, msg Message) error
}

// Consumer is an extension of Plugin that consumes data.
type Consumer interface {
	Plugin
	// Process handles a message (e.g. storing it).
	Process(ctx context.Context, msg Message) error
	// Close cleans up any resources used by the consumer.
	Close() error
}

// Message is the unified data structure that flows between plugins.
type Message struct {
	// Payload holds the primary data (often as []byte).
	Payload interface{}
	// Metadata is an optional map with additional info.
	Metadata map[string]interface{}
	// Timestamp indicates when the message was created.
	Timestamp time.Time
}

// ConsumerRegistry is an interface for processors that can register consumers
type ConsumerRegistry interface {
	RegisterConsumer(consumer Consumer)
}
EOF

# Create a go.mod file for pluginapi
cat > pluginapi/go.mod << 'EOF'
module github.com/withObsrvr/pluginapi

go 1.21
EOF

# Make sure plugins directory exists
mkdir -p plugins

# Build the Go plugin wrapper
echo "Building Go plugin wrapper..."
cd wasm_plugin_wrapper
go build -buildmode=plugin -o ../plugins/flow-processor-latest-ledger.so main.go
cd ..

# Update the pipeline configuration
echo "Updating pipeline configuration..."
cat > /tmp/tmp.8VJNC4xb5F/simple_pipeline.yaml << 'EOF'
source:
  type: BufferedStorageSourceAdapter
  config:
    bucket_name: obrsvr-stellar-ledger-data-mainnet-data/landing/
    network: mainnet
    num_workers: 5
    retry_limit: 3
    retry_wait: 2
    start_ledger: 56146570
    ledgers_per_file: 64
    files_per_partition: 100

processor:
  type: flow/processor/latest-ledger
  config:
    network_passphrase: "Public Global Stellar Network ; September 2015"

consumer:
  type: SaveToZeroMQ
  config:
    address: "tcp://127.0.0.1:5555"
EOF

echo "Updating run_latest_ledger.sh script..."
cat > run_latest_ledger.sh << 'EOF'
#!/bin/bash
set -e

export FLOW_DEBUG=1
export FLOW_METRICS_ADDR=:8080

echo "Starting Flow with latest ledger processor..."
./Flow \
  --config /tmp/tmp.8VJNC4xb5F/simple_pipeline.yaml \
  --plugins-dir ./plugins
EOF

chmod +x run_latest_ledger.sh

echo "Setup completed. Run ./run_latest_ledger.sh to start the application." 