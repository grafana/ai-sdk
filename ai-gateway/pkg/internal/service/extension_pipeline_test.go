package service

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
	"github.com/grafana/ai-sdk/ai-gateway/pkg/extension"
	gatewayauth "github.com/grafana/ai-sdk/ai-gateway/pkg/internal/auth"
	"github.com/grafana/ai-sdk/ai-gateway/pkg/internal/extensionhost"
	"github.com/grafana/ai-sdk/provider"
)

// pipelineRecorder records the request information of every admitted call and
// every observed record.
type pipelineRecorder struct {
	deny     bool
	mu       sync.Mutex
	admitted []extension.Call
	observed []extension.Record
}

func (r *pipelineRecorder) Admit(_ context.Context, call *extension.Call) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.admitted = append(r.admitted, *call)
	if r.deny {
		return extension.ErrDenied
	}
	return nil
}

func (r *pipelineRecorder) Observe(_ context.Context, _ *extension.Call, record extension.Record) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.observed = append(r.observed, record)
	return nil
}

// newExtensionRuntime composes the production model chain: the logical
// observers outside the extension pipeline, outside the selected model.
func newExtensionRuntime(t *testing.T, selector execution.RequestSelector, value any) (*observedRuntime, *extensionhost.Host) {
	t.Helper()
	observed := newObservedRuntime(t, selector)
	definitions := []extension.Definition{{
		Name:             "recorder",
		Scope:            extension.ScopeOperator,
		FailurePolicy:    extension.FailClosed,
		EnabledByDefault: true,
		New:              func(json.RawMessage, extension.Dependencies) (any, error) { return value, nil },
	}}
	plan, err := extensionhost.NewPlan(definitions, nil, nil)
	require.NoError(t, err)
	host, err := extensionhost.New(extensionhost.Config{Plan: plan, Logger: slog.New(slog.NewJSONHandler(observed.logs, nil)), Registerer: observed.telemetry.Registerer(), Clock: time.Now, RequestInfo: ExtensionRequest, DrainTimeout: 10 * time.Millisecond})
	require.NoError(t, err)
	runtime, err := execution.New(execution.Config{Selector: selector, Limits: serviceTestLimits(), Compose: func(selected execution.Selection) (provider.LanguageModel, error) {
		model, err := host.Compose(selected)
		if err != nil {
			return nil, err
		}
		selected.Model = model
		return observed.observers.Compose(selected)
	}})
	require.NoError(t, err)
	observed.runtime = runtime
	return observed, host
}

func TestExtensionPipeline_EverySelectionGetsThePipelineOnce(t *testing.T) {
	const byokKey = "dummy-pipeline-key"
	for _, tc := range []struct {
		name    string
		access  gatewayauth.AccountAccess
		modelID string
		gateway string
		fail    bool
		source  extension.Source
		model   string
		caller  extension.CallerType
	}{
		{name: "operator catalog", access: gatewayauth.ConfiguredAccounts, modelID: "public", source: extension.SourceOperatorCatalog, model: "public", caller: extension.CallerInternalService},
		{name: "alias", access: gatewayauth.ConfiguredAccounts, modelID: "alias", source: extension.SourceOperatorCatalog, model: "public", caller: extension.CallerInternalService},
		{name: "routed", access: gatewayauth.ConfiguredAccounts, modelID: "alias", gateway: `{"models":["other"]}`, fail: true, source: extension.SourceOperatorCatalog, model: "public", caller: extension.CallerInternalService},
		{name: "BYOK", access: gatewayauth.RequestBYOK, modelID: "anthropic/native-model", gateway: `{"byok":{"anthropic":[{"apiKey":"` + byokKey + `"}]}}`, source: extension.SourceRequestBYOK, model: "anthropic/native-model", caller: extension.CallerUser},
	} {
		for _, mode := range []string{"generate", "stream"} {
			t.Run(tc.name+"/"+mode, func(t *testing.T) {
				created := routedFakeCatalog(t, ConfiguredIdentity, nil, map[string]*observabilityTestModel{
					"backend-primary":   seamModel(tc.fail),
					"backend-secondary": seamModel(false),
				})
				recorder := &pipelineRecorder{}
				observed, host := newExtensionRuntime(t, NewSelector(created, &BYOKOptions{Client: seamBYOKClient(), Factory: BYOKIdentity}), recorder)
				var options provider.CallOptions
				if tc.gateway != "" {
					options = routedGateway(tc.gateway)
				}
				observed.call(t, callerContext(t, tc.access), tc.modelID, options, mode == "stream")
				host.Close(context.Background())

				require.Len(t, recorder.admitted, 1)
				admitted := recorder.admitted[0]
				assert.Equal(t, tc.source, admitted.Source)
				assert.Equal(t, tc.source.Keys(), admitted.Keys)
				assert.Equal(t, tc.model, admitted.Model)
				assert.Equal(t, testNamespace, admitted.Namespace)
				assert.Equal(t, tc.caller, admitted.Caller)
				require.Len(t, recorder.observed, 1)
				assert.Equal(t, extension.OutcomeSuccess, recorder.observed[0].Outcome)
				require.EventuallyWithT(t, func(collect *assert.CollectT) {
					assert.Len(collect, observed.records(t, "aisdk.model."+mode+".finish"), 1)
				}, time.Second, time.Millisecond)
				assert.NotContains(t, observed.logs.String(), byokKey)
			})
		}
	}
}

func TestExtensionPipeline_DenialIsADistinctObservedOutcome(t *testing.T) {
	for _, mode := range []string{"generate", "stream"} {
		t.Run(mode, func(t *testing.T) {
			var calls int
			lower := &observabilityTestModel{
				generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
					calls++
					return &provider.GenerateResult{}, nil
				},
				stream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
					calls++
					return &provider.StreamResult{}, nil
				},
			}
			selector := func(_ context.Context, id string, _ provider.CallOptions, _ json.RawMessage) (execution.Selection, error) {
				model, err := ConfiguredIdentity(id, lower)
				if err != nil {
					return execution.Selection{}, err
				}
				return execution.ConfiguredSelection(id, model, nil), nil
			}
			recorder := &pipelineRecorder{deny: true}
			observed, host := newExtensionRuntime(t, selector, recorder)
			call, err := observed.runtime.Call(callerContext(t, gatewayauth.ConfiguredAccounts), "public", provider.CallOptions{Prompt: []provider.Message{provider.UserText("observe me")}})
			require.NoError(t, err)
			if mode == "stream" {
				_, err = call.Stream()
			} else {
				_, err = call.Generate()
			}
			call.Close()
			assert.Equal(t, execution.CategoryPermission, execution.ClassifyProvider(err))
			assert.Zero(t, calls)
			host.Close(context.Background())
			require.Len(t, recorder.observed, 1)
			assert.Equal(t, extension.OutcomeDenied, recorder.observed[0].Outcome)

			require.EventuallyWithT(t, func(collect *assert.CollectT) {
				assert.Len(collect, observed.records(t, "aisdk.model."+mode+".error"), 1)
			}, time.Second, time.Millisecond)
			record := observed.records(t, "aisdk.model."+mode+".error")[0]
			assert.Equal(t, "denied", record["ai_sdk.outcome"])
			assert.Equal(t, "denied", record["ai_sdk.error.type"])
			assert.Equal(t, "INFO", record["level"])
			metrics := testMetrics(t, observed.telemetry)
			assert.Contains(t, metrics, `status="denied"`)
			assert.NotContains(t, metrics, `status="error"`)
			assert.Contains(t, metrics, `grafana_ai_gateway_extension_events_total{event="denied",extension="recorder",phase="admit"} 1`)
		})
	}
}

func TestExtensionRequest(t *testing.T) {
	telemetry, err := NewTelemetry(slog.New(slog.NewJSONHandler(&lockedBuffer{}, nil)))
	require.NoError(t, err)
	telemetry.generateCorrelationID = func() string { return "correlation-1" }
	for _, tc := range []struct {
		access gatewayauth.AccountAccess
		caller extension.CallerType
	}{
		{access: gatewayauth.ConfiguredAccounts, caller: extension.CallerInternalService},
		{access: gatewayauth.RequestBYOK, caller: extension.CallerUser},
	} {
		t.Run(string(tc.access), func(t *testing.T) {
			var got extension.Call
			handler, _ := withAccess(t, gatewayauth.AccessModeCloud, tc.access, http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				got = ExtensionRequest(r.Context())
			}))
			telemetry.Middleware(fixedRoute("language_model"), handler).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/", nil))
			assert.Equal(t, extension.Call{Namespace: testNamespace, Caller: tc.caller, CorrelationID: "correlation-1"}, got)
		})
	}
	assert.Equal(t, extension.Call{}, ExtensionRequest(context.Background()))
}

func TestExtensionPipeline_UnobservedSourceRunsNoPhase(t *testing.T) {
	var calls int
	lower := &observabilityTestModel{generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
		calls++
		return &provider.GenerateResult{}, nil
	}}
	for _, tc := range []struct {
		name   string
		source extension.Source
	}{{name: "empty", source: ""}, {name: "stack-catalog", source: extension.SourceStackCatalog}} {
		source := tc.source
		t.Run(tc.name, func(t *testing.T) {
			recorder := &pipelineRecorder{}
			observed, host := newExtensionRuntime(t, func(_ context.Context, id string, _ provider.CallOptions, _ json.RawMessage) (execution.Selection, error) {
				return execution.Selection{ID: id, Model: lower, Source: source}, nil
			}, recorder)
			_, err := observed.runtime.Call(callerContext(t, gatewayauth.ConfiguredAccounts), "public", provider.CallOptions{Prompt: []provider.Message{provider.UserText("observe me")}})
			category, _ := execution.ClassifySelection(err)
			assert.Equal(t, execution.CategoryInternal, category)
			host.Close(context.Background())
			assert.Empty(t, recorder.admitted)
			assert.Empty(t, recorder.observed)
			assert.Zero(t, calls)
		})
	}
}
