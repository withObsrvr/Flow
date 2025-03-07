package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// Pipeline metrics
	MessagesProcessed = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "flow_messages_processed_total",
			Help: "The total number of processed messages",
		},
		[]string{"tenant_id", "instance_id", "pipeline", "processor"},
	)

	ProcessingDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "flow_message_processing_duration_seconds",
			Help:    "Time spent processing messages",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"tenant_id", "instance_id", "pipeline", "processor"},
	)

	ProcessingErrors = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "flow_processing_errors_total",
			Help: "The total number of processing errors",
		},
		[]string{"tenant_id", "instance_id", "pipeline", "processor", "error_type"},
	)

	// Consumer metrics
	MessagesConsumed = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "flow_messages_consumed_total",
			Help: "The total number of consumed messages",
		},
		[]string{"tenant_id", "instance_id", "pipeline", "consumer"},
	)

	// Ledger specific metrics
	LedgersProcessed = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "flow_ledgers_processed_total",
			Help: "The total number of ledgers processed",
		},
		[]string{"tenant_id", "instance_id", "pipeline", "source"},
	)

	LedgerProcessingDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "flow_ledger_processing_duration_seconds",
			Help:    "Time spent processing ledgers",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"tenant_id", "instance_id", "pipeline", "source"},
	)
)
