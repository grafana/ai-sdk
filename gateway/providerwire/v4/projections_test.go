package providerwirev4

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const interopContractDir = "../../../test/interop/providerwire-v4"

func TestResponseProjections_ValidateContractPayloads(t *testing.T) {
	registry := loadContractRegistry(t)

	unary := readProjection(t, "unary.json")
	require.NoError(t, registry.validate("generate-result", unary))

	body := string(readProjection(t, "stream-clean.sse"))
	require.True(t, strings.HasSuffix(body, "\n\n"))
	frames := strings.Split(strings.TrimSuffix(body, "\n\n"), "\n\n")
	for _, frame := range frames {
		require.NotContains(t, frame, "\nevent:")
		require.True(t, strings.HasPrefix(frame, "data: "), "invalid frame %q", frame)
		require.NotContains(t, frame, "\n")
		require.NoError(t, registry.validate("stream-part", json.RawMessage(strings.TrimPrefix(frame, "data: "))))
	}

	positive := readPositiveCorpus(t)
	for _, testCase := range []struct {
		name      string
		status    int
		retryable bool
	}{
		{name: "error invalid request retry override", status: 400, retryable: true},
		{name: "error internal nonretry override", status: 500, retryable: false},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			fixture := findCorpusCase(t, positive, testCase.name)
			assert.Equal(t, testCase.status, fixture.Status)
			require.NoError(t, registry.validateErrorEnvelope(fixture.Document, fixture.Status))
			var payload struct {
				Error struct {
					IsRetryable bool `json:"isRetryable"`
				} `json:"error"`
			}
			require.NoError(t, json.Unmarshal(fixture.Document, &payload))
			assert.Equal(t, testCase.retryable, payload.Error.IsRetryable)
		})
	}
}

func TestConformanceTransportInputs_ValidateStreamSchema(t *testing.T) {
	registry := loadContractRegistry(t)
	for _, fixture := range []string{
		"generated-files/data-and-url",
		"invalid-provider-tool-input",
		"text-metadata-only-delta",
	} {
		t.Run(fixture, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join("../../../test/conformance/ui", fixture, "input.jsonl"))
			require.NoError(t, err)
			require.True(t, strings.HasSuffix(string(raw), "\n"))
			lines := strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n")
			require.NotEmpty(t, lines)
			for index, line := range lines {
				require.NotEmpty(t, line)
				require.NoError(t, registry.validate("stream-part", json.RawMessage(line)), "line %d", index+1)
			}
		})
	}
}

func TestContractEvidence_PrivacyAndIndex(t *testing.T) {
	captureRaw, err := os.ReadFile(filepath.Join(interopContractDir, "captures", "requests.json"))
	require.NoError(t, err)
	_, err = validateStrictJSON(captureRaw)
	require.NoError(t, err)

	var evidenceRelative []string
	for _, directory := range []string{"captures", "generated", "projections"} {
		err := filepath.Walk(filepath.Join(interopContractDir, directory), func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}
			relative, err := filepath.Rel(interopContractDir, path)
			if err != nil {
				return err
			}
			evidenceRelative = append(evidenceRelative, filepath.ToSlash(relative))
			return nil
		})
		require.NoError(t, err)
	}
	for _, relative := range evidenceRelative {
		raw, err := os.ReadFile(filepath.Join(interopContractDir, filepath.FromSlash(relative)))
		require.NoError(t, err)
		text := string(raw)
		assert.NotRegexp(t, regexp.MustCompile(`/(?:home|Users)/`), text)
		assert.NotRegexp(t, regexp.MustCompile(`Bearer [A-Za-z0-9]`), text)
		assert.NotContains(t, text, "capture-not-a-real-key")
		assert.NotContains(t, text, "synthetic-capture-project")
		assert.NotContains(t, text, "ai-sdk/gateway/")
	}

	indexPath := filepath.Join(interopContractDir, "INDEX.yaml")
	indexRaw, err := os.ReadFile(indexPath)
	require.NoError(t, err)
	indexText := string(indexRaw)
	assert.Contains(t, indexText, "artifactKind: regenerated")
	assert.Contains(t, indexText, "artifactKind: curated-seed")
	assert.Contains(t, indexText, "artifactKind: derived-in-memory")
	assert.Contains(t, indexText, "artifactKind: curated-mutation-recipes")
	assert.Contains(t, indexText, "authority: pinned-stock-client")
	assert.Contains(t, indexText, "authority: local-serialized-projection")
	assert.Contains(t, indexText, "authority: local-contract-policy")
	assert.Contains(t, indexText, "authority: provider-independent-curated-input")
	assert.Contains(t, indexText, "authority: pinned-typescript-ui-expectation")
	assert.Contains(t, indexText, "selectedBedrockUnaryTransport:")
	assert.Contains(t, indexText, "independentUnaryPolicyProjection:")
	assert.Contains(t, indexText, "authority: independent-typescript-h2-projector")
	assert.Contains(t, indexText, "policyProfile: providerwire-v4-h2-unary-v1")
	assert.Contains(t, indexText, "realGoUnaryRuntime:")
	assert.Contains(t, indexText, "pinnedUnaryRuntimeInterop:")
	assert.Contains(t, indexText, "updateCommand: mise run update-providerwire-v4-artifacts")
	assert.Contains(t, indexText, "verificationCommand: mise run check-providerwire-v4")
	assert.Contains(t, indexText, "Vercel private server acceptance")
	assert.Contains(t, indexText, "ProviderWire V4 streaming service")
	assert.Contains(t, indexText, "reusable Go ProviderWire V4 client")
	assert.Contains(t, indexText, "Grafana ProviderWire V4 adoption")
	assert.Contains(t, indexText, "frontend runtime behavior")
	assert.Contains(t, indexText, "canonical JSON member ordering")
	assert.Contains(t, indexText, "providers without provenance-valid unary inputs")
	pathPattern := regexp.MustCompile(`(?m)^\s*-?\s*path:\s*(\S+)\s*$`)
	var indexedEvidence []string
	seenIndexedEvidence := make(map[string]struct{})
	for _, match := range pathPattern.FindAllStringSubmatch(indexText, -1) {
		relative := filepath.ToSlash(filepath.Clean(match[1]))
		_, err := os.Stat(filepath.Clean(filepath.Join(filepath.Dir(indexPath), match[1])))
		require.NoError(t, err, "missing indexed evidence %s", match[1])
		if strings.HasPrefix(relative, "captures/") || strings.HasPrefix(relative, "generated/") || strings.HasPrefix(relative, "projections/") {
			if _, seen := seenIndexedEvidence[relative]; !seen {
				indexedEvidence = append(indexedEvidence, relative)
				seenIndexedEvidence[relative] = struct{}{}
			}
		}
	}
	assert.ElementsMatch(t, evidenceRelative, indexedEvidence)
}

func readProjection(t *testing.T, name string) json.RawMessage {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(interopContractDir, "projections", name))
	require.NoError(t, err)
	return raw
}
