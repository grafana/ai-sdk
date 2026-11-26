package mantle

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

func TestNewResponses_HostedToolControls(t *testing.T) {
	for _, tc := range []struct {
		name    string
		stream  bool
		invalid bool
	}{
		{name: "generate"},
		{name: "stream", stream: true},
		{name: "generate rejects conflicting targets", invalid: true},
		{name: "stream rejects conflicting targets", stream: true, invalid: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls int
			client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				calls++
				var body struct {
					Tools json.RawMessage `json:"tools"`
				}
				require.NoError(t, json.NewDecoder(req.Body).Decode(&body))
				assert.JSONEq(t, `[
					{"type":"code_interpreter","container":{"type":"auto","memory_limit":"4g","network_policy":{"type":"disabled"}}},
					{"type":"mcp","server_label":"docs","tunnel_id":"tunnel_mantle_test","require_approval":"never"}
				]`, string(body.Tools))
				if tc.stream {
					const response = `{"id":"response_mantle_test","created_at":1,"model":"openai.gpt-5.6-luna","object":"response","status":"completed","output":[]}`
					return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(
						`data: {"type":"response.created","sequence_number":0,"response":` + response + "}\n\n" +
							`data: {"type":"response.completed","sequence_number":1,"response":` + response + "}\n\n")), Request: req}, nil
				}
				return jsonHTTPResponse(req, http.StatusOK, responseWithoutOutput), nil
			})}
			model, err := NewResponses(t.Context(), "openai.gpt-5.6-luna", Config{BaseURL: "https://mantle.example.test/openai/v1", SkipAuth: true},
				option.WithHTTPClient(client), option.WithMaxRetries(0))
			require.NoError(t, err)
			call := provider.CallOptions{Tools: []provider.Tool{
				{Type: provider.ToolTypeProvider, ID: "openai.code_interpreter", Name: "python", Args: map[string]json.RawMessage{
					"container": json.RawMessage(`{"memoryLimit":"4g","networkPolicy":{"type":"disabled"}}`),
				}},
				{Type: provider.ToolTypeProvider, ID: "openai.mcp", Name: "docs", Args: map[string]json.RawMessage{
					"serverLabel": json.RawMessage(`"docs"`), "tunnelId": json.RawMessage(`"tunnel_mantle_test"`),
				}},
			}}
			if tc.invalid {
				call.Tools[1].Args["connectorId"] = json.RawMessage(`"connector_mantle_test"`)
			}
			if tc.stream {
				result, err := model.DoStream(t.Context(), call)
				if tc.invalid {
					require.ErrorContains(t, err, "tunnelId cannot be combined")
					assert.Zero(t, calls)
					return
				}
				require.NoError(t, err)
				var attributed bool
				for part := range result.Stream {
					assert.NotEqual(t, provider.PartError, part.Type)
					if part.Type == provider.PartResponseMeta {
						assert.Equal(t, "bedrock-mantle.responses", part.Provider)
						attributed = true
					}
				}
				assert.True(t, attributed)
			} else {
				result, err := model.DoGenerate(t.Context(), call)
				if tc.invalid {
					require.ErrorContains(t, err, "tunnelId cannot be combined")
					assert.Zero(t, calls)
					return
				}
				require.NoError(t, err)
				assert.Equal(t, "bedrock-mantle.responses", result.Response.Provider)
			}
			assert.Equal(t, 1, calls)
		})
	}
}
