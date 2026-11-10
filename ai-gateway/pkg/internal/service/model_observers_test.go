package service

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/grafana/agento11y/go/agento11y"
	"github.com/grafana/agento11y/go/agento11y/testkit"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
	gatewayauth "github.com/grafana/ai-sdk/ai-gateway/pkg/internal/auth"
	"github.com/grafana/ai-sdk/provider"
)

const seamAnthropicStream = "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_1\",\"type\":\"message\",\"role\":\"assistant\",\"model\":\"backend-private\",\"content\":[],\"usage\":{\"input_tokens\":1,\"output_tokens\":0}}}\n\nevent: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":1}}\n\nevent: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"

type observedRuntime struct {
	runtime   *execution.Runtime
	logs      *lockedBuffer
	telemetry *Telemetry
	agent     *AgentObservabilityRuntime
	observers *ModelObservers
	env       *testkit.Env
}

func newObservedRuntime(t *testing.T, selector execution.RequestSelector) *observedRuntime {
	t.Helper()
	env := testkit.NewEnv(t, func(c *agento11y.Config) {
		c.ContentCapture = agento11y.ContentCaptureModeMetadataOnly
		c.Hooks = agento11y.HooksConfig{Enabled: false}
	})
	logs := &lockedBuffer{}
	logger := slog.New(slog.NewJSONHandler(logs, nil))
	telemetry, err := NewTelemetry(logger)
	require.NoError(t, err)
	agent := &AgentObservabilityRuntime{client: env.Client, telemetry: telemetry}
	observers, err := NewModelObservers(telemetry, logger, agent, 10*time.Millisecond)
	require.NoError(t, err)
	runtime, err := execution.New(execution.Config{Selector: selector, Compose: observers.Compose, Limits: serviceTestLimits()})
	require.NoError(t, err)
	return &observedRuntime{runtime: runtime, logs: logs, telemetry: telemetry, agent: agent, observers: observers, env: env}
}

func (o *observedRuntime) call(t *testing.T, ctx context.Context, modelID string, options provider.CallOptions, streaming bool) {
	t.Helper()
	options.Prompt = []provider.Message{provider.UserText("observe me")}
	options.MaxOutputTokens = new(32)
	call, err := o.runtime.Call(ctx, modelID, options)
	require.NoError(t, err)
	defer call.Close()
	if !streaming {
		_, err = call.Generate()
		require.NoError(t, err)
		return
	}
	stream, err := call.Stream()
	require.NoError(t, err)
	defer stream.Close()
	for {
		_, wait := stream.Next()
		if wait != execution.WaitPart {
			require.Equal(t, execution.WaitFinished, wait)
			return
		}
	}
}

func (o *observedRuntime) records(t *testing.T, event string) []map[string]any {
	t.Helper()
	var records []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(o.logs.String()), "\n") {
		if line == "" {
			continue
		}
		var record map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &record))
		if record["ai_sdk.event"] == event {
			records = append(records, record)
		}
	}
	return records
}

func seamModel(fail bool) *observabilityTestModel {
	if fail {
		return &observabilityTestModel{
			generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
				return nil, hostileFallbackError(503)
			},
			stream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
				return nil, hostileFallbackError(503)
			},
		}
	}
	return &observabilityTestModel{
		generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
			return &provider.GenerateResult{
				Content:      []provider.GenerateContentPart{{Type: provider.ContentText, Text: "answer"}},
				FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop},
			}, nil
		},
		stream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
			return fallbackParts(
				provider.StreamPart{Type: provider.PartStreamStart},
				provider.StreamPart{Type: provider.PartTextStart, ID: "text"},
				provider.StreamPart{Type: provider.PartTextDelta, ID: "text", Delta: "answer"},
				provider.StreamPart{Type: provider.PartTextEnd, ID: "text"},
				provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop}, Usage: &provider.Usage{}},
			), nil
		},
	}
}

func seamBYOKClient() *http.Client {
	return &http.Client{Transport: selectionTransport(func(r *http.Request) (*http.Response, error) {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			return nil, err
		}
		contentType, body := "application/json", translationAnthropicBody
		if strings.Contains(string(raw), `"stream":true`) {
			contentType, body = "text/event-stream", seamAnthropicStream
		}
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {contentType}}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	})}
}

func TestModelObservers_IdenticalIdentitiesAcrossSelections(t *testing.T) {
	const byokKey = "dummy-seam-key"
	for _, tc := range []struct {
		name          string
		access        gatewayauth.AccountAccess
		modelID       string
		gateway       string
		failPrimary   bool
		provider      string
		model         string
		metricModel   string
		forbiddenText []string
	}{
		{name: "operator catalog", access: gatewayauth.ConfiguredAccounts, modelID: "public", provider: "grafana", model: "public", metricModel: "public"},
		{name: "operator catalog alias", access: gatewayauth.ConfiguredAccounts, modelID: "alias", provider: "grafana", model: "public", metricModel: "public"},
		{name: "routed", access: gatewayauth.ConfiguredAccounts, modelID: "alias", gateway: `{"models":["other"]}`, failPrimary: true, provider: "grafana", model: "public", metricModel: "public"},
		{name: "BYOK", access: gatewayauth.RequestBYOK, modelID: "anthropic/native-model", gateway: `{"byok":{"anthropic":[{"apiKey":"` + byokKey + `"}]}}`, provider: "anthropic", model: "anthropic/native-model", metricModel: "byok", forbiddenText: []string{byokKey}},
	} {
		for _, mode := range []string{"generate", "stream"} {
			t.Run(tc.name+"/"+mode, func(t *testing.T) {
				created := routedFakeCatalog(t, ConfiguredIdentity, nil, map[string]*observabilityTestModel{
					"backend-primary":   seamModel(tc.failPrimary),
					"backend-secondary": seamModel(false),
				})
				observed := newObservedRuntime(t, NewSelector(created, &BYOKOptions{Client: seamBYOKClient(), Factory: BYOKIdentity}))
				var options provider.CallOptions
				if tc.gateway != "" {
					options = routedGateway(tc.gateway)
				}
				observed.call(t, callerContext(t, tc.access), tc.modelID, options, mode == "stream")

				require.EventuallyWithT(t, func(collect *assert.CollectT) {
					assert.Len(collect, observed.records(t, "aisdk.model."+mode+".finish"), 1)
				}, time.Second, time.Millisecond)
				starts := observed.records(t, "aisdk.model."+mode+".start")
				require.Len(t, starts, 1)
				assert.Equal(t, tc.provider, starts[0]["ai_sdk.provider"])
				assert.Equal(t, tc.model, starts[0]["ai_sdk.model"])
				metrics := testMetrics(t, observed.telemetry)
				assert.Contains(t, metrics, `model="`+tc.metricModel+`"`)
				observed.agent.Close(context.Background())
				generation := observed.env.SingleGenerationJSON(t)
				assert.Equal(t, tc.provider, testkit.StringValue(t, generation, "model", "provider"))
				assert.Equal(t, tc.model, testkit.StringValue(t, generation, "model", "name"))
				exported, err := json.Marshal(generation)
				require.NoError(t, err)
				for _, private := range append([]string{"backend-primary", "backend-secondary", "backend-private"}, tc.forbiddenText...) {
					assert.NotContains(t, observed.logs.String()+metrics+string(exported), private)
				}
			})
		}
	}
}

type requestKeyContextKey struct{}

func TestModelObservers_PerRequestSelection(t *testing.T) {
	var mu sync.Mutex
	var built, invoked []string
	selector := func(ctx context.Context, id string, _ provider.CallOptions, _ json.RawMessage) (execution.Selection, error) {
		key, _ := ctx.Value(requestKeyContextKey{}).(string)
		mu.Lock()
		built = append(built, key)
		mu.Unlock()
		lower := &observabilityTestModel{generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
			mu.Lock()
			invoked = append(invoked, key)
			mu.Unlock()
			return &provider.GenerateResult{
				FinishReason:     provider.FinishReason{Unified: provider.FinishReasonStop},
				Response:         &provider.GenerateResponse{ResponseMetadata: provider.ResponseMetadata{ID: key, ModelID: key}},
				ProviderMetadata: provider.ProviderMetadata{"native": json.RawMessage(`{"key":"` + key + `"}`)},
			}, nil
		}}
		model, err := ConfiguredIdentity(id, lower)
		if err != nil {
			return execution.Selection{}, err
		}
		return execution.ConfiguredSelection(id, model, nil), nil
	}
	observed := newObservedRuntime(t, selector)
	keys := []string{"dummy-request-key-1", "dummy-request-key-2"}
	for _, key := range keys {
		observed.call(t, context.WithValue(t.Context(), requestKeyContextKey{}, key), "stack-model", provider.CallOptions{}, false)
	}

	assert.Equal(t, keys, built)
	assert.Equal(t, keys, invoked)
	require.EventuallyWithT(t, func(collect *assert.CollectT) {
		assert.Len(collect, observed.records(t, "aisdk.model.generate.finish"), 2)
	}, time.Second, time.Millisecond)
	starts := observed.records(t, "aisdk.model.generate.start")
	require.Len(t, starts, 2)
	for _, start := range starts {
		assert.Equal(t, "grafana", start["ai_sdk.provider"])
		assert.Equal(t, "stack-model", start["ai_sdk.model"])
	}
	metrics := testMetrics(t, observed.telemetry)
	assert.Contains(t, metrics, `model="stack-model"`)
	for _, key := range keys {
		assert.NotContains(t, observed.logs.String()+metrics, key)
	}
	observed.agent.Close(context.Background())
	assert.Equal(t, len(keys), observed.env.RequestCount())
}

func TestModelObservers_UnknownSourceFailsBeforeTheModel(t *testing.T) {
	for _, source := range []execution.Source{"", "stack-catalog"} {
		t.Run(string(source), func(t *testing.T) {
			var calls int
			lower := &observabilityTestModel{generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
				calls++
				return &provider.GenerateResult{}, nil
			}}
			observed := newObservedRuntime(t, func(_ context.Context, id string, _ provider.CallOptions, _ json.RawMessage) (execution.Selection, error) {
				return execution.Selection{ID: id, Model: lower, Source: source}, nil
			})
			_, err := observed.runtime.Call(t.Context(), "public", provider.CallOptions{Prompt: []provider.Message{provider.UserText("observe me")}})
			require.Error(t, err)
			category, _ := execution.ClassifySelection(err)
			assert.Equal(t, execution.CategoryInternal, category)
			assert.Zero(t, calls)
			assert.Empty(t, observed.logs.String())
		})
	}
}
