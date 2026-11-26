package openai

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/openai/openai-go/v3/option"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	additionalFunction  = `{"type":"function","name":"lookup","description":"","parameters":{"default":null,"properties":{"value":{"const":9007199254740993}}},"strict":false,"defer_loading":true}`
	additionalCustom    = `{"type":"custom","name":"query","format":{"type":"grammar","syntax":"lark","definition":"start: WORD"}}`
	additionalNamespace = `{"type":"namespace","name":"catalog","description":"","tools":[` + additionalFunction + `]}`
	additionalList      = `[` + additionalFunction + `,` + additionalCustom + `,` + additionalNamespace + `]`
	additionalResult    = `{"type":"function_call_output","call_id":"lookup_1","output":"found"}`
)

func additionalToolItem(list string) string {
	return `{"type":"additional_tools","role":"developer","tools":` + list + `}`
}

func additionalResultPrompt(opts provider.ProviderOptions) []provider.Message {
	result := provider.ToolResultPart("lookup_1", "search", &provider.ToolResultOutput{Type: provider.ToolOutputText, Text: "found"})
	result.ProviderOptions = opts
	return []provider.Message{provider.NewToolMessage(result)}
}

func additionalOptions(t *testing.T, raw string) provider.ProviderOptions {
	t.Helper()
	var opts provider.ProviderOptions
	require.NoError(t, json.Unmarshal([]byte(raw), &opts))
	return opts
}

// These synthetic HTTP captures exercise request conversion, not live provider acceptance.
type additionalToolsRequest struct {
	Input  json.RawMessage `json:"input"`
	Tools  json.RawMessage `json:"tools"`
	Stream bool            `json:"stream"`
}

func additionalToolsModel(transport roundTripFunc, opts ...Option) provider.LanguageModel {
	httpClient := &http.Client{Transport: transport}
	return NewResponses("test-key", "gpt-5.4", append(opts, WithRequestOptions(option.WithHTTPClient(httpClient), option.WithMaxRetries(0)))...)
}

func runAdditionalToolsRequest(t *testing.T, model provider.LanguageModel, streaming bool, opts provider.CallOptions) error {
	t.Helper()
	if streaming {
		result, err := model.DoStream(t.Context(), opts)
		if err == nil {
			for part := range result.Stream {
				assert.NotEqual(t, provider.PartError, part.Type)
			}
		}
		return err
	}
	_, err := model.DoGenerate(t.Context(), opts)
	return err
}

func captureAdditionalToolsRequest(t *testing.T, opts provider.CallOptions, streaming bool, modelOpts ...Option) additionalToolsRequest {
	t.Helper()
	var body additionalToolsRequest
	requests := 0
	m := additionalToolsModel(func(r *http.Request) (*http.Response, error) {
		requests++
		assert.Equal(t, "/v1/responses", r.URL.Path)
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			return nil, err
		}
		return transportResponse(r, streaming), nil
	}, modelOpts...)
	require.NoError(t, runAdditionalToolsRequest(t, m, streaming, opts))
	require.Equal(t, 1, requests)
	assert.Equal(t, streaming, body.Stream)
	return body
}

func TestAdditionalTools_ResultHistory(t *testing.T) {
	for _, streaming := range []bool{false, true} {
		t.Run(fmt.Sprintf("stream=%t", streaming), func(t *testing.T) {
			for _, tc := range []struct {
				name      string
				raw       string
				modelOpts []Option
				wantList  string
			}{
				{name: "omitted", raw: `{"openai":{"irrelevant":true}}`},
				{name: "empty", raw: `{"openai":{"additionalTools":[]}}`},
				{name: "multiple", raw: `{"openai":{"additionalTools":` + additionalList + `,"unrelated":null}}`, wantList: additionalList},
				{name: "openai ignores azure", raw: `{"openai":{"additionalTools":` + additionalList + `},"azure":{"additionalTools":null}}`, wantList: additionalList},
				{name: "azure active ignores openai", raw: `{"azure":{"additionalTools":` + additionalList + `},"openai":{"additionalTools":null}}`, modelOpts: []Option{WithProviderName("custom.azure")}, wantList: additionalList},
				{name: "azure absent field falls back", raw: `{"azure":{"unrelated":true},"openai":{"additionalTools":` + additionalList + `}}`, modelOpts: []Option{WithProviderName("custom.azure")}, wantList: additionalList},
				{name: "azure null namespace falls back", raw: `{"azure":null,"openai":{"additionalTools":` + additionalList + `}}`, modelOpts: []Option{WithProviderName("custom.azure")}, wantList: additionalList},
				{name: "azure empty wins", raw: `{"azure":{"additionalTools":[]},"openai":{"additionalTools":` + additionalList + `}}`, modelOpts: []Option{WithProviderName("custom.azure")}},
			} {
				t.Run(tc.name, func(t *testing.T) {
					prompt := additionalResultPrompt(additionalOptions(t, tc.raw))
					second := provider.ToolResultPart("lookup_2", "search", &provider.ToolResultOutput{Type: provider.ToolOutputText, Text: "found more"})
					second.ProviderOptions = additionalOptions(t, `{"openai":{"additionalTools":[`+additionalFunction+`]}}`)
					prompt[0].Content = append(prompt[0].Content, second)
					opts := provider.CallOptions{Prompt: prompt, Tools: []provider.Tool{{Type: provider.ToolTypeFunction, Name: "search", InputSchema: json.RawMessage(`{"type":"object"}`)}}}
					body := captureAdditionalToolsRequest(t, opts, streaming, tc.modelOpts...)
					want := additionalResult
					if tc.wantList != "" {
						want += "," + additionalToolItem(tc.wantList)
					}
					want += `,{"type":"function_call_output","call_id":"lookup_2","output":"found more"},` + additionalToolItem(`[`+additionalFunction+`]`)
					assert.JSONEq(t, "["+want+"]", string(body.Input))
					assert.Contains(t, string(body.Input), "9007199254740993")
					assert.JSONEq(t, `[{"type":"function","name":"search","parameters":{"type":"object"},"strict":false}]`, string(body.Tools))
				})
			}
		})
	}
}

func TestAdditionalTools_TypedOptions(t *testing.T) {
	for _, tc := range []struct {
		name     string
		tools    []OpenAIAdditionalTool
		wantList string
	}{
		{name: "unset", wantList: additionalList},
		{name: "empty", tools: []OpenAIAdditionalTool{}},
		{name: "typed", tools: []OpenAIAdditionalTool{
			{Type: OpenAIAdditionalToolFunction, Name: "lookup", Parameters: json.RawMessage(`{}`), Strict: new(false)},
			{Type: OpenAIAdditionalToolCustom, Name: "text", Format: &OpenAIAdditionalToolFormat{Type: OpenAIAdditionalToolFormatText}},
			{Type: OpenAIAdditionalToolCustom, Name: "regex", Format: &OpenAIAdditionalToolFormat{Type: OpenAIAdditionalToolFormatGrammar, Syntax: OpenAIAdditionalToolGrammarRegex, Definition: new("")}},
			{Type: OpenAIAdditionalToolNamespace, Name: "catalog", Description: new(""), Tools: []OpenAIAdditionalTool{{Type: OpenAIAdditionalToolFunction, Name: "child", Parameters: json.RawMessage(`{}`)}}},
		}, wantList: `[{"type":"function","name":"lookup","parameters":{},"strict":false},{"type":"custom","name":"text","format":{"type":"text"}},{"type":"custom","name":"regex","format":{"type":"grammar","syntax":"regex","definition":""}},{"type":"namespace","name":"catalog","description":"","tools":[{"type":"function","name":"child","parameters":{}}]}]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			opts := additionalOptions(t, `{"openai":{"additionalTools":`+additionalList+`}}`)
			opts["azure"] = OpenAIPartOptions{AdditionalTools: tc.tools}
			wire, err := json.Marshal(opts)
			require.NoError(t, err)
			for _, partOpts := range []provider.ProviderOptions{opts, additionalOptions(t, string(wire))} {
				body := captureAdditionalToolsRequest(t, provider.CallOptions{Prompt: additionalResultPrompt(partOpts)}, false, WithProviderName("custom.azure"))
				want := additionalResult
				if tc.wantList != "" {
					want += "," + additionalToolItem(tc.wantList)
				}
				assert.JSONEq(t, "["+want+"]", string(body.Input))
			}
		})
	}
}

func TestAdditionalTools_RejectMalformed(t *testing.T) {
	for _, tc := range []struct{ name, list string }{
		{"null", `null`},
		{"not an array", `"invalid"`},
		{"null definition", `[null]`},
		{"unknown type", `[{"type":"web_search","name":"search"}]`},
		{"empty name", `[{"type":"function","name":"","parameters":{}}]`},
		{"missing parameters", `[{"type":"function","name":"lookup"}]`},
		{"non-object parameters", `[{"type":"function","name":"lookup","parameters":[]}]`},
		{"null parameters", `[{"type":"function","name":"lookup","parameters":null}]`},
		{"not a boolean", `[{"type":"function","name":"lookup","parameters":{},"strict":"yes"}]`},
		{"null defer loading", `[{"type":"function","name":"lookup","parameters":{},"defer_loading":null}]`},
		{"unknown definition field", `[{"type":"function","name":"lookup","parameters":{},"async":false}]`},
		{"wrong case", `[{"type":"function","Name":"lookup","parameters":{}}]`},
		{"null description", `[{"type":"custom","name":"lookup","description":null}]`},
		{"null format", `[{"type":"custom","name":"lookup","format":null}]`},
		{"unknown format field", `[{"type":"custom","name":"lookup","format":{"type":"text","description":"invalid"}}]`},
		{"invalid grammar", `[{"type":"custom","name":"lookup","format":{"type":"grammar","syntax":"unknown","definition":""}}]`},
		{"invalid grammar definition", `[{"type":"custom","name":"lookup","format":{"type":"grammar","syntax":"regex","definition":null}}]`},
		{"missing namespace description", `[{"type":"namespace","name":"group","tools":[` + additionalFunction + `]}]`},
		{"empty namespace", `[{"type":"namespace","name":"group","description":"","tools":[]}]`},
		{"custom namespace child", `[{"type":"namespace","name":"group","description":"","tools":[` + additionalCustom + `]}]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			opts := additionalOptions(t, `{"azure":{"additionalTools":`+tc.list+`},"openai":{"additionalTools":`+additionalList+`}}`)
			assertAdditionalToolsRejected(t, additionalResultPrompt(opts), "azure.additionalTools", WithProviderName("custom.azure"))
		})
	}
	t.Run("malformed active namespace", func(t *testing.T) {
		opts := additionalOptions(t, `{"azure":[],"openai":{"additionalTools":`+additionalList+`}}`)
		assertAdditionalToolsRejected(t, additionalResultPrompt(opts), "azure.additionalTools", WithProviderName("custom.azure"))
	})
	t.Run("malformed typed definition", func(t *testing.T) {
		opts := provider.BuildProviderOptions(OpenAIPartOptions{AdditionalTools: []OpenAIAdditionalTool{
			{Type: OpenAIAdditionalToolFunction, Name: "lookup", Parameters: json.RawMessage(`[]`)},
		}})
		assertAdditionalToolsRejected(t, additionalResultPrompt(opts), "openai.additionalTools[0]")
	})
}

func assertAdditionalToolsRejected(t *testing.T, prompt []provider.Message, wantErr string, modelOpts ...Option) {
	t.Helper()
	requests := 0
	for _, streaming := range []bool{false, true} {
		m := additionalToolsModel(func(r *http.Request) (*http.Response, error) {
			requests++
			return transportResponse(r, streaming), nil
		}, modelOpts...)
		assert.ErrorContains(t, runAdditionalToolsRequest(t, m, streaming, provider.CallOptions{Prompt: prompt}), wantErr)
	}
	assert.Zero(t, requests)
}

func TestAdditionalTools_ParallelContinuation(t *testing.T) {
	for _, tc := range []struct {
		name       string
		opts       OpenAIResponsesOptions
		incomplete bool
		want       string
	}{
		{name: "conversation", opts: OpenAIResponsesOptions{Conversation: "conv_1"}, want: `{"type":"function_call_output","call_id":"call_parallel","output":"weather\ncityAttractions"},` + additionalToolItem(`[`+additionalFunction+`]`) + "," + additionalToolItem(`[`+additionalCustom+`]`)},
		{name: "previous response", opts: OpenAIResponsesOptions{PreviousResponseID: "resp_1"}, want: `{"type":"function_call","call_id":"call_parallel","name":"parallel","arguments":%s},{"type":"function_call_output","call_id":"call_parallel","output":"weather\ncityAttractions"},` + additionalToolItem(`[`+additionalFunction+`]`) + "," + additionalToolItem(`[`+additionalCustom+`]`)},
		{name: "incomplete group", opts: OpenAIResponsesOptions{Conversation: "conv_1"}, incomplete: true, want: `{"type":"function_call","call_id":"call_parallel_0","name":"weather","arguments":"{\"location\":\"SF\"}"},{"type":"function_call","call_id":"call_parallel_1","name":"cityAttractions","arguments":"{\"city\":\"Rome\"}"},{"type":"function_call_output","call_id":"call_parallel_1","output":"cityAttractions"},` + additionalToolItem(`[`+additionalCustom+`]`)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			prompt := parallelHistory(t) // results are deliberately reversed
			for i, def := range []string{additionalCustom, additionalFunction} {
				p := &prompt[1].Content[i]
				fields, _, err := provider.ResolveOption[map[string]json.RawMessage](p.ProviderOptions, "openai")
				require.NoError(t, err)
				fields["additionalTools"] = json.RawMessage("[" + def + "]")
				raw, err := json.Marshal(fields)
				require.NoError(t, err)
				p.ProviderOptions["openai"] = provider.RawProviderOption{Key: "openai", Raw: raw}
			}
			if tc.incomplete {
				prompt[1].Content = prompt[1].Content[:1]
			}
			want := tc.want
			if tc.opts.PreviousResponseID != "" {
				input, err := json.Marshal(parallelInput)
				require.NoError(t, err)
				want = fmt.Sprintf(want, input)
			}
			for _, streaming := range []bool{false, true} {
				body := captureAdditionalToolsRequest(t, provider.CallOptions{Prompt: prompt, ProviderOptions: withOpenAIOptions(tc.opts)}, streaming)
				assert.JSONEq(t, "["+want+"]", string(body.Input))
			}
		})
	}
}

func TestAdditionalTools_NativeResultBoundaries(t *testing.T) {
	opts := additionalOptions(t, `{"openai":{"additionalTools":{}}}`)
	definitionOpts := additionalOptions(t, `{"openai":{"additionalTools":`+additionalList+`}}`)
	search := provider.ToolResultPart("search_1", "tool_search", &provider.ToolResultOutput{Type: provider.ToolOutputJSON, JSON: json.RawMessage(`{"tools":[]}`)})
	search.ProviderOptions = opts
	custom := provider.ToolResultPart("custom_1", "custom_lookup", &provider.ToolResultOutput{Type: provider.ToolOutputText, Text: "found"})
	custom.ProviderOptions = opts
	hosted := provider.ToolResultPart("hosted_1", "web_search", &provider.ToolResultOutput{Type: provider.ToolOutputJSON, JSON: json.RawMessage(`{}`)})
	hosted.ProviderOptions = additionalOptions(t, `{"openai":{"additionalTools":{},"itemId":"ws_item"}}`)
	denied := provider.ToolResultPart("denied_1", "local", &provider.ToolResultOutput{Type: provider.ToolOutputExecutionDenied, ProviderOptions: additionalOptions(t, `{"openai":{"approvalId":"approval_1"}}`)})
	denied.ProviderOptions = opts
	bare := provider.ToolResultPart("bare_1", "local", &provider.ToolResultOutput{Type: provider.ToolOutputText, Text: "found", ProviderOptions: definitionOpts})
	body := captureAdditionalToolsRequest(t, provider.CallOptions{
		Prompt: []provider.Message{provider.NewAssistantMessage(hosted), provider.NewToolMessage(search, custom, denied, bare)},
		Tools:  []provider.Tool{{Type: provider.ToolTypeProvider, ID: toolIDCustom, Name: "custom_lookup", Args: map[string]json.RawMessage{"name": json.RawMessage(`"custom_lookup"`)}}},
	}, false)
	assert.JSONEq(t, `[{"type":"item_reference","id":"ws_item"},{"type":"tool_search_output","execution":"client","call_id":"search_1","status":"completed","tools":[]},{"type":"custom_tool_call_output","call_id":"custom_1","output":"found"},{"type":"function_call_output","call_id":"bare_1","output":"found"}]`, string(body.Input))
}
