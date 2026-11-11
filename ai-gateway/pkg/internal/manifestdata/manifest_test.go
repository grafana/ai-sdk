package manifestdata

import (
	"maps"
	"net/http"
	"os"
	"regexp"
	"slices"
	"testing"

	"github.com/grafana/grafana-app-sdk/app"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/kube-openapi/pkg/spec3"

	"github.com/grafana/ai-sdk/ai-gateway/pkg/extension/tenanttype"
	"github.com/grafana/ai-sdk/ai-gateway/pkg/internal/tenantschema"
)

func TestManifest_DeclaresGatewayRoutesAndKinds(t *testing.T) {
	data := LocalManifest().ManifestData
	require.NotNil(t, data)
	assert.Equal(t, "aigateway", data.AppName)
	assert.Equal(t, "aigateway.ext.grafana.app", data.Group)
	assert.Nil(t, data.Operator)
	assert.Empty(t, data.Embed)
	require.Len(t, data.Versions, 1)

	version := data.Versions[0]
	assert.Equal(t, "v1alpha1", version.Name)
	assert.True(t, version.Served)
	assert.Equal(t, map[string][]string{
		"/namespaces/{namespace}/config":         {http.MethodGet},
		"/namespaces/{namespace}/language-model": {http.MethodPost},
	}, operations(version.OpenAPI.Paths))
	require.Len(t, version.Kinds, 2)
	for _, tc := range []struct {
		kind   string
		plural string
		secure []string
	}{
		{kind: "ModelCatalog", plural: "ModelCatalogs", secure: []string{"*"}},
		{kind: "Middleware", plural: "Middlewares"},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			kind := manifestKind(t, version, tc.kind)
			assert.Equal(t, tc.plural, kind.Plural)
			assert.Equal(t, "Namespaced", kind.Scope)
			require.NotNil(t, kind.FolderScoped)
			assert.False(t, *kind.FolderScoped)
			assert.False(t, kind.Conversion)
			var secure []string
			for _, value := range kind.SecureValues {
				secure = append(secure, value.Key)
			}
			assert.Equal(t, tc.secure, secure)
			require.NotNil(t, kind.Admission)
			assert.Nil(t, kind.Admission.Mutation)
			require.NotNil(t, kind.Admission.Validation)
			assert.Equal(t, []app.AdmissionOperation{app.AdmissionOperationCreate, app.AdmissionOperationUpdate}, kind.Admission.Validation.Operations)
		})
	}
}

func TestManifest_MiddlewareSchema(t *testing.T) {
	kind := manifestKind(t, LocalManifest().ManifestData.Versions[0], "Middleware")
	require.NotNil(t, kind.Schema)
	components, err := kind.Schema.AsOpenAPI3()
	require.NoError(t, err)
	root := components.Schemas["Middleware"]
	require.NotNil(t, root)
	assert.ElementsMatch(t, []string{"spec"}, slices.Collect(maps.Keys(root.Value.Properties)), "no secure values")
	spec := components.Schemas["spec"]
	require.NotNil(t, spec)
	assert.ElementsMatch(t, []string{"type", "enabled"}, spec.Value.Required, "no member is required")
	require.NotNil(t, spec.Value.AdditionalProperties.Has)
	assert.False(t, *spec.Value.AdditionalProperties.Has)
	assert.Empty(t, spec.Value.OneOf)
	assert.Empty(t, spec.Value.AnyOf)
	assert.True(t, spec.Value.Properties["type"].Value.Type.Is("string"))
	assert.Empty(t, spec.Value.Properties["type"].Value.Enum, "spec.type has no enum, so stored types survive a rollback")
	assert.True(t, spec.Value.Properties["enabled"].Value.Type.Is("boolean"))

	members, err := tenantschema.Members(tenanttype.All())
	require.NoError(t, err)
	want := []string{"type", "enabled"}
	for _, member := range members {
		want = append(want, member.Name)
	}
	assert.ElementsMatch(t, want, slices.Collect(maps.Keys(spec.Value.Properties)), "members are the projection of the registry")
}

func TestManifest_MiddlewareMembersSurvivePruning(t *testing.T) {
	structural, err := tenantschema.ManifestKind(*LocalManifest().ManifestData, "v1alpha1", "Middleware")
	require.NoError(t, err, "the kind store builds a structural schema, so it prunes and validates the kind")
	for _, tenantType := range tenanttype.All() {
		if tenantType.ConfigSchema.JSON() == nil {
			continue
		}
		t.Run(tenantType.Name, func(t *testing.T) {
			assert.NoError(t, tenantschema.RoundTrip(structural, tenantType.Name, tenantType.Member(), tenantType.Example))
		})
	}
}

func TestManifest_Roles(t *testing.T) {
	data := LocalManifest().ManifestData
	rolePattern := regexp.MustCompile(`^` + data.AppName + `:[a-z0-9]+$`)
	role := func(title, description, kind, permissionSet string) app.ManifestRole {
		return app.ManifestRole{Title: title, Description: description, Kinds: []app.ManifestRoleKind{{Kind: kind, PermissionSet: &permissionSet}}, Routes: []string{}}
	}
	assert.Equal(t, map[string]app.ManifestRole{
		"aigateway:catalogreader":    role("Model catalog reader", "Read AI Gateway model catalogs", "ModelCatalog", app.ManifestRolePermissionSetViewer),
		"aigateway:catalogwriter":    role("Model catalog writer", "Create, update and delete AI Gateway model catalogs", "ModelCatalog", app.ManifestRolePermissionSetEditor),
		"aigateway:middlewarereader": role("Middleware reader", "Read AI Gateway tenant extension configuration", "Middleware", app.ManifestRolePermissionSetViewer),
		"aigateway:middlewarewriter": role("Middleware writer", "Create, update and delete AI Gateway tenant extension configuration", "Middleware", app.ManifestRolePermissionSetEditor),
	}, data.Roles)
	fixedIDs := []string{}
	for name := range data.Roles {
		assert.Regexp(t, rolePattern, name)
		fixedIDs = append(fixedIDs, "fixed:"+data.Group+":"+name)
	}
	assert.ElementsMatch(t, []string{
		"fixed:aigateway.ext.grafana.app:aigateway:catalogreader",
		"fixed:aigateway.ext.grafana.app:aigateway:catalogwriter",
		"fixed:aigateway.ext.grafana.app:aigateway:middlewarereader",
		"fixed:aigateway.ext.grafana.app:aigateway:middlewarewriter",
	}, fixedIDs)
	require.NotNil(t, data.RoleBindings)
	assert.Equal(t, []string{"aigateway:catalogreader", "aigateway:middlewarereader"}, data.RoleBindings.Viewer)
	assert.Empty(t, data.RoleBindings.Editor)
	assert.Equal(t, []string{"aigateway:catalogwriter", "aigateway:middlewarewriter"}, data.RoleBindings.Admin)
	assert.Empty(t, data.RoleBindings.Additional)
}

func manifestKind(t *testing.T, version app.ManifestVersion, name string) app.ManifestVersionKind {
	t.Helper()
	for _, kind := range version.Kinds {
		if kind.Kind == name {
			return kind
		}
	}
	require.Failf(t, "kind not declared", "%s", name)
	return app.ManifestVersionKind{}
}

func TestManifest_GeneratorMatchesRuntimeVersion(t *testing.T) {
	script, err := os.ReadFile("../../../.config/app-sdk/generate-kinds.mjs")
	require.NoError(t, err)
	generator := regexp.MustCompile(`(?m)^const VERSION = '(v\S+)';$`).FindSubmatch(script)
	require.Len(t, generator, 2)

	goMod, err := os.ReadFile("../../../go.mod")
	require.NoError(t, err)
	runtime := regexp.MustCompile(`(?m)^\s*github\.com/grafana/grafana-app-sdk (v\S+)`).FindSubmatch(goMod)
	require.Len(t, runtime, 2)
	assert.Equal(t, string(runtime[1]), string(generator[1]))
}

func operations(paths map[string]spec3.PathProps) map[string][]string {
	result := make(map[string][]string, len(paths))
	for path, props := range paths {
		methods := []string{}
		for method, operation := range map[string]*spec3.Operation{
			http.MethodGet:     props.Get,
			http.MethodPut:     props.Put,
			http.MethodPost:    props.Post,
			http.MethodDelete:  props.Delete,
			http.MethodOptions: props.Options,
			http.MethodHead:    props.Head,
			http.MethodPatch:   props.Patch,
			http.MethodTrace:   props.Trace,
		} {
			if operation != nil {
				methods = append(methods, method)
			}
		}
		result[path] = methods
	}
	return result
}
