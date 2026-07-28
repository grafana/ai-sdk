package openai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/openai/openai-go/v3/option"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTransportSpike_SimpleTextRequest verifies that the openai-go SDK produces
// a Responses request body matching the upstream wire shape for a simple text
// request. This de-risks the SDK-vs-conformance decision (design D1).
func TestTransportSpike_SimpleTextRequest(t *testing.T) {
	var capturedBody []byte
	var capturedPath string
	var capturedAuth string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		capturedAuth = r.Header.Get("Authorization")
		capturedBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id": "resp_123",
			"created_at": 1700000000,
			"model": "gpt-4o",
			"object": "response",
			"status": "completed",
			"output": [
				{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"Hello!","annotations":[]}]}
			],
			"usage": {"input_tokens": 5, "output_tokens": 2, "total_tokens": 7}
		}`))
	}))
	defer srv.Close()

	m := NewResponses("test-key", "gpt-4o",
		WithRequestOptions(option.WithBaseURL(srv.URL), option.WithHTTPClient(srv.Client())),
	)

	res, err := m.DoGenerate(context.Background(), provider.CallOptions{
		Prompt: []provider.Message{
			provider.NewSystemMessage("You are helpful."),
			provider.UserText("Hi"),
		},
		Temperature: ptr(0.7),
	})
	require.NoError(t, err)
	require.NotNil(t, res)

	// Path + auth.
	assert.Equal(t, "/responses", capturedPath)
	assert.Equal(t, "Bearer test-key", capturedAuth)

	// Body shape.
	var body map[string]any
	require.NoError(t, json.Unmarshal(capturedBody, &body))
	t.Logf("captured request body: %s", string(capturedBody))

	assert.Equal(t, "gpt-4o", body["model"])
	assert.Equal(t, 0.7, body["temperature"])

	input, ok := body["input"].([]any)
	require.True(t, ok, "input must be an array")
	require.Len(t, input, 2)

	sys := input[0].(map[string]any)
	assert.Equal(t, "system", sys["role"])

	user := input[1].(map[string]any)
	assert.Equal(t, "user", user["role"])
	userContent := user["content"].([]any)
	require.Len(t, userContent, 1)
	textPart := userContent[0].(map[string]any)
	assert.Equal(t, "input_text", textPart["type"])
	assert.Equal(t, "Hi", textPart["text"])

	// Response conversion produced text content.
	require.Len(t, res.Content, 1)
	assert.Equal(t, provider.ContentText, res.Content[0].Type)
	assert.Equal(t, "Hello!", res.Content[0].Text)
	assert.Equal(t, "resp_123", res.Response.ID)
}

func ptr[T any](v T) *T { return &v }
