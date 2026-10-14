package conformance

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/grafana/ai-sdk"
	"github.com/grafana/ai-sdk/output"
	"github.com/grafana/ai-sdk/provider"
	"github.com/grafana/ai-sdk/schema"
	"github.com/stretchr/testify/require"
)

type uiFixtureModel struct {
	parts []provider.StreamPart
}

func (m uiFixtureModel) SpecificationVersion() string { return "v4" }
func (m uiFixtureModel) Provider() string             { return "test" }
func (m uiFixtureModel) ModelID() string              { return "test-model" }
func (m uiFixtureModel) SupportedURLs() map[string][]*regexp.Regexp {
	return nil
}
func (m uiFixtureModel) DoStream(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
	stream := make(chan provider.StreamPart, len(m.parts))
	for _, part := range m.parts {
		stream <- part
	}
	close(stream)
	return &provider.StreamResult{Stream: stream}, nil
}
func (m uiFixtureModel) DoGenerate(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
	return nil, errors.New("ui fixture model does not support generate")
}

func TestUIConformance_Sources(t *testing.T) {
	fixtureDir := filepath.Join("ui", "sources")
	parts := loadUIFixtureParts(t, filepath.Join(fixtureDir, "input.jsonl"))
	expected := loadUIExpected(t, filepath.Join(fixtureDir, "expected.jsonl"))
	result := aisdk.StreamText(context.Background(), uiFixtureModel{parts: parts}, aisdk.WithModelMessages(provider.UserText("test")))
	var actual []map[string]any
	for chunk := range result.ToUIMessageStream(aisdk.WithUIMessageStreamSources(true), aisdk.WithUIMessageStreamGenerateID(func() string { return "message-1" })) {
		data, err := json.Marshal(chunk)
		require.NoError(t, err)
		var decoded map[string]any
		require.NoError(t, json.Unmarshal(data, &decoded))
		actual = append(actual, decoded)
	}
	require.NoError(t, result.Err())
	require.Equal(t, expected, actual)
}

func TestUIConformance_LocalToolPreliminary(t *testing.T) {
	fixtureDir := filepath.Join("ui", "local-tool-preliminary")
	parts := loadUIFixtureParts(t, filepath.Join(fixtureDir, "input.jsonl"))
	expected := loadUIExpected(t, filepath.Join(fixtureDir, "expected.jsonl"))
	result := aisdk.StreamText(t.Context(), uiFixtureModel{parts: parts},
		aisdk.WithModelMessages(provider.UserText("test")),
		aisdk.WithTools(aisdk.ToolSet{"lookup": {ExecuteStream: func(_ context.Context, _ json.RawMessage, _ aisdk.ToolExecutionOptions, emit func(json.RawMessage) error) error {
			if err := emit(json.RawMessage(`{"stage":"loading"}`)); err != nil {
				return err
			}
			return emit(json.RawMessage(`{"stage":"done"}`))
		}}}),
	)
	var actual []map[string]any
	for chunk := range result.ToUIMessageStream(aisdk.WithUIMessageStreamGenerateID(func() string { return "message-1" })) {
		data, err := json.Marshal(chunk)
		require.NoError(t, err)
		var decoded map[string]any
		require.NoError(t, json.Unmarshal(data, &decoded))
		actual = append(actual, decoded)
	}
	require.NoError(t, result.Err())
	require.Equal(t, expected, actual)
}

func TestUIConformance_ReasoningFiles(t *testing.T) {
	fixtureDir := filepath.Join("ui", "generated-files", "data-and-url")
	parts := loadUIFixtureParts(t, filepath.Join(fixtureDir, "input.jsonl"))
	expected := loadUIExpected(t, filepath.Join(fixtureDir, "expected.jsonl"))

	result := aisdk.StreamText(context.Background(), uiFixtureModel{parts: parts},
		aisdk.WithModelMessages(provider.UserText("test")),
	)
	var actual []map[string]any
	for chunk := range result.ToUIMessageStream(
		aisdk.WithUIMessageStreamGenerateID(func() string { return "message-1" }),
	) {
		data, err := json.Marshal(chunk)
		require.NoError(t, err)
		var decoded map[string]any
		require.NoError(t, json.Unmarshal(data, &decoded))
		actual = append(actual, decoded)
	}

	require.Equal(t, expected, actual)
}

func TestUIConformance_InvalidProviderToolInput(t *testing.T) {
	fixtureDir := filepath.Join("ui", "invalid-provider-tool-input")
	parts := loadUIFixtureParts(t, filepath.Join(fixtureDir, "input.jsonl"))
	expected := loadUIExpected(t, filepath.Join(fixtureDir, "expected.jsonl"))
	inputSchema, err := schema.SchemaFromJSON(json.RawMessage(`{"type":"object","properties":{"query":{"type":"string"}},"required":["query"]}`))
	require.NoError(t, err)

	result := aisdk.StreamText(context.Background(), uiFixtureModel{parts: parts},
		aisdk.WithModelMessages(provider.UserText("test")),
		aisdk.WithTools(aisdk.ToolSet{
			"web_search": {InputSchema: inputSchema},
		}),
	)
	var actual []map[string]any
	for chunk := range result.ToUIMessageStream(
		aisdk.WithUIMessageStreamGenerateID(func() string { return "message-1" }),
	) {
		data, err := json.Marshal(chunk)
		require.NoError(t, err)
		var decoded map[string]any
		require.NoError(t, json.Unmarshal(data, &decoded))
		actual = append(actual, decoded)
	}

	require.Equal(t, expected, actual)
}

func TestUIConformance_TextMetadataOnlyDelta(t *testing.T) {
	fixtureDir := filepath.Join("ui", "text-metadata-only-delta")
	parts := loadUIFixtureParts(t, filepath.Join(fixtureDir, "input.jsonl"))
	expected := loadUIExpected(t, filepath.Join(fixtureDir, "expected.jsonl"))

	result := aisdk.StreamText(context.Background(), uiFixtureModel{parts: parts},
		aisdk.WithModelMessages(provider.UserText("test")),
		aisdk.WithOutput(output.JSON()),
	)
	var actual []map[string]any
	for chunk := range result.ToUIMessageStream(
		aisdk.WithUIMessageStreamGenerateID(func() string { return "message-1" }),
	) {
		data, err := json.Marshal(chunk)
		require.NoError(t, err)
		var decoded map[string]any
		require.NoError(t, json.Unmarshal(data, &decoded))
		actual = append(actual, decoded)
	}

	require.Equal(t, expected, actual)
}

func TestUIConformance_ErrorThenFinish(t *testing.T) {
	fixtureDir := filepath.Join("ui", "error-then-finish")
	parts := loadUIFixtureParts(t, filepath.Join(fixtureDir, "input.jsonl"))
	expected := loadUIExpected(t, filepath.Join(fixtureDir, "expected.jsonl"))

	result := aisdk.StreamText(t.Context(), uiFixtureModel{parts: parts},
		aisdk.WithModelMessages(provider.UserText("test")),
	)
	var actual []map[string]any
	for chunk := range result.ToUIMessageStream(
		aisdk.WithUIMessageStreamGenerateID(func() string { return "message-1" }),
		aisdk.OnUIMessageStreamError(func(error) string { return "provider error" }),
	) {
		data, err := json.Marshal(chunk)
		require.NoError(t, err)
		var decoded map[string]any
		require.NoError(t, json.Unmarshal(data, &decoded))
		actual = append(actual, decoded)
	}

	require.Equal(t, expected, actual)
}

func TestUIConformance_EffectiveToolChoice(t *testing.T) {
	for _, tc := range []struct {
		name   string
		choice provider.ToolChoice
	}{
		{"required-no-call", provider.ToolChoice{Type: provider.ToolChoiceRequired}},
		{"named-miss", provider.ToolChoice{Type: provider.ToolChoiceTool, ToolName: "tool1"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixtureDir := filepath.Join("ui", "effective-tool-choice", tc.name)
			parts := loadUIFixtureParts(t, filepath.Join(fixtureDir, "input.jsonl"))
			expected := loadUIExpected(t, filepath.Join(fixtureDir, "expected.jsonl"))
			inputSchema, err := schema.SchemaFromJSON(json.RawMessage(`{"type":"object","properties":{"value":{"type":"string"}},"required":["value"]}`))
			require.NoError(t, err)
			executions := 0
			result := aisdk.StreamText(t.Context(), uiFixtureModel{parts: parts},
				aisdk.WithModelMessages(provider.UserText("test")),
				aisdk.WithToolChoice(tc.choice),
				aisdk.WithTools(aisdk.ToolSet{
					"tool1": {InputSchema: inputSchema},
					"tool2": {InputSchema: inputSchema, Execute: func(context.Context, json.RawMessage, aisdk.ToolExecutionOptions) (json.RawMessage, error) {
						executions++
						return nil, errors.New("unexpected tool execution")
					}},
				}),
			)
			var actual []map[string]any
			for chunk := range result.ToUIMessageStream(
				aisdk.WithUIMessageStreamGenerateID(func() string { return "message-1" }),
				aisdk.OnUIMessageStreamError(func(error) string { return "tool choice violated" }),
			) {
				data, err := json.Marshal(chunk)
				require.NoError(t, err)
				var decoded map[string]any
				require.NoError(t, json.Unmarshal(data, &decoded))
				actual = append(actual, decoded)
			}
			require.Equal(t, expected, actual)
			require.Error(t, result.Err())
			require.Zero(t, executions)
		})
	}
}

func loadUIExpected(t *testing.T, path string) []map[string]any {
	t.Helper()
	file, err := os.Open(path)
	require.NoError(t, err)
	defer func() { require.NoError(t, file.Close()) }()

	var chunks []map[string]any
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var chunk map[string]any
		require.NoError(t, json.Unmarshal(scanner.Bytes(), &chunk))
		chunks = append(chunks, chunk)
	}
	require.NoError(t, scanner.Err())
	return chunks
}

func loadUIFixtureParts(t *testing.T, path string) []provider.StreamPart {
	t.Helper()
	file, err := os.Open(path)
	require.NoError(t, err)
	defer func() { require.NoError(t, file.Close()) }()

	var parts []provider.StreamPart
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var part provider.StreamPart
		require.NoError(t, json.Unmarshal(scanner.Bytes(), &part))
		parts = append(parts, part)
	}
	require.NoError(t, scanner.Err())
	return parts
}
