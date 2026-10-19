package main

import (
	"log/slog"
	"os"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/grafana/grafana-app-sdk/plugin"

	"github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/gateway"
)

func main() {
	os.Exit(run())
}

func run() int {
	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	gw, err := gateway.New(gateway.Options{LookupEnv: os.LookupEnv, Logger: logger, Registerer: prometheus.DefaultRegisterer})
	if err != nil {
		logger.Error("gateway plugin failed to start", "class", "startup_failure")
		return 1
	}
	logger.Info("gateway plugin lifecycle", "event", "plugin_started")
	err = plugin.Run(gw.Provider(), plugin.WithPluginID(gateway.PluginID), plugin.WithAuthenticator(gw.Authenticator()))
	gw.Close()
	logger.Info("gateway plugin lifecycle", "event", "plugin_stopped")
	if err != nil {
		logger.Error("gateway plugin failed", "class", "plugin_failure")
		return 1
	}
	return 0
}
