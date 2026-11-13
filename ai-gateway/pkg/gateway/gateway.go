package gateway

import (
	"log/slog"
	"os"

	"github.com/prometheus/client_golang/prometheus"

	backendlog "github.com/grafana/grafana-plugin-sdk-go/backend/log"

	"github.com/grafana/grafana-app-sdk/plugin"

	composition "github.com/grafana/ai-sdk/ai-gateway/pkg/internal/gateway"
	"github.com/grafana/ai-sdk/ai-gateway/pkg/internal/pluginlog"
)

// Options configures a Gateway distribution. The zero value runs the shipped
// Gateway.
type Options struct{}

// Run constructs the Gateway from the plugin host environment and serves it
// through the plugin protocol until the host stops the plugin, then flushes
// telemetry. It logs startup and plugin failures itself, with their reasons,
// and returns them; the caller only needs to exit non-zero.
func Run(Options) error {
	logger := slog.New(pluginlog.NewHandler(backendlog.DefaultLogger, slog.LevelInfo))
	gw, err := composition.New(composition.Options{LookupEnv: os.LookupEnv, Logger: logger, Registerer: prometheus.DefaultRegisterer})
	if err != nil {
		logger.Error("gateway plugin failed to start", "class", "startup_failure", "reason", err.Error())
		return err
	}
	if gw.Idle() {
		logger.Info("gateway plugin lifecycle", "event", "plugin_idle", "reason", "no platform signing keys configured: serving no requests")
	} else {
		logger.Info("gateway plugin lifecycle", "event", "plugin_started")
	}
	err = plugin.Run(gw.Provider(), plugin.WithPluginID(composition.PluginID), plugin.WithAuthenticator(gw.Authenticator()))
	gw.Close()
	logger.Info("gateway plugin lifecycle", "event", "plugin_stopped")
	if err != nil {
		logger.Error("gateway plugin failed", "class", "plugin_failure", "reason", err.Error())
		return err
	}
	return nil
}
