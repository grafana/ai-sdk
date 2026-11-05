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
			defer res.Body.Close()
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
	server := httptest.NewServer(testHandler(t, f))
	defer server.Close()
	res, err := http.Post(server.URL+Path, "application/json", strings.NewReader(strings.TrimSuffix(basic, "}")+`,"stream":true}`))
	require.NoError(t, err)
	defer res.Body.Close()
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
