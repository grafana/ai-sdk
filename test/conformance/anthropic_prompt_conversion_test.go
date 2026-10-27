//go:build conformance

package conformance

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/grafana/ai-sdk/provider"
	anthropicProvider "github.com/grafana/ai-sdk/providers/anthropic"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	anthropicPromptConversionDir   = "testdata/anthropic-prompt-conversion"
	anthropicPromptConversionModel = "claude-sonnet-4-5"
	anthropicPromptConversionMax   = 1024
	anthropicPromptConversionReply = `{"id":"msg_1","type":"message","role":"assistant","model":"claude-sonnet-4-5","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","stop_sequence":null,"usage":{"input_tokens":1,"output_tokens":1}}`
)

// anthropicMCPResultFallthroughWarnings lists the upstream warning that the
// registered @ai-sdk/anthropic emits after converting a hosted-MCP result, per
// the anthropic-mcp-result-fallthrough-warning deviation.
var anthropicMCPResultFallthroughWarnings = map[string]string{
	"mcp-result-in-merged-assistant-block": "provider executed tool result for tool search_docs is not supported",
	"mcp-result-error-json-object":         "provider executed tool result for tool search_docs is not supported",
	"mcp-result-missing-server-name":       "provider executed tool result for tool search_docs is not supported",
}

type promptConversionCase struct {
	Name        string             `json:"name"`
	Description string             `json:"description"`
	Prompt      []provider.Message `json:"prompt"`
}

type promptConversionExpectation struct {
	Name     string             `json:"name"`
	Request  json.RawMessage    `json:"request"`
	Warnings []provider.Warning `json:"warnings"`
}

type projectedWarning struct {
	Type    provider.WarningType
	Message string
}

func projectWarnings(warnings []provider.Warning) []projectedWarning {
	projected := []projectedWarning{}
	for _, warning := range warnings {
		projected = append(projected, projectedWarning{Type: warning.Type, Message: warning.Message})
	}
	return projected
}

func decodeExpectedRequest(t *testing.T, raw json.RawMessage) RequestSnapshot {
	t.Helper()
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var snapshot RequestSnapshot
	require.NoError(t, dec.Decode(&snapshot))
	snapshot.Body = normalizeJSONValue("", snapshot.Body)
	return snapshot
}

func TestAnthropicSyntheticPromptConversion(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(anthropicPromptConversionDir, "cases.json"))
	require.NoError(t, err)
	var cases []promptConversionCase
	require.NoError(t, json.Unmarshal(data, &cases))
	require.NotEmpty(t, cases)

	records, err := readJSONL[promptConversionExpectation](filepath.Join(anthropicPromptConversionDir, "expected-results.jsonl"))
	require.NoError(t, err)
	expectations := map[string]promptConversionExpectation{}
	for _, record := range records {
		expectations[record.Name] = record
	}
	require.Len(t, expectations, len(cases), "expected-results.jsonl must cover every case; regenerate with mise run generate-conformance")

	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			expectation, ok := expectations[tc.Name]
			require.True(t, ok, "no upstream expectation for case %q", tc.Name)

			var captured []RequestSnapshot
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				require.NoError(t, err)
				snapshot, err := newRequestSnapshot("anthropic", r, body)
				require.NoError(t, err)
				captured = append(captured, snapshot)
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(anthropicPromptConversionReply))
			}))
			defer server.Close()
			model := anthropicProvider.New("test-api-key", anthropicPromptConversionModel,
				anthropicProvider.WithRequestOptions(option.WithBaseURL(server.URL), option.WithMaxRetries(0)))
			maxOutputTokens := anthropicPromptConversionMax
			result, err := model.DoGenerate(t.Context(), provider.CallOptions{
				Prompt:          tc.Prompt,
				MaxOutputTokens: &maxOutputTokens,
			})
			require.NoError(t, err)

			CompareRequestSnapshots(t, []RequestSnapshot{decodeExpectedRequest(t, expectation.Request)}, captured)

			upstreamWarnings := projectWarnings(expectation.Warnings)
			if message, ok := anthropicMCPResultFallthroughWarnings[tc.Name]; ok {
				fallthroughWarning := projectedWarning{Type: provider.WarnOther, Message: message}
				index := slices.Index(upstreamWarnings, fallthroughWarning)
				require.GreaterOrEqual(t, index, 0, "upstream no longer emits the fall-through warning; revisit the deviation")
				upstreamWarnings = slices.Delete(upstreamWarnings, index, index+1)
			}
			assert.Equal(t, upstreamWarnings, projectWarnings(result.Warnings))
		})
	}
}
