package anthropic

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModel_InvalidToolDoesNotInvokeProvider(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests++ }))
	defer server.Close()
	model := New("key", "claude-sonnet-4-6", WithRequestOptions(option.WithBaseURL(server.URL), option.WithHTTPClient(server.Client())))
	options := provider.CallOptions{Prompt: []provider.Message{provider.UserText("hello")}, Tools: []provider.Tool{{Type: provider.ToolTypeProvider, ID: "anthropic.web_search_20250305", Name: "search", Args: map[string]json.RawMessage{}, Strict: boolPtr(false)}}}
	_, err := model.DoGenerate(context.Background(), options)
	require.Error(t, err)
	_, err = model.DoStream(context.Background(), options)
	require.Error(t, err)
	assert.Zero(t, requests)
}
