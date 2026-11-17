package gateway

import (
	"log/slog"
	"os"

	"github.com/prometheus/client_golang/prometheus"

	backendlog "github.com/grafana/grafana-plugin-sdk-go/backend/log"

	"github.com/grafana/grafana-app-sdk/plugin"

	"github.com/grafana/ai-sdk/ai-gateway/pkg/extension"
	composition "github.com/grafana/ai-sdk/ai-gateway/pkg/internal/gateway"
	"github.com/grafana/ai-sdk/ai-gateway/pkg/internal/pluginlog"
)

// Options configures a Gateway distribution. The zero value runs the Gateway
// without extensions.
type Options struct {
	// Extensions are the distribution's extension definitions, typically
	// append(DefaultExtensions(), private...). Invalid definitions and
	// duplicate names fail startup.
	Extensions []extension.Definition
	// Mandatory names the operator-scope extensions that the operator must
	// configure and enable; startup fails otherwise.
	Mandatory []string
}

// DefaultExtensions returns the extension definitions the shipped Gateway
// registers. There are none in this version.
func DefaultExtensions() []extension.Definition { return []extension.Definition{} }

// Run constructs the Gateway from the plugin host environment and serves it
// through the plugin protocol until the host stops the plugin, then flushes
// telemetry. It logs startup and plugin failures itself, with their reasons,
// and returns them; the caller only needs to exit non-zero.
func Run(options Options) error {
	logger := slog.New(pluginlog.NewHandler(backendlog.DefaultLogger, slog.LevelInfo))
	gw, err := composition.New(composition.Options{
		LookupEnv:  os.LookupEnv,
		Logger:     logger,
		Registerer: prometheus.DefaultRegisterer,
		Extensions: options.Extensions,
		Mandatory:  options.Mandatory,
	})
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
