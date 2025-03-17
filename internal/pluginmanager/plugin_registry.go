package pluginmanager

import (
	"fmt"

	"github.com/withObsrvr/pluginapi"
)

// PluginRegistry holds references to all registered plugins, categorized by type
type PluginRegistry struct {
	Sources    map[string]pluginapi.Source
	Processors map[string]pluginapi.Processor
	Consumers  map[string]pluginapi.Consumer
}

// NewPluginRegistry creates a new plugin registry
func NewPluginRegistry() *PluginRegistry {
	return &PluginRegistry{
		Sources:    make(map[string]pluginapi.Source),
		Processors: make(map[string]pluginapi.Processor),
		Consumers:  make(map[string]pluginapi.Consumer),
	}
}

// Register adds a plugin to the registry based on its type
func (pr *PluginRegistry) Register(p pluginapi.Plugin) error {
	switch p.Type() {
	case pluginapi.SourcePlugin:
		source, ok := p.(pluginapi.Source)
		if !ok {
			return fmt.Errorf("plugin %s claims to be a Source but does not implement the Source interface", p.Name())
		}
		pr.Sources[p.Name()] = source
	case pluginapi.ProcessorPlugin:
		processor, ok := p.(pluginapi.Processor)
		if !ok {
			return fmt.Errorf("plugin %s claims to be a Processor but does not implement the Processor interface", p.Name())
		}
		pr.Processors[p.Name()] = processor
	case pluginapi.ConsumerPlugin:
		consumer, ok := p.(pluginapi.Consumer)
		if !ok {
			return fmt.Errorf("plugin %s claims to be a Consumer but does not implement the Consumer interface", p.Name())
		}
		pr.Consumers[p.Name()] = consumer
	default:
		return fmt.Errorf("unknown plugin type %v for plugin %s", p.Type(), p.Name())
	}
	return nil
}

// GetSource returns a source plugin by name
func (pr *PluginRegistry) GetSource(name string) (pluginapi.Source, error) {
	source, ok := pr.Sources[name]
	if !ok {
		return nil, fmt.Errorf("source plugin %s not found", name)
	}
	return source, nil
}

// GetProcessor returns a processor plugin by name
func (pr *PluginRegistry) GetProcessor(name string) (pluginapi.Processor, error) {
	processor, ok := pr.Processors[name]
	if !ok {
		return nil, fmt.Errorf("processor plugin %s not found", name)
	}
	return processor, nil
}

// GetConsumer returns a consumer plugin by name
func (pr *PluginRegistry) GetConsumer(name string) (pluginapi.Consumer, error) {
	consumer, ok := pr.Consumers[name]
	if !ok {
		return nil, fmt.Errorf("consumer plugin %s not found", name)
	}
	return consumer, nil
}
