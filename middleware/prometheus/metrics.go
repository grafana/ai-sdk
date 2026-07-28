package prometheus

import promclient "github.com/prometheus/client_golang/prometheus"

const (
	metricRequestsTotal            = "aisdk_model_requests_total"
	metricInflightRequests         = "aisdk_model_inflight_requests"
	metricRequestDurationSeconds   = "aisdk_model_request_duration_seconds"
	metricTokensTotal              = "aisdk_model_tokens_total"
	metricTimeToFirstOutputSeconds = "aisdk_model_time_to_first_output_seconds"
	metricStreamChunksTotal        = "aisdk_model_stream_chunks_total"
	metricInterChunkDelaySeconds   = "aisdk_model_inter_chunk_delay_seconds"
)

type collectors struct {
	requests          *promclient.CounterVec
	inflight          *promclient.GaugeVec
	duration          *promclient.HistogramVec
	tokens            *promclient.CounterVec
	timeToFirstOutput *promclient.HistogramVec
	streamChunks      *promclient.CounterVec
	interChunkDelay   *promclient.HistogramVec
}

func newCollectors(config config) *collectors {
	c := &collectors{
		requests: promclient.NewCounterVec(promclient.CounterOpts{
			Name:        metricRequestsTotal,
			Help:        "Total provider language model calls by operation, identity, status, error type, status code, and finish reason.",
			ConstLabels: config.constLabels,
		}, []string{"operation", "provider", "model", "status", "error_type", "status_code", "finish_reason"}),
		inflight: promclient.NewGaugeVec(promclient.GaugeOpts{
			Name:        metricInflightRequests,
			Help:        "In-flight provider language model calls by operation and requested identity.",
			ConstLabels: config.constLabels,
		}, []string{"operation", "provider", "model"}),
		duration: promclient.NewHistogramVec(promclient.HistogramOpts{
			Name:        metricRequestDurationSeconds,
			Help:        "Provider language model call duration in seconds by operation, identity, and status.",
			ConstLabels: config.constLabels,
			Buckets:     config.durationBuckets,
		}, []string{"operation", "provider", "model", "status"}),
		tokens: promclient.NewCounterVec(promclient.CounterOpts{
			Name:        metricTokensTotal,
			Help:        "Provider language model token usage by operation, identity, and token type.",
			ConstLabels: config.constLabels,
		}, []string{"operation", "provider", "model", "token_type"}),
		timeToFirstOutput: promclient.NewHistogramVec(promclient.HistogramOpts{
			Name:        metricTimeToFirstOutputSeconds,
			Help:        "Time from stream provider call start to first payload-bearing output in seconds.",
			ConstLabels: config.constLabels,
			Buckets:     config.timeToFirstOutputBuckets,
		}, []string{"operation", "provider", "model", "status"}),
	}

	if !config.disableStreamChunkMetrics {
		c.streamChunks = promclient.NewCounterVec(promclient.CounterOpts{
			Name:        metricStreamChunksTotal,
			Help:        "Total provider stream parts observed by stream operation, identity, and chunk type.",
			ConstLabels: config.constLabels,
		}, []string{"operation", "provider", "model", "chunk_type"})
		c.interChunkDelay = promclient.NewHistogramVec(promclient.HistogramOpts{
			Name:        metricInterChunkDelaySeconds,
			Help:        "Delay between consecutive payload-bearing provider stream parts in seconds.",
			ConstLabels: config.constLabels,
			Buckets:     config.interChunkDelayBuckets,
		}, []string{"operation", "provider", "model", "chunk_type"})
	}

	return c
}

func (c *collectors) register(registerer promclient.Registerer) error {
	registered := make([]promclient.Collector, 0, 7)
	for _, collector := range c.enabledCollectors() {
		if err := registerer.Register(collector); err != nil {
			for _, previous := range registered {
				registerer.Unregister(previous)
			}
			return err
		}
		registered = append(registered, collector)
	}
	return nil
}

func (c *collectors) enabledCollectors() []promclient.Collector {
	collectors := []promclient.Collector{
		c.requests,
		c.inflight,
		c.duration,
		c.tokens,
		c.timeToFirstOutput,
	}
	if c.streamChunks != nil {
		collectors = append(collectors, c.streamChunks)
	}
	if c.interChunkDelay != nil {
		collectors = append(collectors, c.interChunkDelay)
	}
	return collectors
}
