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

	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
	gatewayauth "github.com/grafana/ai-sdk/ai-gateway/pkg/internal/auth"
	"github.com/grafana/ai-sdk/ai-gateway/pkg/internal/config"
	"github.com/grafana/ai-sdk/ai-gateway/pkg/internal/discovery"
	"github.com/grafana/ai-sdk/ai-gateway/pkg/internal/manifestdata"
	modelcatalogv1alpha1 "github.com/grafana/ai-sdk/ai-gateway/pkg/internal/modelcatalog/v1alpha1"
	"github.com/grafana/ai-sdk/ai-gateway/pkg/internal/outbound"
	"github.com/grafana/ai-sdk/ai-gateway/pkg/internal/service"
	"github.com/grafana/ai-sdk/ai-gateway/pkg/internal/stackcatalog"
	providerv4 "github.com/grafana/ai-sdk/ai-gateway/providerwire/v4"
)

// PluginID is the plugin ID declared in src/plugin.json. The host addresses
// delegated tokens to it, so every build must identify as it.
const PluginID = "grafana-aigateway-app"

// finalFlushBudget bounds the telemetry flush after the plugin host stops the plugin.
const finalFlushBudget = time.Second

// insecureSkipAuthenticationEnabled is the value with which the plugin host
// enables plugin.EnvVarInsecureSkipAuthentication.
const insecureSkipAuthenticationEnabled = "true"

// Options supplies the plugin host environment and process-wide sinks.
type Options struct {
	LookupEnv  config.LookupEnv
	Logger     *slog.Logger
	Registerer prometheus.Registerer
}

// Gateway is the constructed plugin. An idle Gateway has no handler,
// authenticator or sinks.
type Gateway struct {
	handler       http.Handler
	authenticator authn.Authenticator
	sinks         sinks
	egress        outbound.EgressPolicy
}

// New validates settings and constructs every Gateway dependency before any
// request can be served. Configuration is validated before any secret is
// read, and failures are secret-safe.
//
// Without platform signing keys, New returns an idle Gateway without reading
// any other setting: hosts that never delegate requests to the plugin, such
// as a Grafana that routes the Gateway API elsewhere, need no Gateway
// configuration, and the plugin host rejects every request.
func New(options Options) (*Gateway, error) {
	lookupEnv := options.LookupEnv
	keys, err := gatewayauth.SigningKeys(lookupEnv)
	if errors.Is(err, gatewayauth.ErrNoSigningKeys) {
		return newIdle(lookupEnv)
	}
	if err != nil {
		return nil, err
	}
	settings, err := config.ParseSettings(lookupEnv)
	if err != nil {
		return nil, err
	}
	authenticator, err := gatewayauth.NewPlatformAuthenticator(gatewayauth.PlatformConfig{PluginID: PluginID, Group: manifestdata.LocalManifest().ManifestData.Group, Keys: keys})
	if err != nil {
		return nil, err
	}
	// The Agent Observability SDK reads the process environment itself.
	if err := settings.AgentObservability.ValidateAmbientEnvironment(os.LookupEnv); err != nil {
		return nil, err
	}
	file, providers, err := operatorCatalog(settings, lookupEnv)
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
	return &Gateway{handler: handler, authenticator: authenticator, sinks: sinks, egress: settings.Egress}, nil
}

// operatorCatalog parses and validates the operator catalog and resolves its
// provider keys. Without a catalog setting it returns an empty catalog and
// reads no key.
func operatorCatalog(settings config.Settings, lookupEnv config.LookupEnv) (config.File, map[string]config.ResolvedProvider, error) {
	if !settings.HasModelCatalog() {
		return config.File{}, nil, nil
	}
	file, err := config.ParseModelCatalog(settings.ModelCatalog)
	if err != nil {
		return config.File{}, nil, err
	}
	if err := validateProviderEndpoints(file); err != nil {
		return config.File{}, nil, err
	}
	providers, err := file.ResolveProviderSecrets(lookupEnv)
	if err != nil {
		return config.File{}, nil, err
	}
	return file, providers, nil
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
	observers, err := service.NewModelObservers(sinks.telemetry, logger, sinks.agent, execution.DefaultLimits.StreamDrainDuration)
	if err != nil {
		return nil, err
	}
	catalog, err := service.BuildCatalog(file, providers, nativeClient, service.ConfiguredIdentity, sinks.attempts)
	if err != nil {
		return nil, err
	}
	var byokOptions *service.BYOKOptions
	if settings.Access.Mode == gatewayauth.AccessModeCloud {
		byokOptions = &service.BYOKOptions{Client: nativeClient, Factory: service.BYOKIdentity}
	}
	language, err := newLanguageHandler(execution.Config{Selector: service.NewSelector(catalog, byokOptions, &service.StackCatalogOptions{Client: nativeClient, Egress: settings.Egress}), Compose: observers.Compose, Limits: execution.DefaultLimits, ConfiguredAPIKeys: secrets})
	if err != nil {
		return nil, err
	}
	discoveryHandler := discovery.New(catalog, service.StackCatalogLister{Egress: settings.Egress}, providerv4.DiscoveryProjection{}, providerv4.NewHostErrorWriter())
	return newHandler(settings.Access, sinks.telemetry, productionRoutes(discoveryHandler, language))
}

// newLanguageHandler serves ProviderWire V4 over one execution runtime.
func newLanguageHandler(config execution.Config) (http.Handler, error) {
	runtime, err := execution.New(config)
	if err != nil {
		return nil, err
	}
	return providerv4.New(runtime)
}

// newIdle returns the idle Gateway. It refuses to start when unverified
// requests are requested instead, since the plugin host would then serve them.
func newIdle(lookupEnv config.LookupEnv) (*Gateway, error) {
	if skip, _ := lookupEnv(plugin.EnvVarInsecureSkipAuthentication); skip == insecureSkipAuthenticationEnabled {
		return nil, fmt.Errorf("config: set %s or %s to verify platform tokens; %s is not supported", plugin.EnvVarGrafanaJWKSURL, plugin.EnvVarGrafanaJWKS, plugin.EnvVarInsecureSkipAuthentication)
	}
	return &Gateway{}, nil
}

// Idle reports whether the Gateway started without platform signing keys.
// An idle Gateway serves no requests.
func (g *Gateway) Idle() bool { return g.authenticator == nil }

// Handler returns the ProviderWire route handler for the plugin route
// adapter, or nil when the Gateway is idle.
func (g *Gateway) Handler() http.Handler { return g.handler }

// Authenticator returns the plugin authenticator for delegated access tokens,
// or nil when the Gateway is idle.
func (g *Gateway) Authenticator() authn.Authenticator { return g.authenticator }

// Provider returns the app-sdk provider whose app serves Handler and the
// manifest kinds with their admission validators.
func (g *Gateway) Provider() app.Provider {
	return simple.NewAppProvider(manifestdata.LocalManifest(), nil, func(cfg app.Config) (app.App, error) {
		return simple.NewApp(simple.AppConfig{Name: cfg.ManifestData.AppName, KubeConfig: cfg.KubeConfig, Handler: g.handler, ManagedKinds: managedKinds(g.egress)})
	})
}

// managedKinds lists every manifest kind with its admission validator, which
// checks custom endpoints against egress. No kind has a reconciler or
// watcher, so the app starts no informers.
func managedKinds(egress outbound.EgressPolicy) []simple.AppManagedKind {
	return []simple.AppManagedKind{{Kind: modelcatalogv1alpha1.Kind(), Validator: stackcatalog.Validator{Policy: egress}}}
}

// Close flushes Agent Observability and attempt records concurrently for at
// most the final flush budget, then releases them. Later records are dropped
// and counted. Closing an idle Gateway does nothing.
func (g *Gateway) Close() {
	if g.Idle() {
		return
	}
	g.sinks.close()
}
