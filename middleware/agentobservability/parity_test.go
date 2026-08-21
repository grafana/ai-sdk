package agentobservability

import (
	"encoding/json"
	"sort"
	"testing"

	asdk "github.com/anthropics/anthropic-sdk-go"
	agento11yanthropic "github.com/grafana/agento11y/go-providers/anthropic"
	"github.com/grafana/agento11y/go/agento11y"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestParity_PlainText verifies that the ai-sdk mapper produces the same
// agento11y.Generation as the upstream agento11y Anthropic helper for a
// canonical plain-text request/response. The byte-equal target is the JSON
// serialization of the Generation struct modulo recorder-set fields
// (id, trace_id, span_id, started_at, completed_at) and known-divergent
// metadata derived from artifacts the ai-sdk mapper doesn't generate.
func TestParity_PlainText(t *testing.T) {
	const modelName = "claude-sonnet-4-5"
	const userInput = "what is the weather?"
	const assistantText = "It is sunny."

	// Anthropic-SDK form of the same logical request.
	asdkReq := asdk.BetaMessageNewParams{
		Model:     asdk.Model(modelName),
		MaxTokens: 1024,
		Messages: []asdk.BetaMessageParam{
			{
				Role:    asdk.BetaMessageParamRoleUser,
				Content: []asdk.BetaContentBlockParamUnion{{OfText: &asdk.BetaTextBlockParam{Text: userInput}}},
			},
		},
	}
	asdkResp := &asdk.BetaMessage{
		ID:         "msg_parity_text",
		Model:      asdk.Model(modelName),
		StopReason: asdk.BetaStopReasonEndTurn,
		Content: []asdk.BetaContentBlockUnion{
			{Type: "text", Text: assistantText},
		},
		Usage: asdk.BetaUsage{
			InputTokens:  10,
			OutputTokens: 20,
		},
	}
	want, err := agento11yanthropic.FromRequestResponse(asdkReq, asdkResp,
		agento11yanthropic.WithAgentName("agent"),
		agento11yanthropic.WithAgentVersion("v1"),
	)
	require.NoError(t, err)

	// Equivalent ai-sdk form of the same logical request/response.
	aisdkParams := provider.CallOptions{
		Prompt:          []provider.Message{provider.UserText(userInput)},
		MaxOutputTokens: observabilityIntegerPointer(1024),
	}
	aisdkResult := &provider.GenerateResult{
		Content: []provider.GenerateContentPart{
			{Type: provider.ContentText, Text: assistantText},
		},
		FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop, Raw: "end_turn"},
		Usage: provider.Usage{
			InputTokens:  provider.InputTokenUsage{Total: intPtr(10)},
			OutputTokens: provider.OutputTokenUsage{Total: intPtr(20)},
		},
		Response: &provider.GenerateResponse{
			ResponseMetadata: provider.ResponseMetadata{
				ID:      "msg_parity_text",
				ModelID: modelName,
			},
		},
	}
	got := MapGenerateResult(aisdkParams, aisdkResult, ContextInfo{
		AgentName:    "agent",
		AgentVersion: "v1",
	})
	// Stamp the provider name explicitly because the upstream mapper sets
	// Model.Provider from WithProvider() / the SDK default ("anthropic"),
	// while our mapper takes it from the LanguageModel — when MapGenerateResult
	// is called directly (not through middleware) the provider is not
	// populated, so we set it here for the comparison.
	got.Model.Provider = "anthropic"
	got.Model.Name = modelName

	assertGenerationsEquivalent(t, want, got)
}

// TestParity_ToolUse verifies parity on a request with a tool definition
// plus an assistant tool-call response.
func TestParity_ToolUse(t *testing.T) {
	const modelName = "claude-sonnet-4-5"
	const userInput = "what's the weather in SF?"
	const assistantText = "Looking up SF weather…"
	toolSchema := json.RawMessage(`{"type":"object","properties":{"city":{"type":"string"}}}`)

	asdkReq := asdk.BetaMessageNewParams{
		Model:     asdk.Model(modelName),
		MaxTokens: 1024,
		Messages: []asdk.BetaMessageParam{
			{
				Role:    asdk.BetaMessageParamRoleUser,
				Content: []asdk.BetaContentBlockParamUnion{{OfText: &asdk.BetaTextBlockParam{Text: userInput}}},
			},
		},
		Tools: []asdk.BetaToolUnionParam{
			{
				OfTool: &asdk.BetaToolParam{
					Name:        "get_weather",
					Description: asdk.String("Get the weather"),
					InputSchema: asdk.BetaToolInputSchemaParam{
						Properties: map[string]any{
							"city": map[string]any{"type": "string"},
						},
					},
				},
			},
		},
	}
	asdkResp := &asdk.BetaMessage{
		ID:         "msg_parity_tool",
		Model:      asdk.Model(modelName),
		StopReason: asdk.BetaStopReasonToolUse,
		Content: []asdk.BetaContentBlockUnion{
			{Type: "text", Text: assistantText},
			mustParseToolUseBlock(t, "tu_1", "get_weather", `{"city":"SF"}`),
		},
		Usage: asdk.BetaUsage{InputTokens: 15, OutputTokens: 5},
	}
	want, err := agento11yanthropic.FromRequestResponse(asdkReq, asdkResp,
		agento11yanthropic.WithAgentName("agent"),
		agento11yanthropic.WithAgentVersion("v1"),
	)
	require.NoError(t, err)

	aisdkParams := provider.CallOptions{
		Prompt:          []provider.Message{provider.UserText(userInput)},
		MaxOutputTokens: observabilityIntegerPointer(1024),
		Tools: []provider.Tool{{
			Type:        provider.ToolTypeFunction,
			Name:        "get_weather",
			Description: observabilityStringPointer("Get the weather"),
			InputSchema: toolSchema,
		}},
	}
	aisdkResult := &provider.GenerateResult{
		Content: []provider.GenerateContentPart{
			{Type: provider.ContentText, Text: assistantText},
			{
				Type:       provider.ContentToolCall,
				ToolCallID: "tu_1",
				ToolName:   "get_weather",
				Input:      json.RawMessage(`{"city":"SF"}`),
			},
		},
		FinishReason: provider.FinishReason{Unified: provider.FinishReasonToolCalls, Raw: "tool_use"},
		Usage: provider.Usage{
			InputTokens:  provider.InputTokenUsage{Total: intPtr(15)},
			OutputTokens: provider.OutputTokenUsage{Total: intPtr(5)},
		},
		Response: &provider.GenerateResponse{
			ResponseMetadata: provider.ResponseMetadata{ID: "msg_parity_tool", ModelID: modelName},
		},
	}
	got := MapGenerateResult(aisdkParams, aisdkResult, ContextInfo{AgentName: "agent", AgentVersion: "v1"})
	got.Model.Provider = "anthropic"
	got.Model.Name = modelName

	assertGenerationsEquivalent(t, want, got)
}

// assertGenerationsEquivalent compares two agento11y.Generation values modulo
// recorder-set fields and known-divergent fields (artifacts, which we
// intentionally do not produce). The diff prints both sides as indented JSON
// for easier debugging.
func assertGenerationsEquivalent(t *testing.T, want, got agento11y.Generation) {
	t.Helper()

	clean := func(g agento11y.Generation) agento11y.Generation {
		g.ID = ""
		g.TraceID = ""
		g.SpanID = ""
		g.StartedAt = g.StartedAt.UTC()
		g.CompletedAt = g.CompletedAt.UTC()
		// Drop fields neither side fills consistently when invoked outside a
		// recorder context.
		g.ConversationID = ""
		g.ConversationTitle = ""
		g.Mode = ""
		g.OperationName = ""
		// Drop Artifacts: ai-sdk path doesn't generate them; upstream only
		// adds them under WithRawArtifacts().
		g.Artifacts = nil
		return g
	}

	wantClean := clean(want)
	gotClean := clean(got)

	wantJSON, err := json.MarshalIndent(wantClean, "", "  ")
	require.NoError(t, err)
	gotJSON, err := json.MarshalIndent(gotClean, "", "  ")
	require.NoError(t, err)

	if string(wantJSON) != string(gotJSON) {
		t.Errorf("generation parity mismatch.\n--- upstream (agento11y/go-providers/anthropic) ---\n%s\n--- ai-sdk middleware ---\n%s",
			wantJSON, gotJSON)
	}

	// Also assert field-by-field for nicer failure messages on common drift.
	assert.Equal(t, wantClean.Model, gotClean.Model, "model")
	assert.Equal(t, wantClean.SystemPrompt, gotClean.SystemPrompt, "system_prompt")
	assert.Equal(t, wantClean.StopReason, gotClean.StopReason, "stop_reason")
	assert.Equal(t, wantClean.ResponseID, gotClean.ResponseID, "response_id")
	assert.Equal(t, wantClean.ResponseModel, gotClean.ResponseModel, "response_model")
	assert.Equal(t, wantClean.Usage, gotClean.Usage, "usage")
	assertMessagesEqual(t, wantClean.Input, gotClean.Input, "input")
	assertMessagesEqual(t, wantClean.Output, gotClean.Output, "output")
}

func assertMessagesEqual(t *testing.T, want, got []agento11y.Message, label string) {
	t.Helper()
	require.Equal(t, len(want), len(got), "%s: message count", label)
	for i := range want {
		assert.Equal(t, want[i].Role, got[i].Role, "%s[%d].role", label, i)
		require.Equal(t, len(want[i].Parts), len(got[i].Parts), "%s[%d].parts count", label, i)
		for j := range want[i].Parts {
			wp := want[i].Parts[j]
			gp := got[i].Parts[j]
			assert.Equal(t, wp.Kind, gp.Kind, "%s[%d].parts[%d].kind", label, i, j)
			assert.Equal(t, wp.Text, gp.Text, "%s[%d].parts[%d].text", label, i, j)
			assert.Equal(t, wp.Thinking, gp.Thinking, "%s[%d].parts[%d].thinking", label, i, j)
			assert.Equal(t, wp.Metadata.ProviderType, gp.Metadata.ProviderType, "%s[%d].parts[%d].metadata.provider_type", label, i, j)
			if wp.ToolCall != nil || gp.ToolCall != nil {
				require.NotNil(t, wp.ToolCall, "%s[%d].parts[%d].tool_call upstream", label, i, j)
				require.NotNil(t, gp.ToolCall, "%s[%d].parts[%d].tool_call ai-sdk", label, i, j)
				assert.Equal(t, wp.ToolCall.ID, gp.ToolCall.ID, "%s[%d].parts[%d].tool_call.id", label, i, j)
				assert.Equal(t, wp.ToolCall.Name, gp.ToolCall.Name, "%s[%d].parts[%d].tool_call.name", label, i, j)
				assertSortedJSONEqual(t, wp.ToolCall.InputJSON, gp.ToolCall.InputJSON,
					"%s[%d].parts[%d].tool_call.input_json", label, i, j)
			}
		}
	}
}

// assertSortedJSONEqual compares two json.RawMessages after sorting object
// keys, so the comparison is insensitive to key-order differences between
// the producers.
func assertSortedJSONEqual(t *testing.T, want, got json.RawMessage, format string, args ...any) {
	t.Helper()
	if len(want) == 0 && len(got) == 0 {
		return
	}
	wantSorted := sortJSON(t, want)
	gotSorted := sortJSON(t, got)
	assert.JSONEqf(t, string(wantSorted), string(gotSorted), format, args...)
}

func sortJSON(t *testing.T, in json.RawMessage) []byte {
	t.Helper()
	var v any
	if err := json.Unmarshal(in, &v); err != nil {
		t.Fatalf("unmarshal for sort: %v\n%s", err, in)
	}
	v = sortAny(v)
	out, err := json.Marshal(v)
	require.NoError(t, err)
	return out
}

func sortAny(v any) any {
	switch typed := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for k := range typed {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		out := make(map[string]any, len(typed))
		for _, k := range keys {
			out[k] = sortAny(typed[k])
		}
		return out
	case []any:
		for i := range typed {
			typed[i] = sortAny(typed[i])
		}
		return typed
	default:
		return v
	}
}

// mustParseToolUseBlock builds a tool_use BetaContentBlockUnion via JSON
// because the Anthropic SDK's discriminated unions don't expose direct
// constructors for every block type.
func mustParseToolUseBlock(t *testing.T, id, name, inputJSON string) asdk.BetaContentBlockUnion {
	t.Helper()
	rawJSON := `{"type":"tool_use","id":"` + id + `","name":"` + name + `","input":` + inputJSON + `}`
	var block asdk.BetaContentBlockUnion
	require.NoError(t, json.Unmarshal([]byte(rawJSON), &block))
	return block
}
