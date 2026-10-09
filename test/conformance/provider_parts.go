//go:build conformance

package conformance

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/require"
)

const providerPartsFile = "expected-provider-parts.jsonl"

var providerPartsProviders = map[string]bool{"anthropic": true, "openai": true, "bedrock": true, "openai-compatible": true}

// providerPartsAllowlist names cases whose provider parts are known to differ
// from upstream for reasons outside the change under test. Each entry must
// carry the tracking issue.
var providerPartsAllowlist = map[string]string{}

type providerPartsCall = []map[string]any

type recordingModel struct {
	provider.LanguageModel

	mu    sync.Mutex
	calls []*[]provider.StreamPart
}

func newRecordingModel(model provider.LanguageModel) *recordingModel {
	return &recordingModel{LanguageModel: model}
}

func (m *recordingModel) DoStream(ctx context.Context, opts provider.CallOptions) (*provider.StreamResult, error) {
	opts.IncludeRawChunks = true
	result, err := m.LanguageModel.DoStream(ctx, opts)
	if err != nil {
		return nil, err
	}

	recorded := &[]provider.StreamPart{}
	m.mu.Lock()
	m.calls = append(m.calls, recorded)
	m.mu.Unlock()

	out := make(chan provider.StreamPart, 64)
	go func() {
		defer close(out)
		for part := range result.Stream {
			m.mu.Lock()
			*recorded = append(*recorded, part)
			m.mu.Unlock()
			select {
			case out <- part:
			case <-ctx.Done():
				for range result.Stream {
				}
				return
			}
		}
	}()

	forwarded := *result
	forwarded.Stream = out
	return &forwarded, nil
}

func (m *recordingModel) recordedCalls() [][]provider.StreamPart {
	m.mu.Lock()
	defer m.mu.Unlock()
	calls := make([][]provider.StreamPart, len(m.calls))
	for i, call := range m.calls {
		calls[i] = append([]provider.StreamPart(nil), *call...)
	}
	return calls
}

func normalizeProviderCalls(t *testing.T, providerName string, calls [][]provider.StreamPart) []providerPartsCall {
	t.Helper()
	normalized := make([]providerPartsCall, len(calls))
	for i, call := range calls {
		parts := make(providerPartsCall, 0, len(call))
		for _, part := range call {
			data, err := json.Marshal(part)
			require.NoError(t, err, "marshaling provider part")
			var generic map[string]any
			require.NoError(t, json.Unmarshal(data, &generic), "parsing provider part")
			parts = append(parts, normalizeProviderPart(providerName, generic))
		}
		normalized[i] = normalizeGeneratedIDs(parts)
	}
	return normalized
}

// normalizeGeneratedIDs replaces IDs the providers generate themselves: source
// IDs become src-N and MCP tool-call IDs become mcp-N, both per call.
func normalizeGeneratedIDs(parts providerPartsCall) providerPartsCall {
	mcpIDs := map[string]string{}
	for _, part := range parts {
		name, _ := part["toolName"].(string)
		id, _ := part["toolCallId"].(string)
		if part["type"] == string(provider.PartToolCall) && strings.HasPrefix(name, "mcp.") && id != "" {
			if _, seen := mcpIDs[id]; !seen {
				mcpIDs[id] = fmt.Sprintf("mcp-%d", len(mcpIDs))
			}
		}
	}
	sourceCount := 0
	for _, part := range parts {
		if part["type"] == string(provider.PartSource) {
			part["id"] = fmt.Sprintf("src-%d", sourceCount)
			sourceCount++
			continue
		}
		if id, _ := part["toolCallId"].(string); mcpIDs[id] != "" {
			part["toolCallId"] = mcpIDs[id]
		}
	}
	return parts
}

// volatileTimestampProviders derive response-metadata timestamps from the HTTP
// Date header, which differs on every replay.
var volatileTimestampProviders = map[string]bool{"bedrock": true}

func normalizeProviderPart(providerName string, part map[string]any) map[string]any {
	switch part["type"] {
	case string(provider.PartResponseMeta):
		normalized := map[string]any{"type": part["type"]}
		keys := []string{"id", "modelId"}
		if !volatileTimestampProviders[providerName] {
			keys = append(keys, "timestamp")
		}
		for _, key := range keys {
			if value, ok := part[key]; ok {
				normalized[key] = value
			}
		}
		return normalized
	case string(provider.PartError):
		normalized := map[string]any{"type": part["type"]}
		if errValue, _ := part["error"].(map[string]any); errValue != nil {
			if status, _ := errValue["statusCode"].(float64); status > 0 {
				retryable, _ := errValue["isRetryable"].(bool)
				normalized["error"] = map[string]any{"message": errValue["message"], "statusCode": status, "isRetryable": retryable}
			}
		}
		return normalized
	case string(provider.PartToolResult):
		if isError, _ := part["isError"].(bool); !isError {
			part["isError"] = false
		}
	}
	return part
}

func loadExpectedProviderParts(path string) ([]providerPartsCall, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()

	var calls []providerPartsCall
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 1<<20), 64<<20)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var call struct {
			Parts providerPartsCall `json:"parts"`
		}
		if err := json.Unmarshal(line, &call); err != nil {
			return nil, fmt.Errorf("parsing provider parts call: %w", err)
		}
		calls = append(calls, call.Parts)
	}
	return calls, scanner.Err()
}

func providerPartsMismatch(expected, actual []providerPartsCall) string {
	if len(expected) != len(actual) {
		return fmt.Sprintf("model call count: expected %d, got %d", len(expected), len(actual))
	}
	for callIdx := range expected {
		for partIdx := 0; partIdx < len(expected[callIdx]) || partIdx < len(actual[callIdx]); partIdx++ {
			switch {
			case partIdx >= len(actual[callIdx]):
				return fmt.Sprintf("call %d: stream ended at part %d; expected %s", callIdx, partIdx, mustJSON(expected[callIdx][partIdx]))
			case partIdx >= len(expected[callIdx]):
				return fmt.Sprintf("call %d: unexpected part %d: %s", callIdx, partIdx, mustJSON(actual[callIdx][partIdx]))
			}
			want, got := mustJSON(expected[callIdx][partIdx]), mustJSON(actual[callIdx][partIdx])
			if want != got {
				return fmt.Sprintf("call %d part %d (%d expected parts, %d actual):\n  expected: %s\n  actual:   %s",
					callIdx, partIdx, len(expected[callIdx]), len(actual[callIdx]), want, got)
			}
		}
	}
	return ""
}

func mustJSON(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprintf("<unmarshalable: %v>", err)
	}
	return string(data)
}

func assertProviderParts(t *testing.T, tc TestCase, recorder *recordingModel) {
	t.Helper()
	t.Run("provider-parts", func(t *testing.T) {
		if issue, ok := providerPartsAllowlist[tc.Name]; ok {
			t.Skipf("known provider-parts difference tracked in %s", issue)
		}
		expected, err := loadExpectedProviderParts(filepath.Join(tc.Dir, providerPartsFile))
		require.NoError(t, err, "loading %s", providerPartsFile)
		actual := normalizeProviderCalls(t, tc.Provider, recorder.recordedCalls())
		if mismatch := providerPartsMismatch(expected, actual); mismatch != "" {
			require.Fail(t, "provider parts differ from upstream", mismatch)
		}
	})
}
