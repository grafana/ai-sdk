package process

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"syscall"
	"time"

	gatewayauth "github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/auth"
	"github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/config"
	"github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/discovery"
	"github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/outbound"
	"github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/service"
	providerv4 "github.com/grafana/ai-sdk/ai-gateway/providerwire/v4"
	"github.com/grafana/authlib/authn"
)

const listenerStartupTimeout = 5 * time.Second

const (
	processEventStarting          = "process_starting"
	processEventReady             = "process_ready"
	processEventShutdownStarted   = "process_shutdown_started"
	processEventShutdownCompleted = "process_shutdown_completed"
)

// Run validates, constructs, binds, and serves the Gateway until context cancellation.
func Run(ctx context.Context, args []string, lookupEnv config.LookupEnv, listen func(network, address string) (net.Listener, error), logger *slog.Logger) error {
	logProcessEvent(logger, processEventStarting)

	settings, err := config.ParseSettings(args, lookupEnv)
	if err != nil {
		return err
	}
	if err := settings.AgentObservability.ValidateAmbientEnvironment(lookupEnv); err != nil {
		return err
	}
	jwksURL := ""
	if settings.AuthMode == config.AuthModeAccessToken && !settings.AuthUnsafe {
		parsed, err := outbound.ValidateEndpoint(settings.JWKSURL, settings.DeploymentMode)
		if err != nil {
			return fmt.Errorf("gateway process: validating JWKS endpoint: %w", err)
		}
		jwksURL = parsed.String()
	}
	file, err := config.LoadFile(settings.ConfigFile, settings.ConfigMaxBytes)
	if err != nil {
		return err
	}
	for name, provider := range file.Providers {
		if provider.BaseURL == "" {
			continue
		}
		parsed, err := outbound.ValidateEndpoint(provider.BaseURL, settings.DeploymentMode)
		if err != nil {
			return fmt.Errorf("gateway process: validating provider %q endpoint: %w", name, err)
		}
		provider.BaseURL = parsed.String()
		file.Providers[name] = provider
	}
	resolvedProviders, err := file.ResolveProviderSecrets(lookupEnv)
	if err != nil {
		return err
	}
	anthropicClient, err := outbound.NewAnthropicClient(settings.AnthropicResponseHeaderTimeout, settings.AnthropicResponseBytes)
	if err != nil {
		return err
	}

	processContext, cancelProcess := context.WithCancel(context.WithoutCancel(ctx))
	defer cancelProcess()
	var authenticator gatewayauth.RequestAuthenticator
	switch settings.AuthMode {
	case config.AuthModeCloudGateway:
		authenticator = gatewayauth.NewCloudProviderWireAuthenticator()
	case config.AuthModeAccessToken:
		var verifier authn.Authenticator
		if settings.AuthUnsafe {
			verifier, err = gatewayauth.NewUnsafeAuthenticator(settings.Audiences, func(message string) {
				logger.Warn(message)
			})
		} else {
			jwksClient, clientErr := outbound.NewJWKSClient(settings.JWKSRequestTimeout, settings.JWKSResponseBytes)
			if clientErr != nil {
				return clientErr
			}
			var keys *gatewayauth.JWKS
			keys, err = gatewayauth.NewJWKS(processContext, jwksClient, time.Now, gatewayauth.JWKSConfig{
				URL:             jwksURL,
				RequestTimeout:  settings.JWKSRequestTimeout,
				MaxKeys:         settings.JWKSMaxKeys,
				RefreshInterval: settings.JWKSRefreshInterval,
				MaxAge:          settings.JWKSMaxAge,
			})
			if err != nil {
				return err
			}
			verifier, err = gatewayauth.NewAuthenticator(keys, settings.Audiences)
		}
		authenticator = gatewayauth.NewAccessTokenAuthenticator(verifier)
	}
	if err != nil {
		return err
	}
	telemetry, err := service.NewTelemetry(logger, service.TelemetryOptions{
		Region:      settings.ObservationRegion,
		Application: settings.ObservationApplication,
	})
	if err != nil {
		return err
	}
	agentRuntime, err := service.NewAgentObservabilityRuntime(settings.AgentObservability, lookupEnv, telemetry)
	if err != nil {
		return err
	}
	modelFactory, err := service.NewModelObservabilityFactory(telemetry, logger, agentRuntime, settings.ProviderWire.StreamDrainDuration)
	if err != nil {
		agentRuntime.Close()
		return err
	}
	modelCatalog, err := service.BuildCatalog(file, resolvedProviders, anthropicClient, modelFactory)
	if err != nil {
		agentRuntime.Close()
		return err
	}
	errorWriter := providerv4.NewHostErrorWriter()
	discoveryHandler, err := discovery.New(modelCatalog, errorWriter, settings.DiscoveryResponseBytes)
	if err != nil {
		agentRuntime.Close()
		return err
	}
	languageHandler, err := providerv4.New(providerv4.Config{Resolver: modelCatalog, Limits: settings.ProviderWire})
	if err != nil {
		agentRuntime.Close()
		return err
	}
	readiness := &service.Readiness{}
	deps := service.RouterDependencies{
		Readiness:     readiness,
		Telemetry:     telemetry,
		Authenticator: authenticator,
		AuthSource:    gatewayauth.Source(settings.AuthMode),
		ErrorWriter:   errorWriter,
		Discovery:     discoveryHandler,
		LanguageModel: languageHandler,
	}
	var handlers []http.Handler
	addresses := []string{settings.ListenAddress}
	if settings.OperationalListenAddress != "" {
		handlers = []http.Handler{service.NewAPIRouter(deps), service.NewOperationalRouter(deps)}
		addresses = append(addresses, settings.OperationalListenAddress)
	} else {
		handlers = []http.Handler{service.NewRouter(deps)}
	}
	var servers []boundServer
	for i, address := range addresses {
		listener, err := listen("tcp", address)
		if err != nil {
			for _, binding := range servers {
				_ = binding.listener.Close()
			}
			agentRuntime.Close()
			logListenerFailure(logger, "bind", err)
			return fmt.Errorf("gateway process: binding listener: %w", err)
		}
		servers = append(servers, boundServer{
			listener: listener,
			server: &http.Server{
				Handler:           handlers[i],
				ReadHeaderTimeout: settings.ReadHeaderTimeout,
				ReadTimeout:       settings.ReadTimeout,
				WriteTimeout:      settings.WriteTimeout,
				IdleTimeout:       settings.IdleTimeout,
				MaxHeaderBytes:    settings.MaxHeaderBytes,
				BaseContext:       func(net.Listener) context.Context { return processContext },
			},
		})
	}
	return Serve(ctx, cancelProcess, servers, readiness, telemetry, logger, settings.ShutdownTimeout, agentRuntime.Close)
}

type boundServer struct {
	server   *http.Server
	listener net.Listener
}

// Serve owns readiness and cancel-first graceful HTTP shutdown, then runs the
// optional caller-bounded process finalizer before reporting shutdown completion.
func Serve(ctx context.Context, cancel context.CancelFunc, servers []boundServer, readiness *service.Readiness, telemetry *service.Telemetry, logger *slog.Logger, shutdownTimeout time.Duration, finalize func()) error {
	if shutdownTimeout <= 0 || len(servers) == 0 {
		return fmt.Errorf("gateway process: invalid serve dependency")
	}
	startupContext, cancelStartup := context.WithTimeout(ctx, listenerStartupTimeout)
	defer cancelStartup()
	accepted := make(chan struct{}, len(servers))
	release := make(chan struct{})
	probeErrors := make(chan error, len(servers))
	serveErrors := make(chan error, len(servers))
	for _, binding := range servers {
		listener := &startupListener{Listener: binding.listener, accepted: accepted, release: release}
		go func() { serveErrors <- binding.server.Serve(listener) }()
		go func() { probeErrors <- probeListener(startupContext, binding.listener.Addr()) }()
	}

	var result error
	remaining := len(servers)
	pendingAccepts, pendingProbes := len(servers), len(servers)
startup:
	for pendingAccepts > 0 || pendingProbes > 0 {
		select {
		case <-accepted:
			pendingAccepts--
		case err := <-probeErrors:
			pendingProbes--
			if err != nil {
				if ctx.Err() == nil {
					logListenerFailure(logger, "probe", err)
					result = fmt.Errorf("gateway process: probing listener: %w", err)
				}
				break startup
			}
		case err := <-serveErrors:
			remaining--
			logListenerFailure(logger, "serve", err)
			result = fmt.Errorf("gateway process: serving HTTP: %w", err)
			break startup
		case <-startupContext.Done():
			if ctx.Err() == nil {
				logListenerFailure(logger, "startup", startupContext.Err())
				result = fmt.Errorf("gateway process: starting listeners: %w", startupContext.Err())
			}
			break startup
		}
	}
	if result == nil && ctx.Err() == nil {
		readiness.Set(true)
		telemetry.SetReady(true)
		logProcessEvent(logger, processEventReady)
	}
	close(release)
	cancelStartup()
	for range pendingProbes {
		<-probeErrors
	}
	if result == nil && ctx.Err() == nil {
		select {
		case err := <-serveErrors:
			remaining--
			logListenerFailure(logger, "serve", err)
			result = fmt.Errorf("gateway process: serving HTTP: %w", err)
		case <-ctx.Done():
		}
	}

	readiness.Set(false)
	telemetry.SetReady(false)
	logProcessEvent(logger, processEventShutdownStarted)
	defer logProcessEvent(logger, processEventShutdownCompleted)
	if finalize != nil {
		defer finalize()
	}
	cancel()
	shutdownContext, shutdownCancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer shutdownCancel()
	shutdownErrors := make(chan error, len(servers))
	for _, binding := range servers {
		go func() {
			err := binding.server.Shutdown(shutdownContext)
			if err != nil {
				_ = binding.server.Close()
			}
			shutdownErrors <- err
		}()
	}
	for range servers {
		if err := <-shutdownErrors; err != nil && !errors.Is(err, context.DeadlineExceeded) {
			result = errors.Join(result, fmt.Errorf("gateway process: shutting down HTTP: %w", err))
		}
	}
	for range remaining {
		if err := <-serveErrors; err != nil && !errors.Is(err, http.ErrServerClosed) {
			result = errors.Join(result, fmt.Errorf("gateway process: serving HTTP: %w", err))
		}
	}
	return result
}

type startupListener struct {
	net.Listener
	accepted chan<- struct{}
	release  <-chan struct{}
}

func (listener *startupListener) Accept() (net.Conn, error) {
	connection, err := listener.Listener.Accept()
	if err == nil && listener.accepted != nil {
		listener.accepted <- struct{}{}
		<-listener.release
		listener.accepted = nil
	}
	return connection, err
}

func probeListener(ctx context.Context, address net.Addr) error {
	var dialer net.Dialer
	connection, err := dialer.DialContext(ctx, address.Network(), address.String())
	if err != nil {
		return err
	}
	return connection.Close()
}

func logListenerFailure(logger *slog.Logger, stage string, err error) {
	reason := "unknown"
	var errno syscall.Errno
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		reason = "timeout"
	case errors.Is(err, context.Canceled):
		reason = "canceled"
	case errors.Is(err, net.ErrClosed):
		reason = "closed"
	case errors.As(err, &errno):
		reason = errno.Error()
	}
	logger.Error("gateway listener failed", "stage", stage, "reason", reason)
}

func logProcessEvent(logger *slog.Logger, event string) {
	logger.Info("gateway process lifecycle", "event", event)
}
