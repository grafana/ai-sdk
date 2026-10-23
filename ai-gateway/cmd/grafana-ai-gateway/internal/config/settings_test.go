package config

import (
	"testing"
	"time"

	"github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/auth"
	providerv4 "github.com/grafana/ai-sdk/ai-gateway/providerwire/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseSettings_Defaults(t *testing.T) {
	settings, err := ParseSettings(mapLookup(map[string]string{
		"GF_PLUGIN_CONFIG_FILE":        "/tmp/models.yaml",
		"GF_PLUGIN_ACCESS_MODE":        "cloud",
		"GF_PLUGIN_AGENTO11Y_ENABLED":  "true",
		"GF_PLUGIN_AGENTO11Y_ENDPOINT": "collector.example:4317",
	}))
	require.NoError(t, err)
	assert.Equal(t, Settings{
		ConfigFile: "/tmp/models.yaml",
		Access:     auth.AccessPolicy{Mode: auth.AccessModeCloud},
		AgentObservability: AgentObservabilitySettings{
			Enabled:  true,
			Protocol: AgentObservabilityGRPC,
			Endpoint: "collector.example:4317",
			TLS:      true,
		},
		MaxRequestBytes: 1_048_576,
		ProviderWire: providerv4.Limits{
			ModelDuration:       120 * time.Second,
			StreamIdleDuration:  30 * time.Second,
			StreamDrainDuration: time.Second,
		},
	}, settings)
}

func TestParseSettings_EnvironmentBindings(t *testing.T) {
	tests := []struct {
		env   string
		value string
		check func(*testing.T, Settings)
	}{
		{env: "GF_PLUGIN_CONFIG_FILE", value: "/etc/models.yaml", check: func(t *testing.T, s Settings) { assert.Equal(t, "/etc/models.yaml", s.ConfigFile) }},
		{env: "GF_PLUGIN_ACCESS_MODE", value: "self-hosted", check: func(t *testing.T, s Settings) { assert.Equal(t, auth.AccessModeSelfHosted, s.Access.Mode) }},
		{env: "GF_PLUGIN_AGENTO11Y_PROTOCOL", value: "http", check: func(t *testing.T, s Settings) {
			assert.Equal(t, AgentObservabilityHTTP, s.AgentObservability.Protocol)
		}},
		{env: "GF_PLUGIN_AGENTO11Y_ENDPOINT", value: "other.example:4317", check: func(t *testing.T, s Settings) {
			assert.Equal(t, "other.example:4317", s.AgentObservability.Endpoint)
		}},
		{env: "GF_PLUGIN_MAX_REQUEST_BYTES", value: "2048", check: func(t *testing.T, s Settings) { assert.Equal(t, int64(2048), s.MaxRequestBytes) }},
		{env: "GF_PLUGIN_MAX_MODEL_DURATION", value: "45s", check: func(t *testing.T, s Settings) { assert.Equal(t, 45*time.Second, s.ProviderWire.ModelDuration) }},
	}
	for _, tc := range tests {
		t.Run(tc.env, func(t *testing.T) {
			environment := baseSettingsEnvironment()
			environment[tc.env] = tc.value
			if tc.env == "GF_PLUGIN_ACCESS_MODE" {
				delete(environment, "GF_PLUGIN_ACCESS_INTERNAL_SERVICES")
			}
			if tc.env == "GF_PLUGIN_AGENTO11Y_PROTOCOL" {
				environment["GF_PLUGIN_AGENTO11Y_ENDPOINT"] = "https://collector.example/export"
			}
			settings, err := ParseSettings(mapLookup(environment))
			require.NoError(t, err)
			tc.check(t, settings)
		})
	}
}

func TestParseSettings_InvalidValuesNameTheSetting(t *testing.T) {
	for _, tc := range []struct{ env, value string }{
		{env: "GF_PLUGIN_MAX_REQUEST_BYTES", value: "private-value"},
		{env: "GF_PLUGIN_MAX_MODEL_DURATION", value: "private-value"},
		{env: "GF_PLUGIN_AGENTO11Y_ENABLED", value: "private-value"},
		{env: "GF_PLUGIN_AGENTO11Y_TLS", value: "private-value"},
	} {
		t.Run(tc.env, func(t *testing.T) {
			environment := baseSettingsEnvironment()
			environment[tc.env] = tc.value
			_, err := ParseSettings(mapLookup(environment))
			require.EqualError(t, err, "config: "+tc.env+" has an invalid value")
		})
	}
}

func TestParseSettings_EmptyValuesSelectDefaults(t *testing.T) {
	environment := baseSettingsEnvironment()
	environment["GF_PLUGIN_MAX_REQUEST_BYTES"] = ""
	settings, err := ParseSettings(mapLookup(environment))
	require.NoError(t, err)
	assert.Equal(t, int64(defaultRequestBytes), settings.MaxRequestBytes)
}

func TestSettingsValidate_BoundsAndRelationships(t *testing.T) {
	valid, err := ParseSettings(mapLookup(baseSettingsEnvironment()))
	require.NoError(t, err)

	tests := []struct {
		name   string
		mutate func(*Settings)
	}{
		{name: "config file", mutate: func(s *Settings) { s.ConfigFile = " " }},
		{name: "model duration below provider and idle timeouts", mutate: func(s *Settings) { s.ProviderWire.ModelDuration = 29 * time.Second }},
		{name: "request bytes zero", mutate: func(s *Settings) { s.MaxRequestBytes = 0 }},
		{name: "request bytes negative", mutate: func(s *Settings) { s.MaxRequestBytes = -1 }},
		{name: "agento11y protocol", mutate: func(s *Settings) { s.AgentObservability.Protocol = "other" }},
		{name: "agento11y endpoint", mutate: func(s *Settings) { s.AgentObservability.Endpoint = "" }},
		{name: "agento11y endpoint credentials", mutate: func(s *Settings) {
			s.AgentObservability.Protocol = AgentObservabilityHTTP
			s.AgentObservability.Endpoint = "https://user:secret@collector.example/export"
		}},
		{name: "agento11y cleartext with a TLS endpoint", mutate: func(s *Settings) {
			s.AgentObservability.Protocol = AgentObservabilityHTTP
			s.AgentObservability.Endpoint = "https://collector.example/export"
			s.AgentObservability.TLS = false
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			settings := valid
			tc.mutate(&settings)
			require.Error(t, settings.Validate())
		})
	}
}

func TestParseSettings_IgnoresRemovedSettings(t *testing.T) {
	values := baseSettingsEnvironment()
	for _, name := range []string{
		"GRAFANA_AI_GATEWAY_CONFIG_FILE", "GRAFANA_AI_GATEWAY_ACCESS_MODE",
		"GF_PLUGIN_CONFIG_MAX_BYTES", "GF_PLUGIN_AGENTO11Y_QUEUE_SIZE", "GF_PLUGIN_PROVIDERWIRE_STREAM_PARTS",
		"GF_PLUGIN_OBSERVATION_REGION",
	} {
		values[name] = "not-a-valid-value"
	}
	_, err := ParseSettings(mapLookup(values))
	require.NoError(t, err)
}

func TestAgentObservabilitySettings_Disabled(t *testing.T) {
	environment := baseSettingsEnvironment()
	environment["GF_PLUGIN_AGENTO11Y_ENABLED"] = "false"
	delete(environment, "GF_PLUGIN_AGENTO11Y_ENDPOINT")

	settings, err := ParseSettings(mapLookup(environment))
	require.NoError(t, err)
	assert.False(t, settings.AgentObservability.Enabled)
	secret, err := settings.AgentObservability.ResolveAuthSecret(nil)
	require.NoError(t, err)
	assert.Empty(t, secret)
}

func TestAgentObservabilitySettings_ResolveAuthSecretOnceAndKeepFailuresSecretFree(t *testing.T) {
	settings, err := ParseSettings(mapLookup(baseSettingsEnvironment()))
	require.NoError(t, err)

	calls := 0
	secret, err := settings.AgentObservability.ResolveAuthSecret(func(name string) (string, bool) {
		calls++
		assert.Equal(t, "GF_PLUGIN_AGENTO11Y_TOKEN", name)
		return "private-bearer-value", true
	})
	require.NoError(t, err)
	assert.Equal(t, "private-bearer-value", secret)
	assert.Equal(t, 1, calls)

	for _, test := range []struct {
		name   string
		lookup LookupEnv
	}{
		{name: "nil lookup"},
		{name: "unset", lookup: func(string) (string, bool) { return "", false }},
		{name: "empty", lookup: func(string) (string, bool) { return "", true }},
		{name: "blank", lookup: func(string) (string, bool) { return "  ", true }},
		{name: "surrounding whitespace", lookup: func(string) (string, bool) { return " private-bearer-value ", true }},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := settings.AgentObservability.ResolveAuthSecret(test.lookup)
			require.Error(t, err)
			assert.NotContains(t, err.Error(), "private-bearer-value")
		})
	}
}

func TestAgentObservabilitySettings_EndpointTransportMatrix(t *testing.T) {
	tests := []struct {
		name     string
		protocol AgentObservabilityProtocol
		endpoint string
		tls      bool
		valid    bool
	}{
		{name: "grpc tls host", protocol: AgentObservabilityGRPC, endpoint: "collector.example:4317", tls: true, valid: true},
		{name: "grpc tls URL", protocol: AgentObservabilityGRPC, endpoint: "https://collector.example:4317", tls: true, valid: true},
		{name: "grpc cleartext development", protocol: AgentObservabilityGRPC, endpoint: "collector.example:4317", valid: true},
		{name: "grpc path", protocol: AgentObservabilityGRPC, endpoint: "https://collector.example:4317/export", tls: true},
		{name: "grpc raw credentials", protocol: AgentObservabilityGRPC, endpoint: "user@collector.example:4317", tls: true},
		{name: "http tls", protocol: AgentObservabilityHTTP, endpoint: "https://collector.example/api/v1/generations:export", tls: true, valid: true},
		{name: "http cleartext development", protocol: AgentObservabilityHTTP, endpoint: "http://collector.example/export", valid: true},
		{name: "http missing scheme", protocol: AgentObservabilityHTTP, endpoint: "collector.example:8080/export"},
		{name: "scheme mismatch secure", protocol: AgentObservabilityHTTP, endpoint: "http://collector.example/export", tls: true},
		{name: "scheme mismatch cleartext", protocol: AgentObservabilityHTTP, endpoint: "https://collector.example/export"},
		{name: "URL credentials", protocol: AgentObservabilityHTTP, endpoint: "https://user:private@collector.example/export", tls: true},
		{name: "URL query", protocol: AgentObservabilityHTTP, endpoint: "https://collector.example/export?private=value", tls: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateAgentObservabilityEndpoint(test.protocol, test.endpoint, test.tls)
			if test.valid {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.NotContains(t, err.Error(), "private")
		})
	}
}

func TestParseSettings_QueriesOnlyExplicitEnvironmentBindings(t *testing.T) {
	queried := make(map[string]struct{})
	values := baseSettingsEnvironment()
	_, err := ParseSettings(func(name string) (string, bool) {
		queried[name] = struct{}{}
		value, ok := values[name]
		return value, ok
	})
	require.NoError(t, err)
	for _, forbidden := range []string{"SIGIL_ENDPOINT", "SIGIL_PROTOCOL", "OTEL_EXPORTER_OTLP_ENDPOINT", "AWS_REGION", "GOOGLE_CLOUD_REGION"} {
		_, ok := queried[forbidden]
		assert.False(t, ok, forbidden)
	}
}

func TestAgentObservabilitySettings_RejectsEveryAmbientSDKEnvironmentKey(t *testing.T) {
	settings, err := ParseSettings(mapLookup(baseSettingsEnvironment()))
	require.NoError(t, err)
	for _, name := range ambientAgentObservabilityEnvironment {
		t.Run(name, func(t *testing.T) {
			calls := 0
			err := settings.AgentObservability.ValidateAmbientEnvironment(func(candidate string) (string, bool) {
				calls++
				if candidate == name {
					return "private-ambient-value", true
				}
				return "", false
			})
			require.Error(t, err)
			assert.LessOrEqual(t, calls, len(ambientAgentObservabilityEnvironment))
			assert.NotContains(t, err.Error(), name)
			assert.NotContains(t, err.Error(), "private-ambient-value")
		})
	}
	require.NoError(t, settings.AgentObservability.ValidateAmbientEnvironment(func(string) (string, bool) { return "", false }))

	disabled := settings.AgentObservability
	disabled.Enabled = false
	calls := 0
	require.NoError(t, disabled.ValidateAmbientEnvironment(func(string) (string, bool) {
		calls++
		return "private-ambient-value", true
	}))
	assert.Zero(t, calls)
}

func TestAgentObservabilitySettings_RejectsNewSDKEnvironmentKeys(t *testing.T) {
	settings, err := ParseSettings(mapLookup(baseSettingsEnvironment()))
	require.NoError(t, err)
	for _, name := range []string{
		"AGENTO11Y_EXPORT_TIMEOUT_MS", "SIGIL_EXPORT_TIMEOUT_MS",
		"AGENTO11Y_MAX_RETRIES", "SIGIL_MAX_RETRIES",
		"AGENTO11Y_MAX_BACKOFF_MS", "SIGIL_MAX_BACKOFF_MS",
		"AGENTO11Y_QUEUE_SIZE", "SIGIL_QUEUE_SIZE",
		"AGENTO11Y_ENABLE_EXPERIMENTAL_FEATURES", "AGENTO11Y_USE_EXPERIMENTAL_OTEL", "SIGIL_USE_EXPERIMENTAL_OTEL",
	} {
		t.Run(name, func(t *testing.T) {
			err := settings.AgentObservability.ValidateAmbientEnvironment(mapLookup(map[string]string{name: "private-invalid-value"}))
			require.EqualError(t, err, "config: ambient agent observability SDK environment is not allowed")
		})
	}
}

func TestParseSettings_AccessPolicy(t *testing.T) {
	tests := []struct {
		name     string
		mode     string
		services string
		want     auth.AccessPolicy
		wantErr  string
	}{
		{name: "cloud without allowlist", mode: "cloud", want: auth.AccessPolicy{Mode: auth.AccessModeCloud}},
		{name: "cloud allowlist", mode: "cloud", services: "svc-a,other-app-operator", want: auth.AccessPolicy{Mode: auth.AccessModeCloud, InternalServices: []string{"svc-a", "other-app-operator"}}},
		{name: "self-hosted", mode: "self-hosted", want: auth.AccessPolicy{Mode: auth.AccessModeSelfHosted}},
		{name: "missing mode", wantErr: "access mode is required"},
		{name: "unknown mode", mode: "hybrid", wantErr: "cloud or self-hosted"},
		{name: "mode case", mode: "Cloud", wantErr: "cloud or self-hosted"},
		{name: "allowlist in self-hosted", mode: "self-hosted", services: "svc-a", wantErr: "requires cloud access mode"},
		{name: "surrounding whitespace is trimmed", mode: "cloud", services: " svc-a , svc-b ", want: auth.AccessPolicy{Mode: auth.AccessModeCloud, InternalServices: []string{"svc-a", "svc-b"}}},
		{name: "empty entry", mode: "cloud", services: "svc-a,,svc-b", wantErr: "must not be empty"},
		{name: "trailing comma", mode: "cloud", services: "svc-a,", wantErr: "must not be empty"},
		{name: "blank entry", mode: "cloud", services: "svc-a, ", wantErr: "must not be empty"},
		{name: "duplicate after trimming", mode: "cloud", services: "svc-a, svc-a", wantErr: "unique"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			environment := baseSettingsEnvironment()
			environment["GF_PLUGIN_ACCESS_MODE"] = tc.mode
			environment["GF_PLUGIN_ACCESS_INTERNAL_SERVICES"] = tc.services
			settings, err := ParseSettings(mapLookup(environment))
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
				assert.NotContains(t, err.Error(), "svc-a")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, settings.Access)
		})
	}
}

func TestParseSettings_AgentObservabilityTransportSecurityIsExplicit(t *testing.T) {
	environment := baseSettingsEnvironment()
	environment["GF_PLUGIN_AGENTO11Y_TLS"] = "false"
	settings, err := ParseSettings(mapLookup(environment))
	require.NoError(t, err)
	assert.False(t, settings.AgentObservability.TLS)
}

func baseSettingsEnvironment() map[string]string {
	return map[string]string{
		"GF_PLUGIN_CONFIG_FILE":        "/tmp/models.yaml",
		"GF_PLUGIN_ACCESS_MODE":        "cloud",
		"GF_PLUGIN_AGENTO11Y_ENABLED":  "true",
		"GF_PLUGIN_AGENTO11Y_ENDPOINT": "collector.example:4317",
	}
}

func mapLookup(values map[string]string) LookupEnv {
	return func(name string) (string, bool) {
		value, ok := values[name]
		return value, ok
	}
}
