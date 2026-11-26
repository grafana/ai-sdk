package logger

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMiddleware_HostedToolCapture(t *testing.T) {
	const auth = `"fake-tunnel-authorization"`
	const headers = `{"X-Custom":"fake-tunnel-header"}`
	tools := []provider.Tool{
		{Type: provider.ToolTypeProvider, ID: "openai.mcp", Name: "tunnel", Args: map[string]json.RawMessage{
			"tunnelId": json.RawMessage(`"tunnel_test"`), "authorization": json.RawMessage(auth), "headers": json.RawMessage(headers),
		}},
		{Type: provider.ToolTypeProvider, ID: "openai.code_interpreter", Name: "python", Args: map[string]json.RawMessage{
			"container": json.RawMessage(`{"networkPolicy":{"type":"allowlist","allowedDomains":["allowed.example.test"],"domainSecrets":[{"domain":"allowed.example.test","name":"KEY","value":"fake-log-domain-secret"}]}}`),
		}},
	}
	handler := newTestHandler()
	model := Wrap(&mockModel{generateFunc: func(_ context.Context, call provider.CallOptions) (*provider.GenerateResult, error) {
		assert.True(t, string(call.Tools[0].Args["authorization"]) == auth, "authorization changed on the forwarded request")
		assert.True(t, string(call.Tools[0].Args["headers"]) == headers, "headers changed on the forwarded request")
		assert.True(t, strings.Contains(string(call.Tools[1].Args["container"]), "fake-log-domain-secret"), "domain secret changed on the forwarded request")
		return &provider.GenerateResult{}, nil
	}}, Options{
		Logger: slog.New(handler), Capture: CaptureOptions{ToolInputs: true},
		Redactor: RedactorFunc(func(_ context.Context, _ EventKind, attrs []slog.Attr) []slog.Attr { return attrs }),
	})
	_, err := model.DoGenerate(t.Context(), provider.CallOptions{Tools: tools})
	require.NoError(t, err)
	assert.True(t, string(tools[0].Args["headers"]) == headers, "logging mutated caller arguments")
	encoded := handler.JSON(t)
	assert.Contains(t, encoded, "tunnel_test")
	assert.Contains(t, encoded, "allowed.example.test")
	assert.False(t, strings.Contains(encoded, "authorization") || strings.Contains(encoded, "X-Custom"), "tunnel credential fields appeared in logs")
	assert.False(t, strings.Contains(encoded, "fake-tunnel-") || strings.Contains(encoded, "fake-log-domain-secret"), "credential values appeared in logs")
}
