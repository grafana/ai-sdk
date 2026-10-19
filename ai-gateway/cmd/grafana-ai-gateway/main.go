package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/prometheus/client_golang/prometheus"

	backendlog "github.com/grafana/grafana-plugin-sdk-go/backend/log"

	"github.com/grafana/grafana-app-sdk/plugin"

	"github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/gateway"
	"github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/pluginlog"
)

func main() {
	os.Exit(run())
}

func run() int {
	logger := slog.New(pluginlog.NewHandler(backendlog.DefaultLogger, slog.LevelInfo))
	gw, err := gateway.New(gateway.Options{LookupEnv: os.LookupEnv, Logger: logger, Registerer: prometheus.DefaultRegisterer})
	if err != nil {
		logger.Error("gateway plugin failed to start", "class", "startup_failure", "reason", err.Error())
		return 1
	}
	logger.Info("gateway plugin lifecycle", "event", "plugin_started")
	err = plugin.Run(gw.Provider(), plugin.WithPluginID(gateway.PluginID), plugin.WithAuthenticator(gw.Authenticator()))
	ctx, cancel := context.WithTimeout(context.Background(), gateway.FinalFlushBudget)
	gw.Close(ctx)
	cancel()
	logger.Info("gateway plugin lifecycle", "event", "plugin_stopped")
	if err != nil {
		logger.Error("gateway plugin failed", "class", "plugin_failure", "reason", err.Error())
		return 1
	}
	return 0
}
