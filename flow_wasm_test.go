package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/withObsrvr/pluginapi"
)

// TestSource is a simple source adapter for testing
type TestSource struct {
	name       string
	version    string
	config     map[string]interface{}
	consumers  []pluginapi.Consumer
	processors []pluginapi.Processor
}

// NewTestSource creates a new TestSource
func NewTestSource() pluginapi.Plugin {
	return &TestSource{
		name:       "TestSource",
		version:    "1.0.0",
		consumers:  make([]pluginapi.Consumer, 0),
		processors: make([]pluginapi.Processor, 0),
	}
}

// Name returns the name of the plugin
func (s *TestSource) Name() string {
	return s.name
}

// Version returns the version of the plugin
func (s *TestSource) Version() string {
	return s.version
}

// Type returns the type of the plugin
func (s *TestSource) Type() pluginapi.PluginType {
	return pluginapi.SourcePlugin
}

// Initialize sets up the source with the given configuration
func (s *TestSource) Initialize(config map[string]interface{}) error {
	s.config = config
	log.Printf("TestSource initialized with config: %v", config)
	return nil
}

// Subscribe adds a processor to the source
func (s *TestSource) Subscribe(p pluginapi.Processor) {
	s.processors = append(s.processors, p)
}

// Start begins reading from the source
func (s *TestSource) Start(ctx context.Context) error {
	log.Printf("TestSource started")
	return nil
}

// TestConsumer is a simple consumer for testing
type TestConsumer struct {
	name    string
	version string
	config  map[string]interface{}
}

// NewTestConsumer creates a new TestConsumer
func NewTestConsumer() pluginapi.Plugin {
	return &TestConsumer{
		name:    "TestConsumer",
		version: "1.0.0",
	}
}

// Name returns the name of the plugin
func (c *TestConsumer) Name() string {
	return c.name
}

// Version returns the version of the plugin
func (c *TestConsumer) Version() string {
	return c.version
}

// Type returns the type of the plugin
func (c *TestConsumer) Type() pluginapi.PluginType {
	return pluginapi.ConsumerPlugin
}

// Initialize sets up the consumer with the given configuration
func (c *TestConsumer) Initialize(config map[string]interface{}) error {
	c.config = config
	log.Printf("TestConsumer initialized with config: %v", config)
	return nil
}

// Process processes a message
func (c *TestConsumer) Process(ctx context.Context, message pluginapi.Message) error {
	log.Printf("TestConsumer processing message: %v", message)
	return nil
}

// RegisterPlugins registers the test plugins
func RegisterPlugins() {
	log.Println("Registering test plugins...")
	pluginsDir := filepath.Join(".", "plugins")
	if err := os.MkdirAll(pluginsDir, 0755); err != nil {
		log.Fatalf("Failed to create plugins directory: %v", err)
	}

	// Create source plugin file
	sourcePath := filepath.Join(pluginsDir, "test-source.so")
	sourceContent := []byte(`
package main

import (
	"github.com/withObsrvr/pluginapi"
)

// New creates a new TestSource
func New() pluginapi.Plugin {
	return &TestSource{
		name:       "TestSource",
		version:    "1.0.0",
		consumers:  make([]pluginapi.Consumer, 0),
		processors: make([]pluginapi.Processor, 0),
	}
}

// TestSource implementation...
`)
	if err := os.WriteFile(sourcePath, sourceContent, 0644); err != nil {
		log.Fatalf("Failed to write source plugin: %v", err)
	}

	// Create consumer plugin file
	consumerPath := filepath.Join(pluginsDir, "test-consumer.so")
	consumerContent := []byte(`
package main

import (
	"github.com/withObsrvr/pluginapi"
)

// New creates a new TestConsumer
func New() pluginapi.Plugin {
	return &TestConsumer{
		name:    "TestConsumer",
		version: "1.0.0",
	}
}

// TestConsumer implementation...
`)
	if err := os.WriteFile(consumerPath, consumerContent, 0644); err != nil {
		log.Fatalf("Failed to write consumer plugin: %v", err)
	}

	log.Printf("Created test plugins at %s", pluginsDir)
}

func main() {
	fmt.Println("This is just a sample test plugin implementation.")
	fmt.Println("In a real scenario, these would be built as .so files")
}
