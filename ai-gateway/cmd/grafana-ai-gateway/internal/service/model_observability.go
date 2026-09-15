package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/grafana/agento11y/go/agento11y"
	"github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/config"
	"github.com/grafana/ai-sdk/middleware"
	agentmiddleware "github.com/grafana/ai-sdk/middleware/agentobservability"
	logmiddleware "github.com/grafana/ai-sdk/middleware/logger"
	metricsmiddleware "github.com/grafana/ai-sdk/middleware/prometheus"
	"github.com/grafana/ai-sdk/provider"
)

const maxAgentDiagnosticBytes = 2048

// AgentObservabilityRuntime owns the optional process-wide recording client.
type AgentObservabilityRuntime struct {
	client          *agento11y.Client
	telemetry       *Telemetry
	flushTimeout    time.Duration
	shutdownTimeout time.Duration
	mu              sync.Mutex
	recordings      sync.WaitGroup
	closing         bool
	closeOnce       sync.Once
}

// NewAgentObservabilityRuntime consumes prevalidated settings, rejects the
// SDK's ambient configuration layer, resolves the bearer credential once, and
// constructs one metadata-only hooks-disabled client. A disabled development
// configuration returns a no-op runtime.
func NewAgentObservabilityRuntime(settings config.AgentObservabilitySettings, lookupEnv config.LookupEnv, telemetry *Telemetry) (runtime *AgentObservabilityRuntime, err error) {
	if telemetry == nil {
		return nil, fmt.Errorf("gateway service: telemetry is required")
	}
	runtime = &AgentObservabilityRuntime{
		telemetry:       telemetry,
		flushTimeout:    settings.FlushTimeout,
		shutdownTimeout: settings.ShutdownTimeout,
	}
	if !settings.Enabled {
		return runtime, nil
	}
	if err := settings.ValidateAmbientEnvironment(lookupEnv); err != nil {
		return nil, err
	}
	if err := settings.ValidateAmbientEnvironment(os.LookupEnv); err != nil {
		return nil, err
	}
	secret, err := settings.ResolveAuthSecret(lookupEnv)
	if err != nil {
		return nil, err
	}
	protocol := agento11y.GenerationExportProtocolGRPC
	if settings.Protocol == config.AgentObservabilityHTTP {
		protocol = agento11y.GenerationExportProtocolHTTP
	}
	insecure := !settings.TLS
	clientConfig := agento11y.Config{
		GenerationExport: agento11y.GenerationExportConfig{
			Protocol:                   protocol,
			Endpoint:                   settings.Endpoint,
			Auth:                       agento11y.AuthConfig{Mode: agento11y.ExportAuthModeBearer, BearerToken: secret},
			Insecure:                   agento11y.BoolPtr(insecure),
			GRPCMaxSendMessageBytes:    settings.PayloadMaxBytes,
			GRPCMaxReceiveMessageBytes: settings.PayloadMaxBytes,
			BatchSize:                  settings.BatchSize,
			FlushInterval:              settings.FlushInterval,
			QueueSize:                  settings.QueueSize,
			MaxRetries:                 settings.MaxRetries,
			InitialBackoff:             settings.InitialBackoff,
			MaxBackoff:                 settings.MaxBackoff,
			PayloadMaxBytes:            settings.PayloadMaxBytes,
		},
		Hooks:          agento11y.HooksConfig{Enabled: false},
		ContentCapture: agento11y.ContentCaptureModeMetadataOnly,
		Debug:          agento11y.BoolPtr(false),
		Logger:         log.New(agentDiagnosticWriter{telemetry: telemetry}, "", 0),
	}
	defer func() {
		if recover() != nil {
			runtime = nil
			err = fmt.Errorf("gateway service: constructing agent observability client failed")
		}
	}()
	runtime.client = agento11y.NewClient(clientConfig)
	return runtime, nil
}

// Close performs independent bounded flush and shutdown attempts. It never
// returns raw SDK errors to process logging.
func (runtime *AgentObservabilityRuntime) Close() {
	if runtime == nil || runtime.client == nil {
		return
	}
	runtime.closeOnce.Do(func() {
		runtime.mu.Lock()
		runtime.closing = true
		runtime.mu.Unlock()
		if !runtime.waitForRecordings(runtime.flushTimeout) {
			runtime.telemetry.observeAgentExportFailure(agentExportFailureShutdown)
		}
		flushCtx, cancelFlush := context.WithTimeout(context.Background(), runtime.flushTimeout)
		if err := runtime.client.Flush(flushCtx); err != nil {
			runtime.telemetry.observeAgentExportFailure(agentExportFailureShutdown)
		}
		cancelFlush()
		shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), runtime.shutdownTimeout)
		if err := runtime.client.Shutdown(shutdownCtx); err != nil {
			runtime.telemetry.observeAgentExportFailure(agentExportFailureShutdown)
		}
		cancelShutdown()
	})
}

func (runtime *AgentObservabilityRuntime) acquireClient(context.Context) *agento11y.Client {
	if runtime == nil {
		return nil
	}
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if runtime.closing || runtime.client == nil {
		return nil
	}
	runtime.recordings.Add(1)
	return runtime.client
}

func (runtime *AgentObservabilityRuntime) recordingComplete() {
	runtime.recordings.Done()
}

func (runtime *AgentObservabilityRuntime) waitForRecordings(timeout time.Duration) bool {
	done := make(chan struct{})
	go func() {
		runtime.recordings.Wait()
		close(done)
	}()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-done:
		return true
	case <-timer.C:
		return false
	}
}

func (runtime *AgentObservabilityRuntime) recordError(err error) {
	if runtime == nil || err == nil {
		return
	}
	class := agentExportFailureUnknown
	switch {
	case errors.Is(err, agento11y.ErrQueueFull):
		class = agentExportFailureQueue
	case errors.Is(err, agento11y.ErrValidationFailed):
		class = agentExportFailureSerialization
	case errors.Is(err, agento11y.ErrClientShutdown):
		class = agentExportFailureShutdown
	}
	runtime.telemetry.observeAgentExportFailure(class)
}

type agentDiagnosticWriter struct {
	telemetry *Telemetry
}

func (writer agentDiagnosticWriter) Write(message []byte) (int, error) {
	class := agentExportFailureClass("")
	diagnosticBytes := message
	if len(diagnosticBytes) > maxAgentDiagnosticBytes {
		diagnosticBytes = diagnosticBytes[:maxAgentDiagnosticBytes]
	}
	diagnostic := strings.ToLower(string(diagnosticBytes))
	switch {
	case strings.HasPrefix(diagnostic, "agento11y generation rejected id="):
		// The SDK also emits one terminal batch failure for these details.
		// Ignore the per-generation line to avoid double counting a rejection.
		return len(message), nil
	case strings.HasPrefix(diagnostic, "agento11y generation export failed: generation export rejected:"):
		class = agentExportFailureRejected
	case strings.HasPrefix(diagnostic, "agento11y: generation sanitization failed"),
		strings.HasPrefix(diagnostic, "agento11y metric instrument init failed"):
		class = agentExportFailureSerialization
	case strings.HasPrefix(diagnostic, "agento11y export flush on shutdown failed"):
		class = agentExportFailureShutdown
	case strings.HasPrefix(diagnostic, "agento11y generation export failed"),
		strings.HasPrefix(diagnostic, "agento11y generation exporter init failed"):
		class = agentExportFailureTransport
	case strings.Contains(diagnostic, "failed"), strings.Contains(diagnostic, "failure"):
		class = agentExportFailureUnknown
	default:
		return len(message), nil
	}
	writer.telemetry.observeAgentExportFailure(class)
	return len(message), nil
}

// NewModelObservabilityFactory constructs shared middleware once and returns a
// catalog factory that adds the fixed logical chain around each lower model.
func NewModelObservabilityFactory(telemetry *Telemetry, logger *slog.Logger, runtime *AgentObservabilityRuntime, drainTimeout time.Duration) (ModelFactory, error) {
	if telemetry == nil || logger == nil || drainTimeout <= 0 {
		return nil, fmt.Errorf("gateway service: invalid model observability dependency")
	}
	metrics, err := metricsmiddleware.Middleware(metricsmiddleware.Options{
		Registerer:                telemetry.Registerer(),
		IdentitySource:            metricsmiddleware.IdentityRequested,
		StreamDrainTimeout:        drainTimeout,
		DisableStreamChunkMetrics: false,
	})
	if err != nil {
		return nil, fmt.Errorf("gateway service: registering model metrics: %w", err)
	}
	structuredLogger := logmiddleware.Middleware(logmiddleware.Options{
		Logger:             logger,
		DynamicAttrs:       observationLogAttrs,
		Redactor:           closedModelLogRedactor(logmiddleware.DefaultRedactor()),
		IdentitySource:     logmiddleware.IdentityRequested,
		StreamDrainTimeout: drainTimeout,
	})
	chain := modelObservationChain{
		contextBridge:    observationBridgeMiddleware(),
		structuredLogger: structuredLogger,
		prometheus:       metrics,
	}
	if runtime != nil && runtime.client != nil {
		recording := agentmiddleware.RecordingMiddleware(agentmiddleware.RecordingOptions{
			IdentitySource:     agentmiddleware.IdentityRequested,
			ContextSource:      agentmiddleware.ContextProvidedOnly,
			StreamDrainTimeout: drainTimeout,
			ClientResolver:     runtime.acquireClient,
			ContextProvider: func(ctx context.Context) agentmiddleware.ContextInfo {
				return agentmiddleware.ContextInfo{Metadata: observationMetadata(ctx)}
			},
			GenerationFilter: filterAgentGeneration,
			OnRecordError:    runtime.recordError,
			OnRecordComplete: runtime.recordingComplete,
		})
		chain.agentRecording = &recording
	}

	return func(canonicalID string, lower provider.LanguageModel) (provider.LanguageModel, error) {
		return chain.wrap(canonicalID, lower)
	}, nil
}

type modelObservationChain struct {
	contextBridge    middleware.Middleware
	agentRecording   *middleware.Middleware
	structuredLogger middleware.Middleware
	prometheus       middleware.Middleware
}

func (chain modelObservationChain) wrap(canonicalID string, lower provider.LanguageModel) (provider.LanguageModel, error) {
	identity, err := identityModelFactory(canonicalID, lower)
	if err != nil {
		return nil, err
	}
	observers := make([]middleware.Middleware, 0, 4)
	observers = append(observers, chain.contextBridge)
	if chain.agentRecording != nil {
		observers = append(observers, *chain.agentRecording)
	}
	observers = append(observers, chain.structuredLogger, chain.prometheus)
	return middleware.Wrap(middleware.WrapOptions{Model: identity, Middleware: observers}), nil
}

var allowedAgentMetadataKeys = map[string]struct{}{
	"gateway.correlation_id": {},
	"gateway.caller_service": {},
	"gateway.namespace":      {},
	"gateway.region":         {},
	"gateway.application":    {},
}

// filterAgentGeneration removes every metadata/tag/identity field not owned by
// the Gateway observation contract. Content structure remains intact for the
// client's metadata-only transformation, while provider options and raw usage
// metadata are discarded in favor of normalized Generation.Usage.
func filterAgentGeneration(input agentmiddleware.GenerationFilterInput) agento11y.Generation {
	generation := input.Generation
	metadata := make(map[string]any, len(allowedAgentMetadataKeys))
	for key := range allowedAgentMetadataKeys {
		value, ok := generation.Metadata[key]
		if !ok {
			continue
		}
		text, ok := value.(string)
		if !ok || boundedObservationValue(text) == "" {
			continue
		}
		metadata[key] = text
	}
	generation.Metadata = metadata
	generation.Tags = nil
	generation.UserID = ""
	generation.AgentName = ""
	generation.AgentVersion = ""
	generation.ResponseID = ""
	generation.ResponseModel = ""
	generation.ConversationID = ""
	generation.ConversationTitle = ""
	generation.ParentGenerationIDs = nil
	generation.MaxTokens = nil
	generation.Temperature = nil
	generation.TopP = nil
	generation.ToolChoice = nil
	generation.ThinkingEnabled = nil
	generation.EffectiveVersion = ""
	generation.CallError = ""
	generation.StopReason = unifiedAgentStopReason(input.FinishReason)
	return generation
}

func unifiedAgentStopReason(reason provider.FinishReason) string {
	switch reason.Unified {
	case provider.FinishReasonStop,
		provider.FinishReasonLength,
		provider.FinishReasonContentFilter,
		provider.FinishReasonToolCalls,
		provider.FinishReasonError,
		provider.FinishReasonOther:
		return string(reason.Unified)
	default:
		return ""
	}
}
