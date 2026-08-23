package v4

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/grafana/ai-sdk/provider"
	"github.com/grafana/ai-sdk/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func streamRequest(body string) *http.Request {
	req := validRequest(body)
	req.Header.Set(HeaderStreaming, "true")
	return req
}

func makeStream(parts ...provider.StreamPart) <-chan provider.StreamPart {
	stream := make(chan provider.StreamPart, len(parts))
	for _, part := range parts {
		stream <- part
	}
	close(stream)
	return stream
}

func minimumStreamFrameBytes() int {
	largest := 0
	for _, frame := range [][]byte{
		canonicalEmptyStartFrame,
		canonicalRateLimitStreamErrorFrame,
		canonicalOverloadStreamErrorFrame,
		canonicalDependencyStreamErrorFrame,
		canonicalUpstreamStreamErrorFrame,
		canonicalTimeoutStreamErrorFrame,
		canonicalCancellationStreamErrorFrame,
		canonicalInternalStreamErrorFrame,
	} {
		largest = max(largest, len(frame))
	}
	return largest
}

func requireStreamBodyMatchesSchema(t *testing.T, body string) {
	t.Helper()
	compiled, err := schema.CompileSchema(streamEventSchemaJSON)
	require.NoError(t, err)
	frames := strings.Split(strings.TrimSuffix(body, "\n\n"), "\n\n")
	require.NotEmpty(t, frames)
	for _, frame := range frames {
		require.True(t, strings.HasPrefix(frame, "data: "), frame)
		payload := strings.TrimPrefix(frame, "data: ")
		require.NoError(t, compiled.Validate(json.RawMessage(payload)), payload)
	}
}

func finishPart() provider.StreamPart {
	return provider.StreamPart{
		Type:         provider.PartFinish,
		Usage:        &provider.Usage{},
		FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop},
	}
}

func TestStreamFrameEncoding(t *testing.T) {
	t.Run("canonical fallbacks", func(t *testing.T) {
		start, ok := encodeStreamFrame(streamEvent{typeName: provider.PartStreamStart}, 1<<20)
		require.True(t, ok)
		assert.Equal(t, canonicalEmptyStartFrame, start)
		for _, tc := range []struct {
			value safeError
			frame []byte
		}{
			{value: safeError{category: safeRateLimit}, frame: canonicalRateLimitStreamErrorFrame},
			{value: safeError{category: safeOverload}, frame: canonicalOverloadStreamErrorFrame},
			{value: safeError{category: safeFailedDependency}, frame: canonicalDependencyStreamErrorFrame},
			{value: safeError{category: safeUpstream}, frame: canonicalUpstreamStreamErrorFrame},
			{value: safeError{category: safeTimeout}, frame: canonicalTimeoutStreamErrorFrame},
			{value: safeError{category: safeCancellation}, frame: canonicalCancellationStreamErrorFrame},
			{value: safeError{category: safeInternal}, frame: canonicalInternalStreamErrorFrame},
		} {
			assert.Equal(t, tc.frame, streamErrorFrameForSafeError(tc.value))
		}
	})

	t.Run("complete frame below exact and above limits", func(t *testing.T) {
		event := streamEvent{typeName: provider.PartTextDelta, id: "a", delta: ""}
		frame, ok := encodeStreamFrame(event, 1<<20)
		require.True(t, ok)
		for _, tc := range []struct {
			name  string
			limit int64
			ok    bool
		}{
			{name: "below", limit: int64(len(frame) + 1), ok: true},
			{name: "exact", limit: int64(len(frame)), ok: true},
			{name: "above", limit: int64(len(frame) - 1)},
		} {
			t.Run(tc.name, func(t *testing.T) {
				got, ok := encodeStreamFrame(event, tc.limit)
				assert.Equal(t, tc.ok, ok)
				assert.LessOrEqual(t, len(got), int(tc.limit))
			})
		}
	})

	t.Run("invalid utf8 and unsupported event fail before output", func(t *testing.T) {
		invalid := string([]byte{0xff})
		_, ok := encodeStreamFrame(streamEvent{typeName: provider.PartTextDelta, id: "a", delta: invalid}, 1<<20)
		assert.False(t, ok)
		_, ok = encodeStreamFrame(streamEvent{typeName: provider.PartRaw}, 1<<20)
		assert.False(t, ok)
	})

	t.Run("normalized warnings preserve required values", func(t *testing.T) {
		warnings, err := mapStreamWarnings([]provider.Warning{
			{Type: provider.WarnUnsupported, Feature: "secret", Details: "secret"},
			{Type: provider.WarnCompatibility, Feature: "secret", Details: "secret"},
			{Type: provider.WarnDeprecated, Setting: "secret", Message: "secret"},
			{Type: provider.WarnOther, Message: "secret"},
		}, 1<<20)
		require.NoError(t, err)
		frame, ok := encodeStreamFrame(streamEvent{typeName: provider.PartStreamStart, warnings: warnings}, 1<<20)
		require.True(t, ok)
		assert.NotContains(t, string(frame), "secret")
		assert.Contains(t, string(frame), streamWarningUnsupportedDetails)
		assert.Contains(t, string(frame), streamWarningCompatibilityDetails)
		assert.Contains(t, string(frame), streamWarningDeprecatedMessage)
		assert.Contains(t, string(frame), streamWarningOtherMessage)
	})
}

func TestStreamingRuntimeFrameBoundaries(t *testing.T) {
	delta := strings.Repeat("x", 256)
	frame, ok := encodeStreamFrame(streamEvent{typeName: provider.PartTextDelta, id: "a", delta: delta}, 1<<20)
	require.True(t, ok)
	require.Greater(t, len(frame), len(canonicalInternalStreamErrorFrame))
	parts := []provider.StreamPart{
		{Type: provider.PartTextStart, ID: "a"},
		{Type: provider.PartTextDelta, ID: "a", Delta: delta},
		{Type: provider.PartTextEnd, ID: "a"},
		finishPart(),
	}
	for _, tc := range []struct {
		name       string
		limit      int64
		wantFinish bool
	}{
		{name: "exact", limit: int64(len(frame)), wantFinish: true},
		{name: "one byte above", limit: int64(len(frame) - 1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			limits := testLimits()
			limits.StreamFrameBytes = tc.limit
			harness := newRuntimeHarness(t, limits)
			harness.model.stream = func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
				return &provider.StreamResult{Stream: makeStream(parts...)}, nil
			}
			body := harness.serve(streamRequest(`{"prompt":[]}`)).Body.String()
			assert.Equal(t, tc.wantFinish, strings.Contains(body, `"type":"finish"`))
			if tc.wantFinish {
				assert.Contains(t, body, delta)
				assert.NotContains(t, body, `"code":"internal_error"`)
			} else {
				assert.NotContains(t, body, delta)
				assert.Equal(t, 1, strings.Count(body, `"code":"internal_error"`))
			}
		})
	}
}

func TestStreamingRuntimeHappyPathPrivacyAndOrder(t *testing.T) {
	total := 3
	timestamp := time.Date(2026, 8, 23, 1, 2, 3, 456000000, time.FixedZone("private-zone", 2*60*60))
	parts := []provider.StreamPart{
		{Type: provider.PartStreamStart, Warnings: []provider.Warning{{Type: provider.WarnOther, Message: "credential=secret backend=private-model"}}},
		{Type: provider.PartResponseMeta, ResponseID: "response-id", ModelID: "backend-private", Provider: "private-provider", Timestamp: timestamp, ResponseHeaders: map[string]string{"Authorization": "secret"}, ProviderMetadata: provider.ProviderMetadata{"private": json.RawMessage(`{"secret":true}`)}},
		{Type: provider.PartError, APICallError: provider.NewAPICallError(provider.APICallErrorOptions{StatusCode: http.StatusTooManyRequests, Message: "private error", URL: "https://private.invalid", ResponseBody: "secret"})},
		{Type: provider.PartTextStart, ID: "text-1", ProviderMetadata: provider.ProviderMetadata{"private": json.RawMessage(`{"secret":true}`)}},
		{Type: provider.PartTextDelta, ID: "text-1", Delta: ""},
		{Type: provider.PartError, APICallError: nil},
		{Type: provider.PartTextDelta, ID: "text-1", Delta: "hello"},
		{Type: provider.PartTextEnd, ID: "text-1"},
		{Type: provider.PartFinish, Usage: &provider.Usage{InputTokens: provider.InputTokenUsage{Total: &total}, Raw: json.RawMessage(`{"secret":true}`)}, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop}},
	}
	harness := newRuntimeHarness(t, testLimits())
	harness.model.stream = func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
		return &provider.StreamResult{
			Stream:   makeStream(parts...),
			Request:  &provider.RequestMetadata{Body: json.RawMessage(`{"secret":"request"}`)},
			Response: &provider.ResponseHeaders{Headers: map[string]string{"Authorization": "secret"}},
		}, nil
	}
	response := harness.serve(streamRequest(`{"prompt":[]} `))
	require.Equal(t, http.StatusOK, response.Code)
	assert.Equal(t, "text/event-stream", response.Header().Get("Content-Type"))
	assert.Equal(t, "no-cache, no-transform", response.Header().Get("Cache-Control"))
	body := response.Body.String()
	requireStreamBodyMatchesSchema(t, body)
	expected := []string{
		`{"type":"stream-start","warnings":[{"type":"other","message":"the model reported a warning"}]}`,
		`{"type":"response-metadata","id":"response-id","modelId":"canonical/model","timestamp":"2026-08-22T23:02:03.456Z"}`,
		`{"type":"error","error":{"message":"rate limit exceeded","type":"rate_limit_exceeded","param":null,"code":"rate_limit_exceeded","statusCode":429,"retryable":true}}`,
		`{"type":"text-start","id":"text-1"}`,
		`{"type":"text-delta","id":"text-1","delta":""}`,
		`{"type":"error","error":{"message":"internal error","type":"internal_server_error","param":null,"code":"internal_error","statusCode":500,"retryable":true}}`,
		`{"type":"text-delta","id":"text-1","delta":"hello"}`,
		`{"type":"text-end","id":"text-1"}`,
		`{"type":"finish","usage":{"inputTokens":{"total":3},"outputTokens":{}},"finishReason":{"unified":"stop"}}`,
	}
	position := 0
	for _, payload := range expected {
		frame := "data: " + payload + "\n\n"
		next := strings.Index(body[position:], frame)
		require.NotEqual(t, -1, next, frame)
		position += next + len(frame)
	}
	assert.Equal(t, position, len(body))
	for _, private := range []string{"credential", "secret", "private-model", "private-provider", "private.invalid", "Authorization", "request"} {
		assert.NotContains(t, body, private)
	}
	assert.NotContains(t, body, "event:")
	assert.NotContains(t, body, "[DONE]")
	generate, stream := harness.model.invocationCounts()
	assert.Zero(t, generate)
	assert.Equal(t, 1, stream)
}

func TestStreamingRuntimeStartNormalization(t *testing.T) {
	t.Run("provider start is consumed exactly once", func(t *testing.T) {
		harness := newRuntimeHarness(t, testLimits())
		harness.model.stream = func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
			return &provider.StreamResult{Stream: makeStream(
				provider.StreamPart{Type: provider.PartStreamStart, Warnings: []provider.Warning{{Type: provider.WarnDeprecated, Setting: "private", Message: "private"}}},
				finishPart(),
			)}, nil
		}
		response := harness.serve(streamRequest(`{"prompt":[]}`))
		assert.Equal(t, 1, strings.Count(response.Body.String(), `"type":"stream-start"`))
		assert.Contains(t, response.Body.String(), streamWarningDeprecatedMessage)
	})

	t.Run("omitted start inserts empty start before first part", func(t *testing.T) {
		harness := newRuntimeHarness(t, testLimits())
		harness.model.stream = func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
			return &provider.StreamResult{Stream: makeStream(finishPart())}, nil
		}
		response := harness.serve(streamRequest(`{"prompt":[]}`))
		assert.True(t, strings.HasPrefix(response.Body.String(), string(canonicalEmptyStartFrame)))
		assert.Contains(t, response.Body.String(), `"type":"finish"`)
	})

	t.Run("late duplicate unknown and oversized warnings fail safely", func(t *testing.T) {
		tests := []struct {
			name  string
			parts []provider.StreamPart
		}{
			{name: "late", parts: []provider.StreamPart{{Type: provider.PartTextStart, ID: "a"}, {Type: provider.PartStreamStart}}},
			{name: "duplicate", parts: []provider.StreamPart{{Type: provider.PartStreamStart}, {Type: provider.PartStreamStart}}},
			{name: "unknown warning", parts: []provider.StreamPart{{Type: provider.PartStreamStart, Warnings: []provider.Warning{{Type: provider.WarningType("future")}}}}},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				harness := newRuntimeHarness(t, testLimits())
				harness.model.stream = func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
					return &provider.StreamResult{Stream: makeStream(tc.parts...)}, nil
				}
				response := harness.serve(streamRequest(`{"prompt":[]}`))
				assert.Equal(t, 1, strings.Count(response.Body.String(), `"type":"stream-start"`))
				assert.Equal(t, 1, strings.Count(response.Body.String(), `"code":"internal_error"`))
			})
		}

		limits := testLimits()
		warnings := make([]provider.Warning, 1_000)
		for i := range warnings {
			warnings[i].Type = provider.WarnOther
		}
		limits.StreamFrameBytes = int64(minimumStreamFrameBytes())
		harness := newRuntimeHarness(t, limits)
		harness.model.stream = func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
			return &provider.StreamResult{Stream: makeStream(provider.StreamPart{Type: provider.PartStreamStart, Warnings: warnings})}, nil
		}
		response := harness.serve(streamRequest(`{"prompt":[]}`))
		assert.Equal(t, string(canonicalEmptyStartFrame)+string(canonicalInternalStreamErrorFrame), response.Body.String())
	})
}

func TestStreamingRuntimeTextStateAndUnsupportedParts(t *testing.T) {
	invalidUTF8 := string([]byte{0xff})
	tests := []struct {
		name  string
		parts []provider.StreamPart
	}{
		{name: "duplicate metadata", parts: []provider.StreamPart{{Type: provider.PartResponseMeta}, {Type: provider.PartResponseMeta}}},
		{name: "late metadata", parts: []provider.StreamPart{{Type: provider.PartTextStart, ID: "a"}, {Type: provider.PartTextEnd, ID: "a"}, {Type: provider.PartResponseMeta}}},
		{name: "negative timestamp year", parts: []provider.StreamPart{{Type: provider.PartResponseMeta, Timestamp: time.Date(-1, time.January, 1, 0, 0, 0, 0, time.UTC)}}},
		{name: "five digit timestamp year", parts: []provider.StreamPart{{Type: provider.PartResponseMeta, Timestamp: time.Date(10000, time.January, 1, 0, 0, 0, 0, time.UTC)}}},
		{name: "empty id", parts: []provider.StreamPart{{Type: provider.PartTextStart}}},
		{name: "invalid id", parts: []provider.StreamPart{{Type: provider.PartTextStart, ID: invalidUTF8}}},
		{name: "overlap", parts: []provider.StreamPart{{Type: provider.PartTextStart, ID: "a"}, {Type: provider.PartTextStart, ID: "b"}}},
		{name: "mismatched delta", parts: []provider.StreamPart{{Type: provider.PartTextStart, ID: "a"}, {Type: provider.PartTextDelta, ID: "b"}}},
		{name: "end without start", parts: []provider.StreamPart{{Type: provider.PartTextEnd, ID: "a"}}},
		{name: "reused id", parts: []provider.StreamPart{{Type: provider.PartTextStart, ID: "a"}, {Type: provider.PartTextEnd, ID: "a"}, {Type: provider.PartTextStart, ID: "a"}}},
		{name: "reasoning", parts: []provider.StreamPart{{Type: provider.PartReasoningStart, ID: "private"}}},
		{name: "tool", parts: []provider.StreamPart{{Type: provider.PartToolCall, ToolName: "private"}}},
		{name: "file", parts: []provider.StreamPart{{Type: provider.PartFile, Filename: "private"}}},
		{name: "source", parts: []provider.StreamPart{{Type: provider.PartSource, Title: "private"}}},
		{name: "custom", parts: []provider.StreamPart{{Type: provider.PartCustom, Kind: "private"}}},
		{name: "raw", parts: []provider.StreamPart{{Type: provider.PartRaw, RawValue: json.RawMessage(`{"private":true}`)}}},
		{name: "approval", parts: []provider.StreamPart{{Type: provider.PartToolApprovalRequest, ApprovalID: "private"}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			harness := newRuntimeHarness(t, testLimits())
			harness.model.stream = func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
				return &provider.StreamResult{Stream: makeStream(tc.parts...)}, nil
			}
			response := harness.serve(streamRequest(`{"prompt":[]}`))
			assert.Equal(t, http.StatusOK, response.Code)
			assert.Equal(t, 1, strings.Count(response.Body.String(), `"code":"internal_error"`))
			assert.NotContains(t, response.Body.String(), "private")
			assert.NotContains(t, response.Body.String(), `"type":"finish"`)
		})
	}

	t.Run("metadata accepts RFC3339 year boundaries", func(t *testing.T) {
		for _, year := range []int{0, 9999} {
			harness := newRuntimeHarness(t, testLimits())
			harness.model.stream = func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
				return &provider.StreamResult{Stream: makeStream(
					provider.StreamPart{Type: provider.PartResponseMeta, Timestamp: time.Date(year, time.January, 1, 0, 0, 0, 0, time.UTC)},
					finishPart(),
				)}, nil
			}
			response := harness.serve(streamRequest(`{"prompt":[]}`))
			body := response.Body.String()
			requireStreamBodyMatchesSchema(t, body)
			assert.Contains(t, body, fmt.Sprintf(`"timestamp":"%04d-01-01T00:00:00Z"`, year))
			assert.Contains(t, body, `"type":"finish"`)
		}
	})

	t.Run("sequential blocks preserve empty deltas", func(t *testing.T) {
		harness := newRuntimeHarness(t, testLimits())
		harness.model.stream = func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
			return &provider.StreamResult{Stream: makeStream(
				provider.StreamPart{Type: provider.PartTextStart, ID: "a"},
				provider.StreamPart{Type: provider.PartTextDelta, ID: "a", Delta: ""},
				provider.StreamPart{Type: provider.PartTextEnd, ID: "a"},
				provider.StreamPart{Type: provider.PartTextStart, ID: "b"},
				provider.StreamPart{Type: provider.PartTextEnd, ID: "b"},
				finishPart(),
			)}, nil
		}
		response := harness.serve(streamRequest(`{"prompt":[]}`))
		assert.Contains(t, response.Body.String(), `"delta":""`)
		assert.Contains(t, response.Body.String(), `"id":"b"`)
		assert.Contains(t, response.Body.String(), `"type":"finish"`)
	})
}

func TestStreamingRuntimePartLimitAndTerminalAuthority(t *testing.T) {
	t.Run("below and exact limits succeed while first excess is terminal", func(t *testing.T) {
		parts := []provider.StreamPart{{Type: provider.PartStreamStart}, finishPart()}
		for _, tc := range []struct {
			name      string
			limit     int
			wantError bool
		}{
			{name: "below", limit: 3},
			{name: "exact", limit: 2},
			{name: "excess", limit: 1, wantError: true},
		} {
			t.Run(tc.name, func(t *testing.T) {
				limits := testLimits()
				limits.StreamParts = tc.limit
				harness := newRuntimeHarness(t, limits)
				harness.model.stream = func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
					return &provider.StreamResult{Stream: makeStream(parts...)}, nil
				}
				response := harness.serve(streamRequest(`{"prompt":[]}`))
				assert.Equal(t, tc.wantError, strings.Contains(response.Body.String(), `"code":"internal_error"`))
				assert.Equal(t, !tc.wantError, strings.Contains(response.Body.String(), `"type":"finish"`))
			})
		}
	})

	t.Run("all accepted provider families consume the shared count", func(t *testing.T) {
		parts := []provider.StreamPart{
			{Type: provider.PartStreamStart},
			{Type: provider.PartResponseMeta},
			{Type: provider.PartError},
			{Type: provider.PartTextStart, ID: "a"},
			{Type: provider.PartTextDelta, ID: "a", Delta: ""},
			{Type: provider.PartTextEnd, ID: "a"},
			finishPart(),
		}
		for _, limit := range []int{len(parts), len(parts) - 1} {
			limits := testLimits()
			limits.StreamParts = limit
			harness := newRuntimeHarness(t, limits)
			harness.model.stream = func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
				return &provider.StreamResult{Stream: makeStream(parts...)}, nil
			}
			body := harness.serve(streamRequest(`{"prompt":[]}`)).Body.String()
			assert.Equal(t, limit == len(parts), strings.Contains(body, `"type":"finish"`))
			expectedInternalErrors := 1
			if limit != len(parts) {
				expectedInternalErrors = 2
			}
			assert.Equal(t, expectedInternalErrors, strings.Count(body, `"code":"internal_error"`))
		}
	})

	t.Run("finish is final without waiting for close", func(t *testing.T) {
		stream := make(chan provider.StreamPart, 2)
		stream <- finishPart()
		stream <- provider.StreamPart{Type: provider.PartTextStart, ID: "post-finish-private"}
		harness := newRuntimeHarness(t, testLimits())
		harness.model.stream = func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
			return &provider.StreamResult{Stream: stream}, nil
		}
		start := time.Now()
		response := harness.serve(streamRequest(`{"prompt":[]}`))
		assert.Less(t, time.Since(start), 500*time.Millisecond)
		assert.True(t, strings.HasSuffix(response.Body.String(), "data: {\"type\":\"finish\",\"usage\":{\"inputTokens\":{},\"outputTokens\":{}},\"finishReason\":{\"unified\":\"stop\"}}\n\n"))
		assert.NotContains(t, response.Body.String(), "post-finish-private")
	})

	t.Run("continuously ready flood stops at cardinality", func(t *testing.T) {
		limits := testLimits()
		limits.StreamParts = 8
		stream := make(chan provider.StreamPart, 128)
		for i := 0; i < 64; i++ {
			id := fmt.Sprintf("id-%d", i)
			stream <- provider.StreamPart{Type: provider.PartTextStart, ID: id}
			stream <- provider.StreamPart{Type: provider.PartTextEnd, ID: id}
		}
		harness := newRuntimeHarness(t, limits)
		harness.model.stream = func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
			return &provider.StreamResult{Stream: stream}, nil
		}
		response := harness.serve(streamRequest(`{"prompt":[]}`))
		assert.Equal(t, 1, strings.Count(response.Body.String(), `"code":"internal_error"`))
		assert.NotContains(t, response.Body.String(), "id-4")
	})
}

func TestStreamingRuntimeProviderErrorsAndFinishValidation(t *testing.T) {
	t.Run("provider errors remain ordered and non-terminal", func(t *testing.T) {
		harness := newRuntimeHarness(t, testLimits())
		harness.model.stream = func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
			return &provider.StreamResult{Stream: makeStream(
				provider.StreamPart{Type: provider.PartError, APICallError: provider.NewAPICallError(provider.APICallErrorOptions{StatusCode: 503, Message: "private"})},
				provider.StreamPart{Type: provider.PartTextStart, ID: "a"},
				provider.StreamPart{Type: provider.PartError, APICallError: provider.NewAPICallError(provider.APICallErrorOptions{StatusCode: 400, Message: "private"})},
				provider.StreamPart{Type: provider.PartTextDelta, ID: "a", Delta: "ok"},
				provider.StreamPart{Type: provider.PartTextEnd, ID: "a"},
				finishPart(),
			)}, nil
		}
		body := harness.serve(streamRequest(`{"prompt":[]}`)).Body.String()
		first := strings.Index(body, `"code":"overloaded"`)
		second := strings.Index(body, `"code":"failed_dependency"`)
		text := strings.Index(body, `"delta":"ok"`)
		require.GreaterOrEqual(t, first, 0)
		assert.Greater(t, second, first)
		assert.Greater(t, text, second)
		assert.Contains(t, body, `"type":"finish"`)
	})

	t.Run("api error status and wrapped transport are classified safely", func(t *testing.T) {
		tests := []struct {
			name      string
			apiError  *provider.APICallError
			wantError string
		}{
			{
				name:      "invalid http status is internal",
				apiError:  provider.NewAPICallError(provider.APICallErrorOptions{StatusCode: 700, Message: "private", Cause: context.Canceled}),
				wantError: `"error":{"message":"internal error","type":"internal_server_error","param":null,"code":"internal_error","statusCode":500,"retryable":true}`,
			},
			{
				name:      "no status timeout cause is timeout",
				apiError:  provider.NewAPICallError(provider.APICallErrorOptions{Message: "private", Cause: testNetError{timeout: true}}),
				wantError: `"error":{"message":"request timed out","type":"internal_server_error","param":null,"code":"timeout","statusCode":504,"retryable":true}`,
			},
			{
				name:      "no status dns cause is upstream",
				apiError:  provider.NewAPICallError(provider.APICallErrorOptions{Message: "private", Cause: &net.DNSError{Name: "private.internal", Err: "no such host"}}),
				wantError: `"error":{"message":"upstream failure","type":"internal_server_error","param":null,"code":"upstream_error","statusCode":502,"retryable":true}`,
			},
			{
				name:      "valid http 200 error remains upstream",
				apiError:  provider.NewAPICallError(provider.APICallErrorOptions{StatusCode: http.StatusOK, Message: "private", Cause: context.DeadlineExceeded}),
				wantError: `"error":{"message":"upstream failure","type":"internal_server_error","param":null,"code":"upstream_error","statusCode":502,"retryable":true}`,
			},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				harness := newRuntimeHarness(t, testLimits())
				harness.model.stream = func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
					return &provider.StreamResult{Stream: makeStream(
						provider.StreamPart{Type: provider.PartError, APICallError: tc.apiError},
						finishPart(),
					)}, nil
				}
				body := harness.serve(streamRequest(`{"prompt":[]}`)).Body.String()
				assert.Contains(t, body, tc.wantError)
				assert.Equal(t, 1, strings.Count(body, `"type":"error"`))
				assert.Contains(t, body, `"type":"finish"`)
				assert.NotContains(t, body, "private")
			})
		}
	})

	negative := -1
	tooLarge := maxJavaScriptSafeInteger + 1
	invalidUTF8 := string([]byte{0xff})
	tests := []struct {
		name   string
		finish provider.StreamPart
		prefix []provider.StreamPart
	}{
		{name: "nil usage", finish: provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop}}},
		{name: "nil reason", finish: provider.StreamPart{Type: provider.PartFinish, Usage: &provider.Usage{}}},
		{name: "unknown reason", finish: provider.StreamPart{Type: provider.PartFinish, Usage: &provider.Usage{}, FinishReason: &provider.FinishReason{Unified: provider.UnifiedFinishReason("future")}}},
		{name: "invalid raw reason", finish: provider.StreamPart{Type: provider.PartFinish, Usage: &provider.Usage{}, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop, Raw: invalidUTF8}}},
		{name: "negative usage", finish: provider.StreamPart{Type: provider.PartFinish, Usage: &provider.Usage{InputTokens: provider.InputTokenUsage{Total: &negative}}, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop}}},
		{name: "unsafe usage", finish: provider.StreamPart{Type: provider.PartFinish, Usage: &provider.Usage{OutputTokens: provider.OutputTokenUsage{Total: &tooLarge}}, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop}}},
		{name: "finish warnings", finish: provider.StreamPart{Type: provider.PartFinish, Usage: &provider.Usage{}, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop}, Warnings: []provider.Warning{{Type: provider.WarnOther}}}},
		{name: "active block", prefix: []provider.StreamPart{{Type: provider.PartTextStart, ID: "a"}}, finish: finishPart()},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			harness := newRuntimeHarness(t, testLimits())
			parts := append(append([]provider.StreamPart{}, tc.prefix...), tc.finish)
			harness.model.stream = func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
				return &provider.StreamResult{Stream: makeStream(parts...)}, nil
			}
			body := harness.serve(streamRequest(`{"prompt":[]}`)).Body.String()
			assert.NotContains(t, body, `"type":"finish"`)
			assert.Equal(t, 1, strings.Count(body, `"code":"internal_error"`))
		})
	}

	t.Run("premature eof is terminal without synthetic finish or block end", func(t *testing.T) {
		harness := newRuntimeHarness(t, testLimits())
		harness.model.stream = func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
			return &provider.StreamResult{Stream: makeStream(provider.StreamPart{Type: provider.PartTextStart, ID: "a"})}, nil
		}
		body := harness.serve(streamRequest(`{"prompt":[]}`)).Body.String()
		assert.Equal(t, 1, strings.Count(body, `"code":"internal_error"`))
		assert.NotContains(t, body, `"type":"text-end"`)
		assert.NotContains(t, body, `"type":"finish"`)
	})
}

func TestStreamingSetupFailuresAndOwnership(t *testing.T) {
	t.Run("one snapshot establishes immutable owner", func(t *testing.T) {
		tests := []struct {
			name     string
			canceled bool
			expired  bool
			want     setupOwner
		}{
			{name: "claim", want: setupHandler},
			{name: "cancellation", canceled: true, want: setupAbandoned},
			{name: "expiry", expired: true, want: setupAbandoned},
			{name: "cancellation precedence", canceled: true, expired: true, want: setupAbandoned},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				handoff := newStreamSetupHandoff()
				close(handoff.ready)
				assert.Equal(t, tc.want, decideStreamSetup(handoff, tc.canceled, tc.expired))
				assert.Equal(t, tc.want, decideStreamSetup(handoff, !tc.canceled, !tc.expired))
			})
		}
	})

	t.Run("setup failures stay json", func(t *testing.T) {
		tests := []struct {
			name   string
			stream func(context.Context, provider.CallOptions) (*provider.StreamResult, error)
			status int
		}{
			{name: "provider error", stream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
				return nil, provider.NewAPICallError(provider.APICallErrorOptions{StatusCode: 429})
			}, status: 429},
			{name: "panic", stream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) { panic("private") }, status: 500},
			{name: "nil nil", stream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) { return nil, nil }, status: 500},
			{name: "nil channel", stream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
				return &provider.StreamResult{}, nil
			}, status: 500},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				harness := newRuntimeHarness(t, testLimits())
				harness.model.stream = tc.stream
				response := harness.serve(streamRequest(`{"prompt":[]}`))
				assert.Equal(t, tc.status, response.Code)
				assert.Equal(t, "application/json", response.Header().Get("Content-Type"))
				assert.NotContains(t, response.Body.String(), "private")
			})
		}
	})

	t.Run("cancellation after claim remains sse without a first part", func(t *testing.T) {
		stream := make(chan provider.StreamPart)
		harness := newRuntimeHarness(t, testLimits())
		harness.model.stream = func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
			return &provider.StreamResult{Stream: stream}, nil
		}
		ctx, cancel := context.WithCancel(context.Background())
		committed := make(chan struct{})
		writer := &responseWriterProbe{onFlush: func(call int) {
			if call == 1 {
				close(committed)
			}
		}}
		done := make(chan struct{})
		go func() {
			harness.handler.ServeHTTP(writer, streamRequest(`{"prompt":[]}`).WithContext(ctx))
			close(done)
		}()
		select {
		case <-committed:
		case <-time.After(time.Second):
			t.Fatal("stream was not committed")
		}
		cancel()
		<-done
		assert.Equal(t, http.StatusOK, writer.status)
		assert.True(t, strings.HasPrefix(writer.body.String(), string(canonicalEmptyStartFrame)))
		assert.Contains(t, writer.body.String(), `"code":"canceled"`)
	})

	t.Run("result plus error starts asynchronous drain before json return", func(t *testing.T) {
		limits := testLimits()
		limits.StreamDrainDuration = time.Second
		stream := make(chan provider.StreamPart)
		canceled := make(chan struct{})
		harness := newRuntimeHarness(t, limits)
		harness.model.stream = func(ctx context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
			go func() { <-ctx.Done(); close(canceled) }()
			return &provider.StreamResult{Stream: stream}, errors.New("private")
		}
		start := time.Now()
		response := harness.serve(streamRequest(`{"prompt":[]}`))
		assert.Less(t, time.Since(start), 500*time.Millisecond)
		assert.Equal(t, http.StatusInternalServerError, response.Code)
		select {
		case <-canceled:
		case <-time.After(time.Second):
			t.Fatal("provider context was not canceled")
		}
	})

	t.Run("condition after claim remains handler owned", func(t *testing.T) {
		handoff := newStreamSetupHandoff()
		close(handoff.ready)
		assert.Equal(t, setupHandler, decideStreamSetup(handoff, false, false))
		assert.Equal(t, setupHandler, decideStreamSetup(handoff, true, true))
	})

	t.Run("handoff starts drain exactly once", func(t *testing.T) {
		handoff := newStreamSetupHandoff()
		h := newTestHandler(t, testLimits())
		counter := newStreamPartCounter(10)
		stream := makeStream(provider.StreamPart{Type: provider.PartRaw})
		assert.True(t, handoff.startDrain(h, stream, counter))
		assert.False(t, handoff.startDrain(h, stream, counter))
		require.Eventually(t, func() bool { return counter.count.Load() == 1 }, time.Second, time.Millisecond)
	})

	t.Run("cancellation abandonment owns a late stream", func(t *testing.T) {
		release := make(chan struct{})
		lateStream := make(chan provider.StreamPart)
		drained := make(chan struct{})
		harness := newRuntimeHarness(t, testLimits())
		harness.model.stream = func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
			<-release
			go func() {
				lateStream <- provider.StreamPart{Type: provider.PartRaw}
				close(drained)
				close(lateStream)
			}()
			return &provider.StreamResult{Stream: lateStream}, nil
		}
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan *runtimeResponse, 1)
		go func() {
			response := harness.serve(streamRequest(`{"prompt":[]}`).WithContext(ctx))
			done <- &runtimeResponse{code: response.Code, body: response.Body.String()}
		}()
		require.Eventually(t, func() bool {
			_, stream := harness.model.invocationCounts()
			return stream == 1
		}, time.Second, time.Millisecond)
		cancel()
		result := <-done
		assert.Equal(t, 499, result.code)
		close(release)
		select {
		case <-drained:
		case <-time.After(time.Second):
			t.Fatal("late stream was not drained")
		}
	})
}

type runtimeResponse struct {
	code int
	body string
}

type responseWriterProbe struct {
	header       http.Header
	status       int
	writes       int
	body         strings.Builder
	shortWrite   bool
	writeErr     error
	panicWrite   bool
	onWrite      func()
	flushErrors  []error
	flushCalls   int
	onFlush      func(int)
	panicFlushAt int
}

func (w *responseWriterProbe) Header() http.Header {
	if w.header == nil {
		w.header = make(http.Header)
	}
	return w.header
}

func (w *responseWriterProbe) WriteHeader(status int) { w.status = status }

func (w *responseWriterProbe) Write(data []byte) (int, error) {
	w.writes++
	if w.onWrite != nil {
		w.onWrite()
	}
	if w.panicWrite {
		panic("private writer panic")
	}
	if w.writeErr != nil {
		return 0, w.writeErr
	}
	if w.shortWrite {
		return len(data) - 1, nil
	}
	return w.body.Write(data)
}

func (w *responseWriterProbe) FlushError() error {
	w.flushCalls++
	if w.onFlush != nil {
		w.onFlush(w.flushCalls)
	}
	if w.panicFlushAt == w.flushCalls {
		panic("private flush panic")
	}
	if len(w.flushErrors) >= w.flushCalls {
		return w.flushErrors[w.flushCalls-1]
	}
	return nil
}

func TestStreamingWriterAndFlushBehavior(t *testing.T) {
	t.Run("direct and wrapped unsupported flushes are tolerated", func(t *testing.T) {
		for _, err := range []error{http.ErrNotSupported, fmt.Errorf("wrapped: %w", http.ErrNotSupported)} {
			writer := &responseWriterProbe{flushErrors: []error{err, err}}
			assert.True(t, commitStreamResponse(writer))
			assert.True(t, writeCompleteStreamFrame(writer, canonicalEmptyStartFrame))
			assert.Equal(t, 2, writer.flushCalls)
			assert.Equal(t, 1, writer.writes)
		}
	})

	t.Run("header flush failure and panic prevent frames", func(t *testing.T) {
		writers := []*responseWriterProbe{
			{flushErrors: []error{errors.New("private flush")}},
			{panicFlushAt: 1},
		}
		for _, writer := range writers {
			assert.False(t, commitStreamResponse(writer))
			assert.Zero(t, writer.writes)
		}
	})

	t.Run("handler flush failures cancel provider without a second write", func(t *testing.T) {
		for _, tc := range []struct {
			name        string
			flushErrors []error
			wantWrites  int
		}{
			{name: "header", flushErrors: []error{errors.New("private")}},
			{name: "frame", flushErrors: []error{nil, errors.New("private")}, wantWrites: 1},
		} {
			t.Run(tc.name, func(t *testing.T) {
				stream := make(chan provider.StreamPart, 1)
				stream <- finishPart()
				canceled := make(chan struct{})
				harness := newRuntimeHarness(t, testLimits())
				harness.model.stream = func(ctx context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
					go func() { <-ctx.Done(); close(canceled) }()
					return &provider.StreamResult{Stream: stream}, nil
				}
				writer := &responseWriterProbe{flushErrors: tc.flushErrors}
				harness.handler.ServeHTTP(writer, streamRequest(`{"prompt":[]}`))
				assert.Equal(t, tc.wantWrites, writer.writes)
				select {
				case <-canceled:
				case <-time.After(time.Second):
					t.Fatal("provider was not canceled after writer failure")
				}
			})
		}
	})

	t.Run("full write flushes and every writer failure stops without retry", func(t *testing.T) {
		full := &responseWriterProbe{}
		assert.True(t, writeCompleteStreamFrame(full, canonicalEmptyStartFrame))
		assert.Equal(t, 1, full.writes)
		assert.Equal(t, 1, full.flushCalls)
		for _, writer := range []*responseWriterProbe{
			{shortWrite: true},
			{writeErr: errors.New("private")},
			{panicWrite: true},
			{flushErrors: []error{errors.New("private")}},
			{panicFlushAt: 1},
		} {
			assert.False(t, writeCompleteStreamFrame(writer, canonicalEmptyStartFrame))
			assert.Equal(t, 1, writer.writes)
		}
	})
}

type manualProtocolClock struct {
	mu     sync.Mutex
	now    time.Time
	timers map[*manualProtocolTimer]struct{}
}

type manualProtocolTimer struct {
	clock    *manualProtocolClock
	deadline time.Time
	channel  chan time.Time
	stopped  bool
	fired    bool
}

func newManualProtocolClock() *manualProtocolClock {
	return &manualProtocolClock{now: time.Unix(1_000, 0), timers: make(map[*manualProtocolTimer]struct{})}
}

func (c *manualProtocolClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *manualProtocolClock) NewTimer(duration time.Duration) protocolTimer {
	c.mu.Lock()
	defer c.mu.Unlock()
	timer := &manualProtocolTimer{clock: c, deadline: c.now.Add(max(duration, 0)), channel: make(chan time.Time, 1)}
	c.timers[timer] = struct{}{}
	c.fireLocked(timer)
	return timer
}

func (c *manualProtocolClock) Advance(duration time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(duration)
	for timer := range c.timers {
		c.fireLocked(timer)
	}
}

func (c *manualProtocolClock) WakeAll() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for timer := range c.timers {
		if timer.stopped || timer.fired {
			continue
		}
		timer.fired = true
		timer.channel <- c.now
	}
}

func (c *manualProtocolClock) TimerCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.timers)
}

func (c *manualProtocolClock) HasTimerAfter(duration time.Duration) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	deadline := c.now.Add(duration)
	for timer := range c.timers {
		if timer.deadline.Equal(deadline) && !timer.stopped && !timer.fired {
			return true
		}
	}
	return false
}

func (c *manualProtocolClock) fireLocked(timer *manualProtocolTimer) {
	if timer.stopped || timer.fired || c.now.Before(timer.deadline) {
		return
	}
	timer.fired = true
	timer.channel <- c.now
}

func (t *manualProtocolTimer) C() <-chan time.Time { return t.channel }

func (t *manualProtocolTimer) Stop() bool {
	t.clock.mu.Lock()
	defer t.clock.mu.Unlock()
	wasActive := !t.stopped && !t.fired
	t.stopped = true
	delete(t.clock.timers, t)
	return wasActive
}

type sequenceProtocolClock struct {
	calls atomic.Int64
	first time.Time
	later time.Time
}

func (c *sequenceProtocolClock) Now() time.Time {
	if c.calls.Add(1) == 1 {
		return c.first
	}
	return c.later
}

func (*sequenceProtocolClock) NewTimer(time.Duration) protocolTimer {
	return inertProtocolTimer{channel: make(chan time.Time)}
}

type inertProtocolTimer struct {
	channel chan time.Time
}

func (t inertProtocolTimer) C() <-chan time.Time { return t.channel }
func (inertProtocolTimer) Stop() bool            { return true }

func TestStreamPrecedenceAndControlledTimeout(t *testing.T) {
	now := time.Unix(1_000, 0)
	tests := []struct {
		name     string
		canceled bool
		total    time.Time
		idle     time.Time
		want     streamWaitResult
	}{
		{name: "none", total: now.Add(time.Second), idle: now.Add(time.Second)},
		{name: "cancel wins", canceled: true, total: now, idle: now, want: streamWaitCanceled},
		{name: "total wins", total: now, idle: now, want: streamWaitTotalTimeout},
		{name: "idle", total: now.Add(time.Second), idle: now, want: streamWaitIdleTimeout},
		{name: "equality expires", total: now, idle: now.Add(time.Second), want: streamWaitTotalTimeout},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, streamPrecedence(tc.canceled, now, tc.total, tc.idle))
		})
	}

	t.Run("received part is counted before post-receive timeout precedence", func(t *testing.T) {
		base := time.Unix(1_000, 0)
		clock := &sequenceProtocolClock{first: base, later: base.Add(time.Second)}
		h := newTestHandler(t, testLimits())
		h.clock = clock
		counter := newStreamPartCounter(10)
		stream := makeStream(provider.StreamPart{Type: provider.PartRaw})
		_, result := h.waitStreamPart(context.Background(), stream, counter, base.Add(time.Second), base.Add(2*time.Second))
		assert.Equal(t, streamWaitTotalTimeout, result)
		assert.Equal(t, int64(1), counter.count.Load())
	})

	t.Run("total timer cancels provider during setup before json", func(t *testing.T) {
		clock := newManualProtocolClock()
		limits := testLimits()
		limits.ModelDuration = time.Second
		harness := newRuntimeHarness(t, limits)
		harness.handler.clock = clock
		capturedContext := make(chan context.Context, 1)
		harness.model.stream = func(ctx context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
			capturedContext <- ctx
			<-ctx.Done()
			return nil, ctx.Err()
		}
		var modelContext context.Context
		canceledAtWrite := make(chan bool, 1)
		writer := &responseWriterProbe{onWrite: func() { canceledAtWrite <- modelContext.Err() != nil }}
		done := make(chan struct{})
		go func() {
			harness.handler.ServeHTTP(writer, streamRequest(`{"prompt":[]}`))
			close(done)
		}()
		modelContext = <-capturedContext
		require.Eventually(t, func() bool { return clock.HasTimerAfter(time.Second) }, time.Second, time.Millisecond)
		clock.Advance(time.Second)
		assert.True(t, <-canceledAtWrite)
		<-done
	})

	t.Run("idle deadline starts before commitment flush", func(t *testing.T) {
		clock := newManualProtocolClock()
		limits := testLimits()
		limits.ModelDuration = 10 * time.Second
		limits.StreamIdleDuration = time.Second
		harness := newRuntimeHarness(t, limits)
		harness.handler.clock = clock
		capturedContext := make(chan context.Context, 1)
		harness.model.stream = func(ctx context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
			capturedContext <- ctx
			return &provider.StreamResult{Stream: makeStream(finishPart())}, nil
		}
		canceledAtWrite := make(chan bool, 2)
		writer := &responseWriterProbe{
			onFlush: func(call int) {
				if call == 1 {
					clock.Advance(time.Second)
				}
			},
			onWrite: func() {
				modelContext := <-capturedContext
				canceledAtWrite <- modelContext.Err() != nil
				capturedContext <- modelContext
			},
		}
		harness.handler.ServeHTTP(writer, streamRequest(`{"prompt":[]}`))
		assert.True(t, <-canceledAtWrite)
		assert.True(t, <-canceledAtWrite)
		assert.Contains(t, writer.body.String(), `"code":"timeout"`)
		assert.NotContains(t, writer.body.String(), `"type":"finish"`)
	})

	t.Run("accepted activity resets idle deadline", func(t *testing.T) {
		clock := newManualProtocolClock()
		limits := testLimits()
		limits.ModelDuration = 10 * time.Second
		limits.StreamIdleDuration = time.Second
		harness := newRuntimeHarness(t, limits)
		harness.handler.clock = clock
		stream := make(chan provider.StreamPart)
		harness.model.stream = func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
			return &provider.StreamResult{Stream: stream}, nil
		}
		writes := make(chan struct{}, 16)
		writer := &responseWriterProbe{onWrite: func() { writes <- struct{}{} }}
		done := make(chan struct{})
		go func() {
			harness.handler.ServeHTTP(writer, streamRequest(`{"prompt":[]}`))
			close(done)
		}()
		sendAndWait := func(part provider.StreamPart, expectedWrites int) {
			stream <- part
			for range expectedWrites {
				select {
				case <-writes:
				case <-time.After(time.Second):
					t.Fatal("stream event was not written")
				}
			}
			clock.Advance(900 * time.Millisecond)
		}
		sendAndWait(provider.StreamPart{Type: provider.PartResponseMeta}, 2)
		sendAndWait(provider.StreamPart{Type: provider.PartError}, 1)
		sendAndWait(provider.StreamPart{Type: provider.PartTextStart, ID: "a"}, 1)
		sendAndWait(provider.StreamPart{Type: provider.PartTextDelta, ID: "a", Delta: ""}, 1)
		sendAndWait(provider.StreamPart{Type: provider.PartTextEnd, ID: "a"}, 1)
		stream <- finishPart()
		close(stream)
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("active stream did not finish")
		}
		assert.Contains(t, writer.body.String(), `"type":"finish"`)
		assert.NotContains(t, writer.body.String(), `"code":"timeout"`)
	})

	t.Run("idle timer cancels provider before timeout frame", func(t *testing.T) {
		clock := newManualProtocolClock()
		limits := testLimits()
		limits.ModelDuration = 10 * time.Second
		limits.StreamIdleDuration = time.Second
		harness := newRuntimeHarness(t, limits)
		harness.handler.clock = clock
		stream := make(chan provider.StreamPart)
		capturedContext := make(chan context.Context, 1)
		harness.model.stream = func(ctx context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
			capturedContext <- ctx
			return &provider.StreamResult{Stream: stream}, nil
		}
		var modelContext context.Context
		canceledAtWrite := make(chan bool, 2)
		writer := &responseWriterProbe{onWrite: func() { canceledAtWrite <- modelContext.Err() != nil }}
		done := make(chan struct{})
		go func() {
			harness.handler.ServeHTTP(writer, streamRequest(`{"prompt":[]}`))
			close(done)
		}()
		modelContext = <-capturedContext
		require.Eventually(t, func() bool { return clock.HasTimerAfter(time.Second) }, time.Second, time.Millisecond)
		clock.Advance(time.Second)
		<-done
		assert.True(t, <-canceledAtWrite)
		assert.True(t, <-canceledAtWrite)
		assert.Equal(t, http.StatusOK, writer.status)
		assert.True(t, strings.HasPrefix(writer.body.String(), string(canonicalEmptyStartFrame)))
		assert.Contains(t, writer.body.String(), `"code":"timeout"`)
	})
}

func TestStreamDrainBounds(t *testing.T) {
	t.Run("normal close and part bound", func(t *testing.T) {
		clock := newManualProtocolClock()
		h := newTestHandler(t, testLimits())
		h.clock = clock
		stream := make(chan provider.StreamPart, 10)
		for i := 0; i < 10; i++ {
			stream <- provider.StreamPart{Type: provider.PartRaw}
		}
		counter := newStreamPartCounter(3)
		h.drainStream(stream, counter)
		assert.Equal(t, int64(4), counter.count.Load())
	})

	t.Run("delayed close exits within the drain bound", func(t *testing.T) {
		h := newTestHandler(t, testLimits())
		stream := make(chan provider.StreamPart)
		done := make(chan struct{})
		go func() {
			h.drainStream(stream, newStreamPartCounter(10))
			close(done)
		}()
		stream <- provider.StreamPart{Type: provider.PartRaw}
		close(stream)
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("drain did not observe delayed close")
		}
	})

	t.Run("early timer wake is not deadline authority", func(t *testing.T) {
		clock := newManualProtocolClock()
		h := newTestHandler(t, testLimits())
		h.clock = clock
		stream := make(chan provider.StreamPart)
		done := make(chan struct{})
		go func() {
			h.drainStream(stream, newStreamPartCounter(10))
			close(done)
		}()
		require.Eventually(t, func() bool { return clock.HasTimerAfter(h.limits.StreamDrainDuration) }, time.Second, time.Millisecond)
		clock.WakeAll()
		require.Eventually(t, func() bool { return clock.HasTimerAfter(h.limits.StreamDrainDuration) }, time.Second, time.Millisecond)
		select {
		case <-done:
			t.Fatal("early timer wake ended drain")
		default:
		}
		close(stream)
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("drain did not observe close after early wake")
		}
	})

	t.Run("deadline equality stops permanently ready drain", func(t *testing.T) {
		clock := newManualProtocolClock()
		limits := testLimits()
		limits.StreamDrainDuration = time.Second
		h := newTestHandler(t, limits)
		h.clock = clock
		stream := make(chan provider.StreamPart)
		stopProducer := make(chan struct{})
		producerDone := make(chan struct{})
		go func() {
			defer close(producerDone)
			for {
				select {
				case stream <- provider.StreamPart{Type: provider.PartRaw}:
				case <-stopProducer:
					return
				}
			}
		}()
		counter := newStreamPartCounter(1_000_000)
		done := make(chan struct{})
		go func() {
			h.drainStream(stream, counter)
			close(done)
		}()
		require.Eventually(t, func() bool { return clock.TimerCount() == 1 }, time.Second, time.Millisecond)
		clock.Advance(time.Second)
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("drain did not stop at deadline equality")
		}
		close(stopProducer)
		<-producerDone
		assert.Less(t, counter.count.Load(), int64(1_000_000))
	})
}
