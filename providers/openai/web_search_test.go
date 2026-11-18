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

func TestWebSearch_ResultStatus(t *testing.T) {
	const action = `,"action":{"type":"search","query":"go","queries":["go"],"sources":[{"type":"url","url":"https://example.com"}]}`
	_, _, br, err := buildParams("gpt-4o", provider.CallOptions{Tools: []provider.Tool{
		{Type: provider.ToolTypeProvider, ID: toolIDWebSearchPreview, Name: "lookup"},
	}})
	require.NoError(t, err)
	for _, status := range []string{"completed", "failed", "incomplete", "searching", "future_status"} {
		for _, withAction := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/action=%t", status, withAction), func(t *testing.T) {
				wantError := status != "completed"
				want := `{}`
				if wantError {
					want = fmt.Sprintf(`{"status":%q}`, status)
				} else if withAction {
					want = `{"action":{"type":"search","query":"go","queries":["go"]},"sources":[{"type":"url","url":"https://example.com"}]}`
				}
				actionField := ""
				if withAction {
					actionField = action
				}
				item := fmt.Sprintf(`{"type":"web_search_call","id":"ws_1","status":%q%s}`, status, actionField)
				t.Run("generate", func(t *testing.T) {
					resp := decodeResponse(t, `{"id":"resp_1","status":"completed","output":[`+item+`]}`)
					result := mustConvertResponse(t, resp, br)
					wantTypes := []provider.GenerateContentType{provider.ContentToolCall, provider.ContentToolResult}
					if withAction {
						wantTypes = append(wantTypes, provider.ContentSource)
					}
					var types []provider.GenerateContentType
					for _, part := range result.Content {
						types = append(types, part.Type)
					}
					require.Equal(t, wantTypes, types)
					assert.True(t, result.Content[0].ProviderExecuted)
					assert.Equal(t, "lookup", result.Content[0].ToolName)
					part := result.Content[1]
					assert.Equal(t, "ws_1", part.ToolCallID)
					assert.Equal(t, "lookup", part.ToolName)
					assert.Equal(t, wantError, part.IsError)
					assert.JSONEq(t, want, string(part.Result))
					if withAction {
						assert.Equal(t, "https://example.com", result.Content[2].URL)
					}
				})
				t.Run("stream", func(t *testing.T) {
					parts := collectPartsWithBuildResult(t, br,
						`{"type":"response.output_item.added","output_index":0,"item":{"type":"web_search_call","id":"ws_1","status":"in_progress"}}`,
						`{"type":"response.output_item.done","output_index":0,"item":`+item+`}`,
					)
					var calls, results, sources []provider.StreamPart
					for _, part := range parts {
						switch part.Type {
						case provider.PartToolCall:
							calls = append(calls, part)
						case provider.PartToolResult:
							results = append(results, part)
						case provider.PartSource:
							sources = append(sources, part)
						case provider.PartError:
							t.Errorf("search outcome became a stream error: %v", part.APICallError)
						}
					}
					require.Len(t, calls, 1)
					assert.True(t, calls[0].ProviderExecuted)
					assert.Equal(t, "lookup", calls[0].ToolName)
					require.Len(t, results, 1)
					part := results[0]
					assert.Equal(t, "ws_1", part.ToolCallID)
					assert.Equal(t, "lookup", part.ToolName)
					assert.Equal(t, wantError, part.IsError)
					assert.JSONEq(t, want, string(part.Result))
					if withAction {
						require.Len(t, sources, 1)
						require.NotNil(t, sources[0].Source)
						assert.Equal(t, "https://example.com", sources[0].Source.URL)
					} else {
						assert.Empty(t, sources)
					}
				})
			})
		}
	}
}

func TestWebSearch_AssistantHistory(t *testing.T) {
	tools := []provider.Tool{
		{Type: provider.ToolTypeProvider, ID: toolIDWebSearch, Name: "lookup"},
		{Type: provider.ToolTypeProvider, ID: toolIDWebSearchPreview, Name: "preview"},
	}
	cases := []struct {
		name, tool, itemID string
		output             provider.ToolResultOutput
		wantAction         string
		denied             bool
	}{
		{name: "search", tool: "lookup", itemID: "ws_native", output: webSearchJSON(`{"action":{"type":"search","query":"","queries":["go"],"ignored":true},"sources":[{"type":"url","url":"https://example.com","ignored":true},{"type":"api","name":"catalog"}]}`), wantAction: `{"type":"search","query":"","queries":["go"],"sources":[{"type":"url","url":"https://example.com"},{"type":"api","name":"catalog"}]}`},
		{name: "preview search", tool: "preview", output: webSearchJSON(`{"action":{"type":"search","queries":[]},"sources":[]}`), wantAction: `{"type":"search","queries":[],"sources":[]}`},
		{name: "open page", tool: "lookup", output: webSearchJSON(`{"action":{"type":"openPage","url":"https://example.com"},"sources":[{"type":"url","url":"https://example.com"}]}`), wantAction: `{"type":"open_page","url":"https://example.com"}`},
		{name: "find in page", tool: "preview", output: webSearchJSON(`{"action":{"type":"findInPage","pattern":null}}`), wantAction: `{"type":"find_in_page","pattern":null}`},
		{name: "canonical name", tool: "web_search", output: webSearchJSON(`{"action":{"type":"search"}}`), wantAction: `{"type":"search"}`},
		{name: "no action", tool: "lookup", output: webSearchJSON(`{"sources":[]}`)},
		{name: "unknown action", tool: "lookup", output: webSearchJSON(`{"action":{"type":"fetch"}}`)},
		{name: "wrong query type", tool: "lookup", output: webSearchJSON(`{"action":{"type":"search","query":null}}`)},
		{name: "bad query element", tool: "lookup", output: webSearchJSON(`{"action":{"type":"search","queries":[null]}}`)},
		{name: "bad source", tool: "lookup", output: webSearchJSON(`{"action":{"type":"search"},"sources":[{"type":"api","url":"https://example.com"}]}`)},
		{name: "bad sources on page action", tool: "lookup", output: webSearchJSON(`{"action":{"type":"openPage","url":null},"sources":null}`)},
		{name: "malformed JSON", tool: "lookup", output: webSearchJSON(`{"action":`)},
		{name: "non-JSON", tool: "lookup", output: provider.ToolResultOutput{Type: provider.ToolOutputText, Text: `{"action":{"type":"search"}}`}},
		{name: "error JSON", tool: "lookup", output: provider.ToolResultOutput{Type: provider.ToolOutputErrorJSON, JSON: json.RawMessage(`{"action":{"type":"search"}}`)}},
		{name: "status only", tool: "lookup", output: webSearchJSON(`{"status":"failed"}`)},
		{name: "denied", tool: "lookup", output: provider.ToolResultOutput{Type: provider.ToolOutputExecutionDenied}, denied: true},
		{name: "JSON denial", tool: "lookup", output: webSearchJSON(`{"type":"execution-denied","action":{"type":"search"}}`), denied: true},
	}
	for _, tc := range cases {
		for _, state := range []string{"stateless", "stored", "conversation"} {
			for _, streaming := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/stream=%t", tc.name, state, streaming), func(t *testing.T) {
					options := OpenAIResponsesOptions{Store: new(state == "stored")}
					if state == "conversation" {
						options.Conversation = "conv_1"
					}
					part := provider.ToolResultPart("ws_call", tc.tool, &tc.output)
					id := "ws_call"
					if tc.itemID != "" {
						id = tc.itemID
						part.ProviderOptions = provider.BuildProviderOptions(OpenAIPartOptions{ItemID: id})
					}
					call := provider.ContentPart{Type: provider.ContentPartTypeToolCall, ToolCallID: part.ToolCallID, ToolName: tc.tool, ProviderExecuted: true}
					opts := provider.CallOptions{
						Prompt: []provider.Message{provider.NewAssistantMessage(call, part), provider.UserText("next")},
						Tools:  tools, ProviderOptions: withOpenAIOptions(options),
					}
					want := `[{"role":"user","content":[{"type":"input_text","text":"next"}]}]`
					var warnings []provider.Warning
					switch {
					case tc.denied || state == "conversation":
					case state == "stored":
						want = fmt.Sprintf(`[{"type":"item_reference","id":%q},%s`, id, want[1:])
					case tc.wantAction != "":
						want = fmt.Sprintf(`[{"type":"web_search_call","id":%q,"status":"completed","action":%s},%s`, id, tc.wantAction, want[1:])
					default:
						warnings = []provider.Warning{{Type: provider.WarnOther, Message: "Results for OpenAI tool " + tc.tool + " are not sent to the API when store is false"}}
					}
					assertWebSearchHistoryRequest(t, opts, streaming, want, warnings)
				})
			}
		}
	}
}

func webSearchJSON(raw string) provider.ToolResultOutput {
	return provider.ToolResultOutput{Type: provider.ToolOutputJSON, JSON: json.RawMessage(raw)}
}

func assertWebSearchHistoryRequest(t *testing.T, opts provider.CallOptions, streaming bool, want string, warnings []provider.Warning) {
	t.Helper()
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return transportResponse(r, streaming), nil
	})}
	m := NewResponses("test-key", "gpt-4o", WithRequestOptions(option.WithHTTPClient(client), option.WithMaxRetries(0)))
	var request *provider.RequestMetadata
	var gotWarnings []provider.Warning
	if streaming {
		result, err := m.DoStream(t.Context(), opts)
		require.NoError(t, err)
		request = result.Request
		for part := range result.Stream {
			assert.NotEqual(t, provider.PartError, part.Type)
			if part.Type == provider.PartStreamStart {
				gotWarnings = append(gotWarnings, part.Warnings...)
			}
		}
	} else {
		result, err := m.DoGenerate(t.Context(), opts)
		require.NoError(t, err)
		request = result.Request
		gotWarnings = result.Warnings
	}
	require.NotNil(t, request)
	var body struct {
		Input json.RawMessage `json:"input"`
	}
	require.NoError(t, json.Unmarshal(request.Body, &body))
	assert.JSONEq(t, want, string(body.Input))
	assert.Equal(t, warnings, gotWarnings)
}
