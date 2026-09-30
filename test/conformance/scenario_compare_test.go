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
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			for file, body := range map[string]string{
				"config.yaml":             "model: test\nassertOutputValue: true\n",
				"expected.jsonl":          "{\"type\":\"start\"}\n",
				"expected-requests.jsonl": "{\"method\":\"POST\",\"path\":\"/v1\",\"headers\":{},\"body\":{\"count\":1}}\n",
				"expected-object.json":    "{\"value\":true}",
				"expected-usage.json":     "[]",
			} {
				require.NoError(t, os.WriteFile(filepath.Join(dir, file), []byte(body), 0o600))
			}
			result := ScenarioResult{Chunks: []map[string]any{{"type": "start"}}, Object: map[string]any{"value": true}, Usage: []provider.Usage{}}
			requests := []RequestSnapshot{{Method: "POST", Path: "/v1", Headers: map[string]string{}, Body: map[string]any{"count": float64(1)}}}
			tc.mutate(&result, requests)
			errors, err := CompareScenario(dir, result, requests)
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
	errors, err := CompareScenario(dir, ScenarioResult{Generate: generate}, requests)
	require.NoError(t, err)
	assert.Empty(t, errors)
	errors, err = CompareScenario(dir, ScenarioResult{Error: "unsupported"}, nil)
	require.NoError(t, err)
	combined := strings.Join(errors, "\n")
	assert.Contains(t, combined, "expected-generate.json")
	assert.Contains(t, combined, "unexpected execution error")
	assert.Contains(t, combined, "request count mismatch")
}
