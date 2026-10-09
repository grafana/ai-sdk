package config

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadFile_AuthSelection(t *testing.T) {
	for _, tc := range []struct {
		name, yaml string
		valid      bool
	}{
		{"omitted", "", true},
		{"jwt", "auth: {type: jwt, jwt: {jwksURL: https://keys.example/jwks}}", true},
		{"unsafe", "auth: {type: unsafe}", true},
		{"static", "auth: {type: static-key, staticKey: {identities: {deployment: {keyEnv: gateway_key}}}}", true},
		{"binary auth key", "!!binary YXV0aA==: null", false},
		{"root merge null", "<<: {auth: null}", false},
		{"provider merge null", "auth: {type: unsafe, <<: {jwt: null}}", false},
		{"merged selection", "<<: {auth: {type: unsafe}}", false},
		{"boolean env", "auth: {type: static-key, staticKey: {identities: {app: {keyEnv: true}}}}", false},
		{"numeric identity", "auth: {type: static-key, staticKey: {identities: {123: {keyEnv: KEY}}}}", false},
		{"quoted scalar strings", "auth: {type: static-key, staticKey: {identities: {'123': {keyEnv: 'true'}}}}", true},
		{"null", "auth: null", false}, {"bare", "auth:", false}, {"empty", "auth: {}", false},
		{"null jwt", "auth: {type: jwt, jwt: null}", false},
		{"null unused", "auth: {type: unsafe, jwt: null}", false},
		{"null static", "auth: {type: static-key, staticKey: null}", false},
		{"unknown type", "auth: {type: SECRET_SENTINEL}", false},
		{"unknown field", "auth: {type: unsafe, SECRET_SENTINEL: secret}", false},
		{"unselected", "auth: {type: unsafe, jwt: {jwksURL: https://keys.example}}", false},
		{"duplicate", "auth: {type: unsafe, type: SECRET_SENTINEL}", false},
		{"literal key", "auth: {type: static-key, staticKey: {identities: {deployment: {key: SECRET_SENTINEL}}}}", false},
		{"invalid environment", "auth: {type: static-key, staticKey: {identities: {deployment: {keyEnv: SECRET_SENTINEL-invalid}}}}", false},
		{"invalid name", "auth: {type: static-key, staticKey: {identities: {'SECRET_SENTINEL!': {keyEnv: KEY}}}}", false},
		{"empty identities", "auth: {type: static-key, staticKey: {identities: {}}}", false},
		{"quoted off", "server: {cloud: {enabled: 'off'}}", false},
		{"quoted yes", "server: {cloud: {enabled: 'yes'}}", false},
		{"null enabled", "server: {cloud: {enabled: null}}", false},
		{"wrong value", "server: {cloud: {enabled: SECRET_SENTINEL}}", false},
		{"second document", "---\nSECRET_SENTINEL", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			file, err := LoadFile(writeConfigFile(t, minimalConfigYAML+"\n"+tc.yaml+"\n"), 1<<20)
			if tc.valid {
				require.NoError(t, err)
				assert.True(t, file.CloudEnabled())
			} else {
				require.Error(t, err)
				assert.NotContains(t, err.Error(), "SECRET_SENTINEL")
			}
		})
	}
	t.Run("identity bound", func(t *testing.T) {
		identities := map[string]StaticIdentityConfig{}
		for i := 0; i < 65; i++ {
			identities[strings.Repeat("a", i+1)] = StaticIdentityConfig{KeyEnv: "KEY"}
		}
		require.Error(t, (AuthConfig{Type: AuthStaticKey, StaticKey: &StaticKeyConfig{Identities: identities}}).validate())
	})
}

func TestResolveAuthConfig_Selection(t *testing.T) {
	for _, tc := range []struct {
		name     string
		settings Settings
		file     File
		want     AuthType
		invalid  bool
	}{
		{name: "legacy JWT", settings: Settings{JWKSURL: "https://keys.example"}, want: AuthJWT},
		{name: "legacy unsafe", settings: Settings{AuthUnsafe: true}, want: AuthUnsafe},
		{name: "legacy missing", invalid: true},
		{name: "yaml unsafe", file: File{Auth: &AuthConfig{Type: AuthUnsafe}}, want: AuthUnsafe},
		{name: "jwks conflict", settings: Settings{JWKSURL: "https://keys.example"}, file: File{Auth: &AuthConfig{Type: AuthUnsafe}}, invalid: true},
		{name: "unsafe conflict", settings: Settings{AuthUnsafe: true}, file: File{Auth: &AuthConfig{Type: AuthUnsafe}}, invalid: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			auth, err := ResolveAuthConfig(tc.settings, tc.file)
			if tc.invalid {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.want, auth.Type)
			}
		})
	}
}

func TestValidateRuntime_SelectedDependencies(t *testing.T) {
	settings, err := ParseSettings([]string{"--config.file=test.yaml"}, nil)
	require.NoError(t, err)
	settings.CloudListenAddress = "unused invalid"
	settings.JWKSRequestTimeout = -time.Second
	settings.JWKSResponseBytes = -1
	settings.WriteTimeout = 155 * time.Second
	require.NoError(t, settings.ValidateRuntime(EffectiveAuth{Type: AuthStaticKey}, false))
	require.Error(t, settings.ValidateRuntime(EffectiveAuth{Type: AuthStaticKey}, true))
	require.Error(t, settings.ValidateRuntime(EffectiveAuth{Type: AuthJWT, JWKSURL: "https://keys.example"}, false))
	settings.DeploymentMode = DeploymentDevelopment
	settings.PrivateListenAddress = "127.0.0.1:8080"
	settings.OperationalListenAddress = "127.0.0.1:8082"
	require.NoError(t, settings.ValidateRuntime(EffectiveAuth{Type: AuthUnsafe}, false))
	settings.OperationalListenAddress = ":8082"
	require.ErrorContains(t, settings.ValidateRuntime(EffectiveAuth{Type: AuthUnsafe}, false), "loopback")
}

func TestValidateRuntime_ProductionExportMatrix(t *testing.T) {
	for _, authType := range []AuthType{AuthJWT, AuthStaticKey, AuthUnsafe} {
		for _, cloud := range []bool{false, true} {
			for _, enabled := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/cloud=%t/export=%t", authType, cloud, enabled), func(t *testing.T) {
					settings, err := ParseSettings(nil, mapLookup(baseSettingsEnvironment()))
					require.NoError(t, err)
					settings.AgentObservability.Enabled = enabled
					auth := EffectiveAuth{Type: authType, JWKSURL: "https://keys.example"}
					err = settings.ValidateRuntime(auth, cloud)
					valid := authType != AuthUnsafe && (enabled || (authType == AuthStaticKey && !cloud))
					if valid {
						require.NoError(t, err)
					} else {
						require.Error(t, err)
					}
					if enabled {
						settings.AgentObservability.TLS = false
						require.ErrorContains(t, settings.ValidateRuntime(auth, cloud), "TLS")
					}
				})
			}
		}
	}
}

func TestLoadFile_AliasedAuthKey(t *testing.T) {
	data := strings.ReplaceAll(minimalConfigYAML, "anthropic-primary", "auth")
	data = strings.Replace(data, "  auth:", "  &authKey auth:", 1)
	_, err := LoadFile(writeConfigFile(t, data+"\n*authKey: null\n"), 1<<20)
	require.Error(t, err)
}
