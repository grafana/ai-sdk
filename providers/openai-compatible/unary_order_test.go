package openaicompatible

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDoGenerate_ContentOrder(t *testing.T) {
	t.Parallel()
	reasoning := provider.GenerateContentPart{Type: provider.ContentReasoning, Text: "thinking"}
	text := provider.GenerateContentPart{Type: provider.ContentText, Text: "answer"}
	tests := []struct {
		name    string
		message string
		want    []provider.GenerateContentPart
	}{
		{"reasoning and text", `"content":"answer","reasoning_content":"thinking"`, []provider.GenerateContentPart{reasoning, text}},
		{"primary reasoning wins", `"content":"answer","reasoning_content":"thinking","reasoning":"ignored"`, []provider.GenerateContentPart{reasoning, text}},
		{"legacy reasoning", `"content":"answer","reasoning":"thinking"`, []provider.GenerateContentPart{reasoning, text}},
		{"empty primary retains fallback", `"content":"answer","reasoning_content":"","reasoning":"thinking"`, []provider.GenerateContentPart{reasoning, text}},
		{"null primary retains fallback", `"content":"answer","reasoning_content":null,"reasoning":"thinking"`, []provider.GenerateContentPart{reasoning, text}},
		{"empty text", `"content":"","reasoning_content":"thinking"`, []provider.GenerateContentPart{reasoning}},
		{"null text", `"content":null,"reasoning_content":"thinking"`, []provider.GenerateContentPart{reasoning}},
		{"empty reasoning", `"content":"answer","reasoning_content":"","reasoning":""`, []provider.GenerateContentPart{text}},
		{"empty content", `"content":null`, []provider.GenerateContentPart{}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant",` + tc.message + `},"finish_reason":"stop"}]}`))
			}))
			defer server.Close()
			result, err := New("test-model", WithBaseURL(server.URL)).DoGenerate(context.Background(), provider.CallOptions{
				Prompt: []provider.Message{provider.UserText("hi")},
			})
			require.NoError(t, err)
			assert.Equal(t, tc.want, result.Content)
		})
	}
}
