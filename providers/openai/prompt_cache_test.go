package openai

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/openai/openai-go/v3/option"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Synthetic responses mirror the registered upstream prewarm unit examples;
// these are transport tests, not recorded provider fixtures.
const prewarmUsage = `{"input_tokens":2048,"input_tokens_details":{"cached_tokens":0,"cache_write_tokens":2048,"cache_write_tokens_details":{"ttl_30m_tokens":0,"ttl_12h_tokens":2048}},"output_tokens":0,"output_tokens_details":{"reasoning_tokens":0},"total_tokens":2048}`
const prewarmResponse = `{"id":"resp_prewarm","object":"response","created_at":1,"model":"gpt-6.1-sol","status":"completed","output":[],"usage":` + prewarmUsage + `}`

func TestResponses_PromptCacheControls(t *testing.T) {
	for _, namespace := range []string{"openai", "azure"} {
		for _, tc := range []struct {
			name, options, want string
			invalid             bool
		}{
			{name: "omitted", options: `{}`},
			{name: "empty", options: `{"promptCacheOptions":{}}`, want: `{}`},
			{name: "zero value ttl", options: `{"promptCacheOptions":{"ttl":""}}`, want: `{}`},
			{name: "null ttl", options: `{"promptCacheOptions":{"ttl":null}}`, want: `{}`},
			{name: "30m", options: `{"promptCacheOptions":{"mode":"explicit","ttl":"30m"}}`, want: `{"mode":"explicit","ttl":"30m"}`},
			{name: "12h prewarm", options: `{"promptCacheOptions":{"mode":"implicit","ttl":"12h","prewarm":true}}`, want: `{"mode":"implicit","ttl":"12h","prewarm":true}`},
			{name: "false prewarm", options: `{"promptCacheOptions":{"prewarm":false}}`, want: `{"prewarm":false}`},
			{name: "invalid ttl", options: `{"promptCacheOptions":{"ttl":"24h"}}`, invalid: true},
			{name: "invalid type", options: `{"promptCacheOptions":{"ttl":12}}`, invalid: true},
		} {
			t.Run(namespace+"/"+tc.name, func(t *testing.T) {
				var bodies []map[string]any
				client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
					var body map[string]any
					require.NoError(t, json.NewDecoder(req.Body).Decode(&body))
					bodies = append(bodies, body)
					contentType, payload := "application/json", prewarmResponse
					if body["stream"] == true {
						contentType = "text/event-stream"
						payload = "data: " + `{"type":"response.created","sequence_number":0,"response":{"id":"resp_prewarm","created_at":1,"model":"gpt-6.1-sol"}}` + "\n\n" +
							"data: " + `{"type":"response.completed","sequence_number":1,"response":` + prewarmResponse + "}\n\n"
					}
					return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{contentType}}, Body: io.NopCloser(strings.NewReader(payload)), Request: req}, nil
				})}
				model := NewResponses("test-key", "gpt-6.1-sol", WithRequestOptions(option.WithHTTPClient(client), option.WithMaxRetries(0)))
				opts := provider.CallOptions{Prompt: promptCachePrompt(namespace), ProviderOptions: provider.ProviderOptions{namespace: provider.RawProviderOption{Key: namespace, Raw: json.RawMessage(tc.options)}}}
				result, generateErr := model.DoGenerate(t.Context(), opts)
				streamed, streamErr := model.DoStream(t.Context(), opts)
				if tc.invalid {
					require.Error(t, generateErr)
					require.Error(t, streamErr)
					assert.Contains(t, generateErr.Error(), "ttl")
					assert.Contains(t, streamErr.Error(), "ttl")
					assert.Empty(t, bodies)
					return
				}
				require.NoError(t, generateErr)
				require.NoError(t, streamErr)
				assert.Empty(t, result.Content)
				assert.Equal(t, provider.FinishReasonStop, result.FinishReason.Unified)
				checkPrewarmUsage(t, result.Usage)
				finishes := 0
				for part := range streamed.Stream {
					switch part.Type {
					case provider.PartFinish:
						finishes++
						assert.Equal(t, provider.FinishReasonStop, part.FinishReason.Unified)
						require.NotNil(t, part.Usage)
						checkPrewarmUsage(t, *part.Usage)
					case provider.PartStreamStart, provider.PartResponseMeta:
					default:
						t.Errorf("unexpected prewarm stream part: %s", part.Type)
					}
				}
				assert.Equal(t, 1, finishes)
				require.Len(t, bodies, 2)
				for _, body := range bodies {
					checkPromptCacheBreakpoints(t, body["input"])
					if tc.want == "" {
						assert.NotContains(t, body, "prompt_cache_options")
						continue
					}
					raw, err := json.Marshal(body["prompt_cache_options"])
					require.NoError(t, err)
					assert.JSONEq(t, tc.want, string(raw))
				}
			})
		}
	}
}

func checkPrewarmUsage(t *testing.T, usage provider.Usage) {
	t.Helper()
	assert.Equal(t, intPtr(2048), usage.InputTokens.Total)
	assert.Equal(t, intPtr(2048), usage.InputTokens.CacheWrite)
	assert.Equal(t, intPtr(0), usage.InputTokens.NoCache)
	assert.Equal(t, intPtr(0), usage.OutputTokens.Total)
	assert.JSONEq(t, prewarmUsage, string(usage.Raw))
}

func promptCachePrompt(namespace string) []provider.Message {
	options := provider.ProviderOptions{namespace: provider.RawProviderOption{Key: namespace, Raw: json.RawMessage(`{"promptCacheBreakpoint":{"mode":"explicit","ttl":"12h"}}`)}}
	system := provider.NewSystemMessage("stable system")
	system.ProviderOptions = options
	text := provider.TextPart("stable user")
	text.ProviderOptions = options
	prompt := []provider.Message{system, provider.NewUserMessage(text)}
	for _, media := range []string{"image/png", "application/pdf"} {
		for _, data := range []provider.DataContent{{URL: "https://example.com/input"}, {Bytes: []byte("synthetic")}, {Reference: json.RawMessage(`{"openai":"file_1","azure":"file_1"}`)}} {
			part := provider.FilePart(media, data)
			part.ProviderOptions = options
			prompt = append(prompt, provider.NewUserMessage(part))
			output := &provider.ToolResultOutput{Type: provider.ToolOutputContent, Content: []provider.ToolResultContentValue{{Type: provider.ToolContentFile, MediaType: media, Data: &data, ProviderOptions: options}}}
			prompt = append(prompt, provider.NewToolMessage(provider.ToolResultPart("call_1", "tool", output)))
		}
	}
	prompt = append(prompt, provider.NewToolMessage(provider.ToolResultPart("call_2", "tool", &provider.ToolResultOutput{Type: provider.ToolOutputText, Text: "stable result", ProviderOptions: options})))
	prompt = append(prompt, provider.NewToolMessage(provider.ToolResultPart("call_3", "tool", &provider.ToolResultOutput{Type: provider.ToolOutputContent, Content: []provider.ToolResultContentValue{{Type: provider.ToolContentText, Text: "stable content", ProviderOptions: options}}})))
	return prompt
}

func checkPromptCacheBreakpoints(t *testing.T, input any) {
	t.Helper()
	count := 0
	var check func(any)
	check = func(value any) {
		switch value := value.(type) {
		case map[string]any:
			for key, child := range value {
				if key == "prompt_cache_breakpoint" {
					count++
					assert.Equal(t, map[string]any{"mode": "explicit", "ttl": "12h"}, child)
				} else {
					check(child)
				}
			}
		case []any:
			for _, child := range value {
				check(child)
			}
		}
	}
	check(input)
	assert.Equal(t, 16, count)
}
