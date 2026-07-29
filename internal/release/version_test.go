package release

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseVersion_StrictSemVer(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		value   string
		want    string
		wantErr bool
	}{
		{name: "stable", value: "v1.2.3", want: "v1.2.3"},
		{name: "prerelease", value: "v0.2.0-alpha.12", want: "v0.2.0-alpha.12"},
		{name: "missing v", value: "1.2.3", wantErr: true},
		{name: "leading zero", value: "v01.2.3", wantErr: true},
		{name: "missing sequence", value: "v1.2.3-alpha", wantErr: true},
		{name: "build metadata", value: "v1.2.3+build", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := ParseVersion(tt.value)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got.String())
		})
	}
}

func TestNextVersion_PrereleaseLifecycle(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		current    string
		bump       Bump
		prerelease string
		stable     bool
		want       string
		wantErr    bool
	}{
		{name: "stable patch", current: "v1.2.3", bump: BumpPatch, want: "v1.2.4"},
		{name: "start alpha", current: "v1.2.3", bump: BumpMinor, prerelease: "alpha", want: "v1.3.0-alpha.1"},
		{name: "continue alpha", current: "v0.2.0-alpha.1", bump: BumpPatch, want: "v0.2.0-alpha.2"},
		{name: "larger alpha base", current: "v0.2.0-alpha.4", bump: BumpMinor, want: "v0.3.0-alpha.1"},
		{name: "change channel", current: "v0.2.0-alpha.4", bump: BumpPatch, prerelease: "beta", want: "v0.2.0-beta.1"},
		{name: "promote", current: "v0.2.0-alpha.2", bump: BumpPatch, stable: true, want: "v0.2.0"},
		{name: "already stable", current: "v0.2.0", bump: BumpPatch, stable: true, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			current, err := ParseVersion(tt.current)
			require.NoError(t, err)
			got, err := NextVersion(current, tt.bump, tt.prerelease, tt.stable)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got.String())
		})
	}
}
