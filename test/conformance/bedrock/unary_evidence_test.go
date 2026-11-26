//go:build conformance

package bedrock_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUnaryResultEvidence(t *testing.T) {
	binary, err := os.Executable()
	require.NoError(t, err)

	const fixture = "guard-content-intervened"
	const golden = "expected-generate.json"
	for _, tc := range []struct {
		name   string
		mutate func(*testing.T, []byte) []byte
		want   string
	}{
		{
			name: "missing",
			want: "loading expected generate result",
		},
		{
			name:   "missing-modern-namespace",
			mutate: omitModernBedrockNamespace,
			want:   "generate result mismatch",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "bedrock")
			source := filepath.Join("upstream", fixture)
			destination := filepath.Join(root, source)
			require.NoError(t, os.MkdirAll(destination, 0o700))
			for _, filename := range []string{"config.yaml", "input.response.json", "expected-requests.jsonl"} {
				data, err := os.ReadFile(filepath.Join(source, filename))
				require.NoError(t, err)
				require.NoError(t, os.WriteFile(filepath.Join(destination, filename), data, 0o600))
			}
			if tc.mutate != nil {
				data, err := os.ReadFile(filepath.Join(source, golden))
				require.NoError(t, err)
				require.NoError(t, os.WriteFile(filepath.Join(destination, golden), tc.mutate(t, data), 0o600))
			}

			cmd := exec.CommandContext(t.Context(), binary, "-test.v", "-test.run=^TestConformance$/^upstream$/^"+fixture+"$")
			cmd.Dir = root
			output, err := cmd.CombinedOutput()
			var exit *exec.ExitError
			require.ErrorAs(t, err, &exit, "%s", output)
			require.Equal(t, 1, exit.ExitCode(), "%s", output)
			require.Contains(t, string(output), tc.want)
			require.Contains(t, string(output), filepath.Join(source, golden))
		})
	}
}

func omitModernBedrockNamespace(t *testing.T, data []byte) []byte {
	t.Helper()
	var result map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(data, &result))
	var metadata map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(result["providerMetadata"], &metadata))
	require.Contains(t, metadata, "amazonBedrock")
	delete(metadata, "amazonBedrock")
	encoded, err := json.Marshal(metadata)
	require.NoError(t, err)
	result["providerMetadata"] = encoded
	encoded, err = json.Marshal(result)
	require.NoError(t, err)
	return encoded
}
