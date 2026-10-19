package config

import (
	"context"
	"fmt"
	"math"
	"net"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/alecthomas/kingpin/v2"
	"github.com/grafana/ai-sdk/ai-gateway/catalog"
	"github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/auth"
	providerv4 "github.com/grafana/ai-sdk/ai-gateway/providerwire/v4"
)

// LookupEnv resolves one environment variable.
type LookupEnv func(string) (string, bool)

// DeploymentMode controls production-only security restrictions.
type DeploymentMode string

const (
	// DeploymentProduction enables production security restrictions.
	DeploymentProduction DeploymentMode = "production"
	// DeploymentDevelopment permits explicit local-development behavior.
	DeploymentDevelopment DeploymentMode = "development"
)

// AgentObservabilityProtocol selects the generation export transport.
type AgentObservabilityProtocol string

const (
	AgentObservabilityGRPC AgentObservabilityProtocol = "grpc"
	AgentObservabilityHTTP AgentObservabilityProtocol = "http"
)

// AgentObservabilitySettings contains the bounded process-wide exporter policy.
// Export credentials are represented only by an environment-variable name.
type AgentObservabilitySettings struct {
	Enabled         bool
	Protocol        AgentObservabilityProtocol
	Endpoint        string
	TLS             bool
	AuthSecretEnv   string
	QueueSize       int
	BatchSize       int
	PayloadMaxBytes int
	MaxRetries      int
	InitialBackoff  time.Duration
	MaxBackoff      time.Duration
	ExportTimeout   time.Duration
	FlushInterval   time.Duration
	FlushTimeout    time.Duration
	ShutdownTimeout time.Duration
}

// Settings contains every scalar plugin setting.
type Settings struct {
	ConfigFile                     string
	ConfigMaxBytes                 int64
	DeploymentMode                 DeploymentMode
	Access                         auth.AccessPolicy
	ObservationRegion              string
	ObservationApplication         string
	AgentObservability             AgentObservabilitySettings
	AnthropicResponseHeaderTimeout time.Duration
	AnthropicResponseBytes         int64
	ProviderWire                   providerv4.Limits
}

// settingsError names the environment setting whose value failed to parse,
// without the value. Parse errors for explicit arguments keep kingpin's text.
func settingsError(app *kingpin.Application, err error) error {
	for _, flag := range app.Model().Flags {
		for _, value := range flag.Default {
			if flag.Value.Set(value) != nil {
				return fmt.Errorf("config: %s has an invalid value", settingEnvironmentName(flag.Name))
			}
		}
	}
	return fmt.Errorf("config: parsing settings arguments: %w", err)
}

func settingEnvironmentName(flag string) string {
	return "GRAFANA_AI_GATEWAY_" + strings.ToUpper(strings.NewReplacer(".", "_", "-", "_").Replace(flag))
}

// ParseSettings parses flags and explicit environment bindings and validates scalar settings.
func ParseSettings(args []string, lookupEnv LookupEnv) (Settings, error) {
	if lookupEnv == nil {
		lookupEnv = func(string) (string, bool) { return "", false }
	}
	var settings Settings
	var deploymentMode string
	var accessMode, internalServices string
	var agentObservabilityProtocol string
	app := kingpin.New("grafana-ai-gateway", "Grafana AI Gateway plugin")
	app.Flag("config.file", "Model configuration YAML file.").Default(envDefault(lookupEnv, "GRAFANA_AI_GATEWAY_CONFIG_FILE", "")).StringVar(&settings.ConfigFile)
	app.Flag("config.max-bytes", "Maximum model configuration bytes.").Default(envDefault(lookupEnv, "GRAFANA_AI_GATEWAY_CONFIG_MAX_BYTES", "1048576")).Int64Var(&settings.ConfigMaxBytes)
	app.Flag("deployment.mode", "Deployment mode.").Default(envDefault(lookupEnv, "GRAFANA_AI_GATEWAY_DEPLOYMENT_MODE", "production")).StringVar(&deploymentMode)
	app.Flag("access.mode", "Access mode (cloud or self-hosted).").Default(envDefault(lookupEnv, "GRAFANA_AI_GATEWAY_ACCESS_MODE", "")).StringVar(&accessMode)
	app.Flag("access.internal-services", "Comma-separated service identities with configured access in cloud mode.").Default(envDefault(lookupEnv, "GRAFANA_AI_GATEWAY_ACCESS_INTERNAL_SERVICES", "")).StringVar(&internalServices)
	app.Flag("observation.region", "Trusted static observation region.").Default(envDefault(lookupEnv, "GRAFANA_AI_GATEWAY_OBSERVATION_REGION", "")).StringVar(&settings.ObservationRegion)
	app.Flag("observation.application", "Trusted static observation application.").Default(envDefault(lookupEnv, "GRAFANA_AI_GATEWAY_OBSERVATION_APPLICATION", "")).StringVar(&settings.ObservationApplication)
	app.Flag("agento11y.enabled", "Enable Agent Observability generation export.").Default(envDefault(lookupEnv, "GRAFANA_AI_GATEWAY_AGENTO11Y_ENABLED", "false")).BoolVar(&settings.AgentObservability.Enabled)
	app.Flag("agento11y.protocol", "Agent Observability export protocol (grpc or http).").Default(envDefault(lookupEnv, "GRAFANA_AI_GATEWAY_AGENTO11Y_PROTOCOL", "grpc")).StringVar(&agentObservabilityProtocol)
	app.Flag("agento11y.endpoint", "Agent Observability generation export endpoint.").Default(envDefault(lookupEnv, "GRAFANA_AI_GATEWAY_AGENTO11Y_ENDPOINT", "")).StringVar(&settings.AgentObservability.Endpoint)
	app.Flag("agento11y.tls", "Require TLS for Agent Observability export.").Default(envDefault(lookupEnv, "GRAFANA_AI_GATEWAY_AGENTO11Y_TLS", "true")).BoolVar(&settings.AgentObservability.TLS)
	app.Flag("agento11y.auth-secret-env", "Environment variable containing the Agent Observability bearer credential.").Default(envDefault(lookupEnv, "GRAFANA_AI_GATEWAY_AGENTO11Y_AUTH_SECRET_ENV", "")).StringVar(&settings.AgentObservability.AuthSecretEnv)
	app.Flag("agento11y.queue-size", "Maximum queued Agent Observability generations.").Default(envDefault(lookupEnv, "GRAFANA_AI_GATEWAY_AGENTO11Y_QUEUE_SIZE", "2000")).IntVar(&settings.AgentObservability.QueueSize)
	app.Flag("agento11y.batch-size", "Maximum Agent Observability export batch size.").Default(envDefault(lookupEnv, "GRAFANA_AI_GATEWAY_AGENTO11Y_BATCH_SIZE", "100")).IntVar(&settings.AgentObservability.BatchSize)
	app.Flag("agento11y.payload-max-bytes", "Maximum Agent Observability export payload bytes.").Default(envDefault(lookupEnv, "GRAFANA_AI_GATEWAY_AGENTO11Y_PAYLOAD_MAX_BYTES", "16777216")).IntVar(&settings.AgentObservability.PayloadMaxBytes)
	app.Flag("agento11y.max-retries", "Maximum Agent Observability export retries.").Default(envDefault(lookupEnv, "GRAFANA_AI_GATEWAY_AGENTO11Y_MAX_RETRIES", "5")).IntVar(&settings.AgentObservability.MaxRetries)
	app.Flag("agento11y.initial-backoff", "Initial Agent Observability retry backoff.").Default(envDefault(lookupEnv, "GRAFANA_AI_GATEWAY_AGENTO11Y_INITIAL_BACKOFF", "100ms")).DurationVar(&settings.AgentObservability.InitialBackoff)
	app.Flag("agento11y.max-backoff", "Maximum Agent Observability retry backoff.").Default(envDefault(lookupEnv, "GRAFANA_AI_GATEWAY_AGENTO11Y_MAX_BACKOFF", "5s")).DurationVar(&settings.AgentObservability.MaxBackoff)
	app.Flag("agento11y.export-timeout", "Maximum duration of each Agent Observability HTTP or gRPC export attempt.").Default(envDefault(lookupEnv, "GRAFANA_AI_GATEWAY_AGENTO11Y_EXPORT_TIMEOUT", "10s")).DurationVar(&settings.AgentObservability.ExportTimeout)
	app.Flag("agento11y.flush-interval", "Agent Observability asynchronous batch flush interval.").Default(envDefault(lookupEnv, "GRAFANA_AI_GATEWAY_AGENTO11Y_FLUSH_INTERVAL", "1s")).DurationVar(&settings.AgentObservability.FlushInterval)
	app.Flag("agento11y.flush-timeout", "Maximum Agent Observability explicit flush duration.").Default(envDefault(lookupEnv, "GRAFANA_AI_GATEWAY_AGENTO11Y_FLUSH_TIMEOUT", "5s")).DurationVar(&settings.AgentObservability.FlushTimeout)
	app.Flag("agento11y.shutdown-timeout", "Maximum Agent Observability shutdown duration.").Default(envDefault(lookupEnv, "GRAFANA_AI_GATEWAY_AGENTO11Y_SHUTDOWN_TIMEOUT", "5s")).DurationVar(&settings.AgentObservability.ShutdownTimeout)
	app.Flag("anthropic.response-header-timeout", "Model provider response-header timeout (all provider types).").Default(envDefault(lookupEnv, "GRAFANA_AI_GATEWAY_ANTHROPIC_RESPONSE_HEADER_TIMEOUT", "10s")).DurationVar(&settings.AnthropicResponseHeaderTimeout)
	app.Flag("anthropic.response-bytes", "Maximum cumulative model provider response bytes (all provider types).").Default(envDefault(lookupEnv, "GRAFANA_AI_GATEWAY_ANTHROPIC_RESPONSE_BYTES", "16777216")).Int64Var(&settings.AnthropicResponseBytes)
	app.Flag("providerwire.request-bytes", "Maximum ProviderWire request bytes.").Default(envDefault(lookupEnv, "GRAFANA_AI_GATEWAY_PROVIDERWIRE_REQUEST_BYTES", "1048576")).Int64Var(&settings.ProviderWire.RequestBytes)
	app.Flag("providerwire.unary-response-bytes", "Maximum ProviderWire unary response bytes.").Default(envDefault(lookupEnv, "GRAFANA_AI_GATEWAY_PROVIDERWIRE_UNARY_RESPONSE_BYTES", "8388608")).Int64Var(&settings.ProviderWire.UnaryResponseBytes)
	app.Flag("providerwire.stream-parts", "Maximum ProviderWire stream parts.").Default(envDefault(lookupEnv, "GRAFANA_AI_GATEWAY_PROVIDERWIRE_STREAM_PARTS", "100000")).IntVar(&settings.ProviderWire.StreamParts)
	app.Flag("providerwire.stream-frame-bytes", "Maximum ProviderWire stream frame bytes.").Default(envDefault(lookupEnv, "GRAFANA_AI_GATEWAY_PROVIDERWIRE_STREAM_FRAME_BYTES", "1048576")).Int64Var(&settings.ProviderWire.StreamFrameBytes)
	app.Flag("providerwire.model-duration", "Maximum ProviderWire model duration.").Default(envDefault(lookupEnv, "GRAFANA_AI_GATEWAY_PROVIDERWIRE_MODEL_DURATION", "120s")).DurationVar(&settings.ProviderWire.ModelDuration)
	app.Flag("providerwire.stream-idle-duration", "Maximum ProviderWire stream idle duration.").Default(envDefault(lookupEnv, "GRAFANA_AI_GATEWAY_PROVIDERWIRE_STREAM_IDLE_DURATION", "30s")).DurationVar(&settings.ProviderWire.StreamIdleDuration)
	app.Flag("providerwire.stream-drain-duration", "ProviderWire stream drain duration.").Default(envDefault(lookupEnv, "GRAFANA_AI_GATEWAY_PROVIDERWIRE_STREAM_DRAIN_DURATION", "1s")).DurationVar(&settings.ProviderWire.StreamDrainDuration)

	if _, err := app.Parse(args); err != nil {
		return Settings{}, settingsError(app, err)
	}
	settings.DeploymentMode = DeploymentMode(deploymentMode)
	settings.AgentObservability.Protocol = AgentObservabilityProtocol(agentObservabilityProtocol)
	access, err := auth.ParseAccessPolicy(accessMode, internalServices)
	if err != nil {
		return Settings{}, err
	}
	settings.Access = access
	if err := settings.Validate(); err != nil {
		return Settings{}, err
	}
	return settings, nil
}

// Validate validates all scalar process settings without performing I/O.
func (settings Settings) Validate() error {
	if strings.TrimSpace(settings.ConfigFile) == "" {
		return fmt.Errorf("config: config file is required")
	}
	if settings.DeploymentMode != DeploymentProduction && settings.DeploymentMode != DeploymentDevelopment {
		return fmt.Errorf("config: deployment mode must be production or development")
	}
	if err := validateOptionalObservationSetting("region", settings.ObservationRegion); err != nil {
		return err
	}
	if err := validateOptionalObservationSetting("application", settings.ObservationApplication); err != nil {
		return err
	}
	if err := settings.AgentObservability.validate(settings.DeploymentMode); err != nil {
		return err
	}
	byteLimits := []struct {
		name  string
		value int64
	}{
		{name: "config max bytes", value: settings.ConfigMaxBytes},
		{name: "anthropic response bytes", value: settings.AnthropicResponseBytes},
		{name: "providerwire request bytes", value: settings.ProviderWire.RequestBytes},
		{name: "providerwire unary response bytes", value: settings.ProviderWire.UnaryResponseBytes},
		{name: "providerwire stream frame bytes", value: settings.ProviderWire.StreamFrameBytes},
	}
	for _, limit := range byteLimits {
		if limit.value <= 0 {
			return fmt.Errorf("config: %s must be positive", limit.name)
		}
		if limit.value == math.MaxInt64 {
			return fmt.Errorf("config: %s cannot safely use limit+1", limit.name)
		}
	}
	if settings.ProviderWire.StreamParts <= 0 {
		return fmt.Errorf("config: providerwire stream parts must be positive")
	}
	if settings.ProviderWire.StreamParts == math.MaxInt {
		return fmt.Errorf("config: providerwire stream parts cannot safely use limit+1")
	}
	durations := []struct {
		name  string
		value time.Duration
	}{
		{name: "anthropic response-header timeout", value: settings.AnthropicResponseHeaderTimeout},
		{name: "providerwire model duration", value: settings.ProviderWire.ModelDuration},
		{name: "providerwire stream idle duration", value: settings.ProviderWire.StreamIdleDuration},
		{name: "providerwire stream drain duration", value: settings.ProviderWire.StreamDrainDuration},
	}
	for _, duration := range durations {
		if duration.value <= 0 {
			return fmt.Errorf("config: %s must be positive", duration.name)
		}
	}
	if settings.AnthropicResponseBytes >= 32<<20 {
		return fmt.Errorf("config: anthropic response bytes must be below the SDK scanner limit")
	}
	if settings.AnthropicResponseHeaderTimeout > settings.ProviderWire.ModelDuration {
		return fmt.Errorf("config: anthropic response-header timeout must not exceed model duration")
	}
	if settings.ProviderWire.StreamIdleDuration > settings.ProviderWire.ModelDuration {
		return fmt.Errorf("config: providerwire stream idle duration must not exceed model duration")
	}
	if _, err := providerv4.New(providerv4.Config{Selector: providerv4.CatalogSelector(limitValidationResolver{}), Limits: settings.ProviderWire}); err != nil {
		return fmt.Errorf("config: providerwire limits: %w", err)
	}
	return nil
}

var environmentVariableName = regexp.MustCompile(`^[A-Z_][A-Z0-9_]*$`)

var ambientAgentObservabilityEnvironment = []string{
	"AGENTO11Y_ENDPOINT", "SIGIL_ENDPOINT",
	"AGENTO11Y_PROTOCOL", "SIGIL_PROTOCOL",
	"AGENTO11Y_INSECURE", "SIGIL_INSECURE",
	"AGENTO11Y_HEADERS", "SIGIL_HEADERS",
	"AGENTO11Y_AUTH_MODE", "SIGIL_AUTH_MODE",
	"AGENTO11Y_AUTH_TENANT_ID", "SIGIL_AUTH_TENANT_ID",
	"AGENTO11Y_AUTH_TOKEN", "SIGIL_AUTH_TOKEN",
	"AGENTO11Y_AGENT_NAME", "SIGIL_AGENT_NAME",
	"AGENTO11Y_AGENT_VERSION", "SIGIL_AGENT_VERSION",
	"AGENTO11Y_USER_ID", "SIGIL_USER_ID",
	"AGENTO11Y_TAGS", "SIGIL_TAGS",
	"AGENTO11Y_CONTENT_CAPTURE_MODE", "SIGIL_CONTENT_CAPTURE_MODE",
	"AGENTO11Y_DEBUG", "SIGIL_DEBUG",
	"AGENTO11Y_REDACT_INPUT_MESSAGES", "SIGIL_REDACT_INPUT_MESSAGES",
	"AGENTO11Y_EXPORT_TIMEOUT_MS", "SIGIL_EXPORT_TIMEOUT_MS",
	"AGENTO11Y_MAX_RETRIES", "SIGIL_MAX_RETRIES",
	"AGENTO11Y_MAX_BACKOFF_MS", "SIGIL_MAX_BACKOFF_MS",
	"AGENTO11Y_QUEUE_SIZE", "SIGIL_QUEUE_SIZE",
	"AGENTO11Y_ENABLE_EXPERIMENTAL_FEATURES", "AGENTO11Y_USE_EXPERIMENTAL_OTEL", "SIGIL_USE_EXPERIMENTAL_OTEL",
}

func (settings AgentObservabilitySettings) validate(mode DeploymentMode) error {
	if !settings.Enabled {
		return nil
	}
	if settings.Protocol != AgentObservabilityGRPC && settings.Protocol != AgentObservabilityHTTP {
		return fmt.Errorf("config: agent observability protocol must be grpc or http")
	}
	if strings.TrimSpace(settings.Endpoint) != settings.Endpoint || settings.Endpoint == "" {
		return fmt.Errorf("config: agent observability endpoint is required without surrounding whitespace")
	}
	if err := validateAgentObservabilityEndpoint(settings.Protocol, settings.Endpoint, settings.TLS); err != nil {
		return err
	}
	if !environmentVariableName.MatchString(settings.AuthSecretEnv) {
		return fmt.Errorf("config: agent observability auth secret environment reference is required and must be an environment variable name")
	}
	for _, reserved := range ambientAgentObservabilityEnvironment {
		if settings.AuthSecretEnv == reserved {
			return fmt.Errorf("config: agent observability auth secret reference must not use an SDK-reserved environment variable")
		}
	}
	if mode == DeploymentProduction && !settings.TLS {
		return fmt.Errorf("config: agent observability production export requires TLS")
	}
	if settings.QueueSize <= 0 || settings.QueueSize > 1_000_000 {
		return fmt.Errorf("config: agent observability queue size must be between 1 and 1000000")
	}
	if settings.BatchSize <= 0 || settings.BatchSize > settings.QueueSize {
		return fmt.Errorf("config: agent observability batch size must be positive and no greater than queue size")
	}
	if settings.PayloadMaxBytes <= 0 || settings.PayloadMaxBytes > 64<<20 {
		return fmt.Errorf("config: agent observability payload max bytes must be between 1 and 67108864")
	}
	if settings.MaxRetries <= 0 || settings.MaxRetries > 10 {
		return fmt.Errorf("config: agent observability max retries must be between 1 and 10")
	}
	for _, duration := range []struct {
		name  string
		value time.Duration
	}{
		{name: "initial backoff", value: settings.InitialBackoff},
		{name: "maximum backoff", value: settings.MaxBackoff},
		{name: "export timeout", value: settings.ExportTimeout},
		{name: "flush interval", value: settings.FlushInterval},
		{name: "flush timeout", value: settings.FlushTimeout},
		{name: "shutdown timeout", value: settings.ShutdownTimeout},
	} {
		if duration.value <= 0 || duration.value > 5*time.Minute {
			return fmt.Errorf("config: agent observability %s must be positive and no greater than 5m", duration.name)
		}
	}
	if settings.MaxBackoff < settings.InitialBackoff {
		return fmt.Errorf("config: agent observability maximum backoff must be at least initial backoff")
	}
	return nil
}

// ResolveAuthSecret resolves the configured bearer credential once without
// including its environment-variable name or value in failures.
func (settings AgentObservabilitySettings) ResolveAuthSecret(lookupEnv LookupEnv) (string, error) {
	if !settings.Enabled {
		return "", nil
	}
	if lookupEnv == nil {
		return "", fmt.Errorf("config: agent observability auth secret is unavailable")
	}
	secret, ok := lookupEnv(settings.AuthSecretEnv)
	if !ok || strings.TrimSpace(secret) == "" {
		return "", fmt.Errorf("config: agent observability auth secret is unavailable")
	}
	if strings.TrimSpace(secret) != secret {
		return "", fmt.Errorf("config: agent observability auth secret is invalid")
	}
	return secret, nil
}

// ValidateAmbientEnvironment rejects the SDK's independent environment layer.
// Gateway-owned GRAFANA_AI_GATEWAY_* bindings are the only configuration
// source; this prevents NewClient from importing ambient identity, tags,
// headers, endpoints, auth, or capture policy behind the validated settings.
func (settings AgentObservabilitySettings) ValidateAmbientEnvironment(lookupEnv LookupEnv) error {
	if !settings.Enabled || lookupEnv == nil {
		return nil
	}
	for _, name := range ambientAgentObservabilityEnvironment {
		if value, ok := lookupEnv(name); ok && strings.TrimSpace(value) != "" {
			return fmt.Errorf("config: ambient agent observability SDK environment is not allowed")
		}
	}
	return nil
}

func validateAgentObservabilityEndpoint(protocol AgentObservabilityProtocol, endpoint string, tlsEnabled bool) error {
	if strings.Contains(endpoint, "://") {
		parsed, err := url.Parse(endpoint)
		if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
			return fmt.Errorf("config: agent observability endpoint must be an absolute credential-free endpoint without query or fragment")
		}
		expectedScheme := "http"
		if tlsEnabled {
			expectedScheme = "https"
		}
		if parsed.Scheme != expectedScheme {
			return fmt.Errorf("config: agent observability endpoint scheme must match TLS setting")
		}
		if protocol == AgentObservabilityGRPC && parsed.Path != "" {
			return fmt.Errorf("config: agent observability grpc endpoint must not contain a path")
		}
		return nil
	}
	if protocol == AgentObservabilityHTTP {
		return fmt.Errorf("config: agent observability http endpoint must include an http or https scheme")
	}
	if strings.ContainsAny(endpoint, "@/?#") {
		return fmt.Errorf("config: agent observability grpc endpoint must be a credential-free host:port")
	}
	if _, _, err := net.SplitHostPort(endpoint); err != nil {
		return fmt.Errorf("config: agent observability grpc endpoint must use host:port syntax")
	}
	return nil
}

func validateOptionalObservationSetting(name, value string) error {
	if value == "" {
		return nil
	}
	if strings.TrimSpace(value) != value || len(value) > 128 {
		return fmt.Errorf("config: observation %s must be at most 128 characters without surrounding whitespace", name)
	}
	for _, character := range value {
		if character < 0x21 || character > 0x7e {
			return fmt.Errorf("config: observation %s must contain only visible ASCII", name)
		}
	}
	return nil
}

func envDefault(lookupEnv LookupEnv, name, fallback string) string {
	if value, ok := lookupEnv(name); ok {
		return value
	}
	return fallback
}

type limitValidationResolver struct{}

func (limitValidationResolver) ResolveModel(context.Context, string) (catalog.ResolvedModel, error) {
	return catalog.ResolvedModel{}, catalog.ErrUnknownModel
}
