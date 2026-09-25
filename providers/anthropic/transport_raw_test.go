package anthropic

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDoStream_RawEvents(t *testing.T) {
	for _, tc := range []struct {
		name       string
		includeRaw bool
	}{
		{name: "omitted"},
		{name: "false", includeRaw: false},
		{name: "true", includeRaw: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			model, closeServer := newSSEErrorModel(t, "api_error", true)
			defer closeServer()
			result, err := model.DoStream(context.Background(), provider.CallOptions{
				Prompt: []provider.Message{provider.UserText("hello")}, IncludeRawChunks: tc.includeRaw,
			})
			require.NoError(t, err)
			var raw []provider.StreamPart
			var parts []provider.StreamPart
			for part := range result.Stream {
				parts = append(parts, part)
				if part.Type == provider.PartRaw {
					raw = append(raw, part)
				}
			}
			require.NotEmpty(t, parts)
			assert.Equal(t, provider.PartStreamStart, parts[0].Type)
			assert.Equal(t, provider.PartError, parts[len(parts)-1].Type)
			if tc.includeRaw {
				require.Len(t, raw, 4)
				assert.JSONEq(t, `{"type":"message_start","message":{"id":"msg_test","type":"message","role":"assistant","content":[],"model":"claude-test","stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":1,"output_tokens":0}}}`, string(raw[0].RawValue))
				assert.Contains(t, string(raw[1].RawValue), `"type":"content_block_start"`)
				assert.Contains(t, string(raw[2].RawValue), `"type":"content_block_delta"`)
				assert.JSONEq(t, `{"type":"error","error":{"type":"api_error","message":"failed"}}`, string(raw[3].RawValue))
				assert.Equal(t, provider.PartRaw, parts[len(parts)-2].Type)
			} else {
				assert.Empty(t, raw)
			}
		})
	}
}

func TestDoStream_InitialErrorDoesNotExposeRaw(t *testing.T) {
	m, closeServer := newSSEErrorModel(t, "api_error", false)
	defer closeServer()
	result, err := m.DoStream(context.Background(), provider.CallOptions{
		Prompt: []provider.Message{provider.UserText("hello")}, IncludeRawChunks: true,
	})
	require.Error(t, err)
	assert.Nil(t, result)
}

func TestDoStream_RawBeforeFirstVisible(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "event: ping\r\ndata: {\"type\":\"ping\"}\r\n\r\n")
		_, _ = io.WriteString(w, "event: message_start\r\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_test\",\"type\":\"message\",\"role\":\"assistant\",\"content\":[],\"model\":\"claude-test\",\"stop_reason\":null,\"stop_sequence\":null,\"usage\":{\"input_tokens\":1,\"output_tokens\":0}}}\r\n\r\n")
	}))
	defer server.Close()
	m := newTransportTestModel(t, server, false)
	maxTokens := 64
	result, err := m.DoStream(context.Background(), provider.CallOptions{
		Prompt: []provider.Message{provider.UserText("hello")}, MaxOutputTokens: &maxTokens, IncludeRawChunks: true,
	})
	require.NoError(t, err)
	var parts []provider.StreamPart
	for part := range result.Stream {
		parts = append(parts, part)
	}
	require.GreaterOrEqual(t, len(parts), 4)
	assert.Equal(t, provider.PartStreamStart, parts[0].Type)
	assert.Equal(t, provider.PartRaw, parts[1].Type)
	assert.JSONEq(t, `{"type":"ping"}`, string(parts[1].RawValue))
	assert.Equal(t, provider.PartRaw, parts[2].Type)
	assert.Equal(t, provider.PartResponseMeta, parts[3].Type)
}

func TestDoStream_SDKHiddenRawFrames(t *testing.T) {
	for _, tc := range []struct {
		name  string
		frame string
	}{
		{name: "ping", frame: "event: ping\ndata: {\"type\":\"ping\"}\n\n"},
		{name: "malformed ping", frame: "event: ping\ndata: not JSON\n\n"},
		{name: "unknown event", frame: "event: future_event\ndata: {\"type\":\"future_event\"}\n\n"},
		{name: "malformed", frame: "event: message_delta\ndata: not JSON\n\n"},
		{name: "bare data", frame: "event: message_delta\ndata\n\n"},
		{name: "error", frame: "event: error\ndata: {\"type\":\"error\",\"error\":{\"type\":\"api_error\",\"message\":\"failed\"}}\n\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = io.WriteString(w, "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_test\",\"type\":\"message\",\"role\":\"assistant\",\"content\":[],\"model\":\"claude-test\",\"stop_reason\":null,\"stop_sequence\":null,\"usage\":{\"input_tokens\":1,\"output_tokens\":0}}}\n\n")
				_, _ = io.WriteString(w, tc.frame)
			}))
			defer server.Close()
			m := newTransportTestModel(t, server, false)
			maxTokens := 64
			result, err := m.DoStream(context.Background(), provider.CallOptions{
				Prompt: []provider.Message{provider.UserText("hello")}, MaxOutputTokens: &maxTokens, IncludeRawChunks: true,
			})
			require.NoError(t, err)
			var raw []provider.StreamPart
			var parts []provider.StreamPart
			for part := range result.Stream {
				parts = append(parts, part)
				if part.Type == provider.PartRaw {
					raw = append(raw, part)
				}
			}
			require.Len(t, raw, 2)
			if tc.name == "ping" || tc.name == "malformed ping" || tc.name == "unknown event" {
				assert.Equal(t, provider.PartRaw, parts[len(parts)-1].Type)
				switch tc.name {
				case "ping":
					assert.JSONEq(t, `{"type":"ping"}`, string(raw[1].RawValue))
				case "malformed ping":
					assert.Nil(t, raw[1].RawValue)
				case "unknown event":
					assert.JSONEq(t, `{"type":"future_event"}`, string(raw[1].RawValue))
				}
				for _, part := range parts {
					assert.NotEqual(t, provider.PartError, part.Type)
				}
			} else {
				assert.Equal(t, provider.PartRaw, parts[len(parts)-2].Type)
				assert.Equal(t, provider.PartError, parts[len(parts)-1].Type)
				if tc.name == "malformed" || tc.name == "bare data" {
					assert.Nil(t, raw[1].RawValue)
				} else {
					assert.JSONEq(t, `{"type":"error","error":{"type":"api_error","message":"failed"}}`, string(raw[1].RawValue))
				}
			}
		})
	}
}
