package config

import (
	"fmt"
	"net/url"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/alecthomas/kingpin/v2"
)

// GuardAuthMode selects server-owned guard authentication, independently of
// caller authentication and generation-export credentials.
type GuardAuthMode string

const (
	GuardAuthModeBasic  GuardAuthMode = "basic"
	GuardAuthModeBearer GuardAuthMode = "bearer"
)

// GuardSettings defines the operator policy destination and per-call limits.
// Endpoint is a base URL and may have a path prefix.
// AuthSecretEnv references an environment variable; it never contains a secret.
// RetainedBytes bounds additional guard data, not total process memory.
type GuardSettings struct {
	Enabled       bool
	Endpoint      string
	TenantID      string
	AuthMode      GuardAuthMode
	AuthUsername  string
	AuthSecretEnv string
	FailOpen      bool
	Timeout       time.Duration
	BodyBytes     int64
	RetainedBytes int64
	MaxConcurrent int
}

func (settings *GuardSettings) bindFlags(app *kingpin.Application, lookupEnv LookupEnv, authMode *string) {
	app.Flag("guards.enabled", "Enable operator-owned preflight and postflight guards.").Default(envDefault(lookupEnv, "GRAFANA_AI_GATEWAY_GUARDS_ENABLED", "false")).BoolVar(&settings.Enabled)
	app.Flag("guards.endpoint", "Guard service base URL, including any path prefix.").Default(envDefault(lookupEnv, "GRAFANA_AI_GATEWAY_GUARDS_ENDPOINT", "")).StringVar(&settings.Endpoint)
	app.Flag("guards.tenant-id", "Operator-owned guard policy tenant.").Default(envDefault(lookupEnv, "GRAFANA_AI_GATEWAY_GUARDS_TENANT_ID", "")).StringVar(&settings.TenantID)
	app.Flag("guards.auth-mode", "Guard authentication mode: basic or bearer.").Default(envDefault(lookupEnv, "GRAFANA_AI_GATEWAY_GUARDS_AUTH_MODE", "")).StringVar(authMode)
	app.Flag("guards.auth-username", "Server-owned basic authentication username.").Default(envDefault(lookupEnv, "GRAFANA_AI_GATEWAY_GUARDS_AUTH_USERNAME", "")).StringVar(&settings.AuthUsername)
	app.Flag("guards.auth-secret-env", "Environment variable containing the guard authentication secret.").Default(envDefault(lookupEnv, "GRAFANA_AI_GATEWAY_GUARDS_AUTH_SECRET_ENV", "")).StringVar(&settings.AuthSecretEnv)
	app.Flag("guards.fail-open", "Allow eligible operations on guard service failures; never overrides deny or unsafe transformations.").Default(envDefault(lookupEnv, "GRAFANA_AI_GATEWAY_GUARDS_FAIL_OPEN", "false")).BoolVar(&settings.FailOpen)
	app.Flag("guards.timeout", "Maximum duration of each guard evaluation.").Default(envDefault(lookupEnv, "GRAFANA_AI_GATEWAY_GUARDS_TIMEOUT", "5s")).DurationVar(&settings.Timeout)
	app.Flag("guards.body-bytes", "Maximum guard request and decompressed response bytes (up to 4 MiB).").Default(envDefault(lookupEnv, "GRAFANA_AI_GATEWAY_GUARDS_BODY_BYTES", "4194304")).Int64Var(&settings.BodyBytes)
	app.Flag("guards.retained-bytes", "Maximum additional retained guard bytes per call (up to 64 MiB).").Default(envDefault(lookupEnv, "GRAFANA_AI_GATEWAY_GUARDS_RETAINED_BYTES", "8388608")).Int64Var(&settings.RetainedBytes)
	app.Flag("guards.max-concurrent", "Maximum active guarded calls (up to 128).").Default(envDefault(lookupEnv, "GRAFANA_AI_GATEWAY_GUARDS_MAX_CONCURRENT", "8")).IntVar(&settings.MaxConcurrent)
}

func (settings GuardSettings) validate(mode DeploymentMode, modelDuration time.Duration) error {
	if !settings.Enabled {
		return nil
	}
	// outbound depends on config, so scalar validation cannot import it. The
	// runtime must also call outbound.ValidateEndpoint before constructing URLs.
	parsed, err := url.Parse(settings.Endpoint)
	if err != nil || settings.Endpoint == "" || !validGuardCredential(settings.Endpoint) ||
		!parsed.IsAbs() || parsed.Opaque != "" || parsed.Host == "" || parsed.User != nil ||
		parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" {
		return fmt.Errorf("config: guards endpoint must be an absolute credential-free base URL without query or fragment")
	}
	switch mode {
	case DeploymentProduction:
		if parsed.Scheme != "https" {
			return fmt.Errorf("config: guards production endpoint must use https")
		}
	case DeploymentDevelopment:
		if parsed.Scheme != "https" && parsed.Scheme != "http" {
			return fmt.Errorf("config: guards development endpoint must use http or https")
		}
	default:
		return fmt.Errorf("config: guards deployment mode is invalid")
	}
	if !validGuardCredential(settings.TenantID) {
		return fmt.Errorf("config: guards tenant ID is required without surrounding whitespace or control characters")
	}
	switch settings.AuthMode {
	case GuardAuthModeBasic:
		if !validGuardCredential(settings.AuthUsername) || strings.Contains(settings.AuthUsername, ":") {
			return fmt.Errorf("config: guards basic auth username is required without surrounding whitespace, control characters, or colon")
		}
	case GuardAuthModeBearer:
		if settings.AuthUsername != "" {
			return fmt.Errorf("config: guards bearer auth must not specify a username")
		}
	default:
		return fmt.Errorf("config: guards auth mode must be basic or bearer")
	}
	if !environmentVariableName.MatchString(settings.AuthSecretEnv) {
		return fmt.Errorf("config: guards auth secret environment reference is required and must be an environment variable name")
	}
	if settings.Timeout < time.Millisecond || settings.Timeout > 119999*time.Millisecond {
		return fmt.Errorf("config: guards timeout must be between 1ms and 119999ms")
	}
	if settings.Timeout > modelDuration {
		return fmt.Errorf("config: guards timeout must not exceed model duration")
	}
	if settings.BodyBytes <= 0 || settings.BodyBytes > 4<<20 {
		return fmt.Errorf("config: guards body bytes must be between 1 and 4194304")
	}
	if settings.RetainedBytes <= 0 || settings.RetainedBytes > 64<<20 {
		return fmt.Errorf("config: guards retained bytes must be between 1 and 67108864")
	}
	if settings.MaxConcurrent <= 0 || settings.MaxConcurrent > 128 {
		return fmt.Errorf("config: guards maximum concurrency must be between 1 and 128")
	}
	return nil
}

// ResolveAuthSecret reads the operator's Basic password or bearer token.
// Errors disclose neither the environment reference nor its value.
func (settings GuardSettings) ResolveAuthSecret(lookupEnv LookupEnv) (string, error) {
	if !settings.Enabled {
		return "", nil
	}
	if !environmentVariableName.MatchString(settings.AuthSecretEnv) {
		return "", fmt.Errorf("config: guards auth secret is invalid")
	}
	if lookupEnv == nil {
		return "", fmt.Errorf("config: guards auth secret is unavailable")
	}
	secret, ok := lookupEnv(settings.AuthSecretEnv)
	if !ok {
		return "", fmt.Errorf("config: guards auth secret is unavailable")
	}
	if !validGuardCredential(secret) {
		return "", fmt.Errorf("config: guards auth secret is invalid")
	}
	return secret, nil
}

func validGuardCredential(value string) bool {
	if value == "" || strings.TrimSpace(value) != value || !utf8.ValidString(value) {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}
