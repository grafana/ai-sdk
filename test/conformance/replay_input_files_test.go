//go:build conformance

package conformance

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUnaryReplayInputFiles(t *testing.T) {
	for _, tc := range []struct {
		name  string
		files []string
		want  string
	}{
		{name: "unary", files: []string{"input.response.json", "expected-generate.json"}},
		{
			name:  "stream-instead-of-unary",
			files: []string{"input.chunks.txt"},
			want:  "unary replay input is required: missing input.response.json (found [input.chunks.txt])",
		},
		{
			name:  "stream-beside-unary",
			files: []string{"input.response.json", "input-1.chunks.txt"},
			want:  "unary replay requires only input.response.json; found [input-1.chunks.txt input.response.json]",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			for _, name := range tc.files {
				require.NoError(t, os.WriteFile(filepath.Join(dir, name), nil, 0o600))
			}
			files, err := replayInputFiles(dir, OperationGenerate)
			if tc.want != "" {
				require.ErrorContains(t, err, tc.want)
				return
			}
			require.NoError(t, err)
			require.Equal(t, []string{"input.response.json"}, files)
		})
	}
}
