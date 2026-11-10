package service

import (
	"context"
	"encoding/json"
	"log/slog"
	"slices"
	"strings"
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
	return newExtensionRuntimeWith(t, selector, recorderDefinition("recorder", extension.ScopeOperator, value))
}

// recorderDefinition is a default-enabled, fail-closed definition of value.
func recorderDefinition(name string, scope extension.Scope, value any) extension.Definition {
	return extension.Definition{
		Name:             name,
		Scope:            scope,
		FailurePolicy:    extension.FailClosed,
		EnabledByDefault: true,
		New:              func(json.RawMessage, extension.Dependencies) (any, error) { return value, nil },
	}
}

// newExtensionRuntimeWith is newExtensionRuntime with the given definitions.
func newExtensionRuntimeWith(t *testing.T, selector execution.RequestSelector, definitions ...extension.Definition) (*observedRuntime, *extensionhost.Host) {
	t.Helper()
	observed := newObservedRuntime(t, selector)
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

var (
	secondaryCandidate = extension.Candidate{Model: "other", CanonicalModel: "other", Provider: "anthropic", ProviderInstance: "secondary-instance", ProviderModel: "backend-secondary"}
	byokCandidate      = extension.Candidate{Model: "anthropic/native-model", Provider: "anthropic", ProviderModel: "native-model"}
	failedPrimary      = extension.Attempt{Provider: "anthropic", ProviderInstance: "primary-instance", ProviderModel: "backend-primary", Outcome: extension.AttemptFailed, StatusCode: 503}
)

// primaryCandidate is the primary candidate of the configured model public,
// requested as modelID.
func primaryCandidate(modelID string) extension.Candidate {
	return extension.Candidate{Model: modelID, CanonicalModel: "public", Provider: "anthropic", ProviderInstance: "primary-instance", ProviderModel: "backend-primary"}
}

// selectedAttempt is the selected attempt of candidate.
func selectedAttempt(candidate extension.Candidate) extension.Attempt {
	return extension.Attempt{Provider: candidate.Provider, ProviderInstance: candidate.ProviderInstance, ProviderModel: candidate.ProviderModel, Outcome: extension.AttemptSelected}
}

func TestExtensionPipeline_EverySelectionGetsThePipelineOnce(t *testing.T) {
	const byokKey = "dummy-pipeline-key"
	for _, tc := range []struct {
		name       string
		access     gatewayauth.AccountAccess
		modelID    string
		gateway    string
		fail       bool
		source     extension.Source
		canonical  string
		caller     extension.CallerType
		identity   string
		candidates []extension.Candidate
		attempts   []extension.Attempt
	}{
		{name: "operator catalog", access: gatewayauth.ConfiguredAccounts, modelID: "public", source: extension.SourceOperatorCatalog, canonical: "public", caller: extension.CallerInternalService,
			candidates: []extension.Candidate{primaryCandidate("public")}, attempts: []extension.Attempt{selectedAttempt(primaryCandidate("public"))}},
		{name: "alias", access: gatewayauth.ConfiguredAccounts, modelID: "alias", source: extension.SourceOperatorCatalog, canonical: "public", caller: extension.CallerInternalService,
			candidates: []extension.Candidate{primaryCandidate("alias")}, attempts: []extension.Attempt{selectedAttempt(primaryCandidate("alias"))}},
		{name: "routed", access: gatewayauth.ConfiguredAccounts, modelID: "alias", gateway: `{"models":["other"]}`, fail: true, source: extension.SourceOperatorCatalog, canonical: "public", caller: extension.CallerInternalService,
			candidates: []extension.Candidate{primaryCandidate("alias"), secondaryCandidate},
			attempts:   []extension.Attempt{failedPrimary, selectedAttempt(secondaryCandidate)}},
		{name: "BYOK", access: gatewayauth.RequestBYOK, modelID: "anthropic/native-model", gateway: `{"byok":{"anthropic":[{"apiKey":"` + byokKey + `"}]}}`, source: extension.SourceRequestBYOK, caller: extension.CallerUser, identity: "user:u1",
			candidates: []extension.Candidate{byokCandidate}, attempts: []extension.Attempt{selectedAttempt(byokCandidate)}},
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
				assert.Equal(t, tc.modelID, admitted.RequestedModel)
				assert.Equal(t, tc.canonical, admitted.CanonicalModel)
				assert.Equal(t, tc.identity, admitted.CallerIdentity)
				assert.Equal(t, tc.candidates, admitted.Candidates)
				assert.Equal(t, testNamespace, admitted.Namespace)
				assert.Equal(t, tc.caller, admitted.Caller)
				require.Len(t, recorder.observed, 1)
				assert.Equal(t, extension.OutcomeSuccess, recorder.observed[0].Outcome)
				assert.Equal(t, tc.attempts, recorder.observed[0].Attempts)
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
		name     string
		mode     gatewayauth.AccessMode
		service  string
		identity string
		caller   extension.CallerType
	}{
		{name: "internal service", mode: gatewayauth.AccessModeCloud, service: testInternalService, caller: extension.CallerInternalService},
		{name: "internal service for a user", mode: gatewayauth.AccessModeCloud, service: testInternalService, identity: "user:u1", caller: extension.CallerInternalService},
		{name: "user", mode: gatewayauth.AccessModeCloud, service: "front-door", identity: "user:u1", caller: extension.CallerUser},
		{name: "service account", mode: gatewayauth.AccessModeCloud, identity: "service-account:sa1", caller: extension.CallerServiceAccount},
		{name: "self-hosted user", mode: gatewayauth.AccessModeSelfHosted, identity: "user:u2", caller: extension.CallerUser},
		{name: "self-hosted service account", mode: gatewayauth.AccessModeSelfHosted, identity: "service-account:sa2", caller: extension.CallerServiceAccount},
		{name: "identity beyond the log bound", mode: gatewayauth.AccessModeCloud, identity: "user:" + strings.Repeat("u", maxObservationValueLen), caller: extension.CallerUser},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := ExtensionRequest(observedCallerContext(t, telemetry, tc.mode, tc.service, tc.identity))
			assert.Equal(t, extension.Call{Namespace: testNamespace, Caller: tc.caller, CallerIdentity: tc.identity, CorrelationID: "correlation-1"}, got)
		})
	}
	assert.Equal(t, extension.Call{}, ExtensionRequest(context.Background()))
}

func (r *pipelineRecorder) records() []extension.Record {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.observed)
}

func TestExtensionPipeline_StreamRecordAfterTheOverviewSealed(t *testing.T) {
	parts := make(chan provider.StreamPart, 1)
	parts <- provider.StreamPart{Type: provider.PartStreamStart}
	held := &observabilityTestModel{stream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
		return &provider.StreamResult{Stream: parts}, nil
	}}
	created := routedFakeCatalog(t, ConfiguredIdentity, nil, map[string]*observabilityTestModel{
		"backend-primary":   seamModel(true),
		"backend-secondary": held,
	})
	recorder := &pipelineRecorder{}
	observed, host := newExtensionRuntime(t, NewSelector(created, nil), recorder)
	call, err := observed.runtime.Call(callerContext(t, gatewayauth.ConfiguredAccounts), "alias", routedGateway(`{"models":["other"]}`))
	require.NoError(t, err)
	defer call.Close()
	stream, err := call.Stream()
	require.NoError(t, err)
	defer stream.Close()

	_, wait := stream.Next()
	require.Equal(t, execution.WaitPart, wait)
	view := call.View(wait.Err())
	require.NotNil(t, view.Overview)
	require.Len(t, view.Overview.Attempts, 2)
	assert.Empty(t, recorder.records(), "the provider still holds the stream open")

	for _, part := range []provider.StreamPart{
		{Type: provider.PartTextStart, ID: "text"},
		{Type: provider.PartTextDelta, ID: "text", Delta: "answer"},
		{Type: provider.PartTextEnd, ID: "text"},
		{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop}, Usage: &provider.Usage{}},
	} {
		parts <- part
		_, wait = stream.Next()
		require.Equal(t, execution.WaitPart, wait)
	}
	close(parts)
	_, wait = stream.Next()
	require.Equal(t, execution.WaitFinished, wait)
	host.Close(context.Background())

	records := recorder.records()
	require.Len(t, records, 1)
	assert.Equal(t, []extension.Attempt{
		failedPrimary,
		selectedAttempt(secondaryCandidate),
	}, records[0].Attempts)
}

func TestExtensionPipeline_OperatorAndTenantExtensionsSeeTheCallerIdentity(t *testing.T) {
	for _, tc := range []struct {
		name     string
		service  string
		identity string
		modelID  string
		options  provider.CallOptions
	}{
		{name: "internal service for a user", service: testInternalService, identity: "user:audit-uid", modelID: "public"},
		{name: "BYOK service account", identity: "service-account:audit-uid", modelID: "anthropic/native-model", options: routedGateway(`{"byok":{"anthropic":[{"apiKey":"dummy-identity-key"}]}}`)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			created := routedFakeCatalog(t, ConfiguredIdentity, nil, map[string]*observabilityTestModel{"backend-primary": seamModel(false), "backend-secondary": seamModel(false)})
			operator, tenant := &pipelineRecorder{}, &pipelineRecorder{}
			observed, host := newExtensionRuntimeWith(t, NewSelector(created, &BYOKOptions{Client: seamBYOKClient(), Factory: BYOKIdentity}),
				recorderDefinition("operator-audit", extension.ScopeOperator, operator),
				recorderDefinition("tenant-audit", extension.ScopeTenant, tenant),
			)
			observed.call(t, observedCallerContext(t, observed.telemetry, gatewayauth.AccessModeCloud, tc.service, tc.identity), tc.modelID, tc.options, false)
			host.Close(context.Background())
			for _, recorder := range []*pipelineRecorder{operator, tenant} {
				require.Len(t, recorder.admitted, 1)
				assert.Equal(t, tc.identity, recorder.admitted[0].CallerIdentity)
			}
		})
	}
}
