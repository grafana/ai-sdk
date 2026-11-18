package openaicompatible

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestThoughtSignatureRequest(t *testing.T) {
	t.Parallel()

	m := &model{modelID: "test-model", providerName: "custom-provider.chat"}
	for _, tc := range []struct {
		name        string
		callOptions string
		partOptions string
		want        string
	}{
		{
			name:        "raw configured name",
			partOptions: `{"custom-provider":{"thoughtSignature":"raw"}}`,
			want:        "raw",
		},
		{
			name:        "camelCase configured key wins over raw and google",
			callOptions: `{"customProvider":{}}`,
			partOptions: `{"customProvider":{"thoughtSignature":"camel"},"custom-provider":{"thoughtSignature":"raw"},"google":{"thoughtSignature":"fallback"}}`,
			want:        "camel",
		},
		{
			name:        "google fallback without configured namespace",
			partOptions: `{"google":{"thoughtSignature":"fallback"}}`,
			want:        "fallback",
		},
		{
			name:        "google fallback for missing field; do not use unresolved alias",
			callOptions: `{"customProvider":{}}`,
			partOptions: `{"customProvider":{"other":true},"custom-provider":{"thoughtSignature":"raw"},"google":{"thoughtSignature":"fallback"}}`,
			want:        "fallback",
		},
		{
			name:        "google fallback for null field",
			partOptions: `{"custom-provider":{"thoughtSignature":null},"google":{"thoughtSignature":"fallback"}}`,
			want:        "fallback",
		},
		{
			name:        "explicit empty suppresses fallback",
			partOptions: `{"custom-provider":{"thoughtSignature":""},"google":{"thoughtSignature":"fallback"}}`,
		},
		{
			name:        "no signature",
			partOptions: `{}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			call := provider.ToolCallPart("call-1", "weather", json.RawMessage(`{}`))
			require.NoError(t, json.Unmarshal([]byte(tc.partOptions), &call.ProviderOptions))
			opts := provider.CallOptions{Prompt: []provider.Message{provider.NewAssistantMessage(call)}}
			if tc.callOptions != "" {
				require.NoError(t, json.Unmarshal([]byte(tc.callOptions), &opts.ProviderOptions))
			}
			_, body, _, err := m.prepareRequest(opts, false)
			require.NoError(t, err)
			assertRequestThoughtSignature(t, body, tc.want)
		})
	}
}

func TestThoughtSignatureRoundTrip(t *testing.T) {
	t.Parallel()

	var requests [][]byte
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}
		requests = append(requests, body)
		response := `{"choices":[{"message":{"role":"assistant","content":"done"},"finish_reason":"stop"}]}`
		if len(requests) == 1 {
			response = `{"choices":[{"message":{"role":"assistant","tool_calls":[{"id":"call-1","type":"function","function":{"name":"weather","arguments":"{}"},"extra_content":{"google":{"thought_signature":"returned"}}}]},"finish_reason":"tool_calls"}]}`
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(response))}, nil
	})}
	m := New("test-model", WithProviderName("custom-provider.chat"), WithHTTPClient(client))
	ctx := context.Background()
	initial := provider.UserText("What is the weather?")
	result, err := m.DoGenerate(ctx, provider.CallOptions{Prompt: []provider.Message{initial}})
	require.NoError(t, err)
	require.Len(t, result.Content, 1)
	returned := result.Content[0]
	require.Equal(t, provider.ContentToolCall, returned.Type)
	require.Contains(t, returned.ProviderMetadata, "custom-provider")
	call := provider.ToolCallPart(returned.ToolCallID, returned.ToolName, returned.Input)
	call.ProviderOptions = make(provider.ProviderOptions, len(returned.ProviderMetadata))
	for key, raw := range returned.ProviderMetadata {
		call.ProviderOptions[key] = provider.RawProviderOption{Key: key, Raw: raw}
	}
	toolResult := provider.ToolResultPart(returned.ToolCallID, returned.ToolName, &provider.ToolResultOutput{
		Type: provider.ToolOutputJSON,
		JSON: json.RawMessage(`{"weather":"sunny"}`),
	})
	_, err = m.DoGenerate(ctx, provider.CallOptions{Prompt: []provider.Message{
		initial,
		provider.NewAssistantMessage(call),
		provider.NewToolMessage(toolResult),
	}})
	require.NoError(t, err)
	require.Len(t, requests, 2)
	assertRequestThoughtSignature(t, requests[1], "returned")
}

func assertRequestThoughtSignature(t *testing.T, body []byte, want string) {
	t.Helper()
	var request struct {
		Messages []chatMessage `json:"messages"`
	}
	require.NoError(t, json.Unmarshal(body, &request))
	var calls []chatToolCall
	for _, msg := range request.Messages {
		if msg.Role == "assistant" {
			calls = append(calls, msg.ToolCalls...)
		}
	}
	require.Len(t, calls, 1)
	if want == "" {
		assert.Nil(t, calls[0].ExtraContent)
		return
	}
	assert.Equal(t, &extraContent{Google: &googleExtraContent{ThoughtSignature: want}}, calls[0].ExtraContent)
}
