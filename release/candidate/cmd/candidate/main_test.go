package main

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/ai-sdk/release/candidate"
)

func TestRun_Usage(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{name: "no command", wantErr: "usage: candidate"},
		{name: "unknown command", args: []string{"publish"}, wantErr: `unknown command "publish"`},
		{name: "generate without flags", args: []string{"generate"}, wantErr: "-dir and -base are required"},
		{name: "check without flags", args: []string{"check", "-repo", "."}, wantErr: "-repo, -base and -candidate are required"},
		{name: "proxy without modules", args: []string{"proxy", "-repo", ".", "-rev", "HEAD", "-out", "x"}, wantErr: "at least one <module-dir>@<version>"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var stdout bytes.Buffer
			err := run(tc.args, &stdout)
			assert.ErrorContains(t, err, tc.wantErr)
			assert.Empty(t, stdout.String())
		})
	}
}

func TestRun_Proxy(t *testing.T) {
	out := t.TempDir()
	var stdout bytes.Buffer
	require.NoError(t, run([]string{"proxy", "-repo", "../../../..", "-rev", "HEAD", "-out", out, "middleware/logger@v0.99.0"}, &stdout))
	var result candidate.Result
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &result))
	assert.Contains(t, result.Checksums, candidate.CorePath+"/middleware/logger@v0.99.0")
	assert.FileExists(t, filepath.Join(out, "github.com", "grafana", "ai-sdk", "middleware", "logger", "@v", "v0.99.0.zip"))

	err := run([]string{"proxy", "-repo", "../../../..", "-rev", "HEAD", "-out", out, "providers/unknown@v0.1.0"}, &stdout)
	assert.ErrorContains(t, err, "is not <registered-module-dir>@<version>")
}
