package chatcompletions

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
	"github.com/grafana/ai-sdk/provider"
	openaisdk "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const customRequest = `{"model":"alias","messages":[{"role":"user","content":"uppercase hello"}],"tools":[{"type":"custom","custom":{"name":"uppercase","format":{"type":"grammar","grammar":{"syntax":"regex","definition":"[a-z]+"}}}}],"tool_choice":{"type":"custom","custom":{"name":"uppercase"}}}`

func TestOfficialGoCustomToolManualRoundTrip(t *testing.T) {
	f := &fakeModel{generate: func(_ context.Context, o provider.CallOptions) (*provider.GenerateResult, error) {
		out := textResult()
		if len(o.Prompt) > 1 {
			require.Equal(t, provider.RoleTool, o.Prompt[2].Role)
			assert.Equal(t, "HELLO", o.Prompt[2].Content[0].Output.Text)
			assert.Contains(t, o.Prompt[1].Content[0].ProviderOptions, "openai")
			assert.Empty(t, o.Tools)
			return out, nil
		}
		require.Len(t, o.Tools, 1)
		assert.Equal(t, "openai.custom", o.Tools[0].ID)
		out.Content = []provider.GenerateContentPart{{Type: provider.ContentToolCall, ToolCallID: "c", ToolName: "uppercase", Input: json.RawMessage(`"hello"`)}}
		out.FinishReason.Unified = provider.FinishReasonToolCalls
		return out, nil
	}, stream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
		return partsStream(
			provider.StreamPart{Type: provider.PartStreamStart},
			provider.StreamPart{Type: provider.PartToolInputStart, ID: "c", ToolName: "uppercase", InputEncoding: provider.ToolInputEncodingText},
			provider.StreamPart{Type: provider.PartToolInputDelta, ID: "c", Delta: "hel"},
			provider.StreamPart{Type: provider.PartToolInputDelta, ID: "c", Delta: "lo"},
			provider.StreamPart{Type: provider.PartToolInputEnd, ID: "c"},
			provider.StreamPart{Type: provider.PartToolCall, ToolCallID: "c", ToolName: "uppercase", Input: `"hello"`},
			provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonToolCalls}, Usage: &textResult().Usage},
		), nil
	}}
	server := httptest.NewServer(testHandler(t, f))
	defer server.Close()
	client := openaisdk.NewClient(option.WithBaseURL(server.URL+"/v1"), option.WithAPIKey("test"), option.WithMaxRetries(0))
	var params openaisdk.ChatCompletionNewParams
	require.NoError(t, json.Unmarshal([]byte(customRequest), &params))
	params.ToolChoice = openaisdk.ToolChoiceOptionCustomToolChoice(openaisdk.ChatCompletionNamedToolChoiceCustomCustomParam{Name: "uppercase"})
	encoded, _ := json.Marshal(params)
	_, mapErr := mapRequest(encoded)
	require.NoError(t, mapErr, string(encoded))
	completion, err := client.Chat.Completions.New(t.Context(), params)
	require.NoError(t, err)
	require.Len(t, completion.Choices[0].Message.ToolCalls, 1)
	call := completion.Choices[0].Message.ToolCalls[0].AsCustom()
	assert.Equal(t, "uppercase", call.Custom.Name)
	assert.Equal(t, "hello", call.Custom.Input)
	stream := client.Chat.Completions.NewStreaming(t.Context(), params)
	defer func() { _ = stream.Close() }()
	var input strings.Builder
	for stream.Next() {
		var chunk struct {
			Choices []struct {
				Delta struct {
					ToolCalls []struct {
						Custom *customDelta `json:"custom"`
					} `json:"tool_calls"`
				} `json:"delta"`
			} `json:"choices"`
		}
		require.NoError(t, json.Unmarshal([]byte(stream.Current().RawJSON()), &chunk))
		for _, choice := range chunk.Choices {
			for _, call := range choice.Delta.ToolCalls {
				if call.Custom != nil && call.Custom.Input != nil {
					input.WriteString(*call.Custom.Input)
				}
			}
		}
	}
	require.NoError(t, stream.Err())
	assert.Equal(t, "hello", input.String())
	// Execute the requested custom transformation manually, then continue without declarations.
	params.Tools = nil
	params.ToolChoice = openaisdk.ChatCompletionToolChoiceOptionUnionParam{}
	params.Messages = append(params.Messages, completion.Choices[0].Message.ToParam(), openaisdk.ToolMessage(strings.ToUpper(call.Custom.Input), call.ID))
	_, err = client.Chat.Completions.New(t.Context(), params)
	require.NoError(t, err)
}

func TestCustomStreamInputMustMatchFinalText(t *testing.T) {
	r, err := mapRequest([]byte(customRequest))
	require.NoError(t, err)
	s := streamState{textIDs: map[string]bool{}, tools: map[string]*toolState{}}
	for _, p := range []provider.StreamPart{{Type: provider.PartToolInputStart, ID: "c", ToolName: "uppercase", InputEncoding: provider.ToolInputEncodingText}, {Type: provider.PartToolInputDelta, ID: "c", Delta: "hello"}, {Type: provider.PartToolInputEnd, ID: "c"}} {
		_, err := s.consume(p, r)
		require.NoError(t, err)
	}
	_, err = s.consume(provider.StreamPart{Type: provider.PartToolCall, ToolCallID: "c", ToolName: "uppercase", Input: `"different"`}, r)
	require.Error(t, err)
	bad := streamState{textIDs: map[string]bool{}, tools: map[string]*toolState{}}
	_, err = bad.consume(provider.StreamPart{Type: provider.PartToolInputStart, ID: "c", ToolName: "uppercase", InputEncoding: provider.ToolInputEncodingJSON}, r)
	require.Error(t, err)
	_, err = chatTranslator(r)(execution.Target{Type: execution.TargetAnthropic}, r.options)
	require.ErrorIs(t, err, provider.ErrUnsupportedFunctionality)
}

func TestCustomHistoryAndDeclarationValidation(t *testing.T) {
	for _, raw := range []string{
		`{"model":"alias","messages":[{"role":"user","content":"x"}],"tools":[{"type":"custom","custom":{"name":"uppercase","format":{"type":"grammar"}}}]}`,
		`{"model":"alias","messages":[{"role":"assistant","tool_calls":[{"id":"c","type":"custom","custom":{"name":"uppercase","input":{}}}]}]}`,
		`{"model":"alias","messages":[{"role":"user","content":"x"}],"tools":[{"type":"function","function":{"name":"f"}}],"tool_choice":{"type":"custom","custom":{"name":"f"}}}`,
	} {
		_, err := mapRequest([]byte(raw))
		require.Error(t, err)
	}
}
