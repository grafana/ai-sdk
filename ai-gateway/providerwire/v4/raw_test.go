package v4

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProjectRawValue(t *testing.T) {
	for _, tc := range []struct {
		name   string
		raw    string
		want   string
		absent bool
	}{
		{name: "no value", raw: "", absent: true},
		{name: "invalid JSON", raw: `{"type":`, absent: true},
		{name: "event without tools is unchanged", raw: `{"type":"content_block_delta","delta":{"text":"<b>"}}`, want: `{"type":"content_block_delta","delta":{"text":"<b>"}}`},
		{name: "non-object value is unchanged", raw: `["tools",1]`, want: `["tools",1]`},
		{
			name: "clean MCP echo is unchanged",
			raw:  `{"type":"response.created","response":{"tools":[{"type":"mcp","headers":null,"server_url":"https://mcp.example/mcp"}]}}`,
			want: `{"type":"response.created","response":{"tools":[{"type":"mcp","headers":null,"server_url":"https://mcp.example/mcp"}]}}`,
		},
		{
			name: "MCP credentials are removed and other fields kept",
			raw:  `{"type":"response.completed","sequence_number":12345678901234567890,"response":{"output_text":"a < b","tools":[{"type":"mcp","server_label":"docs","authorization":"secret-token","headers":{"Authorization":"Bearer secret-header"},"server_url":"https://user:secret-pass@mcp.example/mcp?API_KEY=secret-query&access_token=secret-access&region=eu"},{"type":"function","name":"lookup","headers":{"kept":"value"}}]}}`,
			want: `{"response":{"output_text":"a \u003c b","tools":[{"headers":null,"server_label":"docs","server_url":"https://mcp.example/mcp?region=eu","type":"mcp"},{"headers":{"kept":"value"},"name":"lookup","type":"function"}]},"sequence_number":12345678901234567890,"type":"response.completed"}`,
		},
		{
			name: "unparseable server URL is removed",
			raw:  `{"response":{"tools":[{"type":"mcp","server_url":"http://[::1"}]}}`,
			want: `{"response":{"tools":[{"type":"mcp"}]}}`,
		},
		{name: "invalid UTF-8", raw: "{\"delta\":\"\xff\"}", absent: true},
		{name: "duplicate key", raw: `{"response":{"tools":[{"type":"mcp","headers":{"a":"secret-header"}}]},"response":{}}`, absent: true},
		{name: "duplicate key after unescaping", raw: `{"tools":[{"type":"mcp","headers":{"a":"secret-header"},"type":"function"}]}`, absent: true},
		{name: "duplicate key in array element", raw: `[{"a":1},{"a":1,"a":2}]`, absent: true},
		{name: "repeated keys in sibling objects", raw: `[{"a":1},{"a":{"a":[]}}]`, want: `[{"a":1},{"a":{"a":[]}}]`},
		{name: "unescaped surrogate kept when nothing changes", raw: `{"note":"\ud800"}`, want: `{"note":"\ud800"}`},
		{
			name: "escaped tools key is projected",
			raw:  `{"response":{"tools":[{"type":"mcp","headers":{"Authorization":"Bearer secret-header"}}]}}`,
			want: `{"response":{"tools":[{"headers":null,"type":"mcp"}]}}`,
		},
		{
			name: "MCP definitions outside response are projected",
			raw:  `{"tools":[{"type":"mcp","authorization":"secret-token","server_url":null}]}`,
			want: `{"tools":[{"server_url":null,"type":"mcp"}]}`,
		},
		{
			name: "server URL with an unparseable query is removed",
			raw:  `{"tools":[{"type":"mcp","server_url":"https://mcp.example/mcp?api_key=secret-query;x=1"},{"type":"mcp","server_url":"https://mcp.example/mcp?api_key=secret-query%zz"}]}`,
			want: `{"tools":[{"type":"mcp"},{"type":"mcp"}]}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := projectRawValue(json.RawMessage(tc.raw))
			if tc.absent {
				assert.False(t, ok)
				return
			}
			require.True(t, ok)
			assert.Equal(t, tc.want, string(got))
			assert.NotContains(t, string(got), "secret")
		})
	}
}

func rawRequest(streaming bool, include string) *http.Request {
	body := `{"prompt":[]` + include + `}`
	if streaming {
		return streamRequest(body)
	}
	return validRequest(body)
}

func TestRuntimeRawOutput(t *testing.T) {
	t.Run("the flag reaches the model", func(t *testing.T) {
		for _, streaming := range []bool{false, true} {
			for _, tc := range []struct {
				name    string
				include string
				want    bool
			}{
				{name: "absent", include: ""},
				{name: "false", include: `,"includeRawChunks":false`},
				{name: "true", include: `,"includeRawChunks":true`, want: true},
			} {
				harness := newRuntimeHarness(t, testLimits())
				response := harness.serve(rawRequest(streaming, tc.include))
				require.Equal(t, http.StatusOK, response.Code, "%s streaming=%v: %s", tc.name, streaming, response.Body.String())
				assert.Equal(t, tc.want, harness.model.options.IncludeRawChunks, "%s streaming=%v", tc.name, streaming)
				assert.NotContains(t, response.Body.String(), `"type":"raw"`)
			}
		}
	})

	echo := `{"type":"response.created","response":{"tools":[{"type":"mcp","headers":{"Authorization":"Bearer secret-header"},"server_url":"https://mcp.example/mcp?api_key=secret-query"}]}}`
	// Adapters send stream-start first; a stream may also open with a raw part.
	parts := func(streamStart bool) []provider.StreamPart {
		var parts []provider.StreamPart
		if streamStart {
			parts = append(parts, provider.StreamPart{Type: provider.PartStreamStart})
		}
		return append(parts, []provider.StreamPart{
			{Type: provider.PartRaw, RawValue: json.RawMessage(echo)},
			{Type: provider.PartTextStart, ID: "a"},
			// A native frame that was not valid JSON reaches the Gateway without a value.
			{Type: provider.PartRaw},
			// Adapters check JSON syntax only, so invalid UTF-8 can arrive.
			{Type: provider.PartRaw, RawValue: json.RawMessage("{\"delta\":\"\xff\"}")},
			{Type: provider.PartTextDelta, ID: "a", Delta: "hello"},
			{Type: provider.PartRaw, RawValue: json.RawMessage(`{"type":"message_stop"}`)},
			{Type: provider.PartTextEnd, ID: "a"},
			finishPart(),
		}...)
	}

	t.Run("unrequested raw parts are dropped", func(t *testing.T) {
		for _, include := range []string{"", `,"includeRawChunks":false`} {
			harness := newRuntimeHarness(t, testLimits())
			harness.model.stream = func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
				// The adapter emits raw parts regardless of the request, as an
				// adapter may for events it does not otherwise represent.
				return &provider.StreamResult{Stream: makeStream(parts(true)...)}, nil
			}
			response := harness.serve(rawRequest(true, include))
			require.Equal(t, http.StatusOK, response.Code)
			body := response.Body.String()
			requireStreamBodyMatchesSchema(t, body)
			assert.NotContains(t, body, `"type":"raw"`)
			assert.Contains(t, body, `"delta":"hello"`)
			assert.Contains(t, body, `"type":"finish"`)
		}
	})

	t.Run("requested raw parts are written in order with credentials removed", func(t *testing.T) {
		for _, streamStart := range []bool{true, false} {
			t.Run(map[bool]string{true: "after stream-start", false: "as the first part"}[streamStart], func(t *testing.T) {
				testRequestedRawParts(t, parts(streamStart))
			})
		}
	})

	t.Run("an oversized raw event ends the stream like any oversized frame", func(t *testing.T) {
		limits := testLimits()
		limits.StreamFrameBytes = int64(minimumStreamFrameBytes()) + 64
		harness := newRuntimeHarness(t, limits)
		harness.model.stream = func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
			large := json.RawMessage(`{"text":"` + strings.Repeat("x", int(limits.StreamFrameBytes)) + `"}`)
			return &provider.StreamResult{Stream: makeStream(
				provider.StreamPart{Type: provider.PartRaw, RawValue: large},
				finishPart(),
			)}, nil
		}
		response := harness.serve(rawRequest(true, `,"includeRawChunks":true`))
		require.Equal(t, http.StatusOK, response.Code)
		body := response.Body.String()
		assert.Equal(t, 1, strings.Count(body, `"code":"internal_error"`))
		assert.NotContains(t, body, `"type":"raw"`)
		assert.NotContains(t, body, `"type":"finish"`)
	})
}

func testRequestedRawParts(t *testing.T, parts []provider.StreamPart) {
	t.Helper()
	harness := newRuntimeHarness(t, testLimits())
	harness.model.stream = func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
		return &provider.StreamResult{Stream: makeStream(parts...)}, nil
	}
	response := harness.serve(rawRequest(true, `,"includeRawChunks":true`))
	require.Equal(t, http.StatusOK, response.Code)
	body := response.Body.String()
	requireStreamBodyMatchesSchema(t, body)
	var types []string
	var raws []string
	for _, frame := range strings.Split(strings.TrimSuffix(body, "\n\n"), "\n\n") {
		var event struct {
			Type     string          `json:"type"`
			RawValue json.RawMessage `json:"rawValue"`
		}
		require.NoError(t, json.Unmarshal([]byte(strings.TrimPrefix(frame, "data: ")), &event))
		types = append(types, event.Type)
		if event.Type == "raw" {
			raws = append(raws, string(event.RawValue))
		}
	}
	assert.Equal(t, []string{"stream-start", "raw", "text-start", "text-delta", "raw", "text-end", "finish"}, types)
	assert.Equal(t, []string{
		`{"response":{"tools":[{"headers":null,"server_url":"https://mcp.example/mcp","type":"mcp"}]},"type":"response.created"}`,
		`{"type":"message_stop"}`,
	}, raws)
	assert.NotContains(t, body, "secret")
}
