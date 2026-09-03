package config

import (
	"context"
	"fmt"
	"math"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/alecthomas/kingpin/v2"
	"github.com/grafana/ai-sdk/ai-gateway/catalog"
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

// Settings contains every scalar process setting.
type Settings struct {
	ConfigFile                     string
	ConfigMaxBytes                 int64
	DeploymentMode                 DeploymentMode
	ListenAddress                  string
	ReadHeaderTimeout              time.Duration
	ReadTimeout                    time.Duration
	WriteTimeout                   time.Duration
	IdleTimeout                    time.Duration
	MaxHeaderBytes                 int
	ResponseGrace                  time.Duration
	ShutdownTimeout                time.Duration
	DiscoveryResponseBytes         int64
	AuthUnsafe                     bool
	JWKSURL                        string
	Audiences                      []string
	JWKSRequestTimeout             time.Duration
	JWKSResponseBytes              int64
	JWKSMaxKeys                    int
	JWKSRefreshInterval            time.Duration
	JWKSMaxAge                     time.Duration
	AnthropicResponseHeaderTimeout time.Duration
	AnthropicResponseBytes         int64
	ProviderWire                   providerv4.Limits
}

// ParseSettings parses flags and explicit environment bindings and validates scalar settings.
func ParseSettings(args []string, lookupEnv LookupEnv) (Settings, error) {
	if lookupEnv == nil {
		lookupEnv = func(string) (string, bool) { return "", false }
	}
	var settings Settings
	var deploymentMode string
	var audiences string
	app := kingpin.New("grafana-ai-gateway", "Authenticated Grafana AI Gateway")
	app.Flag("config.file", "Model configuration YAML file.").Default(envDefault(lookupEnv, "GRAFANA_AI_GATEWAY_CONFIG_FILE", "")).StringVar(&settings.ConfigFile)
	app.Flag("config.max-bytes", "Maximum model configuration bytes.").Default(envDefault(lookupEnv, "GRAFANA_AI_GATEWAY_CONFIG_MAX_BYTES", "1048576")).Int64Var(&settings.ConfigMaxBytes)
	app.Flag("deployment.mode", "Deployment mode.").Default(envDefault(lookupEnv, "GRAFANA_AI_GATEWAY_DEPLOYMENT_MODE", "production")).StringVar(&deploymentMode)
	app.Flag("server.listen-address", "HTTP listen address.").Default(envDefault(lookupEnv, "GRAFANA_AI_GATEWAY_SERVER_LISTEN_ADDRESS", ":8080")).StringVar(&settings.ListenAddress)
	app.Flag("server.read-header-timeout", "HTTP read-header timeout.").Default(envDefault(lookupEnv, "GRAFANA_AI_GATEWAY_SERVER_READ_HEADER_TIMEOUT", "5s")).DurationVar(&settings.ReadHeaderTimeout)
	app.Flag("server.read-timeout", "HTTP request-read timeout.").Default(envDefault(lookupEnv, "GRAFANA_AI_GATEWAY_SERVER_READ_TIMEOUT", "30s")).DurationVar(&settings.ReadTimeout)
	app.Flag("server.write-timeout", "HTTP response-write timeout.").Default(envDefault(lookupEnv, "GRAFANA_AI_GATEWAY_SERVER_WRITE_TIMEOUT", "165s")).DurationVar(&settings.WriteTimeout)
	app.Flag("server.idle-timeout", "HTTP idle timeout.").Default(envDefault(lookupEnv, "GRAFANA_AI_GATEWAY_SERVER_IDLE_TIMEOUT", "120s")).DurationVar(&settings.IdleTimeout)
	app.Flag("server.max-header-bytes", "Go HTTP maximum header parser bytes.").Default(envDefault(lookupEnv, "GRAFANA_AI_GATEWAY_SERVER_MAX_HEADER_BYTES", "65536")).IntVar(&settings.MaxHeaderBytes)
	app.Flag("server.response-grace", "Response completion grace.").Default(envDefault(lookupEnv, "GRAFANA_AI_GATEWAY_SERVER_RESPONSE_GRACE", "5s")).DurationVar(&settings.ResponseGrace)
	app.Flag("server.shutdown-timeout", "Graceful shutdown timeout.").Default(envDefault(lookupEnv, "GRAFANA_AI_GATEWAY_SERVER_SHUTDOWN_TIMEOUT", "15s")).DurationVar(&settings.ShutdownTimeout)
	app.Flag("discovery.response-bytes", "Maximum discovery response bytes.").Default(envDefault(lookupEnv, "GRAFANA_AI_GATEWAY_DISCOVERY_RESPONSE_BYTES", "1048576")).Int64Var(&settings.DiscoveryResponseBytes)
	app.Flag("auth.unsafe", "Enable unsafe development authentication.").Default(envDefault(lookupEnv, "GRAFANA_AI_GATEWAY_AUTH_UNSAFE", "false")).BoolVar(&settings.AuthUnsafe)
	app.Flag("auth.jwks-url", "JWKS endpoint URL.").Default(envDefault(lookupEnv, "GRAFANA_AI_GATEWAY_AUTH_JWKS_URL", "")).StringVar(&settings.JWKSURL)
	app.Flag("auth.audiences", "Comma-separated accepted audiences.").Default(envDefault(lookupEnv, "GRAFANA_AI_GATEWAY_AUTH_AUDIENCES", "ai-sdk")).StringVar(&audiences)
	app.Flag("auth.jwks-timeout", "JWKS request timeout.").Default(envDefault(lookupEnv, "GRAFANA_AI_GATEWAY_AUTH_JWKS_TIMEOUT", "5s")).DurationVar(&settings.JWKSRequestTimeout)
	app.Flag("auth.jwks-response-bytes", "Maximum JWKS response bytes.").Default(envDefault(lookupEnv, "GRAFANA_AI_GATEWAY_AUTH_JWKS_RESPONSE_BYTES", "1048576")).Int64Var(&settings.JWKSResponseBytes)
	app.Flag("auth.jwks-max-keys", "Maximum keys in one JWKS snapshot.").Default(envDefault(lookupEnv, "GRAFANA_AI_GATEWAY_AUTH_JWKS_MAX_KEYS", "128")).IntVar(&settings.JWKSMaxKeys)
	app.Flag("auth.jwks-refresh-interval", "Minimum JWKS refresh interval.").Default(envDefault(lookupEnv, "GRAFANA_AI_GATEWAY_AUTH_JWKS_REFRESH_INTERVAL", "5m")).DurationVar(&settings.JWKSRefreshInterval)
	app.Flag("auth.jwks-max-age", "Maximum JWKS snapshot age.").Default(envDefault(lookupEnv, "GRAFANA_AI_GATEWAY_AUTH_JWKS_MAX_AGE", "15m")).DurationVar(&settings.JWKSMaxAge)
	app.Flag("anthropic.response-header-timeout", "Anthropic response-header timeout.").Default(envDefault(lookupEnv, "GRAFANA_AI_GATEWAY_ANTHROPIC_RESPONSE_HEADER_TIMEOUT", "10s")).DurationVar(&settings.AnthropicResponseHeaderTimeout)
	app.Flag("anthropic.response-bytes", "Maximum cumulative Anthropic response bytes.").Default(envDefault(lookupEnv, "GRAFANA_AI_GATEWAY_ANTHROPIC_RESPONSE_BYTES", "16777216")).Int64Var(&settings.AnthropicResponseBytes)
	app.Flag("providerwire.request-bytes", "Maximum ProviderWire request bytes.").Default(envDefault(lookupEnv, "GRAFANA_AI_GATEWAY_PROVIDERWIRE_REQUEST_BYTES", "1048576")).Int64Var(&settings.ProviderWire.RequestBytes)
	app.Flag("providerwire.unary-response-bytes", "Maximum ProviderWire unary response bytes.").Default(envDefault(lookupEnv, "GRAFANA_AI_GATEWAY_PROVIDERWIRE_UNARY_RESPONSE_BYTES", "8388608")).Int64Var(&settings.ProviderWire.UnaryResponseBytes)
	app.Flag("providerwire.stream-parts", "Maximum ProviderWire stream parts.").Default(envDefault(lookupEnv, "GRAFANA_AI_GATEWAY_PROVIDERWIRE_STREAM_PARTS", "100000")).IntVar(&settings.ProviderWire.StreamParts)
	app.Flag("providerwire.stream-frame-bytes", "Maximum ProviderWire stream frame bytes.").Default(envDefault(lookupEnv, "GRAFANA_AI_GATEWAY_PROVIDERWIRE_STREAM_FRAME_BYTES", "1048576")).Int64Var(&settings.ProviderWire.StreamFrameBytes)
	app.Flag("providerwire.model-duration", "Maximum ProviderWire model duration.").Default(envDefault(lookupEnv, "GRAFANA_AI_GATEWAY_PROVIDERWIRE_MODEL_DURATION", "120s")).DurationVar(&settings.ProviderWire.ModelDuration)
	app.Flag("providerwire.stream-idle-duration", "Maximum ProviderWire stream idle duration.").Default(envDefault(lookupEnv, "GRAFANA_AI_GATEWAY_PROVIDERWIRE_STREAM_IDLE_DURATION", "30s")).DurationVar(&settings.ProviderWire.StreamIdleDuration)
	app.Flag("providerwire.stream-drain-duration", "ProviderWire stream drain duration.").Default(envDefault(lookupEnv, "GRAFANA_AI_GATEWAY_PROVIDERWIRE_STREAM_DRAIN_DURATION", "1s")).DurationVar(&settings.ProviderWire.StreamDrainDuration)

	if _, err := app.Parse(args); err != nil {
		return Settings{}, fmt.Errorf("parsing settings: %w", err)
	}
	settings.DeploymentMode = DeploymentMode(deploymentMode)
	parsedAudiences, err := parseAudiences(audiences)
	if err != nil {
		return Settings{}, err
	}
	settings.Audiences = parsedAudiences
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
	if strings.TrimSpace(settings.ListenAddress) == "" {
		return fmt.Errorf("config: listen address must not be empty")
	}
	listenHost, err := validateListenAddress(settings.ListenAddress)
	if err != nil {
		return err
	}
	byteLimits := []struct {
		name  string
		value int64
	}{
		{name: "config max bytes", value: settings.ConfigMaxBytes},
		{name: "discovery response bytes", value: settings.DiscoveryResponseBytes},
		{name: "jwks response bytes", value: settings.JWKSResponseBytes},
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
	positiveInts := []struct {
		name  string
		value int
		safe  bool
	}{
		{name: "maximum header bytes", value: settings.MaxHeaderBytes},
		{name: "jwks maximum keys", value: settings.JWKSMaxKeys},
		{name: "providerwire stream parts", value: settings.ProviderWire.StreamParts, safe: true},
	}
	for _, limit := range positiveInts {
		if limit.value <= 0 {
			return fmt.Errorf("config: %s must be positive", limit.name)
		}
		if limit.safe && limit.value == math.MaxInt {
			return fmt.Errorf("config: %s cannot safely use limit+1", limit.name)
		}
	}
	if int64(settings.MaxHeaderBytes) > math.MaxInt64-4096 {
		return fmt.Errorf("config: maximum header bytes cannot safely include parser slop")
	}
	durations := []struct {
		name  string
		value time.Duration
	}{
		{name: "read-header timeout", value: settings.ReadHeaderTimeout},
		{name: "read timeout", value: settings.ReadTimeout},
		{name: "write timeout", value: settings.WriteTimeout},
		{name: "idle timeout", value: settings.IdleTimeout},
		{name: "response grace", value: settings.ResponseGrace},
		{name: "shutdown timeout", value: settings.ShutdownTimeout},
		{name: "jwks request timeout", value: settings.JWKSRequestTimeout},
		{name: "jwks refresh interval", value: settings.JWKSRefreshInterval},
		{name: "jwks maximum age", value: settings.JWKSMaxAge},
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
	if settings.JWKSMaxAge < settings.JWKSRefreshInterval {
		return fmt.Errorf("config: jwks maximum age must be at least refresh interval")
	}
	if settings.AnthropicResponseHeaderTimeout > settings.ProviderWire.ModelDuration {
		return fmt.Errorf("config: anthropic response-header timeout must not exceed model duration")
	}
	if settings.ProviderWire.StreamIdleDuration > settings.ProviderWire.ModelDuration {
		return fmt.Errorf("config: providerwire stream idle duration must not exceed model duration")
	}
	if _, err := providerv4.New(providerv4.Config{Resolver: limitValidationResolver{}, Limits: settings.ProviderWire}); err != nil {
		return fmt.Errorf("config: providerwire limits: %w", err)
	}
	minimum, err := checkedDurationSum(settings.ReadTimeout, settings.JWKSRequestTimeout, settings.ProviderWire.ModelDuration, settings.ResponseGrace)
	if err != nil {
		return err
	}
	if settings.WriteTimeout < minimum {
		return fmt.Errorf("config: write timeout must be at least %s", minimum)
	}
	if settings.AuthUnsafe {
		if settings.DeploymentMode != DeploymentDevelopment {
			return fmt.Errorf("config: unsafe authentication requires development mode")
		}
		if settings.JWKSURL != "" {
			return fmt.Errorf("config: unsafe authentication requires an empty jwks URL")
		}
		if err := validateUnsafeListenHost(listenHost); err != nil {
			return err
		}
	} else if settings.JWKSURL == "" {
		return fmt.Errorf("config: jwks URL is required for safe authentication")
	}
	return nil
}

func validateListenAddress(address string) (string, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil || port == "" {
		return "", fmt.Errorf("config: listen address must use TCP host:port syntax")
	}
	if _, err := strconv.ParseUint(port, 10, 16); err != nil {
		return "", fmt.Errorf("config: listen address must use a numeric TCP port")
	}
	if !validListenHost(host) {
		return "", fmt.Errorf("config: listen address contains an invalid TCP host")
	}
	return host, nil
}

func validListenHost(host string) bool {
	if host == "" || strings.EqualFold(host, "localhost") {
		return true
	}
	address := host
	if before, zone, found := strings.Cut(host, "%"); found {
		if zone == "" || strings.TrimSpace(zone) != zone {
			return false
		}
		address = before
	}
	if net.ParseIP(address) != nil {
		return true
	}
	name := strings.TrimSuffix(host, ".")
	if name == "" || len(name) > 253 {
		return false
	}
	for _, label := range strings.Split(name, ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, character := range label {
			if (character < 'a' || character > 'z') && (character < 'A' || character > 'Z') && (character < '0' || character > '9') && character != '-' {
				return false
			}
		}
	}
	return true
}

func validateUnsafeListenHost(host string) error {
	if strings.EqualFold(host, "localhost") {
		return nil
	}
	ipAddress := host
	if before, _, found := strings.Cut(host, "%"); found {
		ipAddress = before
	}
	ip := net.ParseIP(ipAddress)
	if ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("config: unsafe authentication requires a loopback TCP listen address")
	}
	return nil
}

func envDefault(lookupEnv LookupEnv, name, fallback string) string {
	if value, ok := lookupEnv(name); ok {
		return value
	}
	return fallback
}

func parseAudiences(value string) ([]string, error) {
	parts := strings.Split(value, ",")
	seen := make(map[string]struct{}, len(parts))
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			return nil, fmt.Errorf("config: auth audiences must contain only non-empty values")
		}
		if _, exists := seen[part]; exists {
			return nil, fmt.Errorf("config: auth audiences must be unique")
		}
		seen[part] = struct{}{}
		result = append(result, part)
	}
	return result, nil
}

type limitValidationResolver struct{}

func (limitValidationResolver) ResolveModel(context.Context, string) (catalog.ResolvedModel, error) {
	return catalog.ResolvedModel{}, catalog.ErrUnknownModel
}

func checkedDurationSum(values ...time.Duration) (time.Duration, error) {
	var total time.Duration
	for _, value := range values {
		if value > 0 && total > time.Duration(math.MaxInt64)-value {
			return 0, fmt.Errorf("config: minimum write timeout overflows duration")
		}
		total += value
	}
	return total, nil
}
