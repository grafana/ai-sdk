package chatcompletions

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/grafana/ai-sdk/ai-gateway/catalog"
	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeModel struct {
	generate func(context.Context, provider.CallOptions) (*provider.GenerateResult, error)
	stream   func(context.Context, provider.CallOptions) (*provider.StreamResult, error)
}

func (*fakeModel) SpecificationVersion() string               { return "v4" }
func (*fakeModel) Provider() string                           { return "private-provider" }
func (*fakeModel) ModelID() string                            { return "private-model" }
func (*fakeModel) SupportedURLs() map[string][]*regexp.Regexp { return nil }
func (f *fakeModel) DoGenerate(ctx context.Context, o provider.CallOptions) (*provider.GenerateResult, error) {
	return f.generate(ctx, o)
}
func (f *fakeModel) DoStream(ctx context.Context, o provider.CallOptions) (*provider.StreamResult, error) {
	return f.stream(ctx, o)
}
func testHandler(t *testing.T, f *fakeModel) *handler {
	t.Helper()
	c, err := catalog.NewStatic([]catalog.StaticEntry{{Info: catalog.ModelInfo{ID: "canonical", Aliases: []string{"alias"}}, Model: execution.Translate(f, execution.Target{Type: execution.TargetOpenAI})}})
	require.NoError(t, err)
	runtime, err := execution.New(execution.Config{Selector: execution.CatalogSelector(c), Limits: execution.DefaultLimits})
	require.NoError(t, err)
	h, err := New(runtime)
	require.NoError(t, err)
	return h
}

const basic = `{"model":"alias","messages":[{"role":"user","content":"hello"}]}`

func requestWith(t *testing.T, extra string) mappedRequest {
	t.Helper()
	r, err := mapRequest([]byte(strings.TrimSuffix(basic, "}") + extra + "}"))
	require.NoError(t, err)
	return r
}
func textResult() *provider.GenerateResult {
	return &provider.GenerateResult{Content: []provider.GenerateContentPart{{Type: provider.ContentText, Text: "hello"}}, FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop}, Usage: provider.Usage{InputTokens: provider.InputTokenUsage{Total: ptr(3), CacheRead: ptr(1)}, OutputTokens: provider.OutputTokenUsage{Total: ptr(2)}}}
}
func TestRequestDefaultsAndRejections(t *testing.T) {
	tool := `,"tools":[{"type":"function","function":{"name":"weather","parameters":{"type":"object"}}}]`
	for _, strict := range []string{"", `,"strict":null`, `,"strict":false`, `,"strict":true`} {
		t.Run("strict"+strict, func(t *testing.T) {
			raw := strings.Replace(tool, `"parameters"`, `"parameters"`, 1)
			raw = strings.Replace(raw, `"type":"object"}}}]`, `"type":"object"}`+strict+`}}]`, 1)
			r := requestWith(t, raw)
			target := execution.Target{Type: execution.TargetOpenAI}
			var err error
			r.options, err = chatTranslator(r)(target, r.options)
			require.NoError(t, err)
			require.NotNil(t, r.options.Tools[0].Strict)
			assert.Equal(t, strings.Contains(strict, "true"), *r.options.Tools[0].Strict)
			b, err := json.Marshal(r.options.ProviderOptions)
			require.NoError(t, err)
			assert.JSONEq(t, `{"openai":{"store":false,"strictJsonSchema":false}}`, string(b))
		})
	}
	for _, extra := range []string{`,"n":2`, `,"store":true`, `,"temperature":3`, `,"max_tokens":0`, `,"max_tokens":1,"max_completion_tokens":2`, `,"audio":{}`, `,"stream_options":{}`, `,"tool_choice":"required"`, `,"response_format":{"type":"json_schema","json_schema":{"name":"x","schema":{"type":"object","$ref":"https://private/"}}}}`, `,"Model":"other"`} {
		t.Run(extra, func(t *testing.T) {
			_, err := mapRequest([]byte(strings.TrimSuffix(basic, "}") + extra + "}"))
			require.Error(t, err)
		})
	}
	for _, body := range []string{`null`, basic + basic, `{"model":"x","messages":[{"role":"developer","content":"no"}]}`, `{"model":"x","messages":[{"role":"tool","tool_call_id":"orphan","content":"no"}]}`} {
		_, err := mapRequest([]byte(body))
		require.Error(t, err)
	}

}

func TestContinuationAndStructuredValidation(t *testing.T) {
	raw := `{"model":"alias","messages":[{"role":"assistant","content":null,"tool_calls":[{"id":"call","type":"function","function":{"name":"weather","arguments":"{}"}}]},{"role":"tool","tool_call_id":"call","content":"sunny"},{"role":"user","content":[{"type":"text","text":"thanks"}]}]}`
	r, err := mapRequest([]byte(raw))
	require.NoError(t, err)

	require.Len(t, r.options.Prompt, 3)
	assert.Equal(t, "weather", r.options.Prompt[1].Content[0].ToolName)
	_, err = mapRequest([]byte(strings.Replace(raw, `"tool_call_id":"call"`, `"tool_call_id":"bad"`, 1)))
	require.Error(t, err)
	r = requestWith(t, `,"response_format":{"type":"json_schema","json_schema":{"name":"answer","strict":true,"schema":{"type":"object","properties":{"answer":{"type":"string"}},"required":["answer"],"additionalProperties":false}}}`)
	require.NoError(t, r.validateText(`{"answer":"yes"}`, "stop"))
	require.Error(t, r.validateText(`{"answer":1}`, "stop"))
	require.NoError(t, r.validateText(`{"answer":`, "length"))
	require.Error(t, r.validateText(`{"answer":`, "stop"))
}

func TestMappedSettingsReachModel(t *testing.T) {
	for _, extra := range []string{`,"temperature":2`, `,"max_tokens":4097`, `,"seed":1`, `,"reasoning_effort":"high"`, `,"tool_choice":"none"`} {
		t.Run(extra, func(t *testing.T) {
			calls := 0
			h := testHandler(t, &fakeModel{generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
				calls++
				return textResult(), nil
			}})
			w := httptest.NewRecorder()
			req := httptest.NewRequest("POST", Path, strings.NewReader(strings.TrimSuffix(basic, "}")+extra+"}"))
			req.Header.Set("Content-Type", "application/json")
			h.ServeHTTP(w, req)
			assert.Equal(t, 200, w.Code)
			assert.Equal(t, 1, calls)
		})
	}
}
