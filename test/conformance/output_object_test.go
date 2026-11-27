//go:build conformance

package conformance

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadExpectedOutputObject(t *testing.T) {
	tests := []struct {
		name     string
		required bool
		contents string
		present  bool
		want     any
		wantErr  string
	}{
		{name: "strict rejects missing golden", required: true, wantErr: "reading expected-object.json"},
		{name: "strict rejects null golden", required: true, contents: "null", wantErr: "requires a non-null expected-object.json"},
		{name: "strict accepts object golden", required: true, contents: `{"name":"Ada"}`, present: true, want: map[string]any{"name": "Ada"}},
		{name: "legacy permits missing golden"},
		{name: "legacy loads explicit null", contents: "null", present: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "expected-object.json")
			if tc.contents != "" {
				require.NoError(t, os.WriteFile(path, []byte(tc.contents), 0o600))
			}
			value, present, err := loadExpectedOutputObject(path, tc.required)
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.present, present)
			assert.Equal(t, tc.want, value)
		})
	}
}
