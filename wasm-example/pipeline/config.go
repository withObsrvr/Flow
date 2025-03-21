package pipeline

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"../processor"
)

// ProcessorConfig represents a processor configuration in the pipeline
type ProcessorConfig struct {
	Type    string                 `yaml:"type"`
	Runtime string                 `yaml:"runtime,omitempty"`
	Path    string                 `yaml:"path,omitempty"`
	Config  map[string]interface{} `yaml:"config"`
}

// SourceConfig represents a source configuration in the pipeline
type SourceConfig struct {
	Type   string                 `yaml:"type"`
	Config map[string]interface{} `yaml:"config"`
}

// ConsumerConfig represents a consumer configuration in the pipeline
type ConsumerConfig struct {
	Type   string                 `yaml:"type"`
	Config map[string]interface{} `yaml:"config"`
}

// PipelineConfig represents a complete pipeline configuration
type PipelineConfig struct {
	Source     SourceConfig      `yaml:"source"`
	Processors []ProcessorConfig `yaml:"processors"`
	Consumers  []ConsumerConfig  `yaml:"consumers"`
}

// Config represents the full configuration with multiple pipelines
type Config struct {
	Pipelines map[string]PipelineConfig `yaml:"pipelines"`
}

// LoadConfig loads the pipeline configuration from a YAML file
func LoadConfig(filename string) (*Config, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	return ParseConfig(data)
}

// ParseConfig parses a pipeline configuration from YAML data
func ParseConfig(data []byte) (*Config, error) {
	var config Config
	err := yaml.Unmarshal(data, &config)
	if err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	return &config, nil
}

// CreateProcessors initializes processors based on the configuration
func CreateProcessors(ctx context.Context, registry *processor.Registry, pipelineConfig PipelineConfig, pluginsDir string) ([]processor.Processor, error) {
	processors := make([]processor.Processor, 0, len(pipelineConfig.Processors))

	for _, procConfig := range pipelineConfig.Processors {
		var proc processor.Processor
		var err error

		// Handle WebAssembly processors (explicit or by type prefix)
		if procConfig.Runtime == "wasm" || strings.HasPrefix(procConfig.Type, "wasm/") {
			// Determine path to WASM file
			var wasmPath string
			if procConfig.Path != "" {
				// If path is specified, use it
				wasmPath = procConfig.Path
				if !filepath.IsAbs(wasmPath) {
					wasmPath = filepath.Join(pluginsDir, wasmPath)
				}
			} else {
				// Otherwise, derive from type
				procName := strings.TrimPrefix(procConfig.Type, "wasm/")
				wasmPath = filepath.Join(pluginsDir, fmt.Sprintf("%s.wasm", procName))
			}

			// Load the WASM module
			err = registry.LoadWasmModule(ctx, wasmPath, procConfig.Type)
			if err != nil {
				return nil, fmt.Errorf("failed to load WASM processor %s: %w", procConfig.Type, err)
			}

			proc, err = registry.GetProcessor(procConfig.Type)
		} else {
			// Determine path to Go plugin
			pluginFileName := strings.ReplaceAll(procConfig.Type, "/", "-") + ".so"
			pluginPath := filepath.Join(pluginsDir, pluginFileName)

			// Load the Go plugin
			err = registry.LoadGoPlugin(pluginPath, procConfig.Type)
			if err != nil {
				return nil, fmt.Errorf("failed to load Go plugin processor %s: %w", procConfig.Type, err)
			}

			proc, err = registry.GetProcessor(procConfig.Type)
		}

		if err != nil {
			return nil, err
		}

		// Initialize the processor with its configuration
		err = proc.Init(procConfig.Config)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize processor %s: %w", procConfig.Type, err)
		}

		processors = append(processors, proc)
	}

	return processors, nil
}
