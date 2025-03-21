package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"./pipeline"
	"./processor"
)

type mockSource struct{}
type mockConsumer struct{}

func main() {
	// Parse command line flags
	configFile := flag.String("config", "example_pipeline.yaml", "Path to pipeline configuration file")
	pluginsDir := flag.String("plugins-dir", "./processors", "Directory containing processor plugins")
	flag.Parse()

	// Set up context with cancellation for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle OS signals for graceful shutdown
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-signalCh
		log.Printf("Received signal %s, shutting down", sig)
		cancel()
	}()

	// Load pipeline configuration
	config, err := pipeline.LoadConfig(*configFile)
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Create processor registry
	registry := processor.NewRegistry()

	// Create and start each pipeline
	pipelines := make(map[string]*pipeline.Pipeline)
	for name, pipelineConfig := range config.Pipelines {
		log.Printf("Creating pipeline: %s", name)

		// Create processors for the pipeline
		processors, err := pipeline.CreateProcessors(ctx, registry, pipelineConfig, *pluginsDir)
		if err != nil {
			log.Fatalf("Failed to create processors for pipeline %s: %v", name, err)
		}

		// Create source and consumers (simplified for demonstration)
		source := createSource(pipelineConfig.Source)
		consumers := createConsumers(pipelineConfig.Consumers)

		// Create and store the pipeline
		p := pipeline.NewPipeline(name, source, processors, consumers, pipelineConfig)
		pipelines[name] = p

		// Start the pipeline
		go func(p *pipeline.Pipeline) {
			if err := p.Start(ctx); err != nil {
				log.Printf("Pipeline %s stopped with error: %v", p.Name, err)
			}
		}(p)
	}

	// Wait for context cancellation (due to SIGINT/SIGTERM)
	<-ctx.Done()

	// Stop all pipelines
	for _, p := range pipelines {
		log.Printf("Stopping pipeline: %s", p.Name)
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 10*time.Second)
		if err := p.Stop(stopCtx); err != nil {
			log.Printf("Error stopping pipeline %s: %v", p.Name, err)
		}
		stopCancel()
	}

	log.Println("Shutdown complete")
}

// createSource creates a source based on the configuration
// This is a simplified implementation for demonstration
func createSource(config pipeline.SourceConfig) interface{} {
	// In a real implementation, you would create the actual source
	// based on the config.Type and initialize it with config.Config
	log.Printf("Creating source of type: %s", config.Type)
	return &mockSource{}
}

// createConsumers creates consumers based on the configuration
// This is a simplified implementation for demonstration
func createConsumers(configs []pipeline.ConsumerConfig) []interface{} {
	consumers := make([]interface{}, 0, len(configs))
	for _, config := range configs {
		// In a real implementation, you would create the actual consumer
		// based on the config.Type and initialize it with config.Config
		log.Printf("Creating consumer of type: %s", config.Type)
		consumers = append(consumers, &mockConsumer{})
	}
	return consumers
}
