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
	tests := []struct {
		name    string
		action  resource.AdmissionAction
		object  resource.Object
		old     resource.Object
		wantErr string
	}{
		{name: "create with keys", action: resource.AdmissionActionCreate, object: catalog(spec, resource.InlineSecureValues{"anthropic": created(), "openai": created()})},
		{name: "create without a key", action: resource.AdmissionActionCreate, object: catalog(spec, resource.InlineSecureValues{"anthropic": created()}), wantErr: "modelcatalog: providers.openai requires the secure value secure.openai"},
		{name: "create removing a key", action: resource.AdmissionActionCreate, object: catalog(spec, resource.InlineSecureValues{"anthropic": created(), "openai": removed()}), wantErr: "modelcatalog: providers.openai requires the secure value secure.openai"},
		{name: "create with a name", action: resource.AdmissionActionCreate, object: catalog(spec, resource.InlineSecureValues{"anthropic": created(), "openai": named("shared")}), wantErr: "modelcatalog: secure.openai must not reference a secret by name; set create with the key instead"},
		{name: "create with an extra key", action: resource.AdmissionActionCreate, object: catalog(spec, resource.InlineSecureValues{"anthropic": created(), "openai": created(), "other": created()}), wantErr: "modelcatalog: secure.other matches no provider"},
		{name: "create with a custom endpoint", action: resource.AdmissionActionCreate, object: openAIAt("https://llm.example.com/v1")},
		{name: "create with a compatible provider", action: resource.AdmissionActionCreate, object: func() resource.Object {
			changed := testSpec()
			changed.Providers["openai"] = v1alpha1.Provider{Type: "openai-compatible", BaseURL: pointer("https://llm.example.com/v1"), ProviderName: pointer("acme"), SupportsStructuredOutputs: pointer(true)}
			return catalog(changed, resource.InlineSecureValues{"anthropic": created(), "openai": created()})
		}()},
		{name: "create with a plain HTTP endpoint", action: resource.AdmissionActionCreate, object: openAIAt("http://llm.example.com"), wantErr: "modelcatalog: providers.openai.baseURL must use https"},
		{name: "create with a metadata endpoint", action: resource.AdmissionActionCreate, object: openAIAt("https://169.254.169.254"), wantErr: "modelcatalog: providers.openai.baseURL is refused by the egress policy"},
		{name: "create with a denied host", action: resource.AdmissionActionCreate, object: openAIAt("https://denied.example"), wantErr: "modelcatalog: providers.openai.baseURL is refused by the egress policy"},
		{name: "create with an allowed private address", action: resource.AdmissionActionCreate, object: openAIAt("https://10.20.0.5")},
		{name: "update to a refused endpoint", action: resource.AdmissionActionUpdate, object: openAIAt("https://[::ffff:127.0.0.1]"), old: storedCatalog(), wantErr: "modelcatalog: providers.openai.baseURL is refused by the egress policy"},
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
			err := Validator{Policy: testPolicy(t)}.Validate(context.Background(), &app.AdmissionRequest{Action: tc.action, Object: tc.object, OldObject: tc.old})
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
