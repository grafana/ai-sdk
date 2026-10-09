//go:build conformance

package conformance

import (
	"encoding/json"
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
