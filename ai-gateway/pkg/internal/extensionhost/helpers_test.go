package extensionhost

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"regexp"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"github.com/grafana/ai-sdk/ai-gateway/catalog"
	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
	"github.com/grafana/ai-sdk/ai-gateway/internal/testadapter"
	"github.com/grafana/ai-sdk/ai-gateway/pkg/extension"
	"github.com/grafana/ai-sdk/ai-gateway/pkg/extension/tenanttype"
	"github.com/grafana/ai-sdk/provider"
)

// journal records phase events across extensions in order.
type journal struct {
	mu      sync.Mutex
	entries []string
}

func (j *journal) add(entry string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.entries = append(j.entries, entry)
}

func (j *journal) list() []string {
	j.mu.Lock()
	defer j.mu.Unlock()
	return append([]string(nil), j.entries...)
}

// recorder implements every phase, journals each call and delegates to the
// optional hooks.
type recorder struct {
	name            string
	journal         *journal
	admit           func(context.Context, *extension.Call) error
	transform       func(context.Context, *extension.Call, provider.CallOptions) (provider.CallOptions, error)
	inspectRequest  func(context.Context, *extension.Call, provider.CallOptions) error
	inspectResponse func(context.Context, *extension.Call, extension.Part) error
	observe         func(context.Context, *extension.Call, extension.Record) error
	close           func(context.Context) error
}

func (r *recorder) record(entry string) {
	if r.journal != nil {
		r.journal.add(r.name + ":" + entry)
	}
}

func (r *recorder) Admit(ctx context.Context, call *extension.Call) error {
	r.record("admit")
	if r.admit != nil {
		return r.admit(ctx, call)
	}
	return nil
}

func (r *recorder) TransformRequest(ctx context.Context, call *extension.Call, params provider.CallOptions) (provider.CallOptions, error) {
	r.record("transform")
	if r.transform != nil {
		return r.transform(ctx, call, params)
	}
	return params, nil
}

func (r *recorder) InspectRequest(ctx context.Context, call *extension.Call, params provider.CallOptions) error {
	r.record("inspect-request")
	if r.inspectRequest != nil {
		return r.inspectRequest(ctx, call, params)
	}
	return nil
}

func (r *recorder) InspectResponse(ctx context.Context, call *extension.Call, part extension.Part) error {
	r.record("inspect-response:" + string(part.Type))
	if r.inspectResponse != nil {
		return r.inspectResponse(ctx, call, part)
	}
	return nil
}

func (r *recorder) Observe(ctx context.Context, call *extension.Call, record extension.Record) error {
	r.record("observe:" + string(record.Outcome))
	if r.observe != nil {
		return r.observe(ctx, call, record)
	}
	return nil
}

func (r *recorder) Close(ctx context.Context) error {
	if r.close != nil {
		return r.close(ctx)
	}
	return nil
}

// define returns an enabled operator definition whose factory returns value.
func define(name string, value any, change ...func(*extension.Definition)) extension.Definition {
	definition := extension.Definition{
		Name:             name,
		Scope:            extension.ScopeOperator,
		FailurePolicy:    extension.FailClosed,
		EnabledByDefault: true,
		New:              func(json.RawMessage, extension.Dependencies) (any, error) { return value, nil },
	}
	for _, apply := range change {
		apply(&definition)
	}
	return definition
}

func skip(definition *extension.Definition) { definition.FailurePolicy = extension.FailSkip }

func tenant(definition *extension.Definition) { definition.Scope = extension.ScopeTenant }

// typesOf returns the public tenant types the tenant-scope definitions bind
// to: a shipped type for a definition of its name, and otherwise a type
// derived from the definition. A derived type with a schema gets the empty
// example, which its schema must accept.
func typesOf(definitions []extension.Definition) []tenanttype.Type {
	public := map[string]tenanttype.Type{}
	for _, tenantType := range tenanttype.All() {
		public[tenantType.Name] = tenantType
	}
	var types []tenanttype.Type
	for _, definition := range definitions {
		if shipped, ok := public[definition.Name]; ok {
			types = append(types, shipped)
			continue
		}
		if definition.Scope == extension.ScopeTenant {
			tenantType := tenanttype.Type{Name: definition.Name, ConfigSchema: definition.ConfigSchema, EnabledByDefault: definition.EnabledByDefault}
			if definition.ConfigSchema.JSON() != nil {
				tenantType.Example = json.RawMessage(`{}`)
			}
			types = append(types, tenantType)
		}
	}
	return types
}

type harness struct {
	host     *Host
	registry *prometheus.Registry
	logs     *lockedWriter
	model    *fakeModel
}

func newHarness(t *testing.T, definitions ...extension.Definition) *harness {
	t.Helper()
	return newHarnessWith(t, nil, definitions...)
}

// newHarnessWith is newHarness with adjust applied to the host configuration.
func newHarnessWith(t *testing.T, adjust func(*Config), definitions ...extension.Definition) *harness {
	t.Helper()
	require.NoError(t, ValidateDefinitions(definitions, nil, typesOf(definitions)))
	plan, err := NewPlan(definitions, nil, nil)
	require.NoError(t, err)
	registry := prometheus.NewRegistry()
	logs := &lockedWriter{buffer: &bytes.Buffer{}}
	config := Config{
		Plan:       plan,
		Logger:     slog.New(slog.NewJSONHandler(logs, nil)),
		Registerer: registry,
		Clock:      time.Now,
		RequestInfo: func(context.Context) extension.Call {
			return extension.Call{Namespace: "stacks-1", Caller: extension.CallerServiceAccount, CorrelationID: "correlation-1"}
		},
		DrainTimeout: 50 * time.Millisecond,
	}
	if adjust != nil {
		adjust(&config)
	}
	host, err := New(config)
	require.NoError(t, err)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		host.Close(ctx)
	})
	return &harness{host: host, registry: registry, logs: logs, model: &fakeModel{}}
}

// compose returns the pipeline for a selection of h.model from source.
func (h *harness) compose(t *testing.T, source extension.Source) provider.LanguageModel {
	t.Helper()
	model, err := h.host.Compose(execution.Selection{ID: "canonical", Model: h.model, Source: source})
	require.NoError(t, err)
	return model
}

// run invokes the pipeline through a runtime whose selector returns selected,
// as an adapter does: a unary call, or a stream whose overview is projected at
// its first part.
func (h *harness) run(t *testing.T, selected execution.Selection, modelID string, stream bool) testadapter.Outcome {
	t.Helper()
	return h.runContext(t, t.Context(), selected, modelID, stream)
}

// runContext is run under ctx.
func (h *harness) runContext(t *testing.T, ctx context.Context, selected execution.Selection, modelID string, stream bool) testadapter.Outcome {
	t.Helper()
	runtime, err := execution.New(execution.Config{
		Selector: func(context.Context, string, provider.CallOptions, json.RawMessage) (execution.Selection, error) {
			return selected, nil
		},
		Compose: h.host.Compose,
		Limits:  execution.Limits{StreamIdleDuration: time.Second, StreamDrainDuration: 50 * time.Millisecond},
	})
	require.NoError(t, err)
	return testadapter.New(runtime, nil).Run(ctx, modelID, provider.CallOptions{}, stream)
}

// configuredExpected returns the expected attempts of a configured model
// requested as alias, one per backend model.
func configuredExpected(models ...string) []execution.ExpectedAttempt {
	expected := make([]execution.ExpectedAttempt, len(models))
	for i, model := range models {
		expected[i] = execution.ExpectedAttempt{ModelID: "alias", CanonicalID: "canonical", Destination: catalog.ConfiguredCandidate{ProviderInstance: "instance-" + model, Provider: "native", ModelID: model}}
	}
	return expected
}

// flush waits for every dispatched observation.
func (h *harness) flush() { h.host.observations.close(context.Background()) }

func (h *harness) events(extensionName, phase, event string) float64 {
	return counterValue(h.registry, "grafana_ai_gateway_extension_events_total", map[string]string{"extension": extensionName, "phase": phase, "event": event})
}

func (h *harness) dropped() float64 {
	return counterValue(h.registry, "grafana_ai_gateway_extension_observations_dropped_total", nil)
}

// counterValue returns the value of the counter series with labels, or zero.
func counterValue(gatherer prometheus.Gatherer, name string, labels map[string]string) float64 {
	families, err := gatherer.Gather()
	if err != nil {
		return 0
	}
	for _, family := range families {
		if family.GetName() != name {
			continue
		}
	series:
		for _, metric := range family.GetMetric() {
			for _, label := range metric.GetLabel() {
				if want, ok := labels[label.GetName()]; ok && want != label.GetValue() {
					continue series
				}
			}
			return metric.GetCounter().GetValue()
		}
	}
	return 0
}

type lockedWriter struct {
	mu     sync.Mutex
	buffer *bytes.Buffer
}

func (w *lockedWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buffer.Write(p)
}

func (w *lockedWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buffer.String()
}

type fakeModel struct {
	mu       sync.Mutex
	calls    int
	received []provider.CallOptions
	generate func(context.Context) (*provider.GenerateResult, error)
	stream   func(context.Context) (*provider.StreamResult, error)
}

func (m *fakeModel) SpecificationVersion() string               { return "v4" }
func (m *fakeModel) Provider() string                           { return "grafana" }
func (m *fakeModel) ModelID() string                            { return "canonical" }
func (m *fakeModel) SupportedURLs() map[string][]*regexp.Regexp { return nil }

func (m *fakeModel) enter(options provider.CallOptions) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls++
	m.received = append(m.received, options)
}

func (m *fakeModel) callCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls
}

func (m *fakeModel) DoGenerate(ctx context.Context, options provider.CallOptions) (*provider.GenerateResult, error) {
	m.enter(options)
	if m.generate != nil {
		return m.generate(ctx)
	}
	return textResult("hello world"), nil
}

func (m *fakeModel) DoStream(ctx context.Context, options provider.CallOptions) (*provider.StreamResult, error) {
	m.enter(options)
	if m.stream != nil {
		return m.stream(ctx)
	}
	return &provider.StreamResult{Stream: streamOf(textParts("hello ", "world")...)}, nil
}

func textResult(text string) *provider.GenerateResult {
	input, output := 3, 5
	return &provider.GenerateResult{
		Content:      []provider.GenerateContentPart{{Type: provider.ContentText, Text: text}},
		FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop},
		Usage:        provider.Usage{InputTokens: provider.InputTokenUsage{Total: &input}, OutputTokens: provider.OutputTokenUsage{Total: &output}},
	}
}

func textParts(deltas ...string) []provider.StreamPart {
	input, output := 3, 5
	parts := []provider.StreamPart{{Type: provider.PartStreamStart}, {Type: provider.PartTextStart, ID: "t"}}
	for _, delta := range deltas {
		parts = append(parts, provider.StreamPart{Type: provider.PartTextDelta, ID: "t", Delta: delta})
	}
	return append(parts,
		provider.StreamPart{Type: provider.PartTextEnd, ID: "t"},
		provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop}, Usage: &provider.Usage{InputTokens: provider.InputTokenUsage{Total: &input}, OutputTokens: provider.OutputTokenUsage{Total: &output}}},
	)
}

func streamOf(parts ...provider.StreamPart) <-chan provider.StreamPart {
	stream := make(chan provider.StreamPart, len(parts))
	for _, part := range parts {
		stream <- part
	}
	close(stream)
	return stream
}

func collect(t *testing.T, stream <-chan provider.StreamPart) []provider.StreamPart {
	t.Helper()
	var parts []provider.StreamPart
	timeout := time.After(5 * time.Second)
	for {
		select {
		case part, ok := <-stream:
			if !ok {
				return parts
			}
			parts = append(parts, part)
		case <-timeout:
			require.FailNow(t, "stream did not close")
		}
	}
}

func partTypes(parts []provider.StreamPart) []provider.StreamPartType {
	types := make([]provider.StreamPartType, len(parts))
	for i, part := range parts {
		types[i] = part.Type
	}
	return types
}
