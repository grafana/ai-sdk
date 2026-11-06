package chatcompletions

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const twoToolDeclarations = `,"tools":[{"type":"function","function":{"name":"allowed","parameters":{"type":"object"}}},{"type":"function","function":{"name":"forbidden","parameters":{"type":"object"}}}]`
const allowedToolChoice = `,"tool_choice":{"type":"allowed_tools","allowed_tools":{"mode":"required","tools":[{"type":"function","function":{"name":"allowed"}}]}}`

func TestAllowedTools_NativeRequests(t *testing.T) {
	r := requestWith(t, twoToolDeclarations+allowedToolChoice)
	for _, target := range []execution.Target{{Type: execution.TargetOpenAI}, {Type: execution.TargetOpenAICompatible, ProviderName: "vendor.chat"}, {Type: execution.TargetAnthropic}} {
		options, err := chatTranslator(r)(target, r.options)
		require.NoError(t, err)
		for _, streaming := range []bool{false, true} {
			body := captureMediaRequest(t, target, options, streaming)
			var declarations []any
			require.NoError(t, json.Unmarshal(body["tools"], &declarations))
			switch target.Type {
			case execution.TargetOpenAICompatible:
				assert.Len(t, declarations, 2)
				assert.JSONEq(t, `{"type":"allowed_tools","allowed_tools":{"mode":"required","tools":[{"type":"function","function":{"name":"allowed"}}]}}`, string(body["tool_choice"]))
			case execution.TargetOpenAI:
				assert.Len(t, declarations, 2)
				assert.JSONEq(t, `{"type":"allowed_tools","mode":"required","tools":[{"type":"function","name":"allowed"}]}`, string(body["tool_choice"]))
			case execution.TargetAnthropic:
				assert.Len(t, declarations, 1)
				assert.Contains(t, string(body["tools"]), `"allowed"`)
				assert.NotContains(t, string(body["tools"]), `"forbidden"`)
				assert.JSONEq(t, `{"type":"any"}`, string(body["tool_choice"]))
			}
			assert.NotContains(t, body, "allowedTools")
		}
	}
	assert.Len(t, r.options.Tools, 2)
	changed := r.options
	changed.Tools = nil
	_, err := chatTranslator(r)(execution.Target{Type: execution.TargetOpenAI}, changed)
	require.ErrorIs(t, err, provider.ErrUnsupportedFunctionality)
}

func TestAllowedTools_RequestValidation(t *testing.T) {
	for _, refs := range []string{
		`[{"type":"function","function":{"name":"missing"}}]`,
		`[{"type":"custom","custom":{"name":"allowed"}}]`,
		`[{"type":"function","function":{"name":"allowed"}},{"type":"function","function":{"name":"allowed"}}]`,
		`[]`,
	} {
		_, err := mapRequest([]byte(strings.TrimSuffix(basic, "}") + twoToolDeclarations + `,"tool_choice":{"type":"allowed_tools","allowed_tools":{"mode":"required","tools":` + refs + `}}}`))
		require.Error(t, err, refs)
	}
	empty := requestWith(t, twoToolDeclarations+`,"tool_choice":{"type":"allowed_tools","allowed_tools":{"mode":"auto","tools":[]}}`)
	assert.Equal(t, provider.ToolChoiceNone, empty.options.ToolChoice.Type)
	assert.False(t, empty.toolAllowed("allowed"))
	custom := requestWith(t, `,"tools":[{"type":"custom","custom":{"name":"freeform"}}],"tool_choice":{"type":"allowed_tools","allowed_tools":{"mode":"auto","tools":[{"type":"custom","custom":{"name":"freeform"}}]}}`)
	assert.True(t, custom.toolAllowed("freeform"))
	assert.True(t, custom.customTool("freeform"))
}

func TestAllowedTools_ForbiddenOutputBeforePublicToolDelta(t *testing.T) {
	for _, streaming := range []bool{false, true} {
		f := &fakeModel{generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
			result := textResult()
			result.Content = []provider.GenerateContentPart{{Type: provider.ContentToolCall, ToolCallID: "hidden-id", ToolName: "forbidden", Input: json.RawMessage(`{}`)}}
			result.FinishReason.Unified = provider.FinishReasonToolCalls
			return result, nil
		}, stream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
			return partsStream(provider.StreamPart{Type: provider.PartStreamStart}, provider.StreamPart{Type: provider.PartToolInputStart, ID: "hidden-id", ToolName: "forbidden"}, provider.StreamPart{Type: provider.PartToolInputDelta, ID: "hidden-id", Delta: `{"secret":"hidden-argument"}`}), nil
		}}
		body := strings.TrimSuffix(basic, "}") + twoToolDeclarations + allowedToolChoice
		if streaming {
			body += `,"stream":true`
		}
		body += "}"
		req := httptest.NewRequest("POST", Path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		testHandler(t, f).ServeHTTP(w, req)
		assert.Contains(t, w.Body.String(), `"error"`)
		assert.NotContains(t, w.Body.String(), "forbidden")
		assert.NotContains(t, w.Body.String(), "hidden-id")
		assert.NotContains(t, w.Body.String(), "hidden-argument")
		assert.NotContains(t, w.Body.String(), "[DONE]")
	}
}

func TestAllowedTools_CustomNativeAndHistory(t *testing.T) {
	custom := requestWith(t, `,"tools":[{"type":"custom","custom":{"name":"freeform"}}],"tool_choice":{"type":"allowed_tools","allowed_tools":{"mode":"auto","tools":[{"type":"custom","custom":{"name":"freeform"}}]}}`)
	for _, target := range []execution.Target{{Type: execution.TargetOpenAI}, {Type: execution.TargetOpenAICompatible, ProviderName: "vendor.chat"}} {
		options, err := chatTranslator(custom)(target, custom.options)
		require.NoError(t, err)
		body := captureMediaRequest(t, target, options, false)
		assert.Contains(t, string(body["tool_choice"]), `"custom"`)
		assert.Contains(t, string(body["tool_choice"]), `"freeform"`)
	}
	raw := `{"model":"alias","messages":[{"role":"assistant","tool_calls":[{"id":"old","type":"function","function":{"name":"forbidden","arguments":"{}"}}]},{"role":"tool","tool_call_id":"old","content":"previous result"},{"role":"user","content":"continue"}]` + twoToolDeclarations + allowedToolChoice + `}`
	r, err := mapRequest([]byte(raw))
	require.NoError(t, err)
	options, err := chatTranslator(r)(execution.Target{Type: execution.TargetAnthropic}, r.options)
	require.NoError(t, err)
	assert.Equal(t, r.options.Prompt, options.Prompt)
	require.Len(t, options.Tools, 1)
	assert.Equal(t, "allowed", options.Tools[0].Name)
}
