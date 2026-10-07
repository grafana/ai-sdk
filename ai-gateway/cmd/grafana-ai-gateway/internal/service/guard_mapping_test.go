package service

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makeGuardInput(options provider.CallOptions, output []provider.ContentPart) (guardInput, error) {
	return makeGuardInputContext(context.Background(), options, output)
}

func applyGuardTransform(options provider.CallOptions, original guardInput, raw json.RawMessage) (provider.CallOptions, error) {
	return applyGuardTransformContext(context.Background(), options, original, raw)
}

func guardSafeJSON(before, after []byte) bool {
	return guardSafeJSONContext(context.Background(), before, after)
}

func guardJSONValue(raw []byte) (any, error) {
	return guardJSONValueContext(context.Background(), raw)
}

func decodeGuardJSON(raw []byte, out any) error {
	return decodeGuardJSONContext(context.Background(), raw, out)
}

func guardTestOptions() provider.CallOptions {
	strict := true
	tokens := 100
	temperature := 0.2
	return provider.CallOptions{
		Prompt: []provider.Message{
			provider.NewSystemMessage("system secret"), provider.UserText("user secret"),
			provider.NewAssistantMessage(provider.ReasoningPart("thinking unchanged"), provider.ToolCallPart("call-1", "f", json.RawMessage(`{"secret":"old","n":9007199254740993,"decimal":1.2500}`))),
			provider.NewToolMessage(provider.ToolResultPart("call-1", "f", &provider.ToolResultOutput{Type: provider.ToolOutputJSON, JSON: json.RawMessage(`{"secret":"old","n":9007199254740993}`)})),
		}, Tools: []provider.Tool{{Type: provider.ToolTypeFunction, Name: "f", Description: "secret", InputSchema: json.RawMessage(`{"type":"object","description":"old","maxLength":9007199254740993}`), Strict: &strict}},
		ToolChoice: &provider.ToolChoice{Type: provider.ToolChoiceTool, ToolName: "f"}, MaxOutputTokens: &tokens, Temperature: &temperature, Reasoning: provider.ReasoningHigh,
	}
}

func guardTestResponse(t *testing.T, in guardInput) guardResponseInput {
	t.Helper()
	convert := func(messages []guardMessage) []guardResponseMessage {
		var out []guardResponseMessage
		for _, m := range messages {
			next := guardResponseMessage{Role: m.Role, Name: m.Name}
			for _, p := range m.Parts {
				part := guardResponsePart{Kind: p.Kind, Text: p.Text, Thinking: p.Thinking}
				if p.ToolCall != nil {
					part.ToolCall = &guardResponseToolCall{ID: p.ToolCall.ID, Name: p.ToolCall.Name, InputJSON: append([]byte(nil), p.ToolCall.InputJSON...)}
				}
				if p.ToolResult != nil {
					r := p.ToolResult
					part.ToolResult = &guardResponseToolResult{ToolCallID: r.ToolCallID, Name: r.Name, IsError: r.IsError, Content: r.Content, ContentJSON: append([]byte(nil), r.ContentJSON...)}
				}
				next.Parts = append(next.Parts, part)
			}
			out = append(out, next)
		}
		return out
	}
	return guardResponseInput{Messages: convert(in.Messages), Output: convert(in.Output), Tools: append([]guardTool(nil), in.Tools...), SystemPrompt: in.SystemPrompt, ConversationPreview: in.ConversationPreview}
}

func TestGuardMapping(t *testing.T) {
	opts := guardTestOptions()
	in, err := makeGuardInput(opts, nil)
	require.NoError(t, err)
	require.Len(t, in.Messages, 4)
	assert.Equal(t, provider.RoleSystem, in.Messages[0].Role)
	assert.Empty(t, in.SystemPrompt)
	assert.Empty(t, in.ConversationPreview)
	b, err := json.Marshal(in)
	require.NoError(t, err)
	assert.Contains(t, string(b), `"input_json":{"secret":"old","n":9007199254740993,"decimal":1.2500}`)
	assert.Contains(t, string(b), `"input_schema_json":"`)
	assert.Contains(t, string(b), `"kind":"thinking"`)
	post, err := makeGuardInput(opts, []provider.ContentPart{provider.ToolCallPart("output-call", "f", json.RawMessage(`{"secret":"output"}`))})
	require.NoError(t, err)
	require.Len(t, post.Output, 1)
	assert.Equal(t, in.Messages, post.Messages)
	assert.Equal(t, "output-call", post.Output[0].Parts[0].ToolCall.ID)
}

func TestGuardImportedRequests(t *testing.T) {
	for _, tc := range []struct {
		file  string
		phase guardPhase
	}{{"request-preflight.json", guardPreflight}, {"request-postflight-guard.json", guardPostflight}} {
		t.Run(string(tc.phase), func(t *testing.T) {
			b, err := os.ReadFile("testdata/guards/" + tc.file)
			require.NoError(t, err)
			var request struct {
				Phase guardPhase `json:"phase"`
				Input guardInput `json:"input"`
			}
			require.NoError(t, json.Unmarshal(b, &request))
			assert.Equal(t, tc.phase, request.Phase)
			b, err = json.Marshal(request.Input)
			require.NoError(t, err)
			assert.Contains(t, string(b), `"input_json":{"command":"rm -rf /tmp/cache"}`)
			if tc.phase == guardPreflight {
				require.Len(t, request.Input.Tools, 2)
				assert.JSONEq(t, `{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}`, string(request.Input.Tools[1].InputSchemaJSON))
			}
		})
	}
}

func TestGuardUnsupported(t *testing.T) {
	cases := map[string]func(*provider.CallOptions){
		"media": func(o *provider.CallOptions) {
			o.Prompt[1].Content = append(o.Prompt[1].Content, provider.ContentPart{Type: provider.ContentPartTypeFile})
		},
		"nested media": func(o *provider.CallOptions) {
			o.Prompt[3].Content[0].Output = &provider.ToolResultOutput{Type: provider.ToolOutputContent, Content: []provider.ToolResultContentValue{{Type: provider.ToolContentFile}}}
		},
		"input examples": func(o *provider.CallOptions) {
			o.Tools[0].InputExamples = []provider.InputExample{{Input: json.RawMessage(`{}`)}}
		},
		"provider tool": func(o *provider.CallOptions) { o.Tools[0].Type = provider.ToolTypeProvider },
		"stored history": func(o *provider.CallOptions) {
			o.ProviderOptions = provider.BuildProviderOptions(provider.RawProviderOption{Key: "openai", Raw: json.RawMessage(`{"previousResponseId":"r"}`)})
		},
		"instructions": func(o *provider.CallOptions) {
			o.ProviderOptions = provider.BuildProviderOptions(provider.RawProviderOption{Key: "openai", Raw: json.RawMessage(`{"instructions":"secret"}`)})
		},
		"item id": func(o *provider.CallOptions) {
			o.Prompt[1].Content[0].ProviderOptions = provider.BuildProviderOptions(provider.RawProviderOption{Key: "openai", Raw: json.RawMessage(`{"itemId":"r"}`)})
		},
		"headers": func(o *provider.CallOptions) { o.Headers = map[string]string{"x-input": "secret"} },
		"mixed headers": func(o *provider.CallOptions) {
			o.Headers = map[string]string{"User-Agent": "ai/7.0.118", "x-input": "secret"}
		},
		"response schema": func(o *provider.CallOptions) {
			o.ResponseFormat = &provider.ResponseFormat{Type: provider.ResponseFormatJSON, Schema: json.RawMessage(`{"description":"secret"}`)}
		},
		"duplicate JSON": func(o *provider.CallOptions) { o.Prompt[2].Content[1].Input = json.RawMessage(`{"n":1,"n":2}`) },
		"duplicate schema": func(o *provider.CallOptions) {
			o.Tools[0].InputSchema = json.RawMessage(`{"type":"object","type":"string"}`)
		},
		"hidden union field": func(o *provider.CallOptions) { o.Prompt[1].Content[0].Input = json.RawMessage(`{"secret":true}`) },
		"unknown role":       func(o *provider.CallOptions) { o.Prompt[1].Role = "unknown" },
		"tool call user":     func(o *provider.CallOptions) { o.Prompt[2].Role = provider.RoleUser },
		"provider executed":  func(o *provider.CallOptions) { o.Prompt[2].Content[1].ProviderExecuted = true },
		"redacted reasoning": func(o *provider.CallOptions) {
			o.Prompt[2].Content[0].ProviderOptions = provider.BuildProviderOptions(provider.RawProviderOption{Key: "anthropic", Raw: json.RawMessage(`{"redactedData":"secret"}`)})
		},
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			o := guardTestOptions()
			change(&o)
			_, err := makeGuardInput(o, nil)
			assert.ErrorIs(t, err, errGuardUnsupported)
		})
	}
}

func TestGuardTransform(t *testing.T) {
	opts := guardTestOptions()
	opts.Prompt[2].Content[0].ProviderOptions = provider.BuildProviderOptions(provider.RawProviderOption{Key: "anthropic", Raw: json.RawMessage(`{"signature":"signed"}`)})
	in, err := makeGuardInput(opts, nil)
	require.NoError(t, err)
	response := guardTestResponse(t, in)
	response.Messages[0].Parts[0].Text = "safe system"
	response.Messages[1].Parts[0].Text = "safe user"
	response.Messages[2].Parts[1].ToolCall.InputJSON = []byte(`{"secret":"[REDACTED]","n":9007199254740993,"decimal":1.25e0}`)
	response.Messages[3].Parts[0].ToolResult.ContentJSON = []byte(`{"secret":"[REDACTED]","n":9007199254740993}`)
	response.Tools[0].Description = "safe description"
	response.Tools[0].InputSchemaJSON = []byte(`{"type":"object","description":"safe","maxLength":9007199254740993}`)
	b, err := json.Marshal(response)
	require.NoError(t, err)
	got, err := applyGuardTransform(opts, in, b)
	require.NoError(t, err)
	assert.Equal(t, "safe system", got.Prompt[0].Content[0].Text)
	assert.Equal(t, "system secret", opts.Prompt[0].Content[0].Text)
	assert.Equal(t, opts.Prompt[2].Content[0], got.Prompt[2].Content[0])
	assert.Equal(t, opts.ToolChoice, got.ToolChoice)
	assert.Equal(t, opts.MaxOutputTokens, got.MaxOutputTokens)
	assert.Equal(t, opts.Temperature, got.Temperature)
	assert.Equal(t, "safe description", got.Tools[0].Description)
	got.Prompt[2].Content[1].Input[0] = 'x'
	got.Tools[0].InputSchema[0] = 'x'
	assert.Equal(t, byte('{'), opts.Prompt[2].Content[1].Input[0])
	assert.Equal(t, byte('{'), opts.Tools[0].InputSchema[0])
}

func TestGuardAllOptionsPreserved(t *testing.T) {
	o := guardTestOptions()
	f := 0.75
	i := 7
	o.TopP = &f
	o.TopK = &i
	o.PresencePenalty = &f
	o.FrequencyPenalty = &f
	o.Seed = &i
	o.IncludeRawChunks = true
	o.ResponseFormat = &provider.ResponseFormat{Type: provider.ResponseFormatJSON}
	o.StopSequences = []string{}
	o.Tools[0].InputExamples = []provider.InputExample{}
	o.ResponseFormat.Schema = json.RawMessage{}
	o.Headers = map[string]string{"User-Agent": "ai/7.0.118"}
	o.ProviderOptions = provider.ProviderOptions{}
	o.Prompt[0].ProviderOptions = provider.ProviderOptions{}
	o.Prompt[0].Content[0].ProviderOptions = provider.ProviderOptions{}
	o.Tools[0].ProviderOptions = provider.ProviderOptions{}
	o.Tools[0].Args = map[string]json.RawMessage{}
	in, err := makeGuardInput(o, nil)
	require.NoError(t, err)
	b, err := json.Marshal(guardTestResponse(t, in))
	require.NoError(t, err)
	got, err := applyGuardTransform(o, in, b)
	require.NoError(t, err)
	assert.Equal(t, o, got)
	*got.Seed = 999
	assert.Equal(t, 7, *o.Seed)
	got.Headers["new"] = "value"
	assert.Equal(t, map[string]string{"User-Agent": "ai/7.0.118"}, o.Headers)
	got.ProviderOptions["new"] = nil
	assert.Empty(t, o.ProviderOptions)
	got.Prompt[0].ProviderOptions["new"] = nil
	assert.Empty(t, o.Prompt[0].ProviderOptions)
	got.Prompt[0].Content[0].ProviderOptions["new"] = nil
	assert.Empty(t, o.Prompt[0].Content[0].ProviderOptions)
	got.Tools[0].ProviderOptions["new"] = nil
	assert.Empty(t, o.Tools[0].ProviderOptions)
	got.Tools[0].Args["new"] = json.RawMessage(`1`)
	assert.Empty(t, o.Tools[0].Args)
}

func TestGuardStrictJSON(t *testing.T) {
	for _, raw := range []string{`{"a":1,"\u0061":2}`, `{"a":"\ud800"}`, `{"a":"\udc00"}`, `1e10001`, `{"a":1} {}`} {
		_, err := guardJSONValue([]byte(raw))
		assert.ErrorIs(t, err, errGuardTransform)
	}
	assert.True(t, guardSafeJSON([]byte(`{"n":123456789012345678901234567890,"d":0.12345678901234567890}`), []byte(`{"d":1.2345678901234567890e-1,"n":123456789012345678901234567890.0}`)))
	assert.False(t, guardSafeJSON([]byte(`{"n":0.1234567890123456789}`), []byte(`{"n":0.12345678901234568}`)))
	for _, payload := range []string{`{}`, `[123,125]`, `"{}"`, `"e31="`, `"e30=\\n"`, `null`} {
		var r guardResponseInput
		err := decodeGuardJSON([]byte(`{"messages":[{"role":"assistant","parts":[{"kind":"tool_call","tool_call":{"name":"f","input_json":`+payload+`}}]}]}`), &r)
		assert.Error(t, err)
	}
	o := guardTestOptions()
	o.Prompt[1].Content[0].Text = string([]byte{0xff})
	_, err := makeGuardInput(o, nil)
	assert.ErrorIs(t, err, errGuardUnsupported)
}

func TestGuardResultVariants(t *testing.T) {
	for _, output := range []*provider.ToolResultOutput{
		{Type: provider.ToolOutputText, Text: "secret"},
		{Type: provider.ToolOutputErrorText, Text: "secret"},
		{Type: provider.ToolOutputJSON, JSON: json.RawMessage(`null`)},
		{Type: provider.ToolOutputErrorJSON, JSON: json.RawMessage(`{"secret":"old"}`)},
		{Type: provider.ToolOutputExecutionDenied, Reason: "secret"},
		{Type: provider.ToolOutputContent, Content: []provider.ToolResultContentValue{{Type: provider.ToolContentText, Text: "secret"}, {Type: provider.ToolContentText, Text: "second"}}},
	} {
		t.Run(string(output.Type), func(t *testing.T) {
			o := provider.CallOptions{Prompt: []provider.Message{provider.NewToolMessage(provider.ToolResultPart("id", "f", output))}}
			in, err := makeGuardInput(o, nil)
			require.NoError(t, err)
			r := guardTestResponse(t, in)
			if output.Type == provider.ToolOutputContent {
				r.Messages[0].Parts[0].ToolResult.ContentJSON = []byte(`[{"type":"text","text":"safe"},{"type":"text","text":"second"}]`)
			}
			b, err := json.Marshal(r)
			require.NoError(t, err)
			got, err := applyGuardTransform(o, in, b)
			require.NoError(t, err)
			assert.Equal(t, output.Type, got.Prompt[0].Content[0].Output.Type)
			if output.Type == provider.ToolOutputContent {
				assert.Equal(t, "safe", got.Prompt[0].Content[0].Output.Content[0].Text)
				assert.Equal(t, "secret", output.Content[0].Text)
			}
		})
	}
}

func TestGuardUnsafeTransform(t *testing.T) {
	cases := map[string]func(*guardResponseInput){
		"role":          func(r *guardResponseInput) { r.Messages[1].Role = provider.RoleSystem },
		"kind":          func(r *guardResponseInput) { r.Messages[1].Parts[0].Kind = "future" },
		"thinking":      func(r *guardResponseInput) { r.Messages[2].Parts[0].Thinking = "changed" },
		"tool identity": func(r *guardResponseInput) { r.Messages[2].Parts[1].ToolCall.ID = "other" },
		"tool name":     func(r *guardResponseInput) { r.Tools[0].Name = "other" },
		"round integer": func(r *guardResponseInput) {
			r.Messages[2].Parts[1].ToolCall.InputJSON = []byte(`{"secret":"safe","n":9007199254740992,"decimal":1.25}`)
		},
		"round decimal": func(r *guardResponseInput) {
			r.Messages[2].Parts[1].ToolCall.InputJSON = []byte(`{"secret":"safe","n":9007199254740993,"decimal":1.2500000000000001}`)
		},
		"duplicate embedded": func(r *guardResponseInput) { r.Messages[2].Parts[1].ToolCall.InputJSON = []byte(`{"n":1,"n":1}`) },
		"invalid byte JSON":  func(r *guardResponseInput) { r.Messages[2].Parts[1].ToolCall.InputJSON = []byte("not JSON") },
		"missing message":    func(r *guardResponseInput) { r.Messages = r.Messages[:3] },
		"extra output":       func(r *guardResponseInput) { r.Output = []guardResponseMessage{{Role: provider.RoleAssistant}} },
		"preview":            func(r *guardResponseInput) { r.ConversationPreview = "secret" },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			o := guardTestOptions()
			in, err := makeGuardInput(o, nil)
			require.NoError(t, err)
			r := guardTestResponse(t, in)
			change(&r)
			b, err := json.Marshal(r)
			require.NoError(t, err)
			_, err = applyGuardTransform(o, in, b)
			assert.ErrorIs(t, err, errGuardTransform)
			assert.Equal(t, "system secret", o.Prompt[0].Content[0].Text)
		})
	}
	for _, raw := range []string{`null`, `{}`, `{"messages":null}`, `{"Messages":[]}`, `{"messages":[],"messages":[]}`} {
		o := guardTestOptions()
		in, err := makeGuardInput(o, nil)
		require.NoError(t, err)
		_, err = applyGuardTransform(o, in, json.RawMessage(raw))
		assert.ErrorIs(t, err, errGuardTransform)
	}
	o := guardTestOptions()
	in, err := makeGuardInput(o, []provider.ContentPart{provider.TextPart("output")})
	require.NoError(t, err)
	r := guardTestResponse(t, in)
	b, err := json.Marshal(r)
	require.NoError(t, err)
	_, err = applyGuardTransform(o, in, b)
	assert.ErrorIs(t, err, errGuardTransform)
}
