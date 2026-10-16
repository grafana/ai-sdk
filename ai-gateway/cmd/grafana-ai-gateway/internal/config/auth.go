package config

import (
	"fmt"
	"regexp"
)

// AuthType selects a built-in private authenticator.
type AuthType string

const (
	AuthJWT       AuthType = "jwt"
	AuthStaticKey AuthType = "static-key"
	AuthUnsafe    AuthType = "unsafe"
)

// AuthConfig selects exactly one private authentication provider.
type AuthConfig struct {
	Type      AuthType         `yaml:"type"`
	JWT       *JWTConfig       `yaml:"jwt,omitempty"`
	StaticKey *StaticKeyConfig `yaml:"staticKey,omitempty"`
}

// JWTConfig identifies the trusted signing keys.
type JWTConfig struct {
	JWKSURL string `yaml:"jwksURL"`
}

// StaticKeyConfig maps attribution names to environment-backed credentials.
type StaticKeyConfig struct {
	Identities map[string]StaticIdentityConfig `yaml:"identities"`
}

// StaticIdentityConfig references a credential without retaining its value.
type StaticIdentityConfig struct {
	KeyEnv string `yaml:"keyEnv"`
}

// ServerConfig controls optional ingress.
type ServerConfig struct {
	Cloud *CloudConfig `yaml:"cloud,omitempty"`
}

// CloudConfig controls the proxy-only Cloud listener.
type CloudConfig struct {
	Enabled *bool `yaml:"enabled,omitempty"`
}

// EffectiveAuth is the validated, secret-free startup authentication selection.
type EffectiveAuth struct {
	Type       AuthType
	JWKSURL    string
	Identities map[string]StaticIdentityConfig
}

var staticIdentityName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)
var staticEnvironmentName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// CloudEnabled defaults omitted Cloud configuration to enabled.
func (file File) CloudEnabled() bool {
	return file.Server == nil || file.Server.Cloud == nil || file.Server.Cloud.Enabled == nil || *file.Server.Cloud.Enabled
}

func (auth AuthConfig) validate() error {
	switch auth.Type {
	case AuthJWT:
		if auth.JWT == nil || auth.JWT.JWKSURL == "" || auth.StaticKey != nil {
			return fmt.Errorf("config: jwt auth requires only jwt.jwksURL configuration")
		}
	case AuthUnsafe:
		if auth.JWT != nil || auth.StaticKey != nil {
			return fmt.Errorf("config: unsafe auth does not accept provider configuration")
		}
	case AuthStaticKey:
		if auth.JWT != nil || auth.StaticKey == nil {
			return fmt.Errorf("config: static-key auth requires only staticKey configuration")
		}
		if len(auth.StaticKey.Identities) < 1 || len(auth.StaticKey.Identities) > 64 {
			return fmt.Errorf("config: static-key auth requires 1-64 identities")
		}
		for name, identity := range auth.StaticKey.Identities {
			if !staticIdentityName.MatchString(name) {
				return fmt.Errorf("config: invalid static identity name")
			}
			if !staticEnvironmentName.MatchString(identity.KeyEnv) {
				return fmt.Errorf("config: invalid static identity environment reference")
			}
		}
	default:
		return fmt.Errorf("config: auth type must be jwt, static-key or unsafe")
	}
	return nil
}

// ResolveAuthConfig reconciles explicit YAML selection with legacy flags.
func ResolveAuthConfig(settings Settings, file File) (EffectiveAuth, error) {
	if file.Auth == nil {
		if settings.AuthUnsafe {
			if settings.JWKSURL != "" {
				return EffectiveAuth{}, fmt.Errorf("config: unsafe authentication requires an empty jwks URL")
			}
			return EffectiveAuth{Type: AuthUnsafe}, nil
		}
		if settings.JWKSURL == "" {
			return EffectiveAuth{}, fmt.Errorf("config: jwks URL is required for safe authentication")
		}
		return EffectiveAuth{Type: AuthJWT, JWKSURL: settings.JWKSURL}, nil
	}
	if settings.AuthUnsafe || settings.JWKSURL != "" {
		return EffectiveAuth{}, fmt.Errorf("config: explicit auth conflicts with legacy authentication settings")
	}
	if err := file.Auth.validate(); err != nil {
		return EffectiveAuth{}, err
	}
	result := EffectiveAuth{Type: file.Auth.Type}
	if file.Auth.JWT != nil {
		result.JWKSURL = file.Auth.JWT.JWKSURL
	}
	if file.Auth.StaticKey != nil {
		result.Identities = make(map[string]StaticIdentityConfig, len(file.Auth.StaticKey.Identities))
		for name, identity := range file.Auth.StaticKey.Identities {
			result.Identities[name] = identity
		}
	}
	return result, nil
}
