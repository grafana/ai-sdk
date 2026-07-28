package prometheus

import promclient "github.com/prometheus/client_golang/prometheus"

// IdentitySource controls which provider/model identity is used for final metrics.
type IdentitySource string

const (
	// IdentityPreferResponse uses response provider/model metadata for final
	// metrics when both fields are available, falling back to requested identity.
	IdentityPreferResponse IdentitySource = "prefer_response"
	// IdentityRequested always uses the requested model provider/model identity.
	IdentityRequested IdentitySource = "requested"
)

// Options configures Prometheus middleware instrumentation.
type Options struct {
	// Registerer registers all collectors. Nil uses prometheus.DefaultRegisterer.
	Registerer promclient.Registerer
	// ConstLabels are attached to every collector at registration time.
	// Use only process-level labels that do not vary per request.
	ConstLabels promclient.Labels
	// IdentitySource controls provider/model labels for final metrics.
	// Zero value defaults to IdentityPreferResponse.
	IdentitySource IdentitySource
	// NormalizeProvider can bucket or redact provider labels.
	NormalizeProvider func(provider string) string
	// NormalizeModel can bucket or redact model labels. The provider argument
	// is the label value after NormalizeProvider has run.
	NormalizeModel func(provider, model string) string
	// DurationBuckets overrides request duration histogram buckets in seconds.
	DurationBuckets []float64
	// TimeToFirstOutputBuckets overrides TTFT histogram buckets in seconds.
	TimeToFirstOutputBuckets []float64
	// InterChunkDelayBuckets overrides inter-payload chunk delay buckets in seconds.
	InterChunkDelayBuckets []float64
	// DisableStreamChunkMetrics disables per-part counters and inter-chunk histograms.
	DisableStreamChunkMetrics bool
}

var defaultDurationBuckets = []float64{0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60, 120, 300}
var defaultTimeToFirstOutputBuckets = []float64{0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30}
var defaultInterChunkDelayBuckets = []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5}

type config struct {
	registerer                promclient.Registerer
	constLabels               promclient.Labels
	identitySource            IdentitySource
	normalizeProvider         func(provider string) string
	normalizeModel            func(provider, model string) string
	durationBuckets           []float64
	timeToFirstOutputBuckets  []float64
	interChunkDelayBuckets    []float64
	disableStreamChunkMetrics bool
}

func normalizeOptions(opts Options) config {
	registerer := opts.Registerer
	if registerer == nil {
		registerer = promclient.DefaultRegisterer
	}

	identitySource := opts.IdentitySource
	if identitySource == "" {
		identitySource = IdentityPreferResponse
	}

	return config{
		registerer:                registerer,
		constLabels:               cloneLabels(opts.ConstLabels),
		identitySource:            identitySource,
		normalizeProvider:         opts.NormalizeProvider,
		normalizeModel:            opts.NormalizeModel,
		durationBuckets:           bucketsOrDefault(opts.DurationBuckets, defaultDurationBuckets),
		timeToFirstOutputBuckets:  bucketsOrDefault(opts.TimeToFirstOutputBuckets, defaultTimeToFirstOutputBuckets),
		interChunkDelayBuckets:    bucketsOrDefault(opts.InterChunkDelayBuckets, defaultInterChunkDelayBuckets),
		disableStreamChunkMetrics: opts.DisableStreamChunkMetrics,
	}
}

func bucketsOrDefault(got []float64, def []float64) []float64 {
	if len(got) == 0 {
		return append([]float64(nil), def...)
	}
	return append([]float64(nil), got...)
}

func cloneLabels(labels promclient.Labels) promclient.Labels {
	if len(labels) == 0 {
		return nil
	}
	cloned := make(promclient.Labels, len(labels))
	for key, value := range labels {
		cloned[key] = value
	}
	return cloned
}
