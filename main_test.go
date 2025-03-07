// pipeline_test.go
package main

import (
	"context"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/withObsrvr/Flow/internal/metrics"
	"github.com/withObsrvr/pluginapi"
	"gopkg.in/yaml.v2"
)

// SamplePipelineConfig is a YAML representation of pipelines.
const SamplePipelineConfig = `
pipelines:
  PaymentPipeline:
    source:
      type: "BufferedStorageSourceAdapter"
      config:
        bucket_name: "bucket-name"
        network: "testnet"
        buffer_size: 640
        num_workers: 10
        retry_limit: 3
        retry_wait: 5
        start_ledger: 539328
    processors:
      - type: "TransformToAppPayment"
        config:
          network_passphrase: "Test SDF Network ; September 2015"
      - type: "FilterPayments"
        config:
          min_amount: "100.00"
          asset_code: "native"
          network_passphrase: "Test SDF Network ; September 2015"
    consumers:
      - type: "SaveToPostgreSQL"
        config:
          host: "localhost"
          port: 5432
          database: "stellar_events"
          username: "your_user"
          password: "your_password"
          ssl_mode: "disable"
          max_open_conns: 25
          max_idle_conns: 5
`

// dummyPlugin is a simple dummy implementation for testing.
// In a real scenario, these would be loaded dynamically.
type dummySource struct {
	name string
	cfg  map[string]interface{}
}

func (ds *dummySource) Name() string               { return ds.name }
func (ds *dummySource) Version() string            { return "dummy1.0" }
func (ds *dummySource) Type() pluginapi.PluginType { return pluginapi.SourcePlugin }
func (ds *dummySource) Initialize(config map[string]interface{}) error {
	ds.cfg = config
	return nil
}
func (ds *dummySource) Start(ctx context.Context) error    { return nil }
func (ds *dummySource) Stop() error                        { return nil }
func (ds *dummySource) Subscribe(proc pluginapi.Processor) {}

type dummyProcessor struct {
	name string
	cfg  map[string]interface{}
}

func (dp *dummyProcessor) Name() string               { return dp.name }
func (dp *dummyProcessor) Version() string            { return "dummy1.0" }
func (dp *dummyProcessor) Type() pluginapi.PluginType { return pluginapi.ProcessorPlugin }
func (dp *dummyProcessor) Initialize(config map[string]interface{}) error {
	dp.cfg = config
	return nil
}
func (dp *dummyProcessor) Process(ctx context.Context, msg pluginapi.Message) error { return nil }

type dummyConsumer struct {
	name string
	cfg  map[string]interface{}
}

func (dc *dummyConsumer) Name() string               { return dc.name }
func (dc *dummyConsumer) Version() string            { return "dummy1.0" }
func (dc *dummyConsumer) Type() pluginapi.PluginType { return pluginapi.ConsumerPlugin }
func (dc *dummyConsumer) Initialize(config map[string]interface{}) error {
	dc.cfg = config
	return nil
}
func (dc *dummyConsumer) Process(ctx context.Context, msg pluginapi.Message) error { return nil }
func (dc *dummyConsumer) Close() error                                             { return nil }

// TestBuildPipeline verifies that a pipeline is built from the YAML config.
func TestBuildPipeline(t *testing.T) {
	var cfg PipelineConfig
	err := yaml.Unmarshal([]byte(SamplePipelineConfig), &cfg)
	assert.NoError(t, err)

	// Create a dummy registry with factories for the expected plugin types.
	reg := NewPluginRegistry()
	reg.Sources["BufferedStorageSourceAdapter"] = &dummySource{name: "BufferedStorageSourceAdapter"}
	reg.Processors["TransformToAppPayment"] = &dummyProcessor{name: "TransformToAppPayment"}
	reg.Processors["FilterPayments"] = &dummyProcessor{name: "FilterPayments"}
	reg.Consumers["SaveToPostgreSQL"] = &dummyConsumer{name: "SaveToPostgreSQL"}

	pipeDef, ok := cfg.Pipelines["PaymentPipeline"]
	assert.True(t, ok, "PaymentPipeline configuration must exist")

	pipe, err := BuildPipeline("PaymentPipeline", pipeDef, reg)
	assert.NoError(t, err)
	assert.Equal(t, "BufferedStorageSourceAdapter", pipe.Source.Name())
	assert.Len(t, pipe.Processors, 2)
	assert.Len(t, pipe.Consumers, 1)

	// Optionally, simulate processing a message.
	ctx := context.Background()
	msg := pluginapi.Message{
		Payload:   []byte("Test message"),
		Timestamp: time.Now(),
	}
	err = func() error {
		// Run through processors.
		for _, proc := range pipe.Processors {
			if err := proc.Process(ctx, msg); err != nil {
				return err
			}
		}
		// Run through consumers.
		for _, cons := range pipe.Consumers {
			if err := cons.Process(ctx, msg); err != nil {
				return err
			}
		}
		return nil
	}()
	assert.NoError(t, err)
}

func TestMetrics(t *testing.T) {
	// Reset metrics
	metrics.MessagesProcessed.Reset()

	// Simulate activity with new labels
	metrics.MessagesProcessed.WithLabelValues(
		"test_tenant",
		"test_instance",
		"test_pipeline",
		"test_processor",
	).Inc()

	// Verify with updated labels
	metricFamilies, err := prometheus.DefaultGatherer.Gather()
	assert.NoError(t, err)

	for _, mf := range metricFamilies {
		if mf.GetName() == "flow_messages_processed_total" {
			metric := mf.GetMetric()[0]
			assert.Equal(t, "test_tenant", metric.Label[0].GetValue())
			assert.Equal(t, "test_instance", metric.Label[1].GetValue())
			assert.Equal(t, "test_pipeline", metric.Label[2].GetValue())
			assert.Equal(t, "test_processor", metric.Label[3].GetValue())
			assert.Equal(t, float64(1), metric.Counter.GetValue())
			return
		}
	}
	t.Fatal("Expected metric was not found")
}
