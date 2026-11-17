package gateway

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const (
	gatewayModule      = "github.com/grafana/ai-sdk/ai-gateway"
	distributionModule = "example.com/distribution"
	distributionMain   = `package main

import (
	"context"
	"encoding/json"
	"os"

	"github.com/grafana/ai-sdk/ai-gateway/pkg/extension"
	"github.com/grafana/ai-sdk/ai-gateway/pkg/gateway"
)

type audit struct{}

func (audit) Observe(context.Context, *extension.Call, extension.Record) error { return nil }

func main() {
	definition := extension.Definition{
		Name:          "audit",
		Scope:         extension.ScopeOperator,
		FailurePolicy: extension.FailSkip,
		New: func(json.RawMessage, extension.Dependencies) (any, error) {
			return audit{}, nil
		},
	}
	if gateway.Run(gateway.Options{Extensions: append(gateway.DefaultExtensions(), definition)}) != nil {
		os.Exit(1)
	}
}
`
)

type moduleFile struct {
	Go      string
	Require []struct {
		Path    string
		Version string
	}
	Replace []struct {
		Old struct{ Path string }
		New struct {
			Path    string
			Version string
		}
	}
}

func TestRun_CompilesFromAnExternalModule(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a distribution executable")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain unavailable")
	}
	gatewayDir, err := filepath.Abs(filepath.Join("..", ".."))
	require.NoError(t, err)
	var stdout, stderr bytes.Buffer
	edit := exec.Command("go", "mod", "edit", "-json", filepath.Join(gatewayDir, "go.mod"))
	edit.Stdout, edit.Stderr = &stdout, &stderr
	require.NoError(t, edit.Run(), stderr.String())
	var gatewayMod moduleFile
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &gatewayMod))

	var mod strings.Builder
	fmt.Fprintf(&mod, "module %s\n\ngo %s\n\nrequire (\n\t%s v0.0.0-00010101000000-000000000000\n", distributionModule, gatewayMod.Go, gatewayModule)
	for _, requirement := range gatewayMod.Require {
		fmt.Fprintf(&mod, "\t%s %s\n", requirement.Path, requirement.Version)
	}
	fmt.Fprintf(&mod, ")\n\nreplace %s => %s\n", gatewayModule, gatewayDir)
	for _, replacement := range gatewayMod.Replace {
		target := replacement.New.Path
		if replacement.New.Version == "" && !filepath.IsAbs(target) {
			target = filepath.Join(gatewayDir, target)
		}
		fmt.Fprintf(&mod, "replace %s => %s %s\n", replacement.Old.Path, target, replacement.New.Version)
	}
	sums, err := os.ReadFile(filepath.Join(gatewayDir, "go.sum"))
	require.NoError(t, err)

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte(mod.String()), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.sum"), sums, 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.go"), []byte(distributionMain), 0o600))
	build := exec.Command("go", "build", "-o", filepath.Join(dir, "distribution"), ".")
	build.Dir = dir
	build.Env = append(os.Environ(), "GOWORK=off", "GOPROXY=off", "GOFLAGS=-mod=readonly", "GOTOOLCHAIN=local")
	output, err := build.CombinedOutput()
	require.NoError(t, err, string(output))
}

func TestDefaultExtensions_ShippedBuildRegistersNone(t *testing.T) {
	require.Empty(t, DefaultExtensions())
}
