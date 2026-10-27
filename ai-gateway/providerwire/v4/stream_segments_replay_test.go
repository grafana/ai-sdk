package v4

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const upstreamFixtureRoot = "../../../test/conformance"

type upstreamReplayCase struct {
	path string
	adjust func(map[string]any)
}

func dropFalseIsError(part map[string]any) {
	if part["type"] == string(provider.PartToolResult) && part["isError"] == false {
		delete(part, "isError")
	}
}

func projectTimestampEncoding(t *testing.T, part map[string]any) {
	t.Helper()
	value, ok := part["timestamp"].(string)
	if !ok {
		return
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	require.NoError(t, err)
	part["timestamp"] = parsed.UTC().Format(time.RFC3339Nano)
}

func readUpstreamProviderParts(t *testing.T, path string) ([]provider.StreamPart, []map[string]any) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(upstreamFixtureRoot, path, "expected-provider-parts.jsonl"))
	require.NoError(t, err)
	var golden struct {
		Parts []json.RawMessage `json:"parts"`
	}
	require.NoError(t, json.Unmarshal(data, &golden))
	return decodeUpstreamParts(t, golden.Parts)
}

func readSyntheticAnthropicParts(t *testing.T, name string) ([]provider.StreamPart, []map[string]any) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(upstreamFixtureRoot, "testdata", "anthropic-stream-parts", "expected-parts.jsonl"))
	require.NoError(t, err)
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		var golden struct {
			Name  string            `json:"name"`
			Parts []json.RawMessage `json:"parts"`
		}
		require.NoError(t, json.Unmarshal([]byte(line), &golden))
		if golden.Name == name {
			return decodeUpstreamParts(t, golden.Parts)
		}
	}
	require.FailNow(t, "synthetic case not found", name)
	return nil, nil
}

func decodeUpstreamParts(t *testing.T, raws []json.RawMessage) ([]provider.StreamPart, []map[string]any) {
	t.Helper()
	parts := make([]provider.StreamPart, len(raws))
	expected := make([]map[string]any, len(raws))
	for i, raw := range raws {
		require.NoError(t, parts[i].UnmarshalJSON(raw), string(raw))
		require.NoError(t, json.Unmarshal(raw, &expected[i]))
	}
	return parts, expected
}

func replayWrittenEvents(t *testing.T, parts []provider.StreamPart) []map[string]any {
	t.Helper()
	harness := newRuntimeHarness(t, testLimits())
	harness.model.stream = func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
		return &provider.StreamResult{Stream: makeStream(parts...)}, nil
	}
	response := harness.serve(streamRequest(segmentRawRequest))
	require.Equal(t, http.StatusOK, response.Code)
	body := response.Body.String()
	requireStreamBodyMatchesSchema(t, body)
	var events []map[string]any
	for _, frame := range strings.Split(strings.TrimSuffix(body, "\n\n"), "\n\n") {
		var event map[string]any
		require.NoError(t, json.Unmarshal([]byte(strings.TrimPrefix(frame, "data: ")), &event))
		events = append(events, event)
	}
	return events
}

func TestStreamingSegments_UpstreamFixtureReplay(t *testing.T) {
	for _, tc := range []upstreamReplayCase{
		{path: "anthropic/upstream/programmatic-tool-calling", adjust: dropFalseIsError},
		{path: "anthropic/upstream/tool-search-bm25", adjust: dropFalseIsError},
		{path: "anthropic/upstream/tool-search-regex", adjust: dropFalseIsError},
		{path: "anthropic/upstream/tool-search-deferred-bm25", adjust: dropFalseIsError},
		{path: "anthropic/upstream/tool-search-deferred-regex", adjust: dropFalseIsError},
		{path: "openai/upstream/reasoning-encrypted-content"},
		{path: "openai/upstream/shell"},
	} {
		t.Run(tc.path, func(t *testing.T) {
			parts, expected := readUpstreamProviderParts(t, tc.path)
			if tc.adjust != nil {
				for _, part := range expected {
					tc.adjust(part)
				}
			}
			events := replayWrittenEvents(t, parts)
			require.Equal(t, len(expected), len(events), "written event count")
			for i := range expected {
				projectTimestampEncoding(t, expected[i])
				projectTimestampEncoding(t, events[i])
				assert.Equal(t, expected[i], events[i], "part %d", i)
			}
		})
	}
}

func TestStreamingSegments_SyntheticAnthropicReplay(t *testing.T) {
	t.Run("consecutive-messages", func(t *testing.T) {
		parts, expected := readSyntheticAnthropicParts(t, "consecutive-messages")
		events := replayWrittenEvents(t, parts)
		require.Equal(t, len(expected), len(events), "written event count")
		for i := range expected {
			assert.Equal(t, expected[i], events[i], "part %d", i)
		}
	})

	t.Run("overlapping-message-start", func(t *testing.T) {
		parts, expected := readSyntheticAnthropicParts(t, "overlapping-message-start")
		require.Equal(t, string(provider.PartError), expected[len(expected)-1]["type"])
		events := replayWrittenEvents(t, parts)
		require.Equal(t, len(expected)+1, len(events), "written event count")
		for i := range expected[:len(expected)-1] {
			assert.Equal(t, expected[i], events[i], "part %d", i)
		}
		for _, event := range events[len(expected)-1:] {
			assert.Equal(t, "error", event["type"])
			assert.Equal(t, string(codeInternal), event["error"].(map[string]any)["code"])
		}
	})
}
