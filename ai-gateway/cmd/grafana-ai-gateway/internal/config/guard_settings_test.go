package config

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func guardEnvironment() map[string]string {
	env := baseSettingsEnvironment()
	env["GRAFANA_AI_GATEWAY_GUARDS_ENABLED"] = "true"
	env["GRAFANA_AI_GATEWAY_GUARDS_ENDPOINT"] = "https://sigil.example/prefix"
	env["GRAFANA_AI_GATEWAY_GUARDS_TENANT_ID"] = "operator-tenant"
	env["GRAFANA_AI_GATEWAY_GUARDS_AUTH_MODE"] = "basic"
	env["GRAFANA_AI_GATEWAY_GUARDS_AUTH_USERNAME"] = "instance-id"
	env["GRAFANA_AI_GATEWAY_GUARDS_AUTH_SECRET_ENV"] = "GUARD_SECRET"
	return env
}

func TestGuardSettingsDefaultsAndDisablement(t *testing.T) {
	settings, err := ParseSettings(nil, mapLookup(baseSettingsEnvironment()))
	require.NoError(t, err)
	assert.Equal(t, GuardSettings{Timeout: 5 * time.Second, BodyBytes: 4 << 20, RetainedBytes: 8 << 20, MaxConcurrent: 8}, settings.Guards)
	settings.Guards = GuardSettings{Endpoint: "invalid", AuthMode: "invalid", Timeout: -1, BodyBytes: math.MaxInt64, MaxConcurrent: -1}
	require.NoError(t, settings.Validate())
	secret, err := settings.Guards.ResolveAuthSecret(func(string) (string, bool) {
		require.FailNow(t, "disabled settings looked up a secret")
		return "", false
	})
	require.NoError(t, err)
	assert.Empty(t, secret)
}

func TestGuardSettingsBindingsAndFlagPrecedence(t *testing.T) {
	tests := []struct {
		flag, value string
		check       func(*testing.T, GuardSettings)
	}{
		{"enabled", "false", func(t *testing.T, g GuardSettings) { assert.False(t, g.Enabled) }},
		{"endpoint", "https://other.example/base/", func(t *testing.T, g GuardSettings) { assert.Equal(t, "https://other.example/base/", g.Endpoint) }},
		{"tenant-id", "other-tenant", func(t *testing.T, g GuardSettings) { assert.Equal(t, "other-tenant", g.TenantID) }},
		{"auth-mode", "bearer", func(t *testing.T, g GuardSettings) { assert.Equal(t, GuardAuthModeBearer, g.AuthMode) }},
		{"auth-username", "other-instance", func(t *testing.T, g GuardSettings) { assert.Equal(t, "other-instance", g.AuthUsername) }},
		{"auth-secret-env", "OTHER_SECRET", func(t *testing.T, g GuardSettings) { assert.Equal(t, "OTHER_SECRET", g.AuthSecretEnv) }},
		{"fail-open", "true", func(t *testing.T, g GuardSettings) { assert.True(t, g.FailOpen) }},
		{"timeout", "3s", func(t *testing.T, g GuardSettings) { assert.Equal(t, 3*time.Second, g.Timeout) }},
		{"body-bytes", "2048", func(t *testing.T, g GuardSettings) { assert.Equal(t, int64(2048), g.BodyBytes) }},
		{"retained-bytes", "4096", func(t *testing.T, g GuardSettings) { assert.Equal(t, int64(4096), g.RetainedBytes) }},
		{"max-concurrent", "4", func(t *testing.T, g GuardSettings) { assert.Equal(t, 4, g.MaxConcurrent) }},
	}
	for _, tc := range tests {
		t.Run(tc.flag, func(t *testing.T) {
			for _, source := range []string{"environment", "flag"} {
				t.Run(source, func(t *testing.T) {
					env := guardEnvironment()
					if tc.flag == "auth-mode" {
						delete(env, "GRAFANA_AI_GATEWAY_GUARDS_AUTH_USERNAME")
					}
					key := "GRAFANA_AI_GATEWAY_GUARDS_" + strings.ToUpper(strings.ReplaceAll(tc.flag, "-", "_"))
					var args []string
					if source == "environment" {
						env[key] = tc.value
					} else {
						args = []string{"--guards." + tc.flag + "=" + tc.value}
						if tc.flag == "enabled" {
							args = []string{"--no-guards.enabled"}
						}
						if tc.flag == "fail-open" {
							env[key] = "false"
							args = []string{"--guards.fail-open"}
						}
					}
					settings, err := ParseSettings(args, mapLookup(env))
					require.NoError(t, err)
					tc.check(t, settings.Guards)
				})
			}
		})
	}
	settings, err := ParseSettings(nil, mapLookup(guardEnvironment()))
	require.NoError(t, err)
	assert.False(t, settings.Guards.FailOpen)
}

func TestGuardSettingsValidation(t *testing.T) {
	valid, err := ParseSettings(nil, mapLookup(guardEnvironment()))
	require.NoError(t, err)
	tests := []struct {
		name   string
		mutate func(*Settings)
	}{
		{"endpoint required", func(s *Settings) { s.Guards.Endpoint = "" }},
		{"relative", func(s *Settings) { s.Guards.Endpoint = "/prefix" }},
		{"opaque", func(s *Settings) { s.Guards.Endpoint = "https:opaque" }},
		{"no host", func(s *Settings) { s.Guards.Endpoint = "https:///prefix" }},
		{"credentials", func(s *Settings) { s.Guards.Endpoint = "https://user:private@sigil.example" }},
		{"query", func(s *Settings) { s.Guards.Endpoint += "?private=value" }},
		{"forced query", func(s *Settings) { s.Guards.Endpoint += "?" }},
		{"fragment", func(s *Settings) { s.Guards.Endpoint += "#private" }},
		{"surrounding whitespace", func(s *Settings) { s.Guards.Endpoint += " " }},
		{"production HTTP", func(s *Settings) { s.Guards.Endpoint = "http://sigil.example" }},
		{"unsupported scheme", func(s *Settings) { s.DeploymentMode = DeploymentDevelopment; s.Guards.Endpoint = "ftp://sigil.example" }},
		{"tenant required", func(s *Settings) { s.Guards.TenantID = "" }},
		{"tenant whitespace", func(s *Settings) { s.Guards.TenantID = " tenant" }},
		{"tenant control", func(s *Settings) { s.Guards.TenantID = "tenant\r\nprivate" }},
		{"auth required", func(s *Settings) { s.Guards.AuthMode = "" }},
		{"unknown auth", func(s *Settings) { s.Guards.AuthMode = "automatic" }},
		{"basic username required", func(s *Settings) { s.Guards.AuthUsername = "" }},
		{"basic colon", func(s *Settings) { s.Guards.AuthUsername = "user:private" }},
		{"basic control", func(s *Settings) { s.Guards.AuthUsername = "user\tprivate" }},
		{"basic whitespace", func(s *Settings) { s.Guards.AuthUsername = " user" }},
		{"bearer username", func(s *Settings) { s.Guards.AuthMode = GuardAuthModeBearer }},
		{"secret reference required", func(s *Settings) { s.Guards.AuthSecretEnv = "" }},
		{"secret reference invalid", func(s *Settings) { s.Guards.AuthSecretEnv = "literal-private" }},
		{"timeout zero", func(s *Settings) { s.Guards.Timeout = 0 }},
		{"timeout negative", func(s *Settings) { s.Guards.Timeout = -time.Second }},
		{"timeout submillisecond", func(s *Settings) { s.Guards.Timeout = time.Millisecond - time.Nanosecond }},
		{"timeout over hook max", func(s *Settings) { s.Guards.Timeout = 119999*time.Millisecond + time.Nanosecond }},
		{"timeout fallback boundary", func(s *Settings) { s.Guards.Timeout = 120 * time.Second }},
		{"timeout exceeds operation", func(s *Settings) {
			s.Guards.Timeout = 2 * time.Second
			s.ProviderWire.ModelDuration = time.Second
		}},
		{"body zero", func(s *Settings) { s.Guards.BodyBytes = 0 }},
		{"body negative", func(s *Settings) { s.Guards.BodyBytes = -1 }},
		{"body too large", func(s *Settings) { s.Guards.BodyBytes = 4<<20 + 1 }},
		{"body overflow", func(s *Settings) { s.Guards.BodyBytes = math.MaxInt64 }},
		{"retained zero", func(s *Settings) { s.Guards.RetainedBytes = 0 }},
		{"retained too large", func(s *Settings) { s.Guards.RetainedBytes = 64<<20 + 1 }},
		{"retained overflow", func(s *Settings) { s.Guards.RetainedBytes = math.MaxInt64 }},
		{"concurrency zero", func(s *Settings) { s.Guards.MaxConcurrent = 0 }},
		{"concurrency too large", func(s *Settings) { s.Guards.MaxConcurrent = 129 }},
		{"concurrency overflow", func(s *Settings) { s.Guards.MaxConcurrent = math.MaxInt }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := valid
			tc.mutate(&s)
			err := s.Validate()
			require.Error(t, err)
			assert.NotContains(t, err.Error(), "private")
		})
	}
	for _, timeout := range []time.Duration{time.Millisecond, 119999 * time.Millisecond} {
		s := valid
		s.Guards.Timeout = timeout
		require.NoError(t, s.Validate())
	}
	s := valid
	s.Guards.BodyBytes = 4 << 20
	s.Guards.RetainedBytes = 64 << 20
	s.Guards.MaxConcurrent = 128
	require.NoError(t, s.Validate())
	s.DeploymentMode = DeploymentDevelopment
	s.Guards.Endpoint = "http://127.0.0.1:8080/prefix"
	require.NoError(t, s.Validate())
	s.Guards.AuthMode = GuardAuthModeBearer
	s.Guards.AuthUsername = ""
	require.NoError(t, s.Validate())
}

func TestGuardSettingsResolveAuthSecret(t *testing.T) {
	settings, err := ParseSettings(nil, mapLookup(guardEnvironment()))
	require.NoError(t, err)
	calls := 0
	secret, err := settings.Guards.ResolveAuthSecret(func(name string) (string, bool) {
		calls++
		assert.Equal(t, "GUARD_SECRET", name)
		return "private-token", true
	})
	require.NoError(t, err)
	assert.Equal(t, "private-token", secret)
	assert.Equal(t, 1, calls)
	_, err = settings.Guards.ResolveAuthSecret(nil)
	require.EqualError(t, err, "config: guards auth secret is unavailable")
	_, err = settings.Guards.ResolveAuthSecret(func(string) (string, bool) { return "private", false })
	require.EqualError(t, err, "config: guards auth secret is unavailable")
	for _, value := range []string{"", " ", " private", "private ", "private\r\n", "pri\tvate", "pri\x00vate", "pri\x7fvate", "pri\u0085vate", "pri\xffvate"} {
		t.Run(value, func(t *testing.T) {
			_, err := settings.Guards.ResolveAuthSecret(func(string) (string, bool) { return value, true })
			require.EqualError(t, err, "config: guards auth secret is invalid")
		})
	}
}
