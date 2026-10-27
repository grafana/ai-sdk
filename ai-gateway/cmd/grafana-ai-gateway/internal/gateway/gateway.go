// Package gateway composes the Gateway plugin: settings, catalog, BYOK,
// telemetry, caller access and the ProviderWire route handlers.
package gateway

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/grafana/authlib/authn"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/grafana/grafana-app-sdk/app"
	"github.com/grafana/grafana-app-sdk/plugin"
	"github.com/grafana/grafana-app-sdk/simple"

	gatewayauth "github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/auth"
	"github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/config"
	"github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/discovery"
	"github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/manifestdata"
	"github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/outbound"
	"github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/service"
	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
	providerv4 "github.com/grafana/ai-sdk/ai-gateway/providerwire/v4"
)

// PluginID is the plugin ID declared in src/plugin.json. The host addresses
// delegated tokens to it, so every build must identify as it.
const PluginID = "grafana-aigateway-app"

// finalFlushBudget bounds the telemetry flush after the plugin host stops the plugin.
const finalFlushBudget = time.Second

// Options supplies the plugin host environment and process-wide sinks.
type Options struct {
	LookupEnv  config.LookupEnv
	Logger     *slog.Logger
	Registerer prometheus.Registerer
}

// Gateway is the constructed plugin.
type Gateway struct {
	handler       http.Handler
	authenticator authn.Authenticator
	sinks         sinks
}

// New validates settings and constructs every Gateway dependency before any
// request can be served. Configuration is validated before any secret is
// read, and failures are secret-safe.
func New(options Options) (*Gateway, error) {
	lookupEnv := options.LookupEnv
	settings, err := config.ParseSettings(lookupEnv)
	if err != nil {
		return nil, err
	}
	authenticator, err := newAuthenticator(lookupEnv)
	if err != nil {
		return nil, err
	}
	// The Agent Observability SDK reads the process environment itself.
	if err := settings.AgentObservability.ValidateAmbientEnvironment(os.LookupEnv); err != nil {
		return nil, err
	}
	file, err := config.LoadFile(settings.ConfigFile, config.MaxFileBytes)
	if err != nil {
		return nil, err
	}
	if err := validateProviderEndpoints(file); err != nil {
		return nil, err
	}
	providers, err := file.ResolveProviderSecrets(lookupEnv)
	if err != nil {
		return nil, err
	}
	agentSecret, err := settings.AgentObservability.ResolveAuthSecret(lookupEnv)
	if err != nil {
		return nil, err
	}
	sinks, err := newSinks(settings, options, agentSecret)
	if err != nil {
		return nil, err
	}
	handler, err := newRouteHandler(settings, file, providers, configuredSecrets(providers, agentSecret), sinks, options.Logger)
	if err != nil {
		sinks.close()
		return nil, err
	}
	return &Gateway{handler: handler, authenticator: authenticator, sinks: sinks}, nil
}

func validateProviderEndpoints(file config.File) error {
	for name, provider := range file.Providers {
		if provider.BaseURL == "" {
			continue
		}
		parsed, err := outbound.ValidateEndpoint(provider.BaseURL)
		if err != nil {
			return fmt.Errorf("gateway: validating provider %q endpoint: %w", name, err)
		}
		provider.BaseURL = parsed.String()
		file.Providers[name] = provider
	}
	return nil
}

// configuredSecrets lists the provider keys and exporter credential that
// Gateway-produced error summaries must not echo.
func configuredSecrets(providers map[string]config.ResolvedProvider, agentSecret string) []string {
	secrets := make([]string, 0, len(providers)+1)
	for _, provider := range providers {
		secrets = append(secrets, provider.APIKey)
	}
	if agentSecret != "" {
		secrets = append(secrets, agentSecret)
	}
	return secrets
}

// sinks are the telemetry destinations that must be flushed on shutdown.
type sinks struct {
	telemetry *service.Telemetry
	agent     *service.AgentObservabilityRuntime
	attempts  *service.PhysicalAttemptSink
}

func newSinks(settings config.Settings, options Options, agentSecret string) (sinks, error) {
	telemetry, err := service.NewTelemetry(options.Logger, service.TelemetryOptions{Registerer: options.Registerer})
	if err != nil {
		return sinks{}, err
	}
	agent, err := service.NewAgentObservabilityRuntime(settings.AgentObservability, agentSecret, telemetry)
	if err != nil {
		return sinks{}, err
	}
	return sinks{telemetry: telemetry, agent: agent, attempts: service.NewPhysicalAttemptSink(options.Logger, telemetry)}, nil
}

// close flushes the sinks concurrently for at most the final flush budget.
func (s sinks) close() {
	ctx, cancel := context.WithTimeout(context.Background(), finalFlushBudget)
	defer cancel()
	var closing sync.WaitGroup
	closing.Go(func() { s.attempts.Close(ctx) })
	closing.Go(func() { s.agent.Close(ctx) })
	closing.Wait()
}

func newRouteHandler(settings config.Settings, file config.File, providers map[string]config.ResolvedProvider, secrets []string, sinks sinks, logger *slog.Logger) (http.Handler, error) {
	nativeClient, err := outbound.NewModelClient(config.ProviderResponseHeaderTimeout, config.ProviderResponseBytes)
	if err != nil {
		return nil, err
	}
	modelFactory, byokFactory, err := service.NewModelObservabilityFactories(sinks.telemetry, logger, sinks.agent, execution.DefaultLimits.StreamDrainDuration)
	if err != nil {
		return nil, err
	}
	catalog, err := service.BuildCatalog(file, providers, nativeClient, modelFactory, sinks.attempts)
	if err != nil {
		return nil, err
	}
	errorWriter := providerv4.NewHostErrorWriter()
	configuredLanguage, err := newLanguageHandler(execution.Config{Selector: service.NewConfiguredSelector(catalog), Limits: execution.DefaultLimits, ConfiguredAPIKeys: secrets})
	if err != nil {
		return nil, err
	}
	routes := routeHandlers{
		configuredDiscovery: service.ConfiguredDiscovery(discovery.New(catalog, errorWriter), errorWriter),
		configuredLanguage:  configuredLanguage,
	}
	if settings.Access.Mode == gatewayauth.AccessModeCloud {
		byokLanguage, err := newLanguageHandler(execution.Config{Selector: service.NewBYOKSelector(nativeClient, byokFactory), Limits: execution.DefaultLimits, ConfiguredAPIKeys: secrets})
		if err != nil {
			return nil, err
		}
		routes.byokDiscovery = service.BYOKDiscovery(errorWriter)
		routes.byokLanguage = byokLanguage
	}
	return newHandler(settings.Access, sinks.telemetry, errorWriter, routes), nil
}

// newLanguageHandler serves ProviderWire V4 over one execution runtime.
func newLanguageHandler(config execution.Config) (http.Handler, error) {
	runtime, err := execution.New(config)
	if err != nil {
		return nil, err
	}
	return providerv4.New(runtime)
}

func newAuthenticator(lookupEnv config.LookupEnv) (authn.Authenticator, error) {
	keys, err := gatewayauth.SigningKeys(lookupEnv)
	if errors.Is(err, gatewayauth.ErrNoSigningKeys) {
		return nil, fmt.Errorf("config: set %s or %s to verify platform tokens", plugin.EnvVarGrafanaJWKSURL, plugin.EnvVarGrafanaJWKS)
	}
	if err != nil {
		return nil, err
	}
	return gatewayauth.NewPlatformAuthenticator(gatewayauth.PlatformConfig{PluginID: PluginID, Group: manifestdata.LocalManifest().ManifestData.Group, Keys: keys})
}

// Handler returns the ProviderWire route handler for the plugin route adapter.
func (g *Gateway) Handler() http.Handler { return g.handler }

// Authenticator returns the plugin authenticator for delegated access tokens.
func (g *Gateway) Authenticator() authn.Authenticator { return g.authenticator }

// Provider returns the app-sdk provider whose app serves Handler.
func (g *Gateway) Provider() app.Provider {
	return simple.NewAppProvider(manifestdata.LocalManifest(), nil, func(cfg app.Config) (app.App, error) {
		return simple.NewApp(simple.AppConfig{Name: cfg.ManifestData.AppName, KubeConfig: cfg.KubeConfig, Handler: g.handler})
	})
}

// Close flushes Agent Observability and attempt records concurrently for at
// most the final flush budget, then releases them. Later records are dropped
// and counted.
func (g *Gateway) Close() { g.sinks.close() }
