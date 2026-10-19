//go:build mage
// +build mage

package main

import (
	// mage:import
	build "github.com/grafana/grafana-plugin-sdk-go/build"
)

func init() {
	_ = build.SetBeforeBuildCallback(func(cfg build.Config) (build.Config, error) {
		cfg.RootPackagePath = "./cmd/grafana-ai-gateway"
		return cfg, nil
	})
}

// Default configures the default target.
var Default = build.BuildAll
