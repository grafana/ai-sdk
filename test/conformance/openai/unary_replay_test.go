//go:build conformance

package openai_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUnaryReplayRequiresEvidence(t *testing.T) {
	const fixture = "unary-refusal"
	const result = "expected-generate.json"
	const request = "expected-requests.jsonl"
	binary, err := os.Executable()
	require.NoError(t, err)

	for _, tc := range []struct {
		name   string
		file   string
		remove string
		want   string
	}{
		{name: "missing-input", file: "input.response.json", want: "unary replay input is required"},
		{name: "missing-result", file: result, want: "loading expected generate result"},
		{name: "missing-request", file: request, want: "loading expected request inputs"},
		{name: "changed-result", file: result, remove: "providerMetadata", want: "generate result mismatch"},
		{name: "changed-request", file: request, remove: "body.text", want: "request 0 body mismatch"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "openai")
			source := filepath.Join("upstream", fixture)
			require.NoError(t, os.CopyFS(filepath.Join(root, source), os.DirFS(source)))
			path := filepath.Join(root, source, tc.file)
			if tc.remove == "" {
				require.NoError(t, os.Remove(path))
			} else {
				data, err := os.ReadFile(path)
				require.NoError(t, err)
				var object map[string]json.RawMessage
				require.NoError(t, json.Unmarshal(data, &object))
				if tc.remove == "body.text" {
					var body map[string]json.RawMessage
					require.NoError(t, json.Unmarshal(object["body"], &body))
					require.Contains(t, body, "text")
					delete(body, "text")
					object["body"], err = json.Marshal(body)
					require.NoError(t, err)
				} else {
					require.Contains(t, object, tc.remove)
					delete(object, tc.remove)
				}
				data, err = json.Marshal(object)
				require.NoError(t, err)
				require.NoError(t, os.WriteFile(path, append(data, '\n'), 0o600))
			}
			cmd := exec.CommandContext(t.Context(), binary, "-test.v", "-test.run=^TestConformance$/^upstream$/^"+fixture+"$")
			cmd.Dir = root
			output, err := cmd.CombinedOutput()
			var exit *exec.ExitError
			require.ErrorAs(t, err, &exit, "%s", output)
			require.Equal(t, 1, exit.ExitCode(), "%s", output)
			require.Contains(t, string(output), "--- FAIL: TestConformance/upstream/"+fixture)
			require.Contains(t, string(output), tc.want)
		})
	}
}
