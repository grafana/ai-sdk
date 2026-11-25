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

func openAIAt(endpoint string) *v1alpha1.ModelCatalog {
	spec := testSpec()
	spec.Providers["openai"] = v1alpha1.Provider{Type: "openai", BaseURL: pointer(endpoint)}
	return catalog(spec, resource.InlineSecureValues{"anthropic": created(), "openai": created()})
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
		{name: "create empty", action: resource.AdmissionActionCreate, object: catalog(empty, nil), wantErr: "config: at least one provider is required"},
		{name: "create without provider map", action: resource.AdmissionActionCreate, object: catalog(v1alpha1.Spec{Models: empty.Models}, nil), wantErr: "config: at least one provider is required"},
		{name: "update without model map", action: resource.AdmissionActionUpdate, object: catalog(v1alpha1.Spec{Providers: spec.Providers}, nil), old: storedCatalog(), wantErr: "modelcatalog: spec.models must be a map"},
		{name: "create with providers only", action: resource.AdmissionActionCreate, object: catalog(providerOnly, keys)},
		{name: "create provider without key or models", action: resource.AdmissionActionCreate, object: catalog(providerOnly, nil), wantErr: "modelcatalog: providers.anthropic requires the secure value secure.anthropic"},
		{name: "update removing the last model", action: resource.AdmissionActionUpdate, object: catalog(providerOnly, nil), old: storedCatalog()},
		{name: "update removing the last provider and key", action: resource.AdmissionActionUpdate, object: catalog(empty, resource.InlineSecureValues{"anthropic": removed()}), old: catalog(v1alpha1.Spec{Providers: map[string]v1alpha1.Provider{"anthropic": {Type: "anthropic"}}, Models: empty.Models}, resource.InlineSecureValues{"anthropic": named("inline-anthropic")}), wantErr: "config: at least one provider is required"},
		{name: "create with keys", action: resource.AdmissionActionCreate, object: catalog(spec, resource.InlineSecureValues{"anthropic": created(), "openai": created()})},
		{name: "create without a key", action: resource.AdmissionActionCreate, object: catalog(spec, resource.InlineSecureValues{"anthropic": created()}), wantErr: "modelcatalog: providers.openai requires the secure value secure.openai"},
		{name: "create removing a key", action: resource.AdmissionActionCreate, object: catalog(spec, resource.InlineSecureValues{"anthropic": created(), "openai": removed()}), wantErr: "modelcatalog: providers.openai requires the secure value secure.openai"},
		{name: "create with a name", action: resource.AdmissionActionCreate, object: catalog(spec, resource.InlineSecureValues{"anthropic": created(), "openai": named("shared")}), wantErr: "modelcatalog: secure.openai must not reference a secret by name; set create with the key instead"},
		{name: "create with an extra key", action: resource.AdmissionActionCreate, object: catalog(spec, resource.InlineSecureValues{"anthropic": created(), "openai": created(), "other": created()}), wantErr: "modelcatalog: secure.other matches no provider"},
		{name: "create with a listed endpoint", action: resource.AdmissionActionCreate, object: openAIAt("https://llm.example.com/v1/team")},
		{name: "create with a compatible provider", action: resource.AdmissionActionCreate, object: func() resource.Object {
			changed := testSpec()
			changed.Providers["openai"] = v1alpha1.Provider{Type: "openai-compatible", BaseURL: pointer("https://llm.example.com/v1"), ProviderName: pointer("acme"), SupportsStructuredOutputs: pointer(true)}
			return catalog(changed, resource.InlineSecureValues{"anthropic": created(), "openai": created()})
		}()},
		{name: "create with an unlisted endpoint", action: resource.AdmissionActionCreate, object: openAIAt("https://llm.example.com/v10"), wantErr: "modelcatalog: providers.openai.baseURL is not a listed custom endpoint"},
		{name: "update to an unlisted endpoint", action: resource.AdmissionActionUpdate, object: openAIAt("https://169.254.169.254"), old: storedCatalog(), wantErr: "modelcatalog: providers.openai.baseURL is not a listed custom endpoint"},
		{name: "create with the native endpoint", action: resource.AdmissionActionCreate, object: openAIAt("https://api.openai.com/v1")},
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
			err := Validator{Endpoints: testEndpoints(t)}.Validate(context.Background(), &app.AdmissionRequest{Action: tc.action, Object: tc.object, OldObject: tc.old})
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
