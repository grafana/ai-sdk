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

	for _, name := range []string{"stream-clean.sse", "stream-done.sse"} {
		t.Run(name, func(t *testing.T) {
			body := string(readProjection(t, name))
			require.True(t, strings.HasSuffix(body, "\n\n"))
			frames := strings.Split(strings.TrimSuffix(body, "\n\n"), "\n\n")
			for index, frame := range frames {
				require.NotContains(t, frame, "\nevent:")
				require.True(t, strings.HasPrefix(frame, "data: "), "invalid frame %q", frame)
				payload := strings.TrimPrefix(frame, "data: ")
				if payload == "[DONE]" {
					assert.Equal(t, "stream-done.sse", name)
					assert.Equal(t, len(frames)-1, index)
					continue
				}
				require.NotContains(t, frame, "\n")
				require.NoError(t, registry.validate("stream-part", json.RawMessage(payload)))
			}
		})
	}

	errorCases := []struct {
		name      string
		status    int
		retryable bool
	}{
		{name: "error-retryable-400.json", status: 400, retryable: true},
		{name: "error-nonretryable-500.json", status: 500, retryable: false},
	}
	for _, testCase := range errorCases {
		t.Run(testCase.name, func(t *testing.T) {
			raw := readProjection(t, testCase.name)
			require.NoError(t, registry.validate("error", raw))
			assert.Equal(t, testCase.status, nestedErrorStatus(t, raw))
			var payload struct {
				Error struct {
					IsRetryable bool `json:"isRetryable"`
				} `json:"error"`
			}
			require.NoError(t, json.Unmarshal(raw, &payload))
			assert.Equal(t, testCase.retryable, payload.Error.IsRetryable)
		})
	}
}

func TestContractEvidence_PrivacyAndIndex(t *testing.T) {
	captureRaw, err := os.ReadFile(filepath.Join(interopContractDir, "captures", "requests.json"))
	require.NoError(t, err)
	_, err = validateStrictJSON(captureRaw)
	require.NoError(t, err)

	var evidenceRelative []string
	for _, directory := range []string{"captures", "projections"} {
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
		assert.NotContains(t, text, "ai-sdk/gateway/4.0.52")
	}

	indexPath := filepath.Join(interopContractDir, "INDEX.yaml")
	indexRaw, err := os.ReadFile(indexPath)
	require.NoError(t, err)
	indexText := string(indexRaw)
	assert.Contains(t, indexText, "artifactKind: regenerated")
	assert.Contains(t, indexText, "artifactKind: curated")
	assert.Contains(t, indexText, "authority: pinned-stock-client")
	assert.Contains(t, indexText, "authority: local-serialized-projection")
	assert.Contains(t, indexText, "authority: local-contract-fixture")
	assert.Contains(t, indexText, "updateCommand: mise run update-providerwire-v4-captures")
	assert.Contains(t, indexText, "Vercel private server acceptance")
	assert.Contains(t, indexText, "live provider response recording")
	pathPattern := regexp.MustCompile(`(?m)^\s*-?\s*path:\s*(\S+)\s*$`)
	var indexedEvidence []string
	for _, match := range pathPattern.FindAllStringSubmatch(indexText, -1) {
		relative := filepath.ToSlash(filepath.Clean(match[1]))
		_, err := os.Stat(filepath.Clean(filepath.Join(filepath.Dir(indexPath), match[1])))
		require.NoError(t, err, "missing indexed evidence %s", match[1])
		if strings.HasPrefix(relative, "captures/") || strings.HasPrefix(relative, "projections/") {
			indexedEvidence = append(indexedEvidence, relative)
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
