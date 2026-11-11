package tenantconfig

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/grafana/grafana-app-sdk/app"
	"github.com/grafana/grafana-app-sdk/resource"
	"github.com/grafana/grafana-app-sdk/simple"

	"github.com/grafana/ai-sdk/ai-gateway/pkg/extension"
	middlewarev1alpha1 "github.com/grafana/ai-sdk/ai-gateway/pkg/internal/middleware/v1alpha1"
	"github.com/grafana/ai-sdk/schema"
)

var _ simple.KindValidator = Validator{}

const sentinel = "sentinel-tenant-config-value"

func definition(t *testing.T, name string, scope extension.Scope, configSchema string) extension.Definition {
	t.Helper()
	definition := extension.Definition{Name: name, Scope: scope, FailurePolicy: extension.FailSkip, New: func(json.RawMessage, extension.Dependencies) (any, error) { return nil, nil }}
	if configSchema != "" {
		compiled, err := schema.SchemaFromJSON(json.RawMessage(configSchema))
		require.NoError(t, err)
		definition.ConfigSchema = compiled
	}
	return definition
}

const instructionsSchema = `{"type":"object","properties":{"instructions":{"type":"string","maxLength":40},"stop":{"type":"array","items":{"type":"string"}}},"additionalProperties":false}`

func testValidator(t *testing.T) Validator {
	t.Helper()
	return NewValidator([]extension.Definition{
		definition(t, "instructions", extension.ScopeTenant, instructionsSchema),
		definition(t, "marker", extension.ScopeTenant, ""),
		definition(t, "quota", extension.ScopeOperator, `{"type":"object"}`),
	}, true)
}

func middleware(name, extensionType string, enabled bool, config map[string]any) *middlewarev1alpha1.Middleware {
	object := middlewarev1alpha1.NewMiddleware()
	object.Name = name
	object.Spec = middlewarev1alpha1.Spec{Type: extensionType, Enabled: enabled, Config: config}
	return object
}

func named(extensionType string, enabled bool, config map[string]any) *middlewarev1alpha1.Middleware {
	return middleware(extensionType, extensionType, enabled, config)
}

func TestValidator_Validate(t *testing.T) {
	valid := map[string]any{"instructions": "Answer concisely.", "stop": []any{"END"}}
	tightened := map[string]any{"instructions": strings.Repeat("x", 41)}
	oversized := map[string]any{"instructions": strings.Repeat(sentinel, MaxConfigBytes/len(sentinel)+1)}
	markupAtTheBound := map[string]any{"instructions": strings.Repeat("<", MaxConfigBytes-len(`{"instructions":""}`))}
	tests := []struct {
		name    string
		action  resource.AdmissionAction
		object  resource.Object
		old     resource.Object
		wantErr string
	}{
		{name: "create enabled", action: resource.AdmissionActionCreate, object: named("instructions", true, valid)},
		{name: "create disabled", action: resource.AdmissionActionCreate, object: named("instructions", false, valid)},
		{name: "create without config", action: resource.AdmissionActionCreate, object: named("instructions", true, nil)},
		{name: "create without config for a schema-less extension", action: resource.AdmissionActionCreate, object: named("marker", true, nil)},
		{name: "name differs from type", action: resource.AdmissionActionCreate, object: middleware(sentinel, "instructions", true, valid), wantErr: "middleware: metadata.name must equal spec.type"},
		{name: "unregistered type", action: resource.AdmissionActionCreate, object: named("unknown", true, map[string]any{"value": sentinel}), wantErr: `middleware: spec.type "unknown" is not a tenant extension of this Gateway`},
		{name: "operator type", action: resource.AdmissionActionCreate, object: named("quota", true, nil), wantErr: `middleware: spec.type "quota" is not a tenant extension of this Gateway`},
		{name: "wrong value type", action: resource.AdmissionActionCreate, object: named("instructions", true, map[string]any{"instructions": 1}), wantErr: "middleware: spec.config does not match the extension's schema at /instructions"},
		{name: "value over the schema bound", action: resource.AdmissionActionCreate, object: named("instructions", true, map[string]any{"instructions": sentinel + sentinel}), wantErr: "middleware: spec.config does not match the extension's schema at /instructions"},
		{name: "array item", action: resource.AdmissionActionCreate, object: named("instructions", true, map[string]any{"stop": []any{"a", 1}}), wantErr: "middleware: spec.config does not match the extension's schema at /stop/1"},
		{name: "undeclared key", action: resource.AdmissionActionCreate, object: named("instructions", true, map[string]any{sentinel: sentinel}), wantErr: "middleware: spec.config does not match the extension's schema at /"},
		{name: "config for a schema-less extension", action: resource.AdmissionActionCreate, object: named("marker", true, map[string]any{"value": sentinel}), wantErr: "middleware: spec.config is not accepted: the extension takes no configuration"},
		{name: "oversized config", action: resource.AdmissionActionCreate, object: named("instructions", true, oversized), wantErr: "middleware: spec.config exceeds 65536 bytes"},
		{name: "markup is measured unescaped", action: resource.AdmissionActionCreate, object: named("marker", true, markupAtTheBound), wantErr: "middleware: spec.config is not accepted: the extension takes no configuration"},
		{name: "name differs from type on update", action: resource.AdmissionActionUpdate, object: middleware("instructions", "other", false, valid), old: named("instructions", true, valid), wantErr: "middleware: metadata.name must equal spec.type"},
		{name: "update of a registered type", action: resource.AdmissionActionUpdate, object: named("instructions", true, map[string]any{"instructions": "Other."}), old: named("instructions", true, valid)},
		{name: "update failing the schema", action: resource.AdmissionActionUpdate, object: named("instructions", true, map[string]any{"instructions": 1}), old: named("instructions", true, valid), wantErr: "middleware: spec.config does not match the extension's schema at /instructions"},
		{name: "disabling an unregistered type", action: resource.AdmissionActionUpdate, object: named("unknown", false, map[string]any{"value": sentinel}), old: named("unknown", true, map[string]any{"value": sentinel})},
		{name: "disabling without config", action: resource.AdmissionActionUpdate, object: named("unknown", false, map[string]any{}), old: named("unknown", true, nil)},
		{name: "disabling an operator type", action: resource.AdmissionActionUpdate, object: named("quota", false, nil), old: named("quota", true, nil)},
		{name: "disabling a configuration a tightened schema refuses", action: resource.AdmissionActionUpdate, object: named("instructions", false, tightened), old: named("instructions", true, tightened)},
		{name: "disabling an oversized configuration", action: resource.AdmissionActionUpdate, object: named("instructions", false, oversized), old: named("instructions", true, oversized)},
		{name: "keeping an unregistered type enabled", action: resource.AdmissionActionUpdate, object: named("unknown", true, map[string]any{"value": sentinel}), old: named("unknown", true, map[string]any{"value": sentinel}), wantErr: `middleware: spec.type "unknown" is not a tenant extension of this Gateway; an update may only set spec.enabled to false and keep spec.config`},
		{name: "reconfiguring an unregistered type", action: resource.AdmissionActionUpdate, object: named("unknown", false, map[string]any{"value": "other"}), old: named("unknown", true, map[string]any{"value": sentinel}), wantErr: `middleware: spec.type "unknown" is not a tenant extension of this Gateway; an update may only set spec.enabled to false and keep spec.config`},
		{name: "changing a tightened configuration while disabling", action: resource.AdmissionActionUpdate, object: named("instructions", false, map[string]any{"instructions": strings.Repeat("y", 41)}), old: named("instructions", true, tightened), wantErr: "middleware: spec.config does not match the extension's schema at /instructions"},
		{name: "wrong object", action: resource.AdmissionActionCreate, object: &resource.UntypedObject{}, wantErr: "middleware: the admission object is not a Middleware"},
		{name: "wrong stored object", action: resource.AdmissionActionUpdate, object: named("instructions", true, valid), old: &resource.UntypedObject{}, wantErr: "middleware: the stored object is not a Middleware"},
		{name: "delete", action: resource.AdmissionActionDelete, old: named("unknown", true, nil)},
		{name: "connect", action: resource.AdmissionActionConnect},
	}
	validator := testValidator(t)
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validator.Validate(context.Background(), &app.AdmissionRequest{Action: tc.action, Object: tc.object, OldObject: tc.old})
			if tc.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.True(t, apierrors.IsBadRequest(err), "refusals are 400: %v", err)
			assert.EqualError(t, err, tc.wantErr)
			assert.NotContains(t, err.Error(), sentinel)
		})
	}
}

func TestNewValidator_WithoutDefinitions(t *testing.T) {
	err := NewValidator(nil, true).Validate(context.Background(), &app.AdmissionRequest{Action: resource.AdmissionActionCreate, Object: named("instructions", true, nil)})
	assert.EqualError(t, err, `middleware: spec.type "instructions" is not a tenant extension of this Gateway`)
}

func TestValidator_WithoutAPIAccess(t *testing.T) {
	validator := NewValidator([]extension.Definition{definition(t, "instructions", extension.ScopeTenant, instructionsSchema)}, false)
	stored := named("instructions", true, map[string]any{"instructions": "Be brief."})
	for _, tc := range []struct {
		name    string
		request *app.AdmissionRequest
		wantErr string
	}{
		{name: "valid create", request: &app.AdmissionRequest{Action: resource.AdmissionActionCreate, Object: named("instructions", true, map[string]any{"instructions": "Be brief."})}, wantErr: noAPIAccessReason},
		{name: "disable-only update", request: &app.AdmissionRequest{Action: resource.AdmissionActionUpdate, Object: named("instructions", false, map[string]any{"instructions": "Be brief."}), OldObject: stored}, wantErr: noAPIAccessReason},
		{name: "invalid create", request: &app.AdmissionRequest{Action: resource.AdmissionActionCreate, Object: middleware("a", "b", true, nil)}, wantErr: noAPIAccessReason},
		{name: "delete", request: &app.AdmissionRequest{Action: resource.AdmissionActionDelete, OldObject: stored}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := validator.Validate(context.Background(), tc.request)
			if tc.wantErr == "" {
				require.NoError(t, err)
				return
			}
			assert.True(t, apierrors.IsBadRequest(err))
			assert.EqualError(t, err, tc.wantErr)
		})
	}
}
