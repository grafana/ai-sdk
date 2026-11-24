package mantle

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	openaiprovider "github.com/grafana/ai-sdk/providers/openai"
	"github.com/openai/openai-go/v3/option"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewResponses_PromptCachePrewarm(t *testing.T) {
	// Synthetic prewarm response; no live provider or recorded fixture is used.
	const usage = `{"input_tokens":2048,"input_tokens_details":{"cached_tokens":0,"cache_write_tokens":2048,"cache_write_tokens_details":{"ttl_30m_tokens":0,"ttl_12h_tokens":2048}},"output_tokens":0,"output_tokens_details":{"reasoning_tokens":0},"total_tokens":2048}`
	const response = `{"id":"resp_prewarm","status":"completed","output":[],"usage":` + usage + `}`
	for _, ttl := range []openaiprovider.PromptCacheTTL{openaiprovider.PromptCacheTTL30m, openaiprovider.PromptCacheTTL12h, "24h"} {
		t.Run(string(ttl), func(t *testing.T) {
			var bodies []map[string]any
			client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				var body map[string]any
				require.NoError(t, json.NewDecoder(req.Body).Decode(&body))
				bodies = append(bodies, body)
				if body["stream"] == true {
					return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader("data: " + `{"type":"response.completed","sequence_number":0,"response":` + response + "}\n\n")), Request: req}, nil
				}
				return jsonHTTPResponse(req, http.StatusOK, response), nil
			})}
			model, err := NewResponses(t.Context(), "openai.gpt-6.1-sol", Config{BaseURL: "https://provider.example.test/openai/v1", SkipAuth: true}, option.WithHTTPClient(client), option.WithMaxRetries(0))
			require.NoError(t, err)
			prewarm := true
			part := provider.TextPart("Stable prefix")
			part.ProviderOptions = provider.BuildProviderOptions(openaiprovider.OpenAIPartOptions{PromptCacheBreakpoint: &openaiprovider.PromptCacheBreakpoint{Mode: "explicit", TTL: openaiprovider.PromptCacheTTL12h}})
			opts := provider.CallOptions{Prompt: []provider.Message{provider.NewUserMessage(part)}, ProviderOptions: provider.BuildProviderOptions(openaiprovider.OpenAIResponsesOptions{PromptCacheOptions: &openaiprovider.PromptCacheOptions{TTL: ttl, Prewarm: &prewarm}})}
			generated, generateErr := model.DoGenerate(t.Context(), opts)
			streamed, streamErr := model.DoStream(t.Context(), opts)
			if ttl == "24h" {
				require.ErrorContains(t, generateErr, "promptCacheOptions.ttl")
				require.ErrorContains(t, streamErr, "promptCacheOptions.ttl")
				assert.Empty(t, bodies)
				return
			}
			require.NoError(t, generateErr)
			require.NoError(t, streamErr)
			assert.Empty(t, generated.Content)
			assert.Equal(t, provider.FinishReasonStop, generated.FinishReason.Unified)
			assert.JSONEq(t, usage, string(generated.Usage.Raw))
			require.NotNil(t, generated.Usage.InputTokens.CacheWrite)
			assert.Equal(t, 2048, *generated.Usage.InputTokens.CacheWrite)
			finishes := 0
			for part := range streamed.Stream {
				switch part.Type {
				case provider.PartFinish:
					finishes++
					assert.Equal(t, provider.FinishReasonStop, part.FinishReason.Unified)
					require.NotNil(t, part.Usage)
					assert.Equal(t, generated.Usage, *part.Usage)
				case provider.PartStreamStart:
				default:
					t.Errorf("unexpected prewarm stream part: %s", part.Type)
				}
			}
			assert.Equal(t, 1, finishes)
			require.Len(t, bodies, 2)
			for _, body := range bodies {
				assert.Equal(t, map[string]any{"ttl": string(ttl), "prewarm": true}, body["prompt_cache_options"])
				assert.Equal(t, []any{map[string]any{"role": "user", "content": []any{map[string]any{"type": "input_text", "text": "Stable prefix", "prompt_cache_breakpoint": map[string]any{"mode": "explicit", "ttl": "12h"}}}}}, body["input"])
			}
		})
	}
}
