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
	"github.com/grafana/ai-sdk/provider"
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
