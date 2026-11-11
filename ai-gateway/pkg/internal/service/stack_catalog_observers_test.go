package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/grafana/agento11y/go/agento11y/testkit"
	"github.com/grafana/grafana-app-sdk/resource"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
	gatewayauth "github.com/grafana/ai-sdk/ai-gateway/pkg/internal/auth"
	"github.com/grafana/ai-sdk/ai-gateway/pkg/internal/modelcatalog/v1alpha1"
	"github.com/grafana/ai-sdk/fallback"
	"github.com/grafana/ai-sdk/provider"
)

const stackCatalogSecret = "dummy-stack-catalog-key"

// stackObservedCatalog is a stack catalog whose models all use one Anthropic
// provider, so a fake Anthropic transport can serve every call.
func stackObservedCatalog(name string, models ...string) StackCatalog {
	spec := v1alpha1.Spec{Providers: map[string]v1alpha1.Provider{"claude": {Type: "anthropic"}}, Models: map[string]v1alpha1.Model{}}
	for _, id := range models {
		spec.Models[id] = v1alpha1.Model{
			Name:     "Model",
			Aliases:  []string{id + "-alias"},
			Primary:  v1alpha1.Target{Provider: "claude", Model: "claude-primary"},
			Fallback: []v1alpha1.Target{{Provider: "claude", Model: "claude-backup"}},
		}
	}
	return StackCatalog{Name: name, Spec: spec, Keys: resource.DecryptedSecureValues{"claude": stackCatalogSecret}}
}

// stackAnthropicClient serves Anthropic responses. A request for the primary
// backend fails with a retryable error that echoes the catalog key when
// failPrimary is set.
func stackAnthropicClient(failPrimary bool) *http.Client {
	return &http.Client{Transport: selectionTransport(func(r *http.Request) (*http.Response, error) {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			return nil, err
		}
		if failPrimary && strings.Contains(string(raw), `"model":"claude-primary"`) {
			body := `{"type":"error","error":{"type":"overloaded_error","message":"rejected ` + stackCatalogSecret + `"}}`
			return &http.Response{StatusCode: http.StatusServiceUnavailable, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
		}
		contentType, body := "application/json", translationAnthropicBody
		if strings.Contains(string(raw), `"stream":true`) {
			contentType, body = "text/event-stream", seamAnthropicStream
		}
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {contentType}}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	})}
}

func stackSelector(stack StackCatalog, client *http.Client) execution.RequestSelector {
	return func(_ context.Context, id string, _ provider.CallOptions, gateway json.RawMessage) (execution.Selection, error) {
		return SelectStackCatalog(stack, id, gateway, StackCatalogOptions{Client: client, EgressClient: client})
	}
}

func TestStackCatalogObservation_IdentityAndPrivacy(t *testing.T) {
	for _, mode := range []string{"generate", "stream"} {
		t.Run(mode, func(t *testing.T) {
			observed := newObservedRuntime(t, stackSelector(stackObservedCatalog("team-catalog", "stack-model"), stackAnthropicClient(true)))
			call, err := observed.runtime.Call(callerContext(t, gatewayauth.ConfiguredAccounts), "stack-model-alias", provider.CallOptions{Prompt: []provider.Message{provider.UserText("observe me")}, MaxOutputTokens: new(32)})
			require.NoError(t, err)
			if mode == "generate" {
				_, err = call.Generate()
				require.NoError(t, err)
			} else {
				stream, err := call.Stream()
				require.NoError(t, err)
				for {
					_, wait := stream.Next()
					if wait != execution.WaitPart {
						require.Equal(t, execution.WaitFinished, wait)
						break
					}
				}
				stream.Close()
			}
			overview := publishedOverview(t, call.View(nil))
			call.Close()

			require.NotNil(t, overview)
			require.Len(t, overview.Attempts, 2)
			assert.Equal(t, fallback.AttemptFailed, overview.Attempts[0].Outcome)
			assert.Equal(t, "rejected [REDACTED]", overview.Attempts[0].Error.Message)
			assert.Equal(t, fallback.AttemptSelected, overview.Attempts[1].Outcome)

			require.EventuallyWithT(t, func(collect *assert.CollectT) {
				assert.Len(collect, observed.records(t, "aisdk.model."+mode+".finish"), 1)
			}, time.Second, time.Millisecond)
			starts := observed.records(t, "aisdk.model."+mode+".start")
			require.Len(t, starts, 1)
			assert.Equal(t, "grafana", starts[0]["ai_sdk.provider"])
			assert.Equal(t, "stack-model", starts[0]["ai_sdk.model"])
			assert.Equal(t, "team-catalog", starts[0]["catalog"])
			metrics := testMetrics(t, observed.telemetry)
			assert.Contains(t, metrics, `model="catalog"`)
			assert.NotContains(t, metrics, "stack-model")
			assert.NotContains(t, metrics, "team-catalog")

			observed.agent.Close(context.Background())
			generation := observed.env.SingleGenerationJSON(t)
			assert.Equal(t, "grafana", testkit.StringValue(t, generation, "model", "provider"))
			assert.Equal(t, "stack-model", testkit.StringValue(t, generation, "model", "name"))
			exported, err := json.Marshal(generation)
			require.NoError(t, err)
			assert.Contains(t, string(exported), `"gateway.catalog":"team-catalog"`)
			assert.NotContains(t, string(exported), "observe me")
			for _, private := range []string{stackCatalogSecret, "claude-primary", "claude-backup", "stack-model-alias"} {
				assert.NotContains(t, observed.logs.String()+metrics+string(exported), private)
			}
		})
	}
}

var modelRequestSeries = regexp.MustCompile(`(?m)^aisdk_model_requests_total\{`)

func TestStackCatalogObservation_BoundedSeries(t *testing.T) {
	series := func(t *testing.T, catalogs, models int) (int, string) {
		t.Helper()
		var stacks []StackCatalog
		for c := range catalogs {
			ids := make([]string, models)
			for m := range models {
				ids[m] = fmt.Sprintf("model-%d-%d", c, m)
			}
			stacks = append(stacks, stackObservedCatalog(fmt.Sprintf("catalog-%d", c), ids...))
		}
		client := stackAnthropicClient(false)
		observed := newObservedRuntime(t, func(ctx context.Context, id string, options provider.CallOptions, gateway json.RawMessage) (execution.Selection, error) {
			var c int
			_, err := fmt.Sscanf(id, "model-%d-", &c)
			require.NoError(t, err)
			return stackSelector(stacks[c], client)(ctx, id, options, gateway)
		})
		for c := range catalogs {
			for m := range models {
				observed.call(t, t.Context(), fmt.Sprintf("model-%d-%d", c, m), provider.CallOptions{}, false)
			}
		}
		metrics := testMetrics(t, observed.telemetry)
		return len(modelRequestSeries.FindAllString(metrics, -1)), metrics
	}
	one, _ := series(t, 1, 1)
	many, metrics := series(t, 20, 20)
	assert.Positive(t, one)
	assert.Equal(t, one, many)
	assert.Contains(t, metrics, `model="catalog"`)
	assert.NotContains(t, metrics, "model-1-1")
	assert.NotContains(t, metrics, "catalog-1")
}

func TestStackCatalogSelection_WritesNoPhysicalAttemptRecords(t *testing.T) {
	var output lockedBuffer
	telemetry, err := NewTelemetry(nil)
	require.NoError(t, err)
	sink := newPhysicalAttemptSink(hclogLogger(&output), telemetry, 8)
	failing := func() *observabilityTestModel {
		return &observabilityTestModel{generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
			return nil, hostileFallbackError(503)
		}}
	}
	succeeding := &observabilityTestModel{generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
		return &provider.GenerateResult{FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop}}, nil
	}}
	operator := routedFakeCatalog(t, ConfiguredIdentity, sink, map[string]*observabilityTestModel{"backend-primary": failing(), "backend-secondary": succeeding})
	stack := stackObservedCatalog("team-catalog", "stack-model")
	runtime, err := execution.New(execution.Config{Limits: serviceTestLimits(), Selector: func(ctx context.Context, id string, options provider.CallOptions, gateway json.RawMessage) (execution.Selection, error) {
		if id == "stack-model" {
			return SelectStackCatalog(stack, id, gateway, StackCatalogOptions{Client: stackAnthropicClient(true), EgressClient: http.DefaultClient})
		}
		return NewSelector(operator, nil, nil)(ctx, id, options, gateway)
	}})
	require.NoError(t, err)
	for _, tc := range []struct{ id, gateway string }{{id: "stack-model"}, {id: "public", gateway: `{"models":["other"]}`}} {
		options := routedGateway(`{}`)
		if tc.gateway != "" {
			options = routedGateway(tc.gateway)
		}
		options.MaxOutputTokens = new(32)
		call, err := runtime.Call(callerContext(t, gatewayauth.ConfiguredAccounts), tc.id, options)
		require.NoError(t, err)
		_, err = call.Generate()
		require.NoError(t, err)
		require.Len(t, call.View(nil).Overview.Attempts, 2, tc.id)
	}
	sink.Close(context.Background())

	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	require.Len(t, lines, 2, "only the operator-catalog call writes records")
	for _, line := range lines {
		var record physicalAttemptRecord
		require.NoError(t, json.Unmarshal([]byte(line), &record))
		assert.Contains(t, []string{"primary-instance", "secondary-instance"}, record.ProviderInstance)
	}
	assert.NotContains(t, output.String(), "claude")
}
