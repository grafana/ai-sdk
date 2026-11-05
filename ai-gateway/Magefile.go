//go:build mage
// +build mage

package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/magefile/mage/sh"

	// mage:import
	build "github.com/grafana/grafana-plugin-sdk-go/build"
)

const pluginGoFlags = "-mod=readonly -trimpath"

var frontendPackageFiles = []string{"dist/plugin.json", "dist/module.js"}

// Default configures the default target.
var Default = build.BuildAll

// BuildPlugin builds the Linux amd64 and arm64 backends into the built frontend package and adds its license inventory and source revision.
func BuildPlugin() error {
	for _, file := range frontendPackageFiles {
		if _, err := os.Stat(file); err != nil {
			return fmt.Errorf("build the frontend package before the backend: %w", err)
		}
	}
	if err := os.Setenv("GOFLAGS", strings.TrimSpace(os.Getenv("GOFLAGS")+" "+pluginGoFlags)); err != nil {
		return fmt.Errorf("setting GOFLAGS: %w", err)
	}
	backend := build.Build{}
	if err := backend.Linux(); err != nil {
		return err
	}
	if err := backend.LinuxARM64(); err != nil {
		return err
	}
	return sh.RunV("bash", "scripts/package-licenses.sh")
}
