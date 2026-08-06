package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/grafana/ai-sdk/gateway/catalog"
	"github.com/grafana/ai-sdk/gateway/failure"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type catalogResolverFunc func(context.Context, string) (catalog.ResolvedModel, error)

func (f catalogResolverFunc) ResolveModel(ctx context.Context, modelID string) (catalog.ResolvedModel, error) {
	return f(ctx, modelID)
}

func TestGatewayOptions_RegisteredFieldsAndExtensions(t *testing.T) {
	caching := GatewayCachingAuto
	disallowTraining := true
	quotaEntityID := "tenant-a"
	serviceTier := GatewayServiceTierPriority
	sort := GatewaySortTTFT
	user := "user-a"
	zeroRetention := false
	options := GatewayOptions{
		BYOK: map[string][]map[string]json.RawMessage{
			"anthropic": {{"apiKey": json.RawMessage(`"secret"`)}},
		},
		Caching:                &caching,
		DisallowPromptTraining: &disallowTraining,
		Has:                    []GatewayCapability{GatewayCapabilityImplicitCaching},
		Models:                 []string{"fallback"},
		Only:                   []string{"anthropic"},
		Order:                  []string{"anthropic", "openai"},
		ProviderTimeouts:       &GatewayProviderTimeouts{BYOK: map[string]float64{"anthropic": 500}},
		QuotaEntityID:          &quotaEntityID,
		ServiceTier:            &serviceTier,
		Sort:                   &sort,
		Tags:                   []string{"production"},
		User:                   &user,
		ZeroDataRetention:      &zeroRetention,
		Extensions:             map[string]json.RawMessage{"future": json.RawMessage(`{"enabled":true}`)},
	}

	assert.False(t, options.Empty())
	cloned := cloneGatewayOptions(options)
	options.BYOK["anthropic"][0]["apiKey"][1] = 'X'
	options.ProviderTimeouts.BYOK["anthropic"] = 1
	options.Extensions["future"][2] = 'X'
	assert.JSONEq(t, `"secret"`, string(cloned.BYOK["anthropic"][0]["apiKey"]))
	assert.Equal(t, float64(500), cloned.ProviderTimeouts.BYOK["anthropic"])
	assert.JSONEq(t, `{"enabled":true}`, string(cloned.Extensions["future"]))
	assert.True(t, (GatewayOptions{}).Empty())
}

func TestApplyPolicies_TransformsProviderInputInOrder(t *testing.T) {
	call := validTestCall()
	call.CallOptions.Headers = map[string]string{"Authorization": "caller", "X-Trace": "trace"}
	call.GatewayOptions.Extensions = map[string]json.RawMessage{"future": json.RawMessage(`{"mode":"fast"}`)}
	var order []string

	first := CallPolicyFunc(func(_ context.Context, actual GatewayCall) (GatewayCall, error) {
		order = append(order, "first")
		assert.Equal(t, "caller", actual.CallOptions.Headers["Authorization"])
		delete(actual.CallOptions.Headers, "Authorization")
		actual.PolicyMetadata = PolicyMetadata{"decision": json.RawMessage(`"sanitized"`)}
		return actual, nil
	})
	second := CallPolicyFunc(func(_ context.Context, actual GatewayCall) (GatewayCall, error) {
		order = append(order, "second")
		assert.NotContains(t, actual.CallOptions.Headers, "Authorization")
		assert.JSONEq(t, `"sanitized"`, string(actual.PolicyMetadata["decision"]))
		assert.JSONEq(t, `{"mode":"fast"}`, string(actual.GatewayOptions.Extensions["future"]))
		return actual, nil
	})

	got, err := applyPolicies(context.Background(), call, []CallPolicy{first, second})
	require.NoError(t, err)
	assert.Equal(t, []string{"first", "second"}, order)
	assert.NotContains(t, got.CallOptions.Headers, "Authorization")
	assert.Contains(t, call.CallOptions.Headers, "Authorization")
}

func TestApplyPolicies_RejectsProviderBoundInputBeforeResolution(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*GatewayCall)
	}{
		{name: "authorization header", mutate: func(call *GatewayCall) { call.CallOptions.Headers = map[string]string{"Authorization": "caller"} }},
		{name: "provider option", mutate: func(call *GatewayCall) {
			call.CallOptions.ProviderOptions = provider.ProviderOptions{"unsafe": provider.RawProviderOption{Key: "unsafe", Raw: json.RawMessage(`true`)}}
		}},
		{name: "raw chunks", mutate: func(call *GatewayCall) { call.CallOptions.IncludeRawChunks = true }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			call := validTestCall()
			tc.mutate(&call)
			policyCause := errors.New("provider-bound input is prohibited")
			policy := CallPolicyFunc(func(_ context.Context, actual GatewayCall) (GatewayCall, error) {
				return GatewayCall{}, failure.Wrap(failure.ErrForbidden, policyCause)
			})

			_, err := applyPolicies(context.Background(), call, []CallPolicy{policy})
			assert.ErrorIs(t, err, failure.ErrForbidden)
			assert.ErrorIs(t, err, policyCause)
		})
	}
}

func TestApplyPolicies_TrustedMetadataAndIdentityAreImmutable(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*GatewayCall)
	}{
		{name: "protocol", mutate: func(call *GatewayCall) { call.Protocol = "other" }},
		{name: "requested model", mutate: func(call *GatewayCall) { call.RequestedModelID = "other" }},
		{name: "request ID", mutate: func(call *GatewayCall) { call.CallMetadata.RequestID = "other" }},
		{name: "authenticated attributes", mutate: func(call *GatewayCall) { call.CallMetadata.AuthenticatedAttributes["tenant"] = "other" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			policy := CallPolicyFunc(func(_ context.Context, call GatewayCall) (GatewayCall, error) {
				tc.mutate(&call)
				return call, nil
			})
			_, err := applyPolicies(context.Background(), validTestCall(), []CallPolicy{policy})
			assert.ErrorIs(t, err, failure.ErrInternal)
		})
	}
}

func TestGatewayCall_DefensiveCopies(t *testing.T) {
	call := validTestCall()
	call.CallOptions.Headers = map[string]string{"X-Trace": "original"}
	call.PolicyMetadata = PolicyMetadata{"decision": json.RawMessage(`"original"`)}
	cloned := cloneGatewayCall(call)

	call.CallMetadata.AuthenticatedAttributes["tenant"] = "mutated"
	call.CallOptions.Headers["X-Trace"] = "mutated"
	call.PolicyMetadata["decision"][1] = 'X'

	assert.Equal(t, "tenant-a", cloned.CallMetadata.AuthenticatedAttributes["tenant"])
	assert.Equal(t, "original", cloned.CallOptions.Headers["X-Trace"])
	assert.JSONEq(t, `"original"`, string(cloned.PolicyMetadata["decision"]))
}

func TestApplyPolicies_ScalarPointerMutationsAreIsolated(t *testing.T) {
	maxTokens := 10
	strict := true
	caching := GatewayCachingAuto
	zeroRetention := false
	call := validTestCall()
	call.CallOptions.MaxOutputTokens = &maxTokens
	call.CallOptions.Tools = []provider.Tool{{Type: provider.ToolTypeFunction, Name: "tool", Strict: &strict}}
	call.GatewayOptions.Caching = &caching
	call.GatewayOptions.ZeroDataRetention = &zeroRetention

	policy := CallPolicyFunc(func(_ context.Context, actual GatewayCall) (GatewayCall, error) {
		*actual.CallOptions.MaxOutputTokens = 20
		*actual.CallOptions.Tools[0].Strict = false
		*actual.GatewayOptions.Caching = GatewayCachingAuto
		*actual.GatewayOptions.ZeroDataRetention = true
		return actual, nil
	})
	got, err := applyPolicies(context.Background(), call, []CallPolicy{policy})
	require.NoError(t, err)

	assert.Equal(t, 10, maxTokens)
	assert.True(t, strict)
	assert.Equal(t, GatewayCachingAuto, caching)
	assert.False(t, zeroRetention)
	assert.Equal(t, 20, *got.CallOptions.MaxOutputTokens)
	assert.False(t, *got.CallOptions.Tools[0].Strict)
	assert.True(t, *got.GatewayOptions.ZeroDataRetention)
}

func TestValidateCall_RequiresIdentityAndExtractedGatewayOptions(t *testing.T) {
	gatewayOptions := func() provider.ProviderOptions {
		return provider.ProviderOptions{"gateway": provider.RawProviderOption{Key: "gateway", Raw: json.RawMessage(`{}`)}}
	}
	cases := []struct {
		name   string
		mutate func(*GatewayCall)
	}{
		{name: "protocol", mutate: func(call *GatewayCall) { call.Protocol = "" }},
		{name: "model ID", mutate: func(call *GatewayCall) { call.RequestedModelID = "" }},
		{name: "request ID", mutate: func(call *GatewayCall) { call.CallMetadata.RequestID = "" }},
		{name: "top-level gateway namespace", mutate: func(call *GatewayCall) { call.CallOptions.ProviderOptions = gatewayOptions() }},
		{name: "message gateway namespace", mutate: func(call *GatewayCall) {
			call.CallOptions.Prompt = []provider.Message{{Role: provider.RoleUser, ProviderOptions: gatewayOptions()}}
		}},
		{name: "content gateway namespace", mutate: func(call *GatewayCall) {
			call.CallOptions.Prompt = []provider.Message{provider.NewUserMessage(provider.ContentPart{Type: provider.ContentPartTypeText, Text: "text", ProviderOptions: gatewayOptions()})}
		}},
		{name: "function tool gateway namespace", mutate: func(call *GatewayCall) {
			call.CallOptions.Tools = []provider.Tool{{Type: provider.ToolTypeFunction, Name: "tool", ProviderOptions: gatewayOptions()}}
		}},
		{name: "tool output gateway namespace", mutate: func(call *GatewayCall) {
			call.CallOptions.Prompt = []provider.Message{provider.NewToolMessage(provider.ToolResultPart("call", "tool", &provider.ToolResultOutput{Type: provider.ToolOutputText, Text: "ok", ProviderOptions: gatewayOptions()}))}
		}},
		{name: "tool result content gateway namespace", mutate: func(call *GatewayCall) {
			call.CallOptions.Prompt = []provider.Message{provider.NewToolMessage(provider.ToolResultPart("call", "tool", &provider.ToolResultOutput{Type: provider.ToolOutputContent, Content: []provider.ToolResultContentValue{{Type: provider.ToolContentText, Text: "ok", ProviderOptions: gatewayOptions()}}}))}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			call := validTestCall()
			tc.mutate(&call)
			assert.ErrorIs(t, validateCall(call), failure.ErrInvalidCall)
		})
	}
}

func TestGenerate_RejectsNestedGatewayProviderOptionsBeforePolicyAndResolution(t *testing.T) {
	gatewayOptions := provider.ProviderOptions{"gateway": provider.RawProviderOption{Key: "gateway", Raw: json.RawMessage(`{}`)}}
	call := validTestCall()
	call.CallOptions.Prompt = []provider.Message{provider.NewToolMessage(provider.ToolResultPart("call", "tool", &provider.ToolResultOutput{
		Type: provider.ToolOutputContent,
		Content: []provider.ToolResultContentValue{{
			Type: provider.ToolContentText, Text: "ok", ProviderOptions: gatewayOptions,
		}},
	}))}
	policyCalls := 0
	resolverCalls := 0
	gatewayRuntime, err := New(
		ModelResolverFunc(func(context.Context, GatewayCall) (catalog.ResolvedModel, error) {
			resolverCalls++
			return catalog.ResolvedModel{}, nil
		}),
		WithCallPolicies(CallPolicyFunc(func(_ context.Context, call GatewayCall) (GatewayCall, error) {
			policyCalls++
			return call, nil
		})),
	)
	require.NoError(t, err)

	outcome := gatewayRuntime.Generate(context.Background(), call)
	require.NotNil(t, outcome.Failure)
	assert.Equal(t, failure.KindInvalidCall, outcome.Failure.Kind)
	assert.Zero(t, policyCalls)
	assert.Zero(t, resolverCalls)
}

func TestAdaptCatalogResolver(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		resolver, err := AdaptCatalogResolver(nil)
		require.Error(t, err)
		assert.Nil(t, resolver)
	})

	t.Run("resolves with context and requested ID", func(t *testing.T) {
		type contextKey struct{}
		ctx := context.WithValue(context.Background(), contextKey{}, "tenant")
		var gotContext context.Context
		var gotModelID string
		resolver, err := AdaptCatalogResolver(catalogResolverFunc(func(actual context.Context, modelID string) (catalog.ResolvedModel, error) {
			gotContext = actual
			gotModelID = modelID
			return catalog.ResolvedModel{ID: "canonical"}, nil
		}))
		require.NoError(t, err)
		resolved, err := resolver.ResolveModel(ctx, validTestCall())
		require.NoError(t, err)
		assert.Equal(t, "canonical", resolved.ID)
		assert.Equal(t, "model", gotModelID)
		assert.Equal(t, "tenant", gotContext.Value(contextKey{}))
	})

	t.Run("unknown model is categorized", func(t *testing.T) {
		cause := &catalog.UnknownModelError{ModelID: "model"}
		resolver, err := AdaptCatalogResolver(catalogResolverFunc(func(context.Context, string) (catalog.ResolvedModel, error) {
			return catalog.ResolvedModel{}, cause
		}))
		require.NoError(t, err)
		_, err = resolver.ResolveModel(context.Background(), validTestCall())
		assert.ErrorIs(t, err, failure.ErrUnknownModel)
		assert.ErrorIs(t, err, cause)
	})

	t.Run("gateway controls fail closed", func(t *testing.T) {
		calls := 0
		resolver, err := AdaptCatalogResolver(catalogResolverFunc(func(context.Context, string) (catalog.ResolvedModel, error) {
			calls++
			return catalog.ResolvedModel{}, nil
		}))
		require.NoError(t, err)
		call := validTestCall()
		call.GatewayOptions.Models = []string{"fallback"}
		_, err = resolver.ResolveModel(context.Background(), call)
		assert.ErrorIs(t, err, failure.ErrInvalidCall)
		assert.Zero(t, calls)
	})
}

func validTestCall() GatewayCall {
	return GatewayCall{
		Protocol:         ProtocolLanguageModelV4,
		RequestedModelID: "model",
		CallMetadata: CallMetadata{
			RequestID:               "request-1",
			AuthenticatedAttributes: map[string]string{"tenant": "tenant-a"},
		},
	}
}
