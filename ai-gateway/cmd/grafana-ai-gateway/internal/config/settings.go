package config

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/auth"
	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
)

// LookupEnv resolves one environment variable.
type LookupEnv func(string) (string, bool)

// AgentObservabilityProtocol selects the generation export transport.
type AgentObservabilityProtocol string

const (
	AgentObservabilityGRPC AgentObservabilityProtocol = "grpc"
	AgentObservabilityHTTP AgentObservabilityProtocol = "http"
)

// SettingEnvironmentPrefix prefixes every plugin setting. Grafana passes each
// key of its [plugin.grafana-aigateway-app] configuration section to the
// plugin as GF_PLUGIN_<KEY>.
const SettingEnvironmentPrefix = "GF_PLUGIN_"

const (
	// MaxFileBytes bounds the model configuration file.
	MaxFileBytes = 1 << 20
	// ProviderResponseHeaderTimeout bounds the wait for a model provider's
	// response headers.
	ProviderResponseHeaderTimeout = 10 * time.Second
	// ProviderResponseBytes bounds a model provider response body that is not
	// a stream. Streams are decoded incrementally and bounded by time.
	ProviderResponseBytes = 16 << 20
	// MaxProviderAttempts bounds the provider attempts of one request: a
	// configured model's primary and fallbacks, or the request's BYOK accounts
	// for the selected provider. Each attempt is one outbound request, so the
	// bound limits how far fast failures amplify.
	MaxProviderAttempts = 8
)

// Fixed stream durations. The request deadline belongs to the plugin host.
const (
	streamIdleDuration  = 30 * time.Second
	streamDrainDuration = time.Second
)

// AgentObservabilityTokenSetting is the setting key of the Agent
// Observability bearer token. The token is read only by ResolveAuthSecret,
// after the other settings and the model configuration are validated.
const AgentObservabilityTokenSetting = "agento11y_token"

// AgentObservabilitySettings selects the Agent Observability exporter.
type AgentObservabilitySettings struct {
	Enabled  bool
	Protocol AgentObservabilityProtocol
	Endpoint string
	TLS      bool
}

// Settings contains every plugin setting.
type Settings struct {
	ConfigFile         string
	Access             auth.AccessPolicy
	AgentObservability AgentObservabilitySettings
	ProviderWire       execution.Limits
}

// SettingName returns the environment variable of a plugin setting key, as
// Grafana derives it from the [plugin.grafana-aigateway-app] section.
func SettingName(key string) string {
	return SettingEnvironmentPrefix + strings.ToUpper(key)
}

// environment reads typed settings and keeps the first invalid one. An unset
// or empty variable selects the default.
type environment struct {
	lookup LookupEnv
	err    error
}

func (e *environment) raw(key string) (string, string, bool) {
	name := SettingName(key)
	value, ok := e.lookup(name)
	return name, value, ok && value != ""
}

func (e *environment) invalid(name string) {
	if e.err == nil {
		e.err = fmt.Errorf("config: %s has an invalid value", name)
	}
}

func (e *environment) string(key, fallback string) string {
	if _, value, ok := e.raw(key); ok {
		return value
	}
	return fallback
}

func (e *environment) bool(key string, fallback bool) bool {
	name, value, ok := e.raw(key)
	if !ok {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		e.invalid(name)
	}
	return parsed
}

// ParseSettings reads and validates the plugin settings from the environment.
func ParseSettings(lookupEnv LookupEnv) (Settings, error) {
	env := environment{lookup: lookupEnv}
	settings := Settings{
		ConfigFile: env.string("config_file", ""),
		AgentObservability: AgentObservabilitySettings{
			Enabled:  env.bool("agento11y_enabled", false),
			Protocol: AgentObservabilityProtocol(env.string("agento11y_protocol", string(AgentObservabilityGRPC))),
			Endpoint: env.string("agento11y_endpoint", ""),
			TLS:      env.bool("agento11y_tls", true),
		},
		ProviderWire: execution.Limits{
			StreamIdleDuration:  streamIdleDuration,
			StreamDrainDuration: streamDrainDuration,
		},
	}
	if env.err != nil {
		return Settings{}, env.err
	}
	access, err := auth.ParseAccessPolicy(env.string("access_mode", ""), env.string("access_internal_services", ""))
	if err != nil {
		return Settings{}, err
	}
	settings.Access = access
	if err := settings.Validate(); err != nil {
		return Settings{}, err
	}
	return settings, nil
}

// Validate validates the settings without performing I/O.
func (settings Settings) Validate() error {
	if strings.TrimSpace(settings.ConfigFile) == "" {
		return fmt.Errorf("config: config file is required")
	}
	if err := settings.AgentObservability.validate(); err != nil {
		return err
	}
	if err := settings.ProviderWire.Validate(); err != nil {
		return fmt.Errorf("config: providerwire limits: %w", err)
	}
	return nil
}

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

func (settings AgentObservabilitySettings) validate() error {
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
	return nil
}

// ResolveAuthSecret reads the bearer token setting once without including
// its value in failures.
func (settings AgentObservabilitySettings) ResolveAuthSecret(lookupEnv LookupEnv) (string, error) {
	if !settings.Enabled {
		return "", nil
	}
	if lookupEnv == nil {
		return "", fmt.Errorf("config: agent observability auth secret is unavailable")
	}
	secret, ok := lookupEnv(SettingName(AgentObservabilityTokenSetting))
	if !ok || strings.TrimSpace(secret) == "" {
		return "", fmt.Errorf("config: agent observability auth secret is unavailable")
	}
	if strings.TrimSpace(secret) != secret {
		return "", fmt.Errorf("config: agent observability auth secret is invalid")
	}
	return secret, nil
}

// ValidateAmbientEnvironment rejects the SDK's independent environment layer.
// Gateway-owned GF_PLUGIN_* settings are the only configuration
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
