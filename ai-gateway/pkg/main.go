package main

import (
	"os"

	"github.com/grafana/ai-sdk/ai-gateway/pkg/gateway"
)

func main() {
	if gateway.Run(gateway.Options{Extensions: gateway.DefaultExtensions()}) != nil {
		os.Exit(1)
	}
}
