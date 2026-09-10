package agentobservability

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/grafana/agento11y/go/agento11y"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildGenerationStart_ContextKeysWin(t *testing.T) {
	ctx := context.Background()
	ctx = WithGenerationID(ctx, "gen-abc")
	ctx = WithParentGenerationIDs(ctx, "p1", "p2")

	start := BuildGenerationStart(ctx, "anthropic", "claude-3-5-sonnet", ContextInfo{
		UserID:       "user-1",
		AgentName:    "agent",
		AgentVersion: "v1",
		Metadata:     map[string]any{"tenant": "acme"},
		Tags:         map[string]string{"env": "prod"},
	})

	assert.Equal(t, "gen-abc", start.ID)
	assert.Equal(t, []string{"p1", "p2"}, start.ParentGenerationIDs)
	assert.Equal(t, "anthropic", start.Model.Provider)
	assert.Equal(t, "claude-3-5-sonnet", start.Model.Name)
	assert.Equal(t, "user-1", start.UserID)
	assert.Equal(t, "agent", start.AgentName)
	assert.Equal(t, "v1", start.AgentVersion)
	assert.Equal(t, map[string]any{"tenant": "acme"}, start.Metadata)
	assert.Equal(t, map[string]string{"env": "prod"}, start.Tags)
}

func TestBuildGenerationStart_FallsBackToAgento11yContext(t *testing.T) {
	ctx := context.Background()
	ctx = agento11y.WithUserID(ctx, "user-from-ctx")
	ctx = agento11y.WithAgentName(ctx, "agent-from-ctx")
	ctx = agento11y.WithAgentVersion(ctx, "v2")

	// ContextInfo zero -> fall back to agento11y context keys.
	start := BuildGenerationStart(ctx, "anthropic", "claude", ContextInfo{})

	assert.Equal(t, "user-from-ctx", start.UserID)
	assert.Equal(t, "agent-from-ctx", start.AgentName)
	assert.Equal(t, "v2", start.AgentVersion)
}

func TestBuildGenerationStart_ContextInfoWinsOverAgento11yContext(t *testing.T) {
	ctx := context.Background()
	ctx = agento11y.WithUserID(ctx, "user-from-ctx")

	start := BuildGenerationStart(ctx, "anthropic", "claude", ContextInfo{
		UserID: "explicit-user",
	})
	assert.Equal(t, "explicit-user", start.UserID)
}

func TestBuildGenerationStart_DefensiveCopy(t *testing.T) {
	meta := map[string]any{"a": 1}
	tags := map[string]string{"x": "y"}
	start := BuildGenerationStart(context.Background(), "p", "m", ContextInfo{
		Metadata: meta,
		Tags:     tags,
	})
	meta["a"] = 999
	tags["x"] = "mutated"
	assert.Equal(t, 1, start.Metadata["a"], "metadata is defensively cloned")
	assert.Equal(t, "y", start.Tags["x"], "tags are defensively cloned")
}

func TestMapGenerateResult_CanonicalRoundTrip(t *testing.T) {
	maxTok := 512
	temp := 0.7
	topP := 0.95
	params := provider.CallOptions{
		Prompt: []provider.Message{
			provider.NewSystemMessage("you are concise"),
			provider.UserText("hi"),
		},
		Tools: []provider.Tool{
			{
				Type:        provider.ToolTypeFunction,
				Name:        "get_time",
				Description: "Get the current time",
				InputSchema: json.RawMessage(`{"type":"object"}`),
			},
		},
		MaxOutputTokens: &maxTok,
		Temperature:     &temp,
		TopP:            &topP,
		ToolChoice:      &provider.ToolChoice{Type: provider.ToolChoiceAuto},
		ProviderOptions: provider.ProviderOptions{
			"anthropic": provider.RawProviderOption{
				Key: "anthropic",
				Raw: json.RawMessage(`{"thinking":{"type":"enabled","budgetTokens":1024}}`),
			},
		},
	}
	result := &provider.GenerateResult{
		Content: []provider.GenerateContentPart{
			{Type: provider.ContentText, Text: "hello!"},
		},
		FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop},
		Usage: provider.Usage{
			InputTokens:  provider.InputTokenUsage{Total: intPtr(10)},
			OutputTokens: provider.OutputTokenUsage{Total: intPtr(20)},
		},
		Response: &provider.GenerateResponse{
			ResponseMetadata: provider.ResponseMetadata{
				ID:      "resp-1",
				ModelID: "claude-3-5-sonnet-20241022",
			},
		},
	}
	ctxInfo := ContextInfo{
		UserID:       "user-1",
		AgentName:    "lodestone",
		AgentVersion: "v3",
		Metadata:     map[string]any{"tenant_id": "acme"},
		Tags:         map[string]string{"env": "prod"},
	}

	gen := MapGenerateResult(params, result, ctxInfo)

	// Identity / context-derived fields.
	assert.Equal(t, "user-1", gen.UserID)
	assert.Equal(t, "lodestone", gen.AgentName)
	assert.Equal(t, "v3", gen.AgentVersion)

	// System prompt folding.
	assert.Equal(t, "you are concise", gen.SystemPrompt)
	require.Len(t, gen.Input, 1)
	assert.Equal(t, agento11y.RoleUser, gen.Input[0].Role)

	// Tools + controls.
	require.Len(t, gen.Tools, 1)
	assert.Equal(t, "get_time", gen.Tools[0].Name)
	require.NotNil(t, gen.MaxTokens)
	assert.Equal(t, int64(512), *gen.MaxTokens)
	require.NotNil(t, gen.Temperature)
	assert.InDelta(t, 0.7, *gen.Temperature, 1e-9)
	require.NotNil(t, gen.TopP)
	assert.InDelta(t, 0.95, *gen.TopP, 1e-9)
	require.NotNil(t, gen.ToolChoice)
	assert.Equal(t, "auto", *gen.ToolChoice)
	require.NotNil(t, gen.ThinkingEnabled)
	assert.True(t, *gen.ThinkingEnabled)

	// Output + usage + stop reason.
	require.Len(t, gen.Output, 1)
	assert.Equal(t, agento11y.RoleAssistant, gen.Output[0].Role)
	assert.Equal(t, int64(10), gen.Usage.InputTokens)
	assert.Equal(t, int64(20), gen.Usage.OutputTokens)
	assert.Equal(t, "end_turn", gen.StopReason)

	// Metadata merging: anthropic thinking budget + caller metadata.
	assert.Equal(t, int64(1024), gen.Metadata[MetadataThinkingBudgetTokens])
	assert.Equal(t, "acme", gen.Metadata["tenant_id"])

	// Response metadata.
	assert.Equal(t, "resp-1", gen.ResponseID)
	assert.Equal(t, "claude-3-5-sonnet-20241022", gen.ResponseModel)

	// Recorder-set fields must NOT be populated by the mapper.
	assert.Empty(t, gen.ID, "ID is set by recorder, not mapper")
	assert.True(t, gen.StartedAt.IsZero(), "StartedAt is set by recorder")
	assert.True(t, gen.CompletedAt.IsZero(), "CompletedAt is set by recorder")
	assert.Empty(t, gen.TraceID, "TraceID is set by recorder")
	assert.Empty(t, gen.SpanID, "SpanID is set by recorder")
}

func TestMapGenerateResult_ServerToolUsageMetadata(t *testing.T) {
	result := &provider.GenerateResult{Usage: provider.Usage{
		Raw: json.RawMessage(`{"server_tool_use":{"web_search_requests":2,"web_fetch_requests":1}}`),
	}}
	gen := MapGenerateResult(provider.CallOptions{}, result, ContextInfo{Metadata: map[string]any{
		MetadataServerToolUseTotalRequests: "caller override",
	}})

	assert.Equal(t, int64(2), gen.Metadata[MetadataServerToolUseWebSearchRequests])
	assert.Equal(t, int64(1), gen.Metadata[MetadataServerToolUseWebFetchRequests])
	assert.Equal(t, int64(3), gen.Metadata[MetadataServerToolUseTotalRequests])
}

func TestMapGenerateResult_ResponseTransportMetadataOverridesCallerValues(t *testing.T) {
	gen := mapGenerateResultWithStart(
		provider.CallOptions{},
		&provider.GenerateResult{Response: &provider.GenerateResponse{ResponseMetadata: provider.ResponseMetadata{
			Provider: "anthropic", ModelID: "response-model",
		}}},
		ContextInfo{Metadata: map[string]any{
			transportProviderMetadataKey: "spoofed-provider",
			transportModelMetadataKey:    "spoofed-model",
		}},
		agento11y.GenerationStart{Model: agento11y.ModelRef{Provider: "grafana", Name: "requested-model"}},
	)

	assert.Equal(t, "grafana", gen.Metadata[transportProviderMetadataKey])
	assert.Equal(t, "requested-model", gen.Metadata[transportModelMetadataKey])
}

func TestMapGenerateResult_ResponseModelFallsBackToRequestModel(t *testing.T) {
	gen := mapGenerateResultWithStart(
		provider.CallOptions{},
		&provider.GenerateResult{},
		ContextInfo{},
		agento11y.GenerationStart{Model: agento11y.ModelRef{Name: "request-model"}},
	)
	assert.Equal(t, "request-model", gen.ResponseModel)
}

func TestMapGenerateResult_NilResult(t *testing.T) {
	gen := MapGenerateResult(provider.CallOptions{
		Prompt: []provider.Message{provider.UserText("hi")},
	}, nil, ContextInfo{})
	require.Len(t, gen.Input, 1)
	assert.Empty(t, gen.Output, "nil result -> no output")
	assert.Empty(t, gen.StopReason)
}

func intPtr(v int) *int { return &v }
