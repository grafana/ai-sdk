package grafana

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

func TestDecodeGenerate_ProviderMetadata(t *testing.T) {
	for _, tc := range []struct {
		name, metadata, want string
		valid, tool          bool
	}{
		{"openai", `{"openai":{"itemId":"msg_1"}}`, `{"itemId":"msg_1"}`, true, false},
		{"anthropic tool", `{"anthropic":{"caller":{"type":"direct"}}}`, `{"caller":{"type":"direct"}}`, true, true},
		{"openai tool", `{"openai":{"itemId":"fc_1"}}`, `{"itemId":"fc_1"}`, true, true},
		{"unknown namespace", `{"private":{"key":"secret"}}`, "", false, false},
		{"unknown field", `{"openai":{"itemId":"msg_1","key":"secret"}}`, "", false, false},
		{"duplicate outer metadata", `{"private":{"key":"secret"}},"providerMetadata":{"openai":{"itemId":"msg_1"}}`, "", false, false},
		{"escaped duplicate outer metadata", `{"private":{"key":"secret"}},"providerMetad\u0061ta":{"openai":{"itemId":"msg_1"}}`, "", false, false},
		{"extra caller member", `{"anthropic":{"caller":{"type":"direct","key":"secret"}}}`, "", false, true},
		{"unsafe identifier", `{"openai":{"itemId":"Bearer secret"}}`, "", false, false},
		{"wrong caller", `{"anthropic":{"caller":{"type":"tool"}}}`, "", false, true},
		{"malformed", `{"openai":`, "", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := strings.Replace(unaryFixture, `"text":"hello"`, `"text":"hello","providerMetadata":`+tc.metadata, 1)
			if tc.tool {
				body = strings.Replace(unaryFixture, `{"type":"text","text":"hello"}`, `{"type":"tool-call","toolCallId":"call","toolName":"f","input":"{}","providerMetadata":`+tc.metadata+`}`, 1)
			}
			result, err := decodeGenerate([]byte(body))
			if !tc.valid {
				require.Error(t, err)
				assert.Nil(t, result)
				return
			}
			require.NoError(t, err)
			require.Len(t, result.Content, 2)
			for namespace, raw := range result.Content[0].ProviderMetadata {
				assert.JSONEq(t, tc.want, string(raw))
				assert.Contains(t, tc.metadata, namespace)
			}
			assert.Len(t, result.Content[0].ProviderMetadata, 1)
			assert.Nil(t, result.ProviderMetadata)
		})
	}
}

func TestDecodeStreamPart_ProviderMetadata(t *testing.T) {
	for _, tc := range []struct {
		name, event, namespace, want string
		valid                        bool
	}{
		{"text start", `{"type":"text-start","id":"msg_1","providerMetadata":{"openai":{"itemId":"msg_1"}}}`, "openai", `{"itemId":"msg_1"}`, true},
		{"text end", `{"type":"text-end","id":"msg_1","providerMetadata":{"openai":{"itemId":"msg_1"}}}`, "openai", `{"itemId":"msg_1"}`, true},
		{"tool call", `{"type":"tool-call","toolCallId":"call","toolName":"f","input":"{}","providerMetadata":{"anthropic":{"caller":{"type":"direct"}}}}`, "anthropic", `{"caller":{"type":"direct"}}`, true},
		{"tool result", `{"type":"tool-result","toolCallId":"call","toolName":"f","result":{},"providerMetadata":{"anthropic":{"caller":{"type":"direct"}}}}`, "anthropic", `{"caller":{"type":"direct"}}`, true},
		{"openai tool result", `{"type":"tool-result","toolCallId":"call","toolName":"f","result":{},"providerMetadata":{"openai":{"itemId":"fc_1"}}}`, "openai", `{"itemId":"fc_1"}`, true},
		{"unknown namespace", `{"type":"text-start","id":"a","providerMetadata":{"private":{"token":"secret"}}}`, "", "", false},
		{"unknown field", `{"type":"text-start","id":"a","providerMetadata":{"openai":{"itemId":"a","private":"secret"}}}`, "", "", false},
		{"duplicate outer metadata", `{"type":"text-start","id":"a","providerMetadata":{"private":{"key":"secret"}},"providerMetadata":{"openai":{"itemId":"a"}}}`, "", "", false},
		{"escaped duplicate outer metadata", `{"type":"text-start","id":"a","providerMetadata":{"private":{"key":"secret"}},"providerMetad\u0061ta":{"openai":{"itemId":"a"}}}`, "", "", false},
		{"extra caller", `{"type":"tool-call","toolCallId":"call","toolName":"f","input":"{}","providerMetadata":{"anthropic":{"caller":{"type":"direct","extra":true}}}}`, "", "", false},
		{"unexpected placement", `{"type":"finish","finishReason":{"unified":"stop"},"usage":{"inputTokens":{},"outputTokens":{}},"providerMetadata":{"openai":{"itemId":"a"}}}`, "", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			part, err := decodeStreamPart([]byte(tc.event))
			if !tc.valid {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.JSONEq(t, tc.want, string(part.ProviderMetadata[tc.namespace]))
		})
	}
	part, err := decodeStreamPart([]byte(`{"type":"text-start","id":"a","other":"ignored","other":"still ignored"}`))
	require.NoError(t, err)
	assert.Equal(t, provider.PartTextStart, part.Type)
	assert.Nil(t, part.ProviderMetadata)
	_, err = json.Marshal(part)
	require.NoError(t, err)
}

func TestProviderMetadata_DuplicateOuterFieldClosesStream(t *testing.T) {
	frames := sseFrame(`{"type":"text-start","id":"msg_1","providerMetadata":{"private":{"secret":"private-key"}},"providerMetadata":{"openai":{"itemId":"msg_1"}}}`) + sseFrame(finishEvent)
	body := &trackedBody{Reader: strings.NewReader(frames)}
	m := streamFromBody(t, body, nil)
	result, err := m.DoStream(context.Background(), provider.CallOptions{})
	require.NoError(t, err)
	parts := collectParts(t, result)
	require.Len(t, parts, 1)
	assert.Equal(t, provider.PartError, parts[0].Type)
	assert.NotContains(t, parts[0].APICallError.Message, "private-key")
	assert.True(t, body.closed)
}

func TestProviderMetadata_ClientByteLimits(t *testing.T) {
	unary := strings.Replace(unaryFixture, `"text":"hello"`, `"text":"hello","providerMetadata":{"openai":{"itemId":"msg_1"}}`, 1)
	frame := sseFrame(`{"type":"text-start","id":"msg_1","providerMetadata":{"openai":{"itemId":"msg_1"}}}`)
	for _, delta := range []int64{0, -1} {
		name := map[int64]string{0: "exact", -1: "one over"}[delta]
		t.Run(name+" unary", func(t *testing.T) {
			limits := DefaultLimits()
			limits.UnaryBytes = int64(len(unary)) + delta
			p := testProvider(t, func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, unary)
			}, &limits)
			m, err := p.LanguageModel("assistant")
			require.NoError(t, err)
			result, err := m.DoGenerate(context.Background(), provider.CallOptions{})
			if delta < 0 {
				require.Error(t, err)
				assert.Nil(t, result)
			} else {
				require.NoError(t, err)
				assert.JSONEq(t, `{"itemId":"msg_1"}`, string(result.Content[0].ProviderMetadata["openai"]))
			}
		})
		t.Run(name+" SSE event", func(t *testing.T) {
			limits := DefaultLimits()
			limits.StreamEventBytes = int64(len(frame)) + delta
			body := &trackedBody{Reader: strings.NewReader(frame)}
			m := streamFromBody(t, body, &limits)
			result, err := m.DoStream(context.Background(), provider.CallOptions{})
			require.NoError(t, err)
			parts := collectParts(t, result)
			require.Len(t, parts, 1)
			if delta < 0 {
				assert.Equal(t, provider.PartError, parts[0].Type)
			} else {
				assert.JSONEq(t, `{"itemId":"msg_1"}`, string(parts[0].ProviderMetadata["openai"]))
			}
			assert.True(t, body.closed)
		})
	}
}

func TestDecodeStreamPart_UnknownProviderMetadataClosesStream(t *testing.T) {
	frames := sseFrame(`{"type":"text-start","id":"msg_1","providerMetadata":{"openai":{"itemId":"msg_1"}}}`) +
		sseFrame(`{"type":"text-end","id":"msg_1","providerMetadata":{"openai":{"itemId":"msg_1","secret":"private-key"}}}`) + sseFrame(finishEvent)
	body := &trackedBody{Reader: strings.NewReader(frames)}
	m := streamFromBody(t, body, nil)
	result, err := m.DoStream(context.Background(), provider.CallOptions{Prompt: []provider.Message{}})
	require.NoError(t, err)
	parts := collectParts(t, result)
	require.Len(t, parts, 2)
	assert.Equal(t, provider.PartTextStart, parts[0].Type)
	assert.JSONEq(t, `{"itemId":"msg_1"}`, string(parts[0].ProviderMetadata["openai"]))
	assert.Equal(t, provider.PartError, parts[1].Type)
	assert.NotContains(t, parts[1].APICallError.Message, "private-key")
	assert.True(t, body.closed)
}
