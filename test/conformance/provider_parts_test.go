//go:build conformance

package conformance

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/grafana/ai-sdk/provider"
	anthropicProvider "github.com/grafana/ai-sdk/providers/anthropic"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type partsStubModel struct {
	parts []provider.StreamPart
	seen  []provider.CallOptions
}

func (m *partsStubModel) SpecificationVersion() string               { return "v4" }
func (m *partsStubModel) Provider() string                           { return "stub" }
func (m *partsStubModel) ModelID() string                            { return "stub-model" }
func (m *partsStubModel) SupportedURLs() map[string][]*regexp.Regexp { return nil }
func (m *partsStubModel) DoGenerate(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
	return nil, nil
}
func (m *partsStubModel) DoStream(_ context.Context, opts provider.CallOptions) (*provider.StreamResult, error) {
	m.seen = append(m.seen, opts)
	stream := make(chan provider.StreamPart, len(m.parts))
	for _, part := range m.parts {
		stream <- part
	}
	close(stream)
	return &provider.StreamResult{Stream: stream}, nil
}

func TestRecordingModel_RecordsEveryCallWithRawChunks(t *testing.T) {
	stub := &partsStubModel{parts: []provider.StreamPart{
		{Type: provider.PartStreamStart},
		{Type: provider.PartRaw, RawValue: json.RawMessage(`{"type":"message_stop"}`)},
		{Type: provider.PartFinish},
	}}
	recorder := newRecordingModel(stub)

	for range 2 {
		result, err := recorder.DoStream(t.Context(), provider.CallOptions{})
		require.NoError(t, err)
		var forwarded []provider.StreamPartType
		for part := range result.Stream {
			forwarded = append(forwarded, part.Type)
		}
		assert.Equal(t, []provider.StreamPartType{provider.PartStreamStart, provider.PartRaw, provider.PartFinish}, forwarded)
	}

	require.Len(t, stub.seen, 2)
	for _, opts := range stub.seen {
		assert.True(t, opts.IncludeRawChunks)
	}
	calls := recorder.recordedCalls()
	require.Len(t, calls, 2)
	assert.Len(t, calls[0], 3)
	assert.Len(t, calls[1], 3)
}

func TestNormalizeProviderCalls(t *testing.T) {
	stamp := time.Date(2026, 7, 31, 17, 49, 43, 0, time.UTC)
	calls := [][]provider.StreamPart{{
		{Type: provider.PartStreamStart},
		{Type: provider.PartResponseMeta, ResponseID: "msg_1", ModelID: "model", Provider: "anthropic", Timestamp: stamp, ResponseHeaders: map[string]string{"a": "b"}, Usage: &provider.Usage{}},
		{Type: provider.PartSource, Source: &provider.SourceInfo{SourceType: provider.SourceTypeURL, ID: "random-a", URL: "https://a.example"}},
		{Type: provider.PartSource, Source: &provider.SourceInfo{SourceType: provider.SourceTypeURL, ID: "random-b", URL: "https://b.example"}},
		{Type: provider.PartToolCall, ToolCallID: "generated", ToolName: "mcp.lookup"},
		{Type: provider.PartToolApprovalRequest, ToolCallID: "generated", ApprovalID: "approval"},
		{Type: provider.PartToolCall, ToolCallID: "provided", ToolName: "lookup"},
		{Type: provider.PartToolResult, ToolCallID: "provided"},
		{Type: provider.PartError, APICallError: &provider.APICallError{Message: "Overloaded", IsRetryable: true, StatusCode: 529, URL: "http://x"}},
		{Type: provider.PartError, APICallError: &provider.APICallError{Message: "go style", URL: "http://x"}},
	}}

	normalized := normalizeProviderCalls(t, "anthropic", calls)

	require.Len(t, normalized, 1)
	parts := normalized[0]
	assert.Equal(t, map[string]any{"type": "stream-start", "warnings": []any{}}, parts[0])
	assert.Equal(t, map[string]any{"type": "response-metadata", "id": "msg_1", "modelId": "model", "timestamp": "2026-07-31T17:49:43.000Z"}, parts[1])
	assert.Equal(t, "src-0", parts[2]["id"])
	assert.Equal(t, "src-1", parts[3]["id"])
	assert.Equal(t, "mcp-0", parts[4]["toolCallId"])
	assert.Equal(t, "mcp-0", parts[5]["toolCallId"])
	assert.Equal(t, "provided", parts[6]["toolCallId"])
	assert.Equal(t, false, parts[7]["isError"])
	assert.Equal(t, map[string]any{"type": "error", "error": map[string]any{"message": "Overloaded", "statusCode": float64(529), "isRetryable": true}}, parts[8])
	assert.Equal(t, map[string]any{"type": "error"}, parts[9], "errors without a status are compared by position only")

	t.Run("volatile timestamps are dropped for Bedrock", func(t *testing.T) {
		bedrock := normalizeProviderCalls(t, "bedrock", calls[:1])
		assert.Equal(t, map[string]any{"type": "response-metadata", "id": "msg_1", "modelId": "model"}, bedrock[0][1])
	})
}

func TestProviderPartsMismatch(t *testing.T) {
	part := func(kind string) map[string]any { return map[string]any{"type": kind} }
	call := func(kinds ...string) providerPartsCall {
		parts := providerPartsCall{}
		for _, kind := range kinds {
			parts = append(parts, part(kind))
		}
		return parts
	}

	for _, tc := range []struct {
		name     string
		expected []providerPartsCall
		actual   []providerPartsCall
		want     string
	}{
		{name: "equal", expected: []providerPartsCall{call("raw", "finish")}, actual: []providerPartsCall{call("raw", "finish")}},
		{name: "call count", expected: []providerPartsCall{call("finish"), call("finish")}, actual: []providerPartsCall{call("finish")}, want: "model call count"},
		{name: "order", expected: []providerPartsCall{call("raw", "finish")}, actual: []providerPartsCall{call("finish", "raw")}, want: "call 0 part 0"},
		{name: "missing finish", expected: []providerPartsCall{call("raw", "finish")}, actual: []providerPartsCall{call("raw")}, want: "stream ended at part 1"},
		{name: "extra finish", expected: []providerPartsCall{call("finish")}, actual: []providerPartsCall{call("finish", "finish")}, want: "unexpected part 1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mismatch := providerPartsMismatch(tc.expected, tc.actual)
			if tc.want == "" {
				assert.Empty(t, mismatch)
				return
			}
			assert.Contains(t, mismatch, tc.want)
		})
	}
}

func TestLoadExpectedProviderParts(t *testing.T) {
	t.Run("missing golden is an error", func(t *testing.T) {
		_, err := loadExpectedProviderParts(filepath.Join(t.TempDir(), providerPartsFile))
		require.Error(t, err)
		assert.ErrorIs(t, err, os.ErrNotExist)
	})

	t.Run("one call per line", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), providerPartsFile)
		require.NoError(t, os.WriteFile(path, []byte(`{"parts":[{"type":"finish"}]}`+"\n"+`{"parts":[]}`+"\n"), 0o600))
		calls, err := loadExpectedProviderParts(path)
		require.NoError(t, err)
		require.Len(t, calls, 2)
		assert.Len(t, calls[0], 1)
		assert.Empty(t, calls[1])
	})
}

const (
	anthropicStreamPartsDir   = "testdata/anthropic-stream-parts"
	anthropicStreamPartsModel = "claude-sonnet-4-5"
)

type syntheticStreamCase struct {
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Events      []json.RawMessage `json:"events"`
}

type syntheticStreamExpectation struct {
	Name      string            `json:"name"`
	Parts     providerPartsCall `json:"parts"`
	CallError bool              `json:"callError"`
}

func loadSyntheticStreamCases(t *testing.T) ([]syntheticStreamCase, map[string]syntheticStreamExpectation) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(anthropicStreamPartsDir, "cases.json"))
	require.NoError(t, err)
	var cases []syntheticStreamCase
	require.NoError(t, json.Unmarshal(data, &cases))

	records, err := readJSONL[syntheticStreamExpectation](filepath.Join(anthropicStreamPartsDir, "expected-parts.jsonl"))
	require.NoError(t, err)
	expectations := map[string]syntheticStreamExpectation{}
	for _, record := range records {
		expectations[record.Name] = record
	}
	return cases, expectations
}

func syntheticStreamServer(t *testing.T, events []json.RawMessage) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		for _, event := range events {
			var compact bytes.Buffer
			if err := json.Compact(&compact, event); err != nil {
				t.Errorf("compacting synthetic event: %v", err)
				return
			}
			_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", extractSSEEventType(compact.String()), compact.Bytes())
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func TestAnthropicSyntheticStreamParts(t *testing.T) {
	cases, expectations := loadSyntheticStreamCases(t)
	require.NotEmpty(t, cases)
	require.Len(t, expectations, len(cases), "expected-parts.jsonl must cover every case; regenerate with mise run generate-conformance")

	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			expectation, ok := expectations[tc.Name]
			require.True(t, ok, "no upstream expectation for case %q", tc.Name)

			server := syntheticStreamServer(t, tc.Events)
			model := anthropicProvider.New("test-api-key", anthropicStreamPartsModel,
				anthropicProvider.WithRequestOptions(option.WithBaseURL(server.URL), option.WithMaxRetries(0)))
			result, err := model.DoStream(t.Context(), provider.CallOptions{
				Prompt:           []provider.Message{provider.NewUserMessage(provider.TextPart("test"))},
				IncludeRawChunks: true,
			})

			if expectation.CallError {
				require.Error(t, err, "upstream fails the call before returning a stream")
				var apiErr *provider.APICallError
				require.ErrorAs(t, err, &apiErr)
				return
			}
			require.NoError(t, err)
			var parts []provider.StreamPart
			for part := range result.Stream {
				parts = append(parts, part)
			}
			actual := normalizeProviderCalls(t, "anthropic", [][]provider.StreamPart{parts})
			if mismatch := providerPartsMismatch([]providerPartsCall{expectation.Parts}, actual); mismatch != "" {
				require.Fail(t, "provider parts differ from upstream", mismatch)
			}
		})
	}
}

const (
	anthropicUnaryDir   = "testdata/anthropic-unary"
	anthropicUnaryModel = "claude-fable-5-1"
)

type syntheticUnaryCase struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Response    json.RawMessage `json:"response"`
}

type syntheticUnaryExpectation struct {
	Name      string          `json:"name"`
	Result    json.RawMessage `json:"result"`
	CallError bool            `json:"callError"`
}

func TestAnthropicSyntheticUnary(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(anthropicUnaryDir, "cases.json"))
	require.NoError(t, err)
	var cases []syntheticUnaryCase
	require.NoError(t, json.Unmarshal(data, &cases))
	require.NotEmpty(t, cases)

	records, err := readJSONL[syntheticUnaryExpectation](filepath.Join(anthropicUnaryDir, "expected-results.jsonl"))
	require.NoError(t, err)
	expectations := map[string]syntheticUnaryExpectation{}
	for _, record := range records {
		expectations[record.Name] = record
	}
	require.Len(t, expectations, len(cases), "expected-results.jsonl must cover every case; regenerate with mise run generate-conformance")

	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			expectation, ok := expectations[tc.Name]
			require.True(t, ok, "no upstream expectation for case %q", tc.Name)

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write(tc.Response)
			}))
			defer server.Close()
			model := anthropicProvider.New("test-api-key", anthropicUnaryModel,
				anthropicProvider.WithRequestOptions(option.WithBaseURL(server.URL), option.WithMaxRetries(0)))
			maxOutputTokens := 1024
			result, err := model.DoGenerate(t.Context(), provider.CallOptions{
				Prompt:          []provider.Message{provider.NewUserMessage(provider.TextPart("test"))},
				MaxOutputTokens: &maxOutputTokens,
			})

			if expectation.CallError {
				require.Error(t, err, "upstream fails the call")
				return
			}
			require.NoError(t, err)
			actual, err := json.Marshal(generateResultSnapshot{
				Content:          result.Content,
				FinishReason:     result.FinishReason,
				Usage:            result.Usage,
				ProviderMetadata: result.ProviderMetadata,
				Warnings:         result.Warnings,
			})
			require.NoError(t, err)
			require.JSONEq(t, string(expectation.Result), string(actual))
		})
	}
}
