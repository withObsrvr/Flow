package pipeline

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"../processor"
)

// Pipeline represents a running pipeline processing events from a source
// through a series of processors and to consumers
type Pipeline struct {
	Name       string
	Source     interface{} // The actual source interface would be defined elsewhere
	Processors []processor.Processor
	Consumers  []interface{} // The actual consumer interface would be defined elsewhere
	Config     PipelineConfig
}

// NewPipeline creates a new pipeline with the given components
func NewPipeline(name string, source interface{}, processors []processor.Processor, consumers []interface{}, config PipelineConfig) *Pipeline {
	return &Pipeline{
		Name:       name,
		Source:     source,
		Processors: processors,
		Consumers:  consumers,
		Config:     config,
	}
}

// Start starts the pipeline and begins processing events
func (p *Pipeline) Start(ctx context.Context) error {
	log.Printf("Starting pipeline: %s", p.Name)

	// This is a simplified implementation
	// A real implementation would handle events from the source,
	// process them through processors, and send them to consumers

	// Mock event processing for demonstration
	eventCh := make(chan processor.Event)

	// Start a goroutine to simulate events from the source
	go func() {
		defer close(eventCh)

		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(1 * time.Second):
				// Simulate an event
				event := processor.Event{
					LedgerSequence: 12345,
					Data:           []byte(`{"type":"test_event","message":"Hello, world!"}`),
				}

				select {
				case eventCh <- event:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	// Process events from the source
	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()

		for event := range eventCh {
			// Process the event through all processors in sequence
			data := event.Data
			var err error

			for i, proc := range p.Processors {
				log.Printf("Pipeline %s: Processing event with processor %d (%s)", p.Name, i, proc.Metadata()["name"])

				// Create a context with timeout for this processing step
				procCtx, cancel := context.WithTimeout(ctx, 5*time.Second)

				result, procErr := proc.Process(procCtx, processor.Event{
					LedgerSequence: event.LedgerSequence,
					Data:           data,
				})

				cancel()

				if procErr != nil {
					log.Printf("Pipeline %s: Error processing event with processor %d: %v", p.Name, i, procErr)
					err = procErr
					break
				}

				if result.Error != "" {
					log.Printf("Pipeline %s: Processor %d returned error: %s", p.Name, i, result.Error)
					err = fmt.Errorf("processor error: %s", result.Error)
					break
				}

				// Use the output of this processor as input to the next
				data = result.Output
			}

			if err != nil {
				log.Printf("Pipeline %s: Failed to process event: %v", p.Name, err)
				continue
			}

			// Send processed data to consumers
			// (simplified for demonstration)
			for i, consumer := range p.Consumers {
				log.Printf("Pipeline %s: Sending processed event to consumer %d", p.Name, i)

				// In a real implementation, you would call the consumer's methods
				// to handle the processed data
				_ = consumer
			}
		}
	}()

	// Wait for processing to complete
	wg.Wait()
	return nil
}

// Stop stops the pipeline
func (p *Pipeline) Stop(ctx context.Context) error {
	log.Printf("Stopping pipeline: %s", p.Name)
	// Implement cleanup logic here
	return nil
}
