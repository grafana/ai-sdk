package process

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	gatewayauth "github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/auth"
	"github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/config"
	"github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/discovery"
	"github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/outbound"
	"github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/service"
	providerv4 "github.com/grafana/ai-sdk/ai-gateway/providerwire/v4"
	"github.com/grafana/authlib/authn"
)

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
	jwksURL := ""
	if !settings.AuthUnsafe {
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
	clients, err := outbound.NewClients(
		settings.JWKSRequestTimeout,
		settings.AnthropicResponseHeaderTimeout,
		settings.JWKSResponseBytes,
		settings.AnthropicResponseBytes,
	)
	if err != nil {
		return err
	}

	processContext, cancelProcess := context.WithCancel(context.WithoutCancel(ctx))
	defer cancelProcess()
	var authenticator authn.Authenticator
	if settings.AuthUnsafe {
		authenticator, err = gatewayauth.NewUnsafeAuthenticator(settings.Audiences, func(message string) {
			logger.Warn(message)
		})
	} else {
		var keys *gatewayauth.JWKS
		keys, err = gatewayauth.NewJWKS(processContext, clients.JWKS, time.Now, gatewayauth.JWKSConfig{
			URL:             jwksURL,
			RequestTimeout:  settings.JWKSRequestTimeout,
			MaxKeys:         settings.JWKSMaxKeys,
			RefreshInterval: settings.JWKSRefreshInterval,
			MaxAge:          settings.JWKSMaxAge,
		})
		if err != nil {
			return err
		}
		authenticator, err = gatewayauth.NewAuthenticator(keys, settings.Audiences)
	}
	if err != nil {
		return err
	}
	modelCatalog, err := service.BuildCatalog(file, resolvedProviders, clients.Anthropic)
	if err != nil {
		return err
	}
	errorWriter := providerv4.NewHostErrorWriter()
	discoveryHandler, err := discovery.New(modelCatalog, errorWriter, settings.DiscoveryResponseBytes)
	if err != nil {
		return err
	}
	languageHandler, err := providerv4.New(providerv4.Config{Resolver: modelCatalog, Limits: settings.ProviderWire})
	if err != nil {
		return err
	}
	telemetry, err := service.NewTelemetry(logger)
	if err != nil {
		return err
	}
	readiness := &service.Readiness{}
	router := service.NewRouter(service.RouterDependencies{
		Readiness:     readiness,
		Telemetry:     telemetry,
		Authenticator: authenticator,
		ErrorWriter:   errorWriter,
		Discovery:     discoveryHandler,
		LanguageModel: languageHandler,
	})
	listener, err := listen("tcp", settings.ListenAddress)
	if err != nil {
		return fmt.Errorf("gateway process: binding listener: %w", err)
	}
	server := &http.Server{
		Handler:           router,
		ReadHeaderTimeout: settings.ReadHeaderTimeout,
		ReadTimeout:       settings.ReadTimeout,
		WriteTimeout:      settings.WriteTimeout,
		IdleTimeout:       settings.IdleTimeout,
		MaxHeaderBytes:    settings.MaxHeaderBytes,
		BaseContext: func(net.Listener) context.Context {
			return processContext
		},
	}
	return Serve(ctx, cancelProcess, server, listener, readiness, telemetry, logger, settings.ShutdownTimeout)
}

// Serve owns readiness and cancel-first graceful HTTP shutdown.
func Serve(ctx context.Context, cancel context.CancelFunc, server *http.Server, listener net.Listener, readiness *service.Readiness, telemetry *service.Telemetry, logger *slog.Logger, shutdownTimeout time.Duration) error {
	if shutdownTimeout <= 0 {
		return fmt.Errorf("gateway process: invalid serve dependency")
	}
	serveErrors := make(chan error, 1)
	go func() {
		serveErrors <- server.Serve(listener)
	}()
	readiness.Set(true)
	telemetry.SetReady(true)
	logProcessEvent(logger, processEventReady)

	var serveErr error
	serverStopped := false
	select {
	case serveErr = <-serveErrors:
		serverStopped = true
	case <-ctx.Done():
	}

	readiness.Set(false)
	telemetry.SetReady(false)
	logProcessEvent(logger, processEventShutdownStarted)
	defer logProcessEvent(logger, processEventShutdownCompleted)
	cancel()
	if serverStopped {
		if serveErr == nil || errors.Is(serveErr, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("gateway process: serving HTTP: %w", serveErr)
	}

	shutdownContext, shutdownCancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer shutdownCancel()
	shutdownErr := server.Shutdown(shutdownContext)
	if shutdownErr != nil {
		_ = server.Close()
	}
	serveErr = <-serveErrors
	if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
		return fmt.Errorf("gateway process: serving HTTP: %w", serveErr)
	}
	if shutdownErr != nil && !errors.Is(shutdownErr, context.DeadlineExceeded) {
		return fmt.Errorf("gateway process: shutting down HTTP: %w", shutdownErr)
	}
	return nil
}

func logProcessEvent(logger *slog.Logger, event string) {
	logger.Info("gateway process lifecycle", "event", event)
}
