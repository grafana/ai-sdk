package bedrock

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModel_InvalidToolDoesNotInvokeProvider(t *testing.T) {
	requests := 0
	model := newStubBedrockProvider(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests++ }))
	options := provider.CallOptions{Prompt: []provider.Message{provider.UserText("hello")}, Tools: []provider.Tool{{Type: provider.ToolTypeFunction, Name: "search", InputSchema: json.RawMessage(`{}`), Args: map[string]json.RawMessage{}}}}
	_, err := model.DoGenerate(context.Background(), options)
	require.Error(t, err)
	_, err = model.DoStream(context.Background(), options)
	require.Error(t, err)
	assert.Zero(t, requests)
}
