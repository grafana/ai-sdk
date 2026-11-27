//go:build conformance

package openai_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReplayIntegrity(t *testing.T) {
	binary, err := os.Executable()
	require.NoError(t, err)

	const fixture = "multi-tool-chain"
	const requests = "expected-requests.jsonl"
	source := filepath.Join("recorded", fixture)
	for _, tc := range []struct {
		name       string
		extraInput string
		mutate     func(t *testing.T, dir string)
		want       string
	}{
		{name: "unchanged"},
		{
			name:       "unconsumed-response",
			extraInput: "input-5.chunks.txt",
			want:       "replay consumed 4 of 5 inputs; first unserved input: input-5.chunks.txt",
		},
		{
			name:       "numbering-gap",
			extraInput: "input-6.chunks.txt",
			want:       "numbered replay inputs must be contiguous: missing input-5.chunks.txt",
		},
		{
			name:       "mixed-stream-layout",
			extraInput: "input.chunks.txt",
			want:       "input.chunks.txt cannot be combined with other replay inputs",
		},
		{
			name:       "unary-input-in-stream",
			extraInput: "input.response.json",
			want:       "stream replay cannot include input.response.json",
		},
		{
			name: "missing-request-snapshot",
			mutate: func(t *testing.T, dir string) {
				require.NoError(t, os.Remove(filepath.Join(dir, requests)))
			},
			want: "loading expected request inputs " + filepath.Join(source, requests),
		},
		{
			name: "wrong-request-cardinality",
			mutate: func(t *testing.T, dir string) {
				path := filepath.Join(dir, requests)
				data, err := os.ReadFile(path)
				require.NoError(t, err)
				rows := strings.Split(strings.TrimSpace(string(data)), "\n")
				require.Len(t, rows, 4)
				require.NoError(t, os.WriteFile(path, []byte(strings.Join(rows[:3], "\n")+"\n"), 0o600))
			},
			want: "replay must have one expected request per input",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "openai")
			dir := filepath.Join(root, source)
			require.NoError(t, os.CopyFS(dir, os.DirFS(source)))
			if tc.extraInput != "" {
				data, err := os.ReadFile(filepath.Join(dir, "input-4.chunks.txt"))
				require.NoError(t, err)
				require.NoError(t, os.WriteFile(filepath.Join(dir, tc.extraInput), data, 0o600))
			}
			if tc.mutate != nil {
				tc.mutate(t, dir)
			}

			cmd := exec.CommandContext(t.Context(), binary, "-test.v", "-test.run=^TestConformance$/^recorded$/^"+fixture+"$")
			cmd.Dir = root
			output, err := cmd.CombinedOutput()
			if tc.want == "" {
				require.NoError(t, err, "%s", output)
				require.Contains(t, string(output), "--- PASS: TestConformance/recorded/"+fixture+"/provider-parts")
				return
			}
			var exit *exec.ExitError
			require.ErrorAs(t, err, &exit, "%s", output)
			require.Equal(t, 1, exit.ExitCode(), "%s", output)
			require.Contains(t, string(output), "--- FAIL: TestConformance/recorded/"+fixture)
			require.Contains(t, string(output), tc.want)
			if tc.name == "numbering-gap" {
				require.Contains(t, string(output), tc.extraInput)
			}
		})
	}
}
