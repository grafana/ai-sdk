//go:build conformance

package conformance

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCompareScenario_Artifacts(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*ScenarioResult, []RequestSnapshot)
		want   string
	}{
		{"matching", func(*ScenarioResult, []RequestSnapshot) {}, ""},
		{"chunks", func(r *ScenarioResult, _ []RequestSnapshot) { r.Chunks = nil }, "chunk count"},
		{"request", func(_ *ScenarioResult, requests []RequestSnapshot) { requests[0].Path = "/wrong" }, "path mismatch"},
		{"object", func(r *ScenarioResult, _ []RequestSnapshot) { r.Object = nil }, "expected-object.json"},
		{"usage", func(r *ScenarioResult, _ []RequestSnapshot) { r.Usage = nil }, "expected-usage.json"},
		{"error", func(r *ScenarioResult, _ []RequestSnapshot) { r.Error = "rejected" }, "unexpected execution error"},
		{"provider parts", func(r *ScenarioResult, _ []RequestSnapshot) { r.ProviderParts[0][0]["type"] = "text-start" }, "provider parts differ"},
		{"missing provider parts", func(r *ScenarioResult, _ []RequestSnapshot) { r.ProviderParts = nil }, "model call count"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			for file, body := range map[string]string{
				"config.yaml":             "model: test\nassertOutputValue: true\n",
				"expected.jsonl":          "{\"type\":\"start\"}\n",
				"expected-requests.jsonl": "{\"method\":\"POST\",\"path\":\"/v1\",\"headers\":{},\"body\":{\"count\":1}}\n",
				"expected-object.json":    "{\"value\":true}",
				"expected-usage.json":     "[]",
				providerPartsFile:         "{\"parts\":[{\"type\":\"stream-start\"}]}\n",
			} {
				require.NoError(t, os.WriteFile(filepath.Join(dir, file), []byte(body), 0o600))
			}
			result := ScenarioResult{Chunks: []map[string]any{{"type": "start"}}, Object: map[string]any{"value": true}, Usage: []provider.Usage{}, ProviderParts: []providerPartsCall{{{"type": "stream-start"}}}}
			requests := []RequestSnapshot{{Method: "POST", Path: "/v1", Headers: map[string]string{}, Body: map[string]any{"count": float64(1)}}}
			tc.mutate(&result, requests)
			errors, err := CompareScenario(dir, result, requests, false)
			require.NoError(t, err)
			if tc.want == "" {
				assert.Empty(t, errors)
			} else {
				assert.Contains(t, strings.Join(errors, "\n"), tc.want)
			}
		})
	}
}

func TestCompareScenario_UnaryAndEarlyFailure(t *testing.T) {
	dir := filepath.Join("bedrock", "upstream", "json-tool-with-answer")
	data, err := os.ReadFile(filepath.Join(dir, "expected-generate.json"))
	require.NoError(t, err)
	var generate any
	require.NoError(t, json.Unmarshal(data, &generate))
	requests, err := LoadExpectedRequests(filepath.Join(dir, "expected-requests.jsonl"))
	require.NoError(t, err)
	errors, err := CompareScenario(dir, ScenarioResult{Generate: generate}, requests, true)
	require.NoError(t, err)
	assert.Empty(t, errors)
	errors, err = CompareScenario(dir, ScenarioResult{Error: "unsupported"}, nil, true)
	require.NoError(t, err)
	combined := strings.Join(errors, "\n")
	assert.Contains(t, combined, "expected-generate.json")
	assert.Contains(t, combined, "unexpected execution error")
	assert.Contains(t, combined, "request count mismatch")
}

func TestCompareScenario_GatewayExecutionMetadata(t *testing.T) {
	finish := func(metadata map[string]any) map[string]any {
		part := map[string]any{"type": "finish", "finishReason": map[string]any{"unified": "stop"}}
		if metadata != nil {
			part["providerMetadata"] = metadata
		}
		return part
	}
	execution := map[string]any{"attempts": []any{map[string]any{"outcome": "selected"}}}
	native := map[string]any{"anthropic": map[string]any{"stopSequence": nil}}
	for _, tc := range []struct {
		name     string
		expected map[string]any
		actual   map[string]any
		gateway  bool
		want     string
	}{
		{"gateway lane ignores execution", nil, map[string]any{"gateway": map[string]any{"execution": execution}}, true, ""},
		{"gateway lane keeps native metadata", native, map[string]any{"anthropic": native["anthropic"], "gateway": map[string]any{"execution": execution}}, true, ""},
		{"direct lane stays strict", nil, map[string]any{"gateway": map[string]any{"execution": execution}}, false, "provider parts differ"},
		{"other gateway keys stay compared", nil, map[string]any{"gateway": map[string]any{"execution": execution, "nativeMetadata": map[string]any{}}}, true, "provider parts differ"},
		{"native difference stays compared", native, map[string]any{"anthropic": map[string]any{"stopSequence": "x"}, "gateway": map[string]any{"execution": execution}}, true, "provider parts differ"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			expected, err := json.Marshal(map[string]any{"parts": []any{finish(tc.expected)}})
			require.NoError(t, err)
			for file, body := range map[string]string{
				"config.yaml":             "model: test\n",
				"expected.jsonl":          "{\"type\":\"start\"}\n",
				"expected-requests.jsonl": "",
				providerPartsFile:         string(expected) + "\n",
			} {
				require.NoError(t, os.WriteFile(filepath.Join(dir, file), []byte(body), 0o600))
			}
			actual := []providerPartsCall{{finish(tc.actual)}}
			result := ScenarioResult{Chunks: []map[string]any{{"type": "start"}}, ProviderParts: actual}
			errors, err := CompareScenario(dir, result, nil, tc.gateway)
			require.NoError(t, err)
			if tc.want == "" {
				assert.Empty(t, errors)
			} else {
				assert.Contains(t, strings.Join(errors, "\n"), tc.want)
			}
			assert.Contains(t, actual[0][0]["providerMetadata"].(map[string]any), "gateway", "comparison must not mutate the capture")
		})
	}
}
