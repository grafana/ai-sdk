// Package gateway composes the Gateway plugin: settings, catalog, BYOK,
// telemetry, caller access and the ProviderWire route handlers.
package gateway

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/grafana/authlib/authn"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/grafana/grafana-app-sdk/app"
	"github.com/grafana/grafana-app-sdk/simple"

	gatewayauth "github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/auth"
	"github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/config"
	"github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/discovery"
	"github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/manifestdata"
	"github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/outbound"
	"github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/service"
	providerv4 "github.com/grafana/ai-sdk/ai-gateway/providerwire/v4"
)

// PluginID is the plugin ID declared in src/plugin.json. The host addresses
// delegated tokens to it, so every build must identify as it.
const PluginID = "grafana-aigateway-app"

// FinalFlushBudget bounds the telemetry flush after the plugin host stops the plugin.
const FinalFlushBudget = time.Second

// EnvInsecureSkipAuthentication names the plugin host setting for local
// development without platform signing keys.
const EnvInsecureSkipAuthentication = "GF_PLUGIN_INSECURE_SKIP_AUTHENTICATION"

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
	close         func(context.Context)
}

// New validates settings and constructs every Gateway dependency before any
// request can be served. Failures are secret-safe.
func New(options Options) (*Gateway, error) {
	lookupEnv := options.LookupEnv
	if lookupEnv == nil {
		lookupEnv = func(string) (string, bool) { return "", false }
	}
	settings, err := config.ParseSettings(nil, lookupEnv)
	if err != nil {
		return nil, err
	}
	authenticator, err := newAuthenticator(settings.DeploymentMode, lookupEnv)
	if err != nil {
		return nil, err
	}
	if err := settings.AgentObservability.ValidateAmbientEnvironment(lookupEnv); err != nil {
		return nil, err
	}
	file, err := config.LoadFile(settings.ConfigFile, settings.ConfigMaxBytes)
	if err != nil {
		return nil, err
	}
	for name, provider := range file.Providers {
		if provider.BaseURL == "" {
			continue
		}
		parsed, err := outbound.ValidateEndpoint(provider.BaseURL, settings.DeploymentMode)
		if err != nil {
			return nil, fmt.Errorf("gateway: validating provider %q endpoint: %w", name, err)
		}
		provider.BaseURL = parsed.String()
		file.Providers[name] = provider
	}
	resolvedProviders, err := file.ResolveProviderSecrets(lookupEnv)
	if err != nil {
		return nil, err
	}
	nativeClient, err := outbound.NewAnthropicClient(settings.AnthropicResponseHeaderTimeout, settings.AnthropicResponseBytes)
	if err != nil {
		return nil, err
	}
	telemetry, err := service.NewTelemetry(options.Logger, service.TelemetryOptions{
		Region:      settings.ObservationRegion,
		Application: settings.ObservationApplication,
		Registerer:  options.Registerer,
	})
	if err != nil {
		return nil, err
	}
	configuredAPIKeys := make([]string, 0, len(resolvedProviders)+1)
	for _, provider := range resolvedProviders {
		configuredAPIKeys = append(configuredAPIKeys, provider.APIKey)
	}
	agentRuntime, err := service.NewAgentObservabilityRuntime(settings.AgentObservability, func(name string) (string, bool) {
		value, exists := lookupEnv(name)
		if exists && name == settings.AgentObservability.AuthSecretEnv {
			configuredAPIKeys = append(configuredAPIKeys, value)
		}
		return value, exists
	}, telemetry)
	if err != nil {
		return nil, err
	}
	physicalSink := service.NewPhysicalAttemptSink(options.Logger, telemetry)
	closeSinks := func(ctx context.Context) {
		var closing sync.WaitGroup
		closing.Go(func() { physicalSink.Close(ctx) })
		closing.Go(func() { agentRuntime.Close(ctx) })
		closing.Wait()
	}
	closeAll := func() {
		ctx, cancel := context.WithTimeout(context.Background(), FinalFlushBudget)
		defer cancel()
		closeSinks(ctx)
	}
	modelFactory, byokFactory, err := service.NewModelObservabilityFactories(telemetry, options.Logger, agentRuntime, settings.ProviderWire.StreamDrainDuration)
	if err != nil {
		closeAll()
		return nil, err
	}
	modelCatalog, err := service.BuildCatalog(file, resolvedProviders, nativeClient, modelFactory, physicalSink)
	if err != nil {
		closeAll()
		return nil, err
	}
	errorWriter := providerv4.NewHostErrorWriter()
	configuredLanguage, err := providerv4.New(providerv4.Config{Selector: service.NewConfiguredSelector(modelCatalog), Limits: settings.ProviderWire, ConfiguredAPIKeys: configuredAPIKeys})
	if err != nil {
		closeAll()
		return nil, err
	}
	routes := routeHandlers{
		configuredDiscovery: service.ConfiguredDiscovery(discovery.New(modelCatalog, errorWriter), errorWriter),
		configuredLanguage:  configuredLanguage,
	}
	if settings.Access.Mode == gatewayauth.AccessModeCloud {
		byokLanguage, err := providerv4.New(providerv4.Config{Selector: service.NewBYOKSelector(nativeClient, byokFactory), Limits: settings.ProviderWire, ConfiguredAPIKeys: configuredAPIKeys})
		if err != nil {
			closeAll()
			return nil, err
		}
		routes.byokDiscovery = service.BYOKDiscovery(errorWriter)
		routes.byokLanguage = byokLanguage
	}
	return &Gateway{
		handler:       newHandler(settings.Access, telemetry, errorWriter, routes),
		authenticator: authenticator,
		close:         closeSinks,
	}, nil
}

func newAuthenticator(mode config.DeploymentMode, lookupEnv config.LookupEnv) (authn.Authenticator, error) {
	insecure, _ := lookupEnv(EnvInsecureSkipAuthentication)
	skip := insecure == "true"
	if skip && mode != config.DeploymentDevelopment {
		return nil, errors.New("config: insecure authentication skipping requires development mode")
	}
	group := manifestdata.LocalManifest().ManifestData.Group
	keys, err := gatewayauth.SigningKeys(lookupEnv)
	switch {
	case err == nil:
		return gatewayauth.NewPlatformAuthenticator(gatewayauth.PlatformConfig{PluginID: PluginID, Group: group, Keys: keys})
	case !errors.Is(err, gatewayauth.ErrNoSigningKeys):
		return nil, err
	case skip:
		return gatewayauth.NewPlatformAuthenticator(gatewayauth.PlatformConfig{PluginID: PluginID, Group: group, Unsafe: true})
	default:
		return nil, fmt.Errorf("config: set %s or %s to verify platform tokens", gatewayauth.EnvSigningKeysURL, gatewayauth.EnvSigningKeys)
	}
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

// Close flushes Agent Observability and attempt records concurrently until
// ctx ends, then releases them. Later records are dropped and counted.
func (g *Gateway) Close(ctx context.Context) { g.close(ctx) }
