//go:build conformance

package conformance

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/grafana/ai-sdk/provider"
	anthropicProvider "github.com/grafana/ai-sdk/providers/anthropic"
	"github.com/stretchr/testify/require"
)

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

	file, err := os.Open(filepath.Join(anthropicStreamPartsDir, "expected-parts.jsonl"))
	require.NoError(t, err)
	defer func() { _ = file.Close() }()
	expectations := map[string]syntheticStreamExpectation{}
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 1<<20), 64<<20)
	for scanner.Scan() {
		var expectation syntheticStreamExpectation
		require.NoError(t, json.Unmarshal(scanner.Bytes(), &expectation))
		expectations[expectation.Name] = expectation
	}
	require.NoError(t, scanner.Err())
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
