package responses

import (
	"encoding/json"
	"testing"

	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const weatherTool = `{"type":"function","name":"weather","parameters":{"type":"object","properties":{"city":{"type":"string"}},"required":["city"]}}`

func body(extra string) string {
	return `{"model":"alias","input":[{"role":"user","content":"hello"}]` + extra + `}`
}

func TestMapRequest_Accepted(t *testing.T) {
	for _, tc := range []struct {
		name  string
		body  string
		check func(t *testing.T, r mappedRequest)
	}{
		{"string input", `{"model":"alias","input":"hello"}`, func(t *testing.T, r mappedRequest) {
			require.Len(t, r.options.Prompt, 1)
			assert.Equal(t, provider.RoleUser, r.options.Prompt[0].Role)
			assert.Equal(t, "hello", r.options.Prompt[0].Content[0].Text)
		}},
		{"codex fields", body(`,"store":false,"include":["reasoning.encrypted_content"],"prompt_cache_key":"k","client_metadata":{"x":"y"},"stream":true,"parallel_tool_calls":false,"tools":[` + weatherTool + `],"tool_choice":"auto"`), func(t *testing.T, r mappedRequest) {
			assert.True(t, boolValue(r.Stream))
			assert.Nil(t, r.options.ProviderOptions)
			require.Len(t, r.options.Tools, 1)
			assert.Equal(t, provider.ToolChoiceAuto, r.options.ToolChoice.Type)
		}},
		{"developer and system messages", `{"model":"alias","input":[{"type":"message","role":"developer","content":[{"type":"input_text","text":"be "},{"type":"input_text","text":"brief"}]},{"role":"user","content":"hi"},{"role":"system","content":"later"}]}`, func(t *testing.T, r mappedRequest) {
			require.Len(t, r.options.Prompt, 3)
			assert.Equal(t, provider.RoleSystem, r.options.Prompt[0].Role)
			assert.Equal(t, "be brief", r.options.Prompt[0].Content[0].Text)
			assert.Equal(t, provider.RoleSystem, r.options.Prompt[2].Role)
		}},
		{"function call continuation", `{"model":"alias","tools":[` + weatherTool + `],"input":[{"role":"user","content":"weather?"},{"type":"reasoning","id":"rs_1","summary":[{"type":"summary_text","text":"thinking"}],"encrypted_content":"opaque"},{"type":"function_call","id":"fc_1","call_id":"call_1","name":"weather","arguments":"{\"city\":\"Rio\"}","status":"completed"},{"type":"function_call_output","call_id":"call_1","output":"sunny"},{"role":"assistant","content":[{"type":"output_text","text":"Sunny.","annotations":[]}]}]}`, func(t *testing.T, r mappedRequest) {
			require.Len(t, r.options.Prompt, 4)
			assistant := r.options.Prompt[1]
			require.Len(t, assistant.Content, 2)
			assert.Equal(t, provider.ContentPartTypeReasoning, assistant.Content[0].Type)
			assert.Equal(t, "thinking", assistant.Content[0].Text)
			assert.Equal(t, "call_1", assistant.Content[1].ToolCallID)
			assert.Equal(t, provider.RoleTool, r.options.Prompt[2].Role)
			assert.Equal(t, "weather", r.options.Prompt[2].Content[0].ToolName)
			assert.Equal(t, "sunny", r.options.Prompt[2].Content[0].Output.Text)
			assert.Equal(t, "Sunny.", r.options.Prompt[3].Content[0].Text)
		}},
		{"images", `{"model":"alias","input":[{"role":"user","content":[{"type":"input_image","image_url":"data:image/png;base64,AAAA"},{"type":"input_image","image_url":"https://example.com/a.png","detail":"auto"}]}]}`, func(t *testing.T, r mappedRequest) {
			parts := r.options.Prompt[0].Content
			require.Len(t, parts, 2)
			assert.Equal(t, "image/png", parts[0].MediaType)
			assert.Equal(t, "AAAA", parts[0].Data.Base64)
			assert.Equal(t, "https://example.com/a.png", parts[1].Data.URL)
		}},
		{"reasoning and text settings", body(`,"reasoning":{"effort":"high","summary":"auto"},"text":{"verbosity":"low","format":{"type":"json_schema","name":"answer","strict":true,"schema":{"type":"object"}}}`), func(t *testing.T, r mappedRequest) {
			assert.Equal(t, provider.ReasoningHigh, r.options.Reasoning)
			assert.Equal(t, "auto", r.reasoningSummary)
			assert.Equal(t, "low", r.verbosity)
			assert.True(t, r.strictOutput)
			require.NotNil(t, r.options.ResponseFormat)
			assert.Equal(t, "answer", *r.options.ResponseFormat.Name)
		}},
		{"named tool choice", body(`,"tools":[` + weatherTool + `],"tool_choice":{"type":"function","name":"weather"}`), func(t *testing.T, r mappedRequest) {
			assert.Equal(t, provider.ToolChoiceTool, r.options.ToolChoice.Type)
			assert.Equal(t, "weather", r.options.ToolChoice.ToolName)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, err := mapRequest([]byte(tc.body))
			require.NoError(t, err)
			tc.check(t, r)
		})
	}
}

func TestMapRequest_Refused(t *testing.T) {
	for _, raw := range []string{
		`null`,
		body(`,"previous_response_id":"resp_1"`),
		body(`,"store":true`),
		body(`,"instructions":"x"`),
		body(`,"max_output_tokens":10`),
		body(`,"include":["file_search_call.results"]`),
		body(`,"client_metadata":"x"`),
		body(`,"tools":[{"type":"web_search"}]`),
		body(`,"tools":[{"type":"custom","name":"apply_patch","format":{"type":"text"}}]`),
		body(`,"tools":[` + weatherTool + `,` + weatherTool + `]`),
		body(`,"tool_choice":"required"`),
		body(`,"tool_choice":{"type":"function","name":"missing"}`),
		body(`,"reasoning":{"effort":"max"}`),
		body(`,"reasoning":{"summary":"long"}`),
		body(`,"text":{"verbosity":"loud"}`),
		body(`,"text":{"format":{"type":"json_schema","name":"x","schema":{"$ref":"https://private/"}}}`),
		`{"model":"","input":"x"}`,
		`{"model":"alias","input":[]}`,
		`{"model":"alias","input":[{"type":"custom_tool_call","call_id":"c","name":"apply_patch","input":"x"}]}`,
		`{"model":"alias","input":[{"type":"custom_tool_call_output","call_id":"c","output":"x"}]}`,
		`{"model":"alias","input":[{"type":"function_call_output","call_id":"orphan","output":"x"}]}`,
		`{"model":"alias","input":[{"type":"function_call","call_id":"c","name":"weather","arguments":"{}"}]}`,
		`{"model":"alias","input":[{"role":"user","content":[{"type":"input_file","file_id":"f"}]}]}`,
		`{"model":"alias","input":[{"role":"user","content":[{"type":"output_text","text":"x"}]}]}`,
		`{"model":"alias","input":[{"role":"user","content":[{"type":"input_image","image_url":"file:///etc/passwd"}]}]}`,
		`{"model":"alias","input":[{"role":"user","content":[{"type":"input_image","image_url":"https://a/b.png","detail":"high"}]}]}`,
		`{"model":"alias","input":[{"role":"developer","content":[{"type":"input_image","image_url":"https://a/b.png"}]}]}`,
		`{"model":"alias","input":[{"role":"tool","content":"x"}]}`,
		`{"model":"alias","input":[{"type":"reasoning","summary":[],"content":[{"type":"reasoning_text","text":"x"}]}]}`,
	} {
		t.Run(raw, func(t *testing.T) {
			_, err := mapRequest([]byte(raw))
			require.ErrorIs(t, err, errRequest)
		})
	}
}

func TestTranslator_Targets(t *testing.T) {
	r, err := mapRequest([]byte(body(`,"parallel_tool_calls":false,"reasoning":{"summary":"auto"},"text":{"verbosity":"low"}`)))
	require.NoError(t, err)
	for _, tc := range []struct {
		target execution.Target
		want   string
	}{
		{execution.Target{Type: execution.TargetOpenAI}, `{"openai":{"store":false,"strictJsonSchema":false,"parallelToolCalls":false,"reasoningSummary":"auto","textVerbosity":"low"}}`},
		{execution.Target{Type: execution.TargetOpenAICompatible, ProviderName: "vendor.chat"}, `{"vendor":{"store":false,"strictJsonSchema":false,"parallel_tool_calls":false}}`},
		{execution.Target{Type: execution.TargetAnthropic}, `{"anthropic":{"disableParallelToolUse":true}}`},
	} {
		t.Run(string(tc.target.Type), func(t *testing.T) {
			options, err := translator(r)(tc.target, r.options)
			require.NoError(t, err)
			encoded, err := json.Marshal(options.ProviderOptions)
			require.NoError(t, err)
			assert.JSONEq(t, tc.want, string(encoded))
			assert.Nil(t, r.options.ProviderOptions)
		})
	}
	_, err = translator(r)(execution.Target{Type: "unknown"}, r.options)
	require.ErrorIs(t, err, execution.ErrTranslation)
}
