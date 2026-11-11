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
	"github.com/grafana/ai-sdk/ai-gateway/pkg/extension/tenanttype"
	"github.com/grafana/ai-sdk/schema"
)

var _ simple.KindValidator = Validator{}

const sentinel = "sentinel-tenant-config-value"

const accessSchema = `{"type":"object","properties":{"perModel":{"type":"object","additionalProperties":{"type":"object","properties":{"maxTokens":{"type":"integer","minimum":1}},"additionalProperties":false}},"rules":{"type":"array","items":{"type":"object","properties":{"models":{"type":"array","items":{"type":"string","maxLength":20}},"limits":{"type":"object","properties":{"rpm":{"type":"integer","minimum":1}},"additionalProperties":false}},"required":["models"],"additionalProperties":false}},"note":{"type":"string","maxLength":40}},"additionalProperties":false}`

func bind(t *testing.T, name string, scope extension.Scope, configSchema string) extension.Definition {
	t.Helper()
	tenantType := tenanttype.Type{Name: name}
	if configSchema != "" {
		compiled, err := schema.SchemaFromJSON(json.RawMessage(configSchema))
		require.NoError(t, err)
		tenantType.ConfigSchema = compiled
	}
	definition := tenantType.Bind(tenanttype.Implementation{FailurePolicy: extension.FailSkip, New: func(json.RawMessage, extension.Dependencies) (any, error) { return nil, nil }})
	definition.Scope = scope
	return definition
}

func testValidator(t *testing.T) Validator {
	t.Helper()
	return NewValidator([]extension.Definition{
		bind(t, "model-access", extension.ScopeTenant, accessSchema),
		bind(t, "marker", extension.ScopeTenant, ""),
		bind(t, "quota", extension.ScopeOperator, `{"type":"object","properties":{"limit":{"type":"integer"}},"additionalProperties":false}`),
	})
}

// object returns a Middleware named name whose spec is the JSON spec.
func object(name, spec string) *Middleware {
	middleware := &Middleware{RawSpec: json.RawMessage(spec)}
	middleware.Name = name
	return middleware
}

func access(enabled bool, member string) *Middleware {
	spec := `{"type":"model-access","enabled":` + map[bool]string{true: "true", false: "false"}[enabled]
	if member != "" {
		spec += `,"modelAccess":` + member
	}
	return object("model-access", spec+"}")
}

func TestValidator_Validate(t *testing.T) {
	valid := `{"rules":[{"models":["balanced"],"limits":{"rpm":9007199254740993}}],"note":"Answer concisely."}`
	tightened := `{"note":"` + strings.Repeat("x", 41) + `"}`
	oversized := `{"note":"` + strings.Repeat(sentinel, MaxMemberBytes/len(sentinel)+1) + `"}`
	markupAtTheBound := `{"note":"` + strings.Repeat("<", MaxMemberBytes-len(`{"note":""}`)) + `"}`
	tests := []struct {
		name    string
		action  resource.AdmissionAction
		object  resource.Object
		old     resource.Object
		wantErr string
	}{
		{name: "create enabled", action: resource.AdmissionActionCreate, object: access(true, valid)},
		{name: "create disabled", action: resource.AdmissionActionCreate, object: access(false, valid)},
		{name: "create disabled without a member", action: resource.AdmissionActionCreate, object: access(false, "")},
		{name: "create without configuration", action: resource.AdmissionActionCreate, object: object("marker", `{"type":"marker","enabled":true}`)},
		{name: "name differs from type", action: resource.AdmissionActionCreate, object: object(sentinel, `{"type":"model-access","enabled":true,"modelAccess":{}}`), wantErr: "middleware: metadata.name must equal spec.type"},
		{name: "name differs from type on update", action: resource.AdmissionActionUpdate, object: object("model-access", `{"type":"marker","enabled":false}`), old: access(true, valid), wantErr: "middleware: metadata.name must equal spec.type"},
		{name: "member of another type", action: resource.AdmissionActionCreate, object: object("model-access", `{"type":"model-access","enabled":true,"marker":{"v":"`+sentinel+`"}}`), wantErr: "middleware: spec may set only spec.modelAccess, the member of spec.type"},
		{name: "two members", action: resource.AdmissionActionCreate, object: object("model-access", `{"type":"model-access","enabled":true,"modelAccess":{},"`+sentinel+`":{}}`), wantErr: "middleware: spec may set only spec.modelAccess, the member of spec.type"},
		{name: "undecodable spec", action: resource.AdmissionActionCreate, object: object("model-access", `{"type":"model-access","type":"marker","enabled":true}`), wantErr: "middleware: spec is not a valid Middleware spec"},
		{name: "enabled without its member", action: resource.AdmissionActionCreate, object: access(true, ""), wantErr: "middleware: spec.modelAccess is required when spec.enabled is true"},
		{name: "member for a type without configuration", action: resource.AdmissionActionCreate, object: object("marker", `{"type":"marker","enabled":true,"marker":{}}`), wantErr: `middleware: spec.type "marker" takes no configuration member`},
		{name: "unregistered type", action: resource.AdmissionActionCreate, object: object("unknown", `{"type":"unknown","enabled":true,"unknown":{"v":"`+sentinel+`"}}`), wantErr: `middleware: spec.type "unknown" is not a tenant extension of this Gateway`},
		{name: "operator type", action: resource.AdmissionActionCreate, object: object("quota", `{"type":"quota","enabled":true,"quota":{"limit":1}}`), wantErr: `middleware: spec.type "quota" is not a tenant extension of this Gateway`},
		{name: "wrong value type", action: resource.AdmissionActionCreate, object: access(true, `{"note":1}`), wantErr: "middleware: spec.modelAccess does not match the extension's schema at /note"},
		{name: "member not an object", action: resource.AdmissionActionCreate, object: access(true, `"`+sentinel+`"`), wantErr: "middleware: spec.modelAccess does not match the extension's schema at /"},
		{name: "null member", action: resource.AdmissionActionCreate, object: access(true, `null`), wantErr: "middleware: spec.modelAccess does not match the extension's schema at /"},
		{name: "constraint the manifest drops", action: resource.AdmissionActionCreate, object: access(true, `{"rules":[{"models":["`+sentinel+sentinel+`"]}]}`), wantErr: "middleware: spec.modelAccess does not match the extension's schema at /rules/0/models/0"},
		{name: "nested location", action: resource.AdmissionActionCreate, object: access(true, `{"rules":[{"models":[],"limits":{"rpm":0}}]}`), wantErr: "middleware: spec.modelAccess does not match the extension's schema at /rules/0/limits/rpm"},
		{name: "missing nested required field", action: resource.AdmissionActionCreate, object: access(true, `{"rules":[{}]}`), wantErr: "middleware: spec.modelAccess does not match the extension's schema at /rules/0"},
		{name: "undeclared key", action: resource.AdmissionActionCreate, object: access(true, `{"`+sentinel+`":"`+sentinel+`"}`), wantErr: "middleware: spec.modelAccess does not match the extension's schema at /"},
		{name: "typed map key", action: resource.AdmissionActionCreate, object: access(true, `{"perModel":{"`+sentinel+`":{"maxTokens":0}}}`), wantErr: "middleware: spec.modelAccess does not match the extension's schema at /perModel/*/maxTokens"},
		{name: "oversized member", action: resource.AdmissionActionCreate, object: access(true, oversized), wantErr: "middleware: spec.modelAccess exceeds 65536 bytes"},
		{name: "markup is measured unescaped", action: resource.AdmissionActionCreate, object: access(true, markupAtTheBound), wantErr: "middleware: spec.modelAccess does not match the extension's schema at /note"},
		{name: "update of an implemented type", action: resource.AdmissionActionUpdate, object: access(true, `{"note":"Other."}`), old: access(true, valid)},
		{name: "update failing the schema", action: resource.AdmissionActionUpdate, object: access(true, `{"note":1}`), old: access(true, valid), wantErr: "middleware: spec.modelAccess does not match the extension's schema at /note"},
		{name: "disabling an unregistered type", action: resource.AdmissionActionUpdate, object: object("unknown", `{"type":"unknown","enabled":false,"unknown":{"v":"`+sentinel+`"}}`), old: object("unknown", `{"type":"unknown","enabled":true,"unknown":{"v":"`+sentinel+`"}}`)},
		{name: "disabling with the member removed", action: resource.AdmissionActionUpdate, object: object("unknown", `{"type":"unknown","enabled":false}`), old: object("unknown", `{"type":"unknown","enabled":true,"unknown":{"v":1}}`)},
		{name: "disabling with the member reformatted", action: resource.AdmissionActionUpdate, object: object("unknown", `{"type":"unknown","enabled":false,"unknown":{ "b":9007199254740993, "a":[1] }}`), old: object("unknown", `{"type":"unknown","enabled":true,"unknown":{"a":[1],"b":9007199254740993}}`)},
		{name: "disabling an operator type", action: resource.AdmissionActionUpdate, object: object("quota", `{"type":"quota","enabled":false}`), old: object("quota", `{"type":"quota","enabled":true}`)},
		{name: "disabling a configuration a tightened schema refuses", action: resource.AdmissionActionUpdate, object: access(false, tightened), old: access(true, tightened)},
		{name: "disabling an oversized member", action: resource.AdmissionActionUpdate, object: access(false, oversized), old: access(true, oversized)},
		{name: "disabling a stored spec that can't be decoded", action: resource.AdmissionActionUpdate, object: access(false, ""), old: object("model-access", `{"type":`)},
		{name: "keeping an unregistered type enabled", action: resource.AdmissionActionUpdate, object: object("unknown", `{"type":"unknown","enabled":true,"unknown":{"v":1}}`), old: object("unknown", `{"type":"unknown","enabled":true,"unknown":{"v":1}}`), wantErr: `middleware: spec.type "unknown" is not a tenant extension of this Gateway; an update may only set spec.enabled to false and keep or remove spec.unknown`},
		{name: "reconfiguring an unregistered type", action: resource.AdmissionActionUpdate, object: object("unknown", `{"type":"unknown","enabled":false,"unknown":{"v":"`+sentinel+`"}}`), old: object("unknown", `{"type":"unknown","enabled":true,"unknown":{"v":1}}`), wantErr: `middleware: spec.type "unknown" is not a tenant extension of this Gateway; an update may only set spec.enabled to false and keep or remove spec.unknown`},
		{name: "changing a member that number precision would hide", action: resource.AdmissionActionUpdate, object: object("unknown", `{"type":"unknown","enabled":false,"unknown":{"v":9007199254740993}}`), old: object("unknown", `{"type":"unknown","enabled":true,"unknown":{"v":9007199254740992}}`), wantErr: "is not a tenant extension of this Gateway"},
		{name: "changing a tightened configuration while disabling", action: resource.AdmissionActionUpdate, object: access(false, `{"note":"`+strings.Repeat("y", 41)+`"}`), old: access(true, tightened), wantErr: "middleware: spec.modelAccess does not match the extension's schema at /note"},
		{name: "wrong object", action: resource.AdmissionActionCreate, object: &resource.UntypedObject{}, wantErr: "middleware: the admission object is not a Middleware"},
		{name: "wrong stored object", action: resource.AdmissionActionUpdate, object: access(true, valid), old: &resource.UntypedObject{}, wantErr: "middleware: the stored object is not a Middleware"},
		{name: "delete", action: resource.AdmissionActionDelete, old: object("unknown", `{}`)},
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
			assert.Contains(t, err.Error(), tc.wantErr)
			assert.NotContains(t, err.Error(), sentinel)
		})
	}
}

func TestNewValidator_WithoutDefinitions(t *testing.T) {
	err := NewValidator(nil).Validate(context.Background(), &app.AdmissionRequest{Action: resource.AdmissionActionCreate, Object: access(true, `{}`)})
	assert.EqualError(t, err, `middleware: spec.type "model-access" is not a tenant extension of this Gateway`)
}
