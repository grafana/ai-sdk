package main

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
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
