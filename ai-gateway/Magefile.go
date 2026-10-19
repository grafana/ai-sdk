//go:build mage

package main

import (
	build "github.com/grafana/grafana-plugin-sdk-go/build"
	"github.com/magefile/mage/mg"
)

func init() {
	_ = build.SetBeforeBuildCallback(func(cfg build.Config) (build.Config, error) {
		cfg.RootPackagePath = "./cmd/grafana-ai-gateway"
		return cfg, nil
	})
}

// Default is the target mage runs without arguments.
var Default = Linux

// Linux builds the Linux amd64 and arm64 plugin backends.
func Linux() {
	b := build.Build{}
	mg.Deps(b.Linux, b.LinuxARM64)
}
