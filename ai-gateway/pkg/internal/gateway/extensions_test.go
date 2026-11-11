package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/ai-sdk/ai-gateway/pkg/extension"
	"github.com/grafana/ai-sdk/ai-gateway/pkg/extension/tenanttype"
	"github.com/grafana/ai-sdk/ai-gateway/test/platformhost"
	"github.com/grafana/ai-sdk/provider"
	"github.com/grafana/ai-sdk/schema"
)

const (
	extensionSecretVariable = "QUOTA_SECRET"
	extensionSecret         = "extension-secret-value"
	permissionDocument      = `{"error":{"message":"forbidden","type":"forbidden","param":null,"code":"forbidden"}}`
	unavailableDocument     = `{"error":{"message":"service unavailable","type":"internal_server_error","param":null,"code":"unavailable"}}`
)

type limitConfig struct {
	Limit int `json:"limit" jsonschema:"minimum=1"`
}

// gatewayExtension implements the phases its hooks set.
type gatewayExtension struct {
	admit   func() error
	inspect func(extension.Part) error
	closed  *atomic.Bool
}

func (e gatewayExtension) Admit(context.Context, *extension.Call) error {
	if e.admit != nil {
		return e.admit()
	}
	return nil
}

func (e gatewayExtension) InspectResponse(_ context.Context, _ *extension.Call, part extension.Part) error {
	if e.inspect != nil {
		return e.inspect(part)
	}
	return nil
}

func (e gatewayExtension) Close(context.Context) error {
	if e.closed != nil {
		e.closed.Store(true)
	}
	return nil
}

type headerTransform struct{}

func (headerTransform) TransformRequest(_ context.Context, _ *extension.Call, params provider.CallOptions) (provider.CallOptions, error) {
	params.Headers = map[string]string{"Authorization": "Bearer injected-private"}
	return params, nil
}

func gatewayDefinition(name string, value any, change ...func(*extension.Definition)) extension.Definition {
	definition := extension.Definition{
		Name:             name,
		Scope:            extension.ScopeOperator,
		FailurePolicy:    extension.FailClosed,
		EnabledByDefault: true,
		New:              func(json.RawMessage, extension.Dependencies) (any, error) { return value, nil },
	}
	for _, apply := range change {
		apply(&definition)
	}
	return definition
}

func quotaWithSecret(t *testing.T, received *string) extension.Definition {
	t.Helper()
	configSchema, err := schema.SchemaFor[limitConfig]()
	require.NoError(t, err)
	return extension.Definition{
		Name:          "quota",
		Scope:         extension.ScopeOperator,
		FailurePolicy: extension.FailClosed,
		ConfigSchema:  configSchema,
		Secrets:       []string{"apiKey"},
		New: func(_ json.RawMessage, deps extension.Dependencies) (any, error) {
			if received != nil {
				*received = deps.Secrets["apiKey"]
			}
			return gatewayExtension{}, nil
		},
	}
}

func TestNew_ExtensionStartup(t *testing.T) {
	validSetting := "quota: {config: {limit: 5}, apiKeyEnv: " + extensionSecretVariable + "}"
	instructions := gatewayDefinition("instructions", gatewayExtension{}, func(d *extension.Definition) { d.Scope = extension.ScopeTenant })
	for _, tc := range []struct {
		name        string
		extensions  func(t *testing.T) []extension.Definition
		mandatory   []string
		setting     string
		idle        bool
		secretReads bool
		message     string
	}{
		{name: "duplicate names on an idle host", idle: true, extensions: func(t *testing.T) []extension.Definition {
			return []extension.Definition{gatewayDefinition("audit", gatewayExtension{}), gatewayDefinition("audit", gatewayExtension{})}
		}, message: `extension name "audit" is registered twice`},
		{name: "mandatory tenant extension", extensions: func(*testing.T) []extension.Definition { return []extension.Definition{instructions} }, mandatory: []string{"instructions"}, message: `mandatory extension "instructions" must have operator scope`},
		{name: "unknown mandatory extension", extensions: func(*testing.T) []extension.Definition { return nil }, mandatory: []string{"quota"}, message: `mandatory extension "quota" is not registered`},
		{name: "mandatory unconfigured", extensions: func(t *testing.T) []extension.Definition { return []extension.Definition{quotaWithSecret(t, nil)} }, mandatory: []string{"quota"}, message: "GF_PLUGIN_EXTENSIONS.quota is mandatory"},
		{name: "mandatory disabled", extensions: func(t *testing.T) []extension.Definition { return []extension.Definition{quotaWithSecret(t, nil)} }, mandatory: []string{"quota"}, setting: "quota: {enabled: false, apiKeyEnv: " + extensionSecretVariable + "}", message: "GF_PLUGIN_EXTENSIONS.quota is mandatory"},
		{name: "mandatory invalid", extensions: func(t *testing.T) []extension.Definition { return []extension.Definition{quotaWithSecret(t, nil)} }, mandatory: []string{"quota"}, setting: "quota: {config: {limit: config-private}, apiKeyEnv: " + extensionSecretVariable + "}", message: "GF_PLUGIN_EXTENSIONS.quota.config does not match the extension's schema at /limit"},
		{name: "unknown extension", extensions: func(*testing.T) []extension.Definition { return nil }, setting: "quota: {}", message: "GF_PLUGIN_EXTENSIONS.quota is not a registered extension"},
		{name: "tenant extension configured", extensions: func(*testing.T) []extension.Definition { return []extension.Definition{instructions} }, setting: "instructions: {}", message: "GF_PLUGIN_EXTENSIONS.instructions is a tenant extension"},
		{name: "malformed setting", extensions: func(*testing.T) []extension.Definition { return nil }, setting: "- config-private", message: "GF_PLUGIN_EXTENSIONS must be a mapping"},
		{name: "missing secret variable", secretReads: true, extensions: func(t *testing.T) []extension.Definition { return []extension.Definition{quotaWithSecret(t, nil)} }, setting: strings.Replace(validSetting, extensionSecretVariable, "QUOTA_UNSET", 1), message: `GF_PLUGIN_EXTENSIONS.quota.apiKeyEnv "QUOTA_UNSET" is unset or empty`},
		{name: "factory failure", secretReads: true, extensions: func(t *testing.T) []extension.Definition {
			failing := quotaWithSecret(t, nil)
			failing.New = func(json.RawMessage, extension.Dependencies) (any, error) { return nil, errors.New(extensionSecret) }
			return []extension.Definition{failing}
		}, mandatory: []string{"quota"}, setting: validSetting, message: `constructing extension "quota" failed`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := newStartupEnvironment(t, "https://provider.example")
			env.values[extensionSecretVariable] = extensionSecret
			if tc.idle {
				delete(env.values, "GRAFANA_JWKS")
			}
			if tc.setting != "" {
				env.values["GF_PLUGIN_EXTENSIONS"] = tc.setting
			}
			_, logs, err := env.startWith(t, tc.extensions(t), tc.mandatory)
			require.ErrorContains(t, err, tc.message)
			if !tc.secretReads {
				assert.Empty(t, env.secrets)
			}
			for _, private := range []string{extensionSecret, providerSecret, "config-private"} {
				assert.NotContains(t, err.Error(), private)
				assert.NotContains(t, logs, private)
			}
		})
	}

	t.Run("tenant extension without a public type on an idle host", func(t *testing.T) {
		env := newStartupEnvironment(t, "https://provider.example")
		delete(env.values, "GRAFANA_JWKS")
		_, err := New(Options{LookupEnv: env.lookup, Extensions: []extension.Definition{instructions}})
		require.ErrorContains(t, err, `tenant extension "instructions" has no public tenant type`)
		bound := tenanttype.Type{Name: "instructions"}.Bind(tenanttype.Implementation{FailurePolicy: extension.FailClosed, New: instructions.New})
		gw, err := New(Options{LookupEnv: env.lookup, Extensions: []extension.Definition{bound}, TenantTypes: []tenanttype.Type{{Name: "instructions"}}})
		require.NoError(t, err)
		assert.True(t, gw.Idle())
	})

	t.Run("idle host does not read the setting", func(t *testing.T) {
		env := newStartupEnvironment(t, "https://provider.example")
		delete(env.values, "GRAFANA_JWKS")
		env.values["GF_PLUGIN_EXTENSIONS"] = "- invalid"
		gw, _, err := env.startWith(t, []extension.Definition{quotaWithSecret(t, nil)}, []string{"quota"})
		require.NoError(t, err)
		assert.True(t, gw.Idle())
		assert.Empty(t, env.secrets)
	})

	t.Run("mandatory extension configured", func(t *testing.T) {
		env := newStartupEnvironment(t, "https://provider.example")
		env.values[extensionSecretVariable] = extensionSecret
		env.values["GF_PLUGIN_EXTENSIONS"] = validSetting
		var received string
		gw, logs, err := env.startWith(t, []extension.Definition{quotaWithSecret(t, &received)}, []string{"quota"})
		require.NoError(t, err)
		assert.False(t, gw.Idle())
		assert.Equal(t, extensionSecret, received)
		assert.Equal(t, 1, env.secrets[extensionSecretVariable])
		assert.NotContains(t, logs, extensionSecret)
	})

	t.Run("close joins the final flush", func(t *testing.T) {
		env := newStartupEnvironment(t, "https://provider.example")
		var closed atomic.Bool
		gw, _, err := env.startWith(t, []extension.Definition{gatewayDefinition("audit", gatewayExtension{closed: &closed})}, nil)
		require.NoError(t, err)
		started := time.Now()
		gw.Close()
		assert.True(t, closed.Load())
		assert.Less(t, time.Since(started), finalFlushBudget)
	})
}

func TestGateway_Extensions(t *testing.T) {
	newExtensionHarness := func(t *testing.T, env map[string]string, definitions ...extension.Definition) (*harness, credential) {
		t.Helper()
		h := newHarnessWithOptions(t, platformhost.TopologyCloud, harnessOptions{fallback: true, extensions: definitions, env: env})
		return h, accessToken(h.serviceToken(t, testInternalService))
	}
	denyText := func(text string) func(extension.Part) error {
		return func(part extension.Part) error {
			if part.Type == extension.PartText && strings.Contains(part.Text, text) {
				return extension.ErrDenied
			}
			return nil
		}
	}

	t.Run("admit denial makes no provider call and is not retried", func(t *testing.T) {
		h, internal := newExtensionHarness(t, nil, gatewayDefinition("quota", gatewayExtension{admit: func() error { return extension.ErrDenied }}))
		for _, streaming := range []bool{false, true} {
			status, body := do(t, languageRequest(t, context.Background(), h.url("language-model"), unaryBody, streaming, internal))
			assert.Equal(t, http.StatusForbidden, status)
			assert.JSONEq(t, permissionDocument, body)
		}
		assert.Zero(t, h.anthropic.requests.Load())
		metrics := h.metrics(t)
		assert.Contains(t, metrics, `status="denied"`)
		assert.NotContains(t, metrics, `status="error"`)
		assert.Contains(t, metrics, `grafana_ai_gateway_extension_events_total{event="denied",extension="quota",phase="admit"} 2`)
		assert.Contains(t, h.logs.String(), `"ai_sdk.outcome":"denied"`)
	})

	t.Run("output denial after fallback carries the overview", func(t *testing.T) {
		h, internal := newExtensionHarness(t, nil, gatewayDefinition("guard", gatewayExtension{inspect: denyText("hello")}))
		h.anthropic.failModel.Store("backend-private")
		status, body := do(t, languageRequest(t, context.Background(), h.url("language-model"), unaryBody, false, internal))
		assert.Equal(t, http.StatusForbidden, status)
		assert.NotContains(t, body, "hello")
		var document struct {
			Error            json.RawMessage `json:"error"`
			ProviderMetadata struct {
				Gateway struct {
					Execution struct {
						Attempts []struct {
							Outcome string `json:"outcome"`
						} `json:"attempts"`
					} `json:"execution"`
				} `json:"gateway"`
			} `json:"providerMetadata"`
		}
		require.NoError(t, json.Unmarshal([]byte(body), &document))
		assert.JSONEq(t, `{"message":"forbidden","type":"forbidden","param":null,"code":"forbidden"}`, string(document.Error))
		require.Len(t, document.ProviderMetadata.Gateway.Execution.Attempts, 2)
		assert.Equal(t, "failed", document.ProviderMetadata.Gateway.Execution.Attempts[0].Outcome)
		assert.Equal(t, "selected", document.ProviderMetadata.Gateway.Execution.Attempts[1].Outcome)
		assert.Equal(t, int32(2), h.anthropic.requests.Load())
	})

	t.Run("stream abort after commitment", func(t *testing.T) {
		h, internal := newExtensionHarness(t, nil, gatewayDefinition("guard", gatewayExtension{inspect: denyText("second")}))
		status, body := do(t, languageRequest(t, context.Background(), h.url("language-model"), unaryBody, true, internal))
		require.Equal(t, http.StatusOK, status)
		var types []string
		for _, frame := range strings.Split(strings.TrimSpace(body), "\n\n") {
			var event struct {
				Type  string `json:"type"`
				Delta string `json:"delta"`
				Error struct {
					Code string `json:"code"`
				} `json:"error"`
			}
			require.NoError(t, json.Unmarshal([]byte(strings.TrimPrefix(frame, "data: ")), &event), frame)
			types = append(types, event.Type)
			if event.Type == "error" {
				assert.Equal(t, "forbidden", event.Error.Code)
			}
		}
		require.NotEmpty(t, types)
		assert.Equal(t, "stream-start", types[0])
		assert.Equal(t, []string{"text-start", "text-delta", "error"}, types[len(types)-3:])
		assert.NotContains(t, body, "second")
		require.EventuallyWithT(t, func(collect *assert.CollectT) {
			assert.Contains(collect, h.logs.String(), `"ai_sdk.event":"aisdk.model.stream.error"`)
		}, time.Second, time.Millisecond)
		assert.Contains(t, h.logs.String(), `"ai_sdk.outcome":"denied"`)
		metrics := h.metrics(t)
		assert.Contains(t, metrics, `operation="stream",provider="grafana",status="denied"`)
		assert.NotContains(t, metrics, `status="error"`)
		assert.Contains(t, metrics, `grafana_ai_gateway_extension_events_total{event="denied",extension="guard",phase="inspect-response"} 1`)
	})

	t.Run("protected header from a transform is refused before the provider", func(t *testing.T) {
		h, internal := newExtensionHarness(t, nil, gatewayDefinition("headers", headerTransform{}))
		status, body := do(t, languageRequest(t, context.Background(), h.url("language-model"), unaryBody, false, internal))
		assert.Equal(t, http.StatusServiceUnavailable, status)
		assert.JSONEq(t, unavailableDocument, body)
		assert.Zero(t, h.anthropic.requests.Load())
		assert.NotContains(t, h.logs.String(), "injected-private")
	})

	t.Run("extension secrets are redacted from failure summaries", func(t *testing.T) {
		h, internal := newExtensionHarness(t, map[string]string{"GF_PLUGIN_EXTENSIONS": "quota: {config: {limit: 1}, apiKeyEnv: " + extensionSecretVariable + "}", extensionSecretVariable: extensionSecret}, quotaWithSecret(t, nil))
		h.anthropic.failModel.Store("backend-private")
		h.anthropic.failMessage.Store(extensionSecret)
		status, body := do(t, languageRequest(t, context.Background(), h.url("language-model"), unaryBody, false, internal))
		assert.Equal(t, http.StatusOK, status, body)
		assert.Contains(t, body, "[REDACTED]")
		assert.NotContains(t, body, extensionSecret)
	})
}

// typesOf returns public tenant types the tenant-scope definitions bind to.
// A type with a schema gets the empty example, which its schema must accept.
func typesOf(definitions []extension.Definition) []tenanttype.Type {
	var types []tenanttype.Type
	for _, definition := range definitions {
		if definition.Scope != extension.ScopeTenant {
			continue
		}
		tenantType := tenanttype.Type{Name: definition.Name, ConfigSchema: definition.ConfigSchema, EnabledByDefault: definition.EnabledByDefault}
		if definition.ConfigSchema.JSON() != nil {
			tenantType.Example = json.RawMessage(`{}`)
		}
		types = append(types, tenantType)
	}
	return types
}
