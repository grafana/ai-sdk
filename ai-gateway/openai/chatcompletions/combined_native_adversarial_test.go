package chatcompletions

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
	"github.com/grafana/ai-sdk/provider"
	compatible "github.com/grafana/ai-sdk/providers/openai-compatible"
	openaisdk "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const combinedModeration = `{"input":{"type":"moderation_results","model":"omni","results":[]},"output":{"type":"error","code":"unavailable","message":"retry later"}}`
const combinedUsage = `{"prompt_tokens":2,"completion_tokens":4,"total_tokens":6,"prompt_tokens_details":{"cache_write_tokens":0,"image_tokens":0,"text_tokens":2},"completion_tokens_details":{"text_tokens":4}}`

func TestOfficialGoCombinedNativeChoicesMetadata(t *testing.T) {
	native := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var body map[string]json.RawMessage
		require.NoError(t, json.NewDecoder(req.Body).Decode(&body))
		assert.JSONEq(t, `2`, string(body["n"]))
		assert.JSONEq(t, `{"mode":"explicit","ttl":"30m"}`, string(body["prompt_cache_options"]))
		assert.Contains(t, string(body["messages"]), `"prompt_cache_breakpoint":{"mode":"explicit"}`)
		assert.Contains(t, string(body["tools"]), `"type":"custom"`)
		if string(body["stream"]) != "true" {
			_, _ = fmt.Fprintf(w, `{"id":"native","model":"native-model","created":123,"service_tier":"priority","system_fingerprint":"fp","choices":[{"index":1,"message":{"content":null,"tool_calls":[{"id":"same","type":"custom","custom":{"name":"echo","input":"b"}}]},"finish_reason":"tool_calls"},{"index":0,"message":{"content":null,"tool_calls":[{"id":"same","type":"custom","custom":{"name":"echo","input":"a"}}]},"finish_reason":"tool_calls"}],"usage":%s,"moderation":%s}`, combinedUsage, combinedModeration)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "data: {\"id\":\"native\",\"model\":\"native-model\",\"created\":123,\"service_tier\":\"priority\",\"system_fingerprint\":\"fp\",\"choices\":[{\"index\":0,\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"same\",\"type\":\"custom\",\"custom\":{\"name\":\"echo\",\"input\":\"a\"}}]}},{\"index\":1,\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"same\",\"type\":\"custom\",\"custom\":{\"name\":\"echo\",\"input\":\"b\"}}]}}]}\n\n")
		_, _ = fmt.Fprint(w, "data: {\"choices\":[{\"index\":1,\"delta\":{},\"finish_reason\":\"tool_calls\"},{\"index\":0,\"delta\":{},\"finish_reason\":\"tool_calls\"}]}\n\n")
		_, _ = fmt.Fprintf(w, "data: {\"choices\":[],\"usage\":%s,\"moderation\":%s}\n\ndata: [DONE]\n\n", combinedUsage, combinedModeration)
	}))
	defer native.Close()
	model := compatible.New("native", compatible.WithBaseURL(native.URL), compatible.WithProviderName("custom.chat"))
	server := httptest.NewServer(nativeChoiceHandler(t, execution.Translate(model, execution.Target{Type: execution.TargetOpenAICompatible, ProviderName: "custom.chat"})))
	defer server.Close()
	client := openaisdk.NewClient(option.WithBaseURL(server.URL+"/v1"), option.WithAPIKey("test"), option.WithMaxRetries(0))
	var params openaisdk.ChatCompletionNewParams
	require.NoError(t, json.Unmarshal([]byte(`{"model":"alias","n":2,"messages":[{"role":"user","content":[{"type":"text","text":"hi","prompt_cache_breakpoint":{"mode":"explicit"}}]}],"prompt_cache_options":{"mode":"explicit","ttl":"30m"},"tools":[{"type":"custom","custom":{"name":"echo","format":{"type":"text"}}}],"moderation":{"model":"omni"}}`), &params))
	out, err := client.Chat.Completions.New(t.Context(), params)
	require.NoError(t, err)
	require.Len(t, out.Choices, 2)
	assert.Equal(t, "priority", string(out.ServiceTier))
	assert.Equal(t, "fp", out.SystemFingerprint) //nolint:staticcheck // Verify compatibility with the deprecated public Chat field.
	assert.Equal(t, int64(6), out.Usage.TotalTokens)
	byIndex := map[int64]string{}
	for _, c := range out.Choices {
		byIndex[c.Index] = c.Message.ToolCalls[0].AsCustom().Custom.Input
	}
	assert.Equal(t, map[int64]string{0: "a", 1: "b"}, byIndex)
	assert.JSONEq(t, combinedModeration, out.Moderation.RawJSON())
	params.StreamOptions = openaisdk.ChatCompletionStreamOptionsParam{IncludeUsage: openaisdk.Bool(true), IncludeObfuscation: openaisdk.Bool(true)}
	stream := client.Chat.Completions.NewStreaming(t.Context(), params)
	collected := map[int64]string{}
	finishes := 0
	usages := 0
	moderations := 0
	for stream.Next() {
		c := stream.Current()
		var raw struct {
			Obfuscation string          `json:"obfuscation"`
			Moderation  json.RawMessage `json:"moderation"`
			Choices     []struct {
				Index int64
				Delta struct {
					ToolCalls []struct {
						Custom struct {
							Input string `json:"input"`
						} `json:"custom"`
					} `json:"tool_calls"`
				}
				Finish string `json:"finish_reason"`
			}
		}
		require.NoError(t, json.Unmarshal([]byte(c.RawJSON()), &raw))
		assert.NotEmpty(t, raw.Obfuscation)
		if len(raw.Moderation) > 0 {
			moderations++
			assert.JSONEq(t, combinedModeration, string(raw.Moderation))
		}
		for _, choice := range raw.Choices {
			for _, call := range choice.Delta.ToolCalls {
				collected[choice.Index] += call.Custom.Input
			}
			if choice.Finish != "" {
				finishes++
			}
		}
		if len(c.Choices) == 0 {
			usages++
			assert.Equal(t, int64(6), c.Usage.TotalTokens)
			assert.Contains(t, c.RawJSON(), `"cache_write_tokens":0`)
		}
	}
	require.NoError(t, stream.Err())
	require.NoError(t, stream.Close())
	assert.Equal(t, byIndex, collected)
	assert.Equal(t, 2, finishes)
	assert.Equal(t, 1, usages)
	assert.Greater(t, moderations, 0)
}

func TestEveryNativeChoiceValidatesCompleteJSON(t *testing.T) {
	for _, invalid := range []bool{false, true} {
		for _, streaming := range []bool{false, true} {
			t.Run(fmt.Sprintf("invalid=%t/stream=%t", invalid, streaming), func(t *testing.T) {
				texts := []string{`{"n":0}`, `{"n":1}`}
				if invalid {
					texts[1] = `{"n":`
				}
				f := &fakeModel{generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
					r := nativeChoiceResult()
					for i := range r.Choices {
						r.Choices[i].Content = []provider.GenerateContentPart{{Type: provider.ContentText, Text: texts[r.Choices[i].Index]}}
						r.Choices[i].FinishReason.Unified = provider.FinishReasonStop
					}
					return r, nil
				}, stream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
					parts := []provider.StreamPart{{Type: provider.PartStreamStart}}
					for i, text := range texts {
						parts = append(parts, indexedPart(i, provider.StreamPart{Type: provider.PartTextStart, ID: "t"}), indexedPart(i, provider.StreamPart{Type: provider.PartTextDelta, ID: "t", Delta: text}), indexedPart(i, provider.StreamPart{Type: provider.PartTextEnd, ID: "t"}), indexedPart(i, provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop}, Usage: &provider.Usage{}}))
					}
					parts = append(parts, provider.StreamPart{Type: provider.PartChoicesFinish, Usage: &textResult().Usage})
					return partsStream(parts...), nil
				}}
				h := nativeChoiceHandler(t, execution.Translate(f, execution.Target{Type: execution.TargetOpenAICompatible}))
				body := strings.TrimSuffix(choiceRequest(streaming), "}") + `,"response_format":{"type":"json_object"}}`
				req := httptest.NewRequest(http.MethodPost, Path, strings.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				out := httptest.NewRecorder()
				h.ServeHTTP(out, req)
				if invalid {
					assert.Contains(t, out.Body.String(), `"error":`)
					assert.NotContains(t, out.Body.String(), "[DONE]")
					if !streaming {
						assert.Equal(t, 502, out.Code)
					}
				} else {
					assert.Equal(t, 200, out.Code)
					assert.NotContains(t, out.Body.String(), `"error":`)
				}
			})
		}
	}
}
