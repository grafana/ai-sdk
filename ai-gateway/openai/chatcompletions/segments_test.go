package chatcompletions

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Synthetic native segments exercise the runtime contract, including reused IDs.
func TestMultipleNativeSegments(t *testing.T) {
	for _, missingUsage := range []bool{false, true} {
		t.Run(map[bool]string{false: "known usage", true: "unknown usage"}[missingUsage], func(t *testing.T) {
			first := textParts()
			first = append(first, provider.StreamPart{Type: provider.PartRaw, RawValue: json.RawMessage(`{"native":"trailer"}`)})
			second := append([]provider.StreamPart{{Type: provider.PartResponseMeta, ResponseID: "second"}}, textParts()[1:]...)
			if missingUsage {
				second[len(second)-1].Usage = &provider.Usage{}
			}
			f := &fakeModel{stream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
				return partsStream(append(first, second...)...), nil
			}}
			server := httptest.NewServer(testHandler(t, f))
			defer server.Close()
			res, err := http.Post(server.URL+Path, "application/json", strings.NewReader(strings.TrimSuffix(basic, "}")+`,"stream":true,"stream_options":{"include_usage":true}}`))
			require.NoError(t, err)
			defer func() { _ = res.Body.Close() }()
			body, err := io.ReadAll(res.Body)
			require.NoError(t, err)
			require.NotContains(t, string(body), `"error"`)
			assert.Equal(t, 1, strings.Count(string(body), "[DONE]"))
			assert.Equal(t, 1, strings.Count(string(body), `"finish_reason":"stop"`))
			assert.Equal(t, 2, strings.Count(string(body), `"content":"hello"`))
			if missingUsage {
				assert.NotContains(t, string(body), `"total_tokens"`)
			} else {
				assert.Contains(t, string(body), `"total_tokens":10`)
			}
		})
	}
}

func TestErrorAfterNativeFinish(t *testing.T) {
	parts := append(textParts(), provider.StreamPart{Type: provider.PartError, APICallError: provider.NewAPICallError(provider.APICallErrorOptions{Message: "native secret", StatusCode: 503})})
	f := &fakeModel{stream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
		return partsStream(parts...), nil
	}}
	server := httptest.NewServer(testHandler(t, f, "native secret"))
	defer server.Close()
	res, err := http.Post(server.URL+Path, "application/json", strings.NewReader(strings.TrimSuffix(basic, "}")+`,"stream":true}`))
	require.NoError(t, err)
	defer func() { _ = res.Body.Close() }()
	body, err := io.ReadAll(res.Body)
	require.NoError(t, err)
	assert.Contains(t, string(body), `"error"`)
	assert.NotContains(t, string(body), "[DONE]")
	assert.NotContains(t, string(body), "native secret")
}

func TestSegmentUsageOverflow(t *testing.T) {
	total := &usage{Total: int(^uint(0) >> 1)}
	require.Error(t, addUsage(total, &usage{Total: 1}))
}

func TestAggregatedSegmentValidation(t *testing.T) {
	tools := []provider.StreamPart{{Type: provider.PartResponseMeta}, {Type: provider.PartToolCall, ToolCallID: "same", ToolName: "weather", Input: `{}`}, {Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonToolCalls}, Usage: &provider.Usage{}}}
	text := append([]provider.StreamPart{{Type: provider.PartResponseMeta}}, textParts()[1:]...)
	jsonSegment := append([]provider.StreamPart(nil), text...)
	jsonSegment[2].Delta = "{}"
	for _, tc := range []struct {
		name, extra string
		parts       []provider.StreamPart
		fail        bool
		finish      string
	}{
		{"concatenated JSON", `,"response_format":{"type":"json_object"}`, append(append([]provider.StreamPart(nil), jsonSegment...), jsonSegment...), true, ""},
		{"tools then text", `,"tools":[{"type":"function","function":{"name":"weather"}}]`, append(append([]provider.StreamPart(nil), tools...), text...), false, "tool_calls"},
		{"parallel across segments", `,"tools":[{"type":"function","function":{"name":"weather"}}],"parallel_tool_calls":false`, append(append([]provider.StreamPart(nil), tools...), tools...), true, ""},
		{"duplicate tool IDs rejected by runtime", `,"tools":[{"type":"function","function":{"name":"weather"}}]`, append(append([]provider.StreamPart(nil), tools...), tools...), true, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &fakeModel{stream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
				return partsStream(tc.parts...), nil
			}}
			req := httptest.NewRequest("POST", Path, strings.NewReader(strings.TrimSuffix(basic, "}")+`,"stream":true`+tc.extra+"}"))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			testHandler(t, f).ServeHTTP(w, req)
			body := w.Body.String()
			if tc.fail {
				assert.Contains(t, body, `"error"`)
				assert.NotContains(t, body, "[DONE]")
			} else {
				assert.NotContains(t, body, `"error"`)
				assert.Contains(t, body, `"finish_reason":"`+tc.finish+`"`)
				assert.Contains(t, body, "[DONE]")
			}
		})
	}
}
