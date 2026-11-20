package stackcatalog

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/grafana/grafana-app-sdk/app"
	"github.com/grafana/grafana-app-sdk/resource"
	"github.com/grafana/grafana-app-sdk/simple"

	"github.com/grafana/ai-sdk/ai-gateway/pkg/internal/modelcatalog/v1alpha1"
)

var _ simple.KindValidator = Validator{}

const sentinel = "sentinel-plaintext-provider-key"

func created() resource.InlineSecureValue {
	return resource.InlineSecureValue{Create: resource.NewSecretValue(sentinel)}
}

func named(name string) resource.InlineSecureValue { return resource.InlineSecureValue{Name: name} }

func removed() resource.InlineSecureValue { return resource.InlineSecureValue{Remove: true} }

func catalog(spec v1alpha1.Spec, secure resource.InlineSecureValues) *v1alpha1.ModelCatalog {
	object := v1alpha1.NewModelCatalog()
	object.Spec = spec
	object.Secure = secure
	return object
}

func storedCatalog() *v1alpha1.ModelCatalog {
	return catalog(testSpec(), resource.InlineSecureValues{"anthropic": named("inline-anthropic"), "openai": named("inline-openai")})
}

func withoutOpenAI() v1alpha1.Spec {
	spec := testSpec()
	delete(spec.Providers, "openai")
	model := spec.Models["balanced"]
	model.Fallback = nil
	spec.Models["balanced"] = model
	return spec
}

func TestValidator_Blocklist(t *testing.T) {
	tests := []struct {
		name     string
		action   resource.AdmissionAction
		provider string
		id       string
		enabled  bool
		conflict bool
	}{
		{name: "create with primary conflict", action: resource.AdmissionActionCreate, provider: "anthropic", id: "claude-sonnet-4-6", enabled: true, conflict: true},
		{name: "update with fallback conflict while off", action: resource.AdmissionActionUpdate, provider: "openai", id: "gpt-5", conflict: true},
		{name: "create with future blocklist while off", action: resource.AdmissionActionCreate, provider: "openai", id: "unlisted"},
		{name: "update with saved key", action: resource.AdmissionActionUpdate, provider: "openai", id: "unlisted", enabled: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			spec := testSpec()
			provider := spec.Providers[tc.provider]
			provider.ServeAllModels = new(tc.enabled)
			provider.ModelBlocklist = []string{tc.id}
			spec.Providers[tc.provider] = provider
			var keys resource.InlineSecureValues
			if tc.action == resource.AdmissionActionCreate {
				keys = resource.InlineSecureValues{"anthropic": created(), "openai": created()}
			}
			err := Validator{}.Validate(t.Context(), &app.AdmissionRequest{Action: tc.action, Object: catalog(spec, keys), OldObject: storedCatalog()})
			if !tc.conflict {
				require.NoError(t, err)
				return
			}
			require.EqualError(t, err, "modelcatalog: providers."+tc.provider+".modelBlocklist[0] conflicts with a listed target")
			assert.True(t, apierrors.IsBadRequest(err))
		})
	}
}

func TestValidator_Validate(t *testing.T) {
	spec := testSpec()
	empty := *v1alpha1.NewSpec()
	providerOnly := v1alpha1.Spec{Providers: spec.Providers, Models: empty.Models}
	keys := resource.InlineSecureValues{"anthropic": created(), "openai": created()}
	tests := []struct {
		name    string
		action  resource.AdmissionAction
		object  resource.Object
		old     resource.Object
		wantErr string
	}{
		{name: "create empty", action: resource.AdmissionActionCreate, object: catalog(empty, nil)},
		{name: "create without provider map", action: resource.AdmissionActionCreate, object: catalog(v1alpha1.Spec{Models: empty.Models}, nil), wantErr: "modelcatalog: spec.providers must be a map"},
		{name: "update without model map", action: resource.AdmissionActionUpdate, object: catalog(v1alpha1.Spec{Providers: spec.Providers}, nil), old: storedCatalog(), wantErr: "modelcatalog: spec.models must be a map"},
		{name: "create with providers only", action: resource.AdmissionActionCreate, object: catalog(providerOnly, keys)},
		{name: "create provider without key or models", action: resource.AdmissionActionCreate, object: catalog(providerOnly, nil), wantErr: "modelcatalog: providers.anthropic requires the secure value secure.anthropic"},
		{name: "update removing the last model", action: resource.AdmissionActionUpdate, object: catalog(providerOnly, nil), old: storedCatalog()},
		{name: "update removing all providers and keys", action: resource.AdmissionActionUpdate, object: catalog(empty, resource.InlineSecureValues{"anthropic": removed(), "openai": removed()}), old: storedCatalog()},
		{name: "update leaving an orphan key", action: resource.AdmissionActionUpdate, object: catalog(empty, resource.InlineSecureValues{"anthropic": removed()}), old: storedCatalog(), wantErr: "modelcatalog: secure.openai matches no provider"},
		{name: "create with keys", action: resource.AdmissionActionCreate, object: catalog(spec, resource.InlineSecureValues{"anthropic": created(), "openai": created()})},
		{name: "create without a key", action: resource.AdmissionActionCreate, object: catalog(spec, resource.InlineSecureValues{"anthropic": created()}), wantErr: "modelcatalog: providers.openai requires the secure value secure.openai"},
		{name: "create removing a key", action: resource.AdmissionActionCreate, object: catalog(spec, resource.InlineSecureValues{"anthropic": created(), "openai": removed()}), wantErr: "modelcatalog: providers.openai requires the secure value secure.openai"},
		{name: "create with a name", action: resource.AdmissionActionCreate, object: catalog(spec, resource.InlineSecureValues{"anthropic": created(), "openai": named("shared")}), wantErr: "modelcatalog: secure.openai must not reference a secret by name; set create with the key instead"},
		{name: "create with an extra key", action: resource.AdmissionActionCreate, object: catalog(spec, resource.InlineSecureValues{"anthropic": created(), "openai": created(), "other": created()}), wantErr: "modelcatalog: secure.other matches no provider"},
		{name: "create with base URL", action: resource.AdmissionActionCreate, object: func() resource.Object {
			changed := testSpec()
			endpoint := "https://attacker.invalid"
			changed.Providers["openai"] = v1alpha1.Provider{Type: "openai", BaseURL: &endpoint}
			return catalog(changed, resource.InlineSecureValues{"anthropic": created(), "openai": created()})
		}(), wantErr: "modelcatalog: providers.openai.baseURL is not supported in a model catalog"},
		{name: "update without secure", action: resource.AdmissionActionUpdate, object: catalog(spec, nil), old: storedCatalog()},
		{name: "update replacing a key", action: resource.AdmissionActionUpdate, object: catalog(spec, resource.InlineSecureValues{"openai": created()}), old: storedCatalog()},
		{name: "update repeating stored names", action: resource.AdmissionActionUpdate, object: storedCatalog(), old: storedCatalog()},
		{name: "update removing a used key", action: resource.AdmissionActionUpdate, object: catalog(spec, resource.InlineSecureValues{"openai": removed()}), old: storedCatalog(), wantErr: "modelcatalog: providers.openai requires the secure value secure.openai"},
		{name: "update removing a provider and its key", action: resource.AdmissionActionUpdate, object: catalog(withoutOpenAI(), resource.InlineSecureValues{"openai": removed()}), old: storedCatalog()},
		{name: "update removing a provider but keeping its key", action: resource.AdmissionActionUpdate, object: catalog(withoutOpenAI(), nil), old: storedCatalog(), wantErr: "modelcatalog: secure.openai matches no provider"},
		{name: "update with another name", action: resource.AdmissionActionUpdate, object: catalog(spec, resource.InlineSecureValues{"openai": named("shared")}), old: storedCatalog(), wantErr: "modelcatalog: secure.openai must not reference a secret by name; set create with the key instead"},
		{name: "update swapping stored names", action: resource.AdmissionActionUpdate, object: catalog(spec, resource.InlineSecureValues{"anthropic": named("inline-openai"), "openai": named("inline-anthropic")}), old: storedCatalog(), wantErr: "modelcatalog: secure.anthropic must not reference a secret by name; set create with the key instead"},
		{name: "remove with create", action: resource.AdmissionActionUpdate, object: catalog(spec, resource.InlineSecureValues{"openai": {Create: resource.NewSecretValue(sentinel), Remove: true}}), old: storedCatalog(), wantErr: "modelcatalog: secure.openai must set only one of create, name or remove"},
		{name: "create with name", action: resource.AdmissionActionUpdate, object: catalog(spec, resource.InlineSecureValues{"openai": {Create: resource.NewSecretValue(sentinel), Name: "inline-openai"}}), old: storedCatalog(), wantErr: "modelcatalog: secure.openai must set only one of create, name or remove"},
		{name: "empty entry", action: resource.AdmissionActionUpdate, object: catalog(spec, resource.InlineSecureValues{"openai": {}}), old: storedCatalog(), wantErr: "modelcatalog: secure.openai must set create, name or remove"},
		{name: "invalid key is not echoed", action: resource.AdmissionActionCreate, object: catalog(spec, resource.InlineSecureValues{"anthropic": created(), "openai": created(), sentinel + " key": {}}), wantErr: "modelcatalog: a secure key that is not a provider name must set create, name or remove"},
		{name: "wrong object", action: resource.AdmissionActionCreate, object: &resource.UntypedObject{}, wantErr: "modelcatalog: the admission object is not a ModelCatalog"},
		{name: "wrong stored object", action: resource.AdmissionActionUpdate, object: catalog(spec, nil), old: &resource.UntypedObject{}, wantErr: "modelcatalog: the stored object is not a ModelCatalog"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := Validator{}.Validate(context.Background(), &app.AdmissionRequest{Action: tc.action, Object: tc.object, OldObject: tc.old})
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
