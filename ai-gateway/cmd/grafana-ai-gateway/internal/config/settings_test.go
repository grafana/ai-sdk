package config

import (
	"math"
	"testing"
	"time"

	providerv4 "github.com/grafana/ai-sdk/ai-gateway/providerwire/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseSettings_Defaults(t *testing.T) {
	settings, err := ParseSettings(nil, mapLookup(map[string]string{
		"GRAFANA_AI_GATEWAY_CONFIG_FILE":   "/tmp/models.yaml",
		"GRAFANA_AI_GATEWAY_AUTH_JWKS_URL": "https://auth.example/jwks",
	}))
	require.NoError(t, err)
	assert.Equal(t, Settings{
		ConfigFile:                     "/tmp/models.yaml",
		ConfigMaxBytes:                 1_048_576,
		DeploymentMode:                 DeploymentProduction,
		ListenAddress:                  ":8080",
		ReadHeaderTimeout:              5 * time.Second,
		ReadTimeout:                    30 * time.Second,
		WriteTimeout:                   165 * time.Second,
		IdleTimeout:                    120 * time.Second,
		MaxHeaderBytes:                 65_536,
		ResponseGrace:                  5 * time.Second,
		ShutdownTimeout:                15 * time.Second,
		DiscoveryResponseBytes:         1_048_576,
		AuthUnsafe:                     false,
		JWKSURL:                        "https://auth.example/jwks",
		Audiences:                      []string{"ai-sdk"},
		JWKSRequestTimeout:             5 * time.Second,
		JWKSResponseBytes:              1_048_576,
		JWKSMaxKeys:                    128,
		JWKSRefreshInterval:            5 * time.Minute,
		JWKSMaxAge:                     15 * time.Minute,
		AnthropicResponseHeaderTimeout: 10 * time.Second,
		AnthropicResponseBytes:         16_777_216,
		ProviderWire: providerv4.Limits{
			RequestBytes:        1_048_576,
			UnaryResponseBytes:  8_388_608,
			StreamParts:         100_000,
			StreamFrameBytes:    1_048_576,
			ModelDuration:       120 * time.Second,
			StreamIdleDuration:  30 * time.Second,
			StreamDrainDuration: time.Second,
		},
	}, settings)
}

func TestParseSettings_ExactFlagAndEnvironmentBindings(t *testing.T) {
	tests := []struct {
		flag    string
		env     string
		value   string
		prepare func(map[string]string)
		check   func(*testing.T, Settings)
	}{
		{flag: "config.file", env: "GRAFANA_AI_GATEWAY_CONFIG_FILE", value: "/other/models.yaml", check: func(t *testing.T, s Settings) { assert.Equal(t, "/other/models.yaml", s.ConfigFile) }},
		{flag: "config.max-bytes", env: "GRAFANA_AI_GATEWAY_CONFIG_MAX_BYTES", value: "2048", check: func(t *testing.T, s Settings) { assert.Equal(t, int64(2048), s.ConfigMaxBytes) }},
		{flag: "deployment.mode", env: "GRAFANA_AI_GATEWAY_DEPLOYMENT_MODE", value: "development", check: func(t *testing.T, s Settings) { assert.Equal(t, DeploymentDevelopment, s.DeploymentMode) }},
		{flag: "server.listen-address", env: "GRAFANA_AI_GATEWAY_SERVER_LISTEN_ADDRESS", value: "127.0.0.1:9000", check: func(t *testing.T, s Settings) { assert.Equal(t, "127.0.0.1:9000", s.ListenAddress) }},
		{flag: "server.read-header-timeout", env: "GRAFANA_AI_GATEWAY_SERVER_READ_HEADER_TIMEOUT", value: "4s", check: func(t *testing.T, s Settings) { assert.Equal(t, 4*time.Second, s.ReadHeaderTimeout) }},
		{flag: "server.read-timeout", env: "GRAFANA_AI_GATEWAY_SERVER_READ_TIMEOUT", value: "20s", check: func(t *testing.T, s Settings) { assert.Equal(t, 20*time.Second, s.ReadTimeout) }},
		{flag: "server.write-timeout", env: "GRAFANA_AI_GATEWAY_SERVER_WRITE_TIMEOUT", value: "200s", check: func(t *testing.T, s Settings) { assert.Equal(t, 200*time.Second, s.WriteTimeout) }},
		{flag: "server.idle-timeout", env: "GRAFANA_AI_GATEWAY_SERVER_IDLE_TIMEOUT", value: "60s", check: func(t *testing.T, s Settings) { assert.Equal(t, 60*time.Second, s.IdleTimeout) }},
		{flag: "server.max-header-bytes", env: "GRAFANA_AI_GATEWAY_SERVER_MAX_HEADER_BYTES", value: "32768", check: func(t *testing.T, s Settings) { assert.Equal(t, 32768, s.MaxHeaderBytes) }},
		{flag: "server.response-grace", env: "GRAFANA_AI_GATEWAY_SERVER_RESPONSE_GRACE", value: "4s", check: func(t *testing.T, s Settings) { assert.Equal(t, 4*time.Second, s.ResponseGrace) }},
		{flag: "server.shutdown-timeout", env: "GRAFANA_AI_GATEWAY_SERVER_SHUTDOWN_TIMEOUT", value: "10s", check: func(t *testing.T, s Settings) { assert.Equal(t, 10*time.Second, s.ShutdownTimeout) }},
		{flag: "discovery.response-bytes", env: "GRAFANA_AI_GATEWAY_DISCOVERY_RESPONSE_BYTES", value: "2048", check: func(t *testing.T, s Settings) { assert.Equal(t, int64(2048), s.DiscoveryResponseBytes) }},
		{flag: "auth.unsafe", env: "GRAFANA_AI_GATEWAY_AUTH_UNSAFE", value: "true", prepare: unsafeAuthEnvironment, check: func(t *testing.T, s Settings) { assert.True(t, s.AuthUnsafe) }},
		{flag: "auth.jwks-url", env: "GRAFANA_AI_GATEWAY_AUTH_JWKS_URL", value: "https://other.example/jwks", check: func(t *testing.T, s Settings) { assert.Equal(t, "https://other.example/jwks", s.JWKSURL) }},
		{flag: "auth.audiences", env: "GRAFANA_AI_GATEWAY_AUTH_AUDIENCES", value: "one,two", check: func(t *testing.T, s Settings) { assert.Equal(t, []string{"one", "two"}, s.Audiences) }},
		{flag: "auth.jwks-timeout", env: "GRAFANA_AI_GATEWAY_AUTH_JWKS_TIMEOUT", value: "4s", check: func(t *testing.T, s Settings) { assert.Equal(t, 4*time.Second, s.JWKSRequestTimeout) }},
		{flag: "auth.jwks-response-bytes", env: "GRAFANA_AI_GATEWAY_AUTH_JWKS_RESPONSE_BYTES", value: "2048", check: func(t *testing.T, s Settings) { assert.Equal(t, int64(2048), s.JWKSResponseBytes) }},
		{flag: "auth.jwks-max-keys", env: "GRAFANA_AI_GATEWAY_AUTH_JWKS_MAX_KEYS", value: "64", check: func(t *testing.T, s Settings) { assert.Equal(t, 64, s.JWKSMaxKeys) }},
		{flag: "auth.jwks-refresh-interval", env: "GRAFANA_AI_GATEWAY_AUTH_JWKS_REFRESH_INTERVAL", value: "4m", check: func(t *testing.T, s Settings) { assert.Equal(t, 4*time.Minute, s.JWKSRefreshInterval) }},
		{flag: "auth.jwks-max-age", env: "GRAFANA_AI_GATEWAY_AUTH_JWKS_MAX_AGE", value: "20m", check: func(t *testing.T, s Settings) { assert.Equal(t, 20*time.Minute, s.JWKSMaxAge) }},
		{flag: "anthropic.response-header-timeout", env: "GRAFANA_AI_GATEWAY_ANTHROPIC_RESPONSE_HEADER_TIMEOUT", value: "9s", check: func(t *testing.T, s Settings) { assert.Equal(t, 9*time.Second, s.AnthropicResponseHeaderTimeout) }},
		{flag: "anthropic.response-bytes", env: "GRAFANA_AI_GATEWAY_ANTHROPIC_RESPONSE_BYTES", value: "8388608", check: func(t *testing.T, s Settings) { assert.Equal(t, int64(8_388_608), s.AnthropicResponseBytes) }},
		{flag: "providerwire.request-bytes", env: "GRAFANA_AI_GATEWAY_PROVIDERWIRE_REQUEST_BYTES", value: "2048", check: func(t *testing.T, s Settings) { assert.Equal(t, int64(2048), s.ProviderWire.RequestBytes) }},
		{flag: "providerwire.unary-response-bytes", env: "GRAFANA_AI_GATEWAY_PROVIDERWIRE_UNARY_RESPONSE_BYTES", value: "4194304", check: func(t *testing.T, s Settings) { assert.Equal(t, int64(4_194_304), s.ProviderWire.UnaryResponseBytes) }},
		{flag: "providerwire.stream-parts", env: "GRAFANA_AI_GATEWAY_PROVIDERWIRE_STREAM_PARTS", value: "50000", check: func(t *testing.T, s Settings) { assert.Equal(t, 50000, s.ProviderWire.StreamParts) }},
		{flag: "providerwire.stream-frame-bytes", env: "GRAFANA_AI_GATEWAY_PROVIDERWIRE_STREAM_FRAME_BYTES", value: "524288", check: func(t *testing.T, s Settings) { assert.Equal(t, int64(524288), s.ProviderWire.StreamFrameBytes) }},
		{flag: "providerwire.model-duration", env: "GRAFANA_AI_GATEWAY_PROVIDERWIRE_MODEL_DURATION", value: "100s", check: func(t *testing.T, s Settings) { assert.Equal(t, 100*time.Second, s.ProviderWire.ModelDuration) }},
		{flag: "providerwire.stream-idle-duration", env: "GRAFANA_AI_GATEWAY_PROVIDERWIRE_STREAM_IDLE_DURATION", value: "20s", check: func(t *testing.T, s Settings) { assert.Equal(t, 20*time.Second, s.ProviderWire.StreamIdleDuration) }},
		{flag: "providerwire.stream-drain-duration", env: "GRAFANA_AI_GATEWAY_PROVIDERWIRE_STREAM_DRAIN_DURATION", value: "2s", check: func(t *testing.T, s Settings) { assert.Equal(t, 2*time.Second, s.ProviderWire.StreamDrainDuration) }},
	}

	for _, tc := range tests {
		t.Run(tc.flag, func(t *testing.T) {
			for _, source := range []string{"environment", "flag"} {
				t.Run(source, func(t *testing.T) {
					environment := baseSettingsEnvironment()
					if tc.prepare != nil {
						tc.prepare(environment)
					}
					var args []string
					if source == "environment" {
						environment[tc.env] = tc.value
					} else if tc.flag == "auth.unsafe" {
						args = []string{"--auth.unsafe"}
					} else {
						args = []string{"--" + tc.flag + "=" + tc.value}
					}
					settings, err := ParseSettings(args, mapLookup(environment))
					require.NoError(t, err)
					tc.check(t, settings)
				})
			}
		})
	}
}

func TestSettingsValidate_BoundsAndRelationships(t *testing.T) {
	valid, err := ParseSettings(nil, mapLookup(baseSettingsEnvironment()))
	require.NoError(t, err)

	tests := []struct {
		name   string
		mutate func(*Settings)
	}{
		{name: "write budget", mutate: func(s *Settings) { s.WriteTimeout = 159 * time.Second }},
		{name: "write budget overflow", mutate: func(s *Settings) {
			s.ReadTimeout = time.Duration(math.MaxInt64)
			s.WriteTimeout = time.Duration(math.MaxInt64)
		}},
		{name: "jwks age", mutate: func(s *Settings) { s.JWKSMaxAge = s.JWKSRefreshInterval - time.Second }},
		{name: "anthropic header exceeds model", mutate: func(s *Settings) { s.AnthropicResponseHeaderTimeout = s.ProviderWire.ModelDuration + time.Second }},
		{name: "stream idle exceeds model", mutate: func(s *Settings) { s.ProviderWire.StreamIdleDuration = s.ProviderWire.ModelDuration + time.Second }},
		{name: "anthropic scanner bound", mutate: func(s *Settings) { s.AnthropicResponseBytes = 32 << 20 }},
		{name: "byte zero", mutate: func(s *Settings) { s.ConfigMaxBytes = 0 }},
		{name: "byte limit+1 overflow", mutate: func(s *Settings) { s.ConfigMaxBytes = math.MaxInt64 }},
		{name: "integer zero", mutate: func(s *Settings) { s.JWKSMaxKeys = 0 }},
		{name: "integer limit+1 overflow", mutate: func(s *Settings) { s.ProviderWire.StreamParts = math.MaxInt }},
		{name: "header parser slop overflow", mutate: func(s *Settings) { s.MaxHeaderBytes = math.MaxInt }},
		{name: "listen address missing port", mutate: func(s *Settings) { s.ListenAddress = "127.0.0.1" }},
		{name: "listen address nonnumeric port", mutate: func(s *Settings) { s.ListenAddress = "127.0.0.1:http" }},
		{name: "listen address invalid host", mutate: func(s *Settings) { s.ListenAddress = "bad host:8080" }},
		{name: "duration zero", mutate: func(s *Settings) { s.ShutdownTimeout = 0 }},
		{name: "unsafe production", mutate: func(s *Settings) { s.AuthUnsafe = true; s.JWKSURL = "" }},
		{name: "unsafe with jwks", mutate: func(s *Settings) {
			s.DeploymentMode = DeploymentDevelopment
			s.AuthUnsafe = true
			s.ListenAddress = "127.0.0.1:8080"
		}},
		{name: "unsafe wildcard listener", mutate: func(s *Settings) {
			s.DeploymentMode = DeploymentDevelopment
			s.AuthUnsafe = true
			s.JWKSURL = ""
			s.ListenAddress = ":8080"
		}},
		{name: "unsafe non-loopback listener", mutate: func(s *Settings) {
			s.DeploymentMode = DeploymentDevelopment
			s.AuthUnsafe = true
			s.JWKSURL = ""
			s.ListenAddress = "192.0.2.1:8080"
		}},
		{name: "safe without jwks", mutate: func(s *Settings) { s.JWKSURL = "" }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			settings := valid
			tc.mutate(&settings)
			require.Error(t, settings.Validate())
		})
	}
}

func TestParseSettings_RejectsUnknownAndInvalidValues(t *testing.T) {
	environment := mapLookup(baseSettingsEnvironment())
	_, err := ParseSettings([]string{"--unknown=true"}, environment)
	require.Error(t, err)
	_, err = ParseSettings([]string{"--deployment.mode=other"}, environment)
	require.Error(t, err)
	_, err = ParseSettings([]string{"--auth.audiences=one,,two"}, environment)
	require.Error(t, err)
	_, err = ParseSettings([]string{"--auth.audiences=one,one"}, environment)
	require.Error(t, err)

	removed := []struct {
		flag string
		env  string
	}{
		{flag: "providerwire.json-depth", env: "GRAFANA_AI_GATEWAY_PROVIDERWIRE_JSON_DEPTH"},
		{flag: "providerwire.json-tokens", env: "GRAFANA_AI_GATEWAY_PROVIDERWIRE_JSON_TOKENS"},
		{flag: "providerwire.number-bytes", env: "GRAFANA_AI_GATEWAY_PROVIDERWIRE_NUMBER_BYTES"},
		{flag: "providerwire.error-response-bytes", env: "GRAFANA_AI_GATEWAY_PROVIDERWIRE_ERROR_RESPONSE_BYTES"},
	}
	for _, setting := range removed {
		t.Run("removed flag "+setting.flag, func(t *testing.T) {
			_, err := ParseSettings([]string{"--" + setting.flag + "=1"}, environment)
			require.Error(t, err)
		})
		t.Run("removed environment "+setting.flag, func(t *testing.T) {
			values := baseSettingsEnvironment()
			values[setting.env] = "not-a-valid-value"
			_, err := ParseSettings(nil, mapLookup(values))
			require.NoError(t, err)
		})
	}
}

func baseSettingsEnvironment() map[string]string {
	return map[string]string{
		"GRAFANA_AI_GATEWAY_CONFIG_FILE":   "/tmp/models.yaml",
		"GRAFANA_AI_GATEWAY_AUTH_JWKS_URL": "https://auth.example/jwks",
	}
}

func unsafeAuthEnvironment(environment map[string]string) {
	environment["GRAFANA_AI_GATEWAY_DEPLOYMENT_MODE"] = "development"
	environment["GRAFANA_AI_GATEWAY_AUTH_JWKS_URL"] = ""
	environment["GRAFANA_AI_GATEWAY_SERVER_LISTEN_ADDRESS"] = "127.0.0.1:8080"
}

func mapLookup(values map[string]string) LookupEnv {
	return func(name string) (string, bool) {
		value, ok := values[name]
		return value, ok
	}
}
