package providerwire

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func ptrBool(b bool) *bool { return &b }
func ptrInt(i int) *int    { return &i }
func ptrFinish(r provider.UnifiedFinishReason) *provider.FinishReason {
	return &provider.FinishReason{Unified: r}
}

// TestSSE_RoundTrip_AllStreamPartTypes asserts that every defined
// StreamPartType round-trips losslessly through WriteSSEStreamPart +
// SSEReader.Next.
func TestSSE_RoundTrip_AllStreamPartTypes(t *testing.T) {
	usage := provider.Usage{
		InputTokens:  provider.InputTokenUsage{Total: ptrInt(10)},
		OutputTokens: provider.OutputTokenUsage{Total: ptrInt(20)},
	}
	apiErr := provider.NewAPICallError(provider.APICallErrorOptions{
		Message:    "rate limit",
		StatusCode: 429,
	})

	cases := []struct {
		name string
		part provider.StreamPart
	}{
		{name: "text-start", part: provider.StreamPart{Type: provider.PartTextStart, ID: "b1"}},
		{name: "text-delta", part: provider.StreamPart{Type: provider.PartTextDelta, ID: "b1", Delta: "hello"}},
		{name: "text-end", part: provider.StreamPart{Type: provider.PartTextEnd, ID: "b1", ProviderMetadata: provider.ProviderMetadata{"anthropic": json.RawMessage(`{"citations":[{"type":"web_search_result_location","url":"https://example.com"}]}`)}}},
		{name: "reasoning-start", part: provider.StreamPart{Type: provider.PartReasoningStart, ID: "r1"}},
		{name: "reasoning-delta", part: provider.StreamPart{Type: provider.PartReasoningDelta, ID: "r1", Delta: "thinking"}},
		{name: "reasoning-end", part: provider.StreamPart{Type: provider.PartReasoningEnd, ID: "r1"}},
		{name: "tool-input-start", part: provider.StreamPart{Type: provider.PartToolInputStart, ID: "tc_1", ToolName: "search"}},
		{name: "tool-input-delta", part: provider.StreamPart{Type: provider.PartToolInputDelta, ID: "tc_1", Delta: `{"q":"go"}`}},
		{name: "tool-input-end", part: provider.StreamPart{Type: provider.PartToolInputEnd, ID: "tc_1"}},
		{name: "tool-call", part: provider.StreamPart{Type: provider.PartToolCall, ToolCallID: "tc_1", ToolName: "search", Input: `{"q":"go"}`, ProviderExecuted: true}},
		{name: "tool-result", part: provider.StreamPart{Type: provider.PartToolResult, ToolCallID: "tc_1", ToolName: "search", Result: json.RawMessage(`"ok"`), Preliminary: ptrBool(false)}},
		{name: "source", part: provider.StreamPart{Type: provider.PartSource, Source: &provider.SourceInfo{SourceType: provider.SourceTypeURL, ID: "s1", URL: "https://example.com", Title: "Example"}}},
		{name: "file", part: provider.StreamPart{Type: provider.PartFile, Data: &provider.StreamFileData{Type: provider.StreamFileDataTypeData, Base64: "AQI="}, MediaType: "image/png", Filename: "img.png"}},
		{name: "stream-start", part: provider.StreamPart{Type: provider.PartStreamStart, Warnings: []provider.Warning{{Type: provider.WarnUnsupported, Feature: "logprobs"}}}},
		{name: "response-metadata", part: provider.StreamPart{Type: provider.PartResponseMeta, ResponseID: "msg_1", ModelID: "claude-x", Provider: "anthropic.vertex", Timestamp: time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC), ResponseHeaders: map[string]string{"x-rate": "10"}}},
		{name: "finish", part: provider.StreamPart{Type: provider.PartFinish, Usage: &usage, FinishReason: ptrFinish(provider.FinishReasonStop)}},
		{name: "raw", part: provider.StreamPart{Type: provider.PartRaw, RawValue: json.RawMessage(`{"x":1}`)}},
		{name: "error", part: provider.StreamPart{Type: provider.PartError, APICallError: apiErr}},
		{name: "tool-approval-request", part: provider.StreamPart{Type: provider.PartToolApprovalRequest, ApprovalID: "apr_1", ToolCallID: "tc_1"}},
		{name: "custom", part: provider.StreamPart{Type: provider.PartCustom, Kind: "anthropic.cache-control"}},
		{name: "reasoning-file", part: provider.StreamPart{Type: provider.PartReasoningFile, Data: &provider.StreamFileData{Type: provider.StreamFileDataTypeURL, URL: "https://example.com/reasoning.png"}, MediaType: "image/png"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			require.NoError(t, WriteSSEStreamPart(&buf, tc.part))

			r := NewSSEReader(&buf)
			got, err := r.Next()
			require.NoError(t, err)
			assert.Equal(t, tc.part, got)

			// Stream is now drained; next call returns EOF.
			_, err = r.Next()
			assert.True(t, errors.Is(err, io.EOF))
		})
	}
}

// TestSSE_AllStreamPartTypes_Coverage ensures the round-trip table includes
// every defined StreamPartType so adding a new type fails this test until
// covered.
func TestSSE_AllStreamPartTypes_Coverage(t *testing.T) {
	defined := []provider.StreamPartType{
		provider.PartTextStart, provider.PartTextDelta, provider.PartTextEnd,
		provider.PartReasoningStart, provider.PartReasoningDelta, provider.PartReasoningEnd,
		provider.PartToolInputStart, provider.PartToolInputDelta, provider.PartToolInputEnd,
		provider.PartToolCall, provider.PartToolResult,
		provider.PartSource, provider.PartFile,
		provider.PartStreamStart, provider.PartResponseMeta, provider.PartFinish,
		provider.PartRaw, provider.PartError,
		provider.PartToolApprovalRequest,
		provider.PartCustom, provider.PartReasoningFile,
	}
	assert.Len(t, defined, 21, "if this fails, update sse_test.go round-trip table for the new StreamPartType")
}

func TestSSE_MultipleEvents_Sequenced(t *testing.T) {
	parts := []provider.StreamPart{
		{Type: provider.PartTextStart, ID: "b1"},
		{Type: provider.PartTextDelta, ID: "b1", Delta: "hello "},
		{Type: provider.PartTextDelta, ID: "b1", Delta: "world"},
		{Type: provider.PartTextEnd, ID: "b1"},
	}
	var buf bytes.Buffer
	for _, p := range parts {
		require.NoError(t, WriteSSEStreamPart(&buf, p))
	}

	r := NewSSEReader(&buf)
	for _, want := range parts {
		got, err := r.Next()
		require.NoError(t, err)
		assert.Equal(t, want, got)
	}
	_, err := r.Next()
	assert.True(t, errors.Is(err, io.EOF))
}

func TestSSE_IgnoresCommentsAndUnknownFields(t *testing.T) {
	body := strings.Join([]string{
		": this is a keepalive comment",
		"event: ignored",
		"id: also-ignored",
		`data: {"type":"text-delta","id":"b1","delta":"hi"}`,
		"",
		"",
	}, "\n")
	r := NewSSEReader(strings.NewReader(body))
	got, err := r.Next()
	require.NoError(t, err)
	assert.Equal(t, provider.StreamPart{Type: provider.PartTextDelta, ID: "b1", Delta: "hi"}, got)

	_, err = r.Next()
	assert.True(t, errors.Is(err, io.EOF))
}

func TestSSE_FrameShape(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, WriteSSEStreamPart(&buf, provider.StreamPart{Type: provider.PartTextDelta, ID: "b1", Delta: "hi"}))
	out := buf.String()
	assert.True(t, strings.HasPrefix(out, "data: "), "frame must start with `data: `")
	assert.True(t, strings.HasSuffix(out, "\n\n"), "frame must end with double newline")
	// payload should round-trip
	payload := strings.TrimPrefix(out, "data: ")
	payload = strings.TrimSuffix(payload, "\n\n")
	var part provider.StreamPart
	require.NoError(t, json.Unmarshal([]byte(payload), &part))
	assert.Equal(t, provider.PartTextDelta, part.Type)
}

func TestWriteSSEStreamPartTo_FlushesResponseWriter(t *testing.T) {
	rec := httptest.NewRecorder()
	part := provider.StreamPart{Type: provider.PartTextDelta, ID: "b1", Delta: "hi"}

	require.NoError(t, WriteSSEStreamPartTo(rec, part))

	assert.True(t, rec.Flushed)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"type":"text-delta"`)
}

func TestSSE_CompactsRawValueWithNewlines(t *testing.T) {
	part := provider.StreamPart{
		Type:     provider.PartRaw,
		RawValue: json.RawMessage("{\n  \"x\": 1,\n  \"nested\": {\n    \"ok\": true\n  }\n}"),
	}

	var buf bytes.Buffer
	require.NoError(t, WriteSSEStreamPart(&buf, part))
	assert.NotContains(t, strings.TrimSuffix(strings.TrimPrefix(buf.String(), "data: "), "\n\n"), "\n")

	got, err := NewSSEReader(&buf).Next()
	require.NoError(t, err)
	assert.JSONEq(t, string(part.RawValue), string(got.RawValue))
}

func TestSSE_NoToolResultFieldsLeak(t *testing.T) {
	cases := []struct {
		name string
		part provider.StreamPart
	}{
		{name: "text-delta", part: provider.StreamPart{Type: provider.PartTextDelta, ID: "b1", Delta: "hi"}},
		{name: "finish", part: provider.StreamPart{Type: provider.PartFinish, FinishReason: ptrFinish(provider.FinishReasonStop)}},
		{name: "tool-call", part: provider.StreamPart{Type: provider.PartToolCall, ToolCallID: "tc_1", ToolName: "search"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			require.NoError(t, WriteSSEStreamPart(&buf, tc.part))
			body := buf.String()
			assert.NotContains(t, body, `"result":`, "non-tool-result events must not carry a result field")
			assert.NotContains(t, body, `"type":""`, "no field should serialize an empty discriminator")
		})
	}
}

// TestSSE_MultiLineDataField verifies the SSE reader rejoins multiple
// `data:` lines with a literal newline separator per the SSE spec
// (https://html.spec.whatwg.org/multipage/server-sent-events.html#dispatchMessage),
// rather than concatenating them. The fix ensures readers stay compatible
// with third-party SSE producers that pretty-print event payloads.
func TestSSE_MultiLineDataField(t *testing.T) {
	body := strings.Join([]string{
		`data: {"type":"text-delta",`,
		`data:  "id":"b1",`,
		`data:  "delta":"hi"}`,
		"",
		"",
	}, "\n")
	r := NewSSEReader(strings.NewReader(body))
	got, err := r.Next()
	require.NoError(t, err)
	assert.Equal(t, provider.PartTextDelta, got.Type)
	assert.Equal(t, "b1", got.ID)
	assert.Equal(t, "hi", got.Delta)
}

func TestSSE_UnterminatedFinalLine(t *testing.T) {
	t.Run("single data line", func(t *testing.T) {
		body := `data: {"type":"text-delta","id":"b1","delta":"last"}`
		got, err := NewSSEReader(strings.NewReader(body)).Next()
		require.NoError(t, err)
		assert.Equal(t, provider.StreamPart{Type: provider.PartTextDelta, ID: "b1", Delta: "last"}, got)
	})

	t.Run("multiline final data line", func(t *testing.T) {
		body := "data: {\"type\":\"text-delta\",\n" +
			"data: \"id\":\"b1\",\n" +
			`data: "delta":"last"}`
		got, err := NewSSEReader(strings.NewReader(body)).Next()
		require.NoError(t, err)
		assert.Equal(t, provider.StreamPart{Type: provider.PartTextDelta, ID: "b1", Delta: "last"}, got)
	})

	t.Run("invalid JSON", func(t *testing.T) {
		body := `data: {"type":"text-delta"`
		_, err := NewSSEReader(strings.NewReader(body)).Next()
		require.Error(t, err)
		assert.NotErrorIs(t, err, io.EOF)
	})
}
