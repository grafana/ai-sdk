package grafana

import (
	"fmt"

	"github.com/grafana/ai-sdk/provider"
)

var _ provider.ProviderOption = GrafanaOptions{}

// CaptureMode is the graded Agent Observability content-capture mode requested by
// a client for a single request. Its values mirror the server-side capture mode
// set. It is a named string type with typed constants; callers MUST use the
// constants rather than bare string literals.
//
// CaptureMode shapes the payload of a generation record that still occurs. It
// has no "off" value: to suppress the record entirely, set
// AgentObservabilityControl.Disabled instead.
type CaptureMode string

const (
	// CaptureModeFull exports all content.
	CaptureModeFull CaptureMode = "full"
	// CaptureModeMetadataOnly preserves message structure, tool names, usage,
	// and timing but strips text, tool arguments, tool results, thinking,
	// system prompts, conversation titles, and raw artifacts.
	CaptureModeMetadataOnly CaptureMode = "metadata_only"
	// CaptureModeFullWithMetadataSpans exports full content to the private
	// gRPC ingest destination while stripping content from shared OTel spans.
	CaptureModeFullWithMetadataSpans CaptureMode = "full_with_metadata_spans"
)

// valid reports whether m is one of the defined CaptureMode constants.
func (m CaptureMode) valid() bool {
	switch m {
	case CaptureModeFull, CaptureModeMetadataOnly, CaptureModeFullWithMetadataSpans:
		return true
	default:
		return false
	}
}

// AgentObservabilityControl carries per-request client control over the
// server-side Agent Observability middleware. A nil *AgentObservabilityControl means
// "no client preference; the backend default applies".
//
// Disabled and CaptureMode are orthogonal: Disabled suppresses the generation
// record entirely, whereas CaptureMode shapes the payload of a record that
// still occurs. When both are set, Disabled wins.
type AgentObservabilityControl struct {
	// Disabled, when non-nil and true, instructs the server-side middleware to
	// short-circuit and produce no generation record for the request. A nil
	// value applies the backend default.
	Disabled *bool `json:"disabled,omitempty"`
	// CaptureMode, when non-empty, requests a graded capture mode for the
	// request, overriding the backend tenant default. It must be one of the
	// defined CaptureMode constants.
	CaptureMode CaptureMode `json:"captureMode,omitempty"`
}

// TracingControl carries per-request client control over the server-side
// tracing middleware. A nil *TracingControl means "no client preference; the
// backend default applies".
type TracingControl struct {
	// Disabled, when non-nil and true, instructs the server-side tracing
	// middleware to short-circuit for the request. A nil value applies the
	// backend default.
	Disabled *bool `json:"disabled,omitempty"`
}

// MetricsControl carries per-request client control over the server-side
// metrics middleware. A nil *MetricsControl means "no client preference; the
// backend default applies".
type MetricsControl struct {
	// Disabled, when non-nil and true, instructs the server-side metrics
	// middleware to short-circuit for the request. A nil value applies the
	// backend default.
	Disabled *bool `json:"disabled,omitempty"`
}

// UsageControl carries per-request client control over the server-side usage
// tracking middleware. A nil *UsageControl means "no client preference; the
// backend default applies".
type UsageControl struct {
	// Disabled, when non-nil and true, instructs the server-side usage
	// tracking middleware to short-circuit for the request. A nil value
	// applies the backend default.
	Disabled *bool `json:"disabled,omitempty"`
}

// GrafanaOptions carries per-request, client-supplied control over the
// server-side middleware stack of the Grafana hosted provider. It implements
// provider.ProviderOption with ProviderKey "grafana" and is attached to a
// request through CallOptions.ProviderOptions (for example via
// provider.BuildProviderOptions and the orchestration layer's
// WithProviderOptions).
//
// Each field is a pointer to a per-middleware control struct. A nil field
// means "no client preference for that middleware; the backend default
// applies". Each control struct carries a Disabled knob for full suppression;
// graded controls such as Agent Observability capture mode live on the relevant
// control.
type GrafanaOptions struct {
	AgentObservability *AgentObservabilityControl `json:"agentObservability,omitempty"`
	Tracing            *TracingControl            `json:"tracing,omitempty"`
	Metrics            *MetricsControl            `json:"metrics,omitempty"`
	Usage              *UsageControl              `json:"usage,omitempty"`
}

// ProviderKey identifies the Grafana provider namespace.
func (GrafanaOptions) ProviderKey() string { return "grafana" }

// Validate checks the option against its known fields, returning an error for
// values that are recognized fields but carry invalid content (for example a
// CaptureMode outside the defined constants). It is intended for client-side
// use before a request is sent so misuse is surfaced early.
func (o GrafanaOptions) Validate() error {
	if o.AgentObservability != nil {
		if o.AgentObservability.CaptureMode != "" && !o.AgentObservability.CaptureMode.valid() {
			return fmt.Errorf("grafana: invalid Agent Observability captureMode %q", o.AgentObservability.CaptureMode)
		}
	}
	return nil
}
