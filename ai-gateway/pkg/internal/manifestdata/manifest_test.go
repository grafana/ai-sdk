package manifestdata

import (
	"net/http"
	"os"
	"regexp"
	"testing"

	"github.com/grafana/grafana-app-sdk/app"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/kube-openapi/pkg/spec3"
)

func TestManifest_DeclaresGatewayRoutesAndTheCatalogKind(t *testing.T) {
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
	require.Len(t, version.Kinds, 1)
	kind := version.Kinds[0]
	assert.Equal(t, "ModelCatalog", kind.Kind)
	assert.Equal(t, "ModelCatalogs", kind.Plural)
	assert.Equal(t, "Namespaced", kind.Scope)
	require.NotNil(t, kind.FolderScoped)
	assert.False(t, *kind.FolderScoped)
	assert.False(t, kind.Conversion)
	require.Len(t, kind.SecureValues, 1)
	assert.Equal(t, "*", kind.SecureValues[0].Key)
	require.NotNil(t, kind.Admission)
	assert.Nil(t, kind.Admission.Mutation)
	require.NotNil(t, kind.Admission.Validation)
	assert.Equal(t, []app.AdmissionOperation{app.AdmissionOperationCreate, app.AdmissionOperationUpdate}, kind.Admission.Validation.Operations)
	assert.Equal(t, map[string][]string{
		"/namespaces/{namespace}/config":                              {http.MethodGet},
		"/namespaces/{namespace}/language-model":                      {http.MethodPost},
		"/namespaces/{namespace}/modelcatalogs/{name}/config":         {http.MethodGet},
		"/namespaces/{namespace}/modelcatalogs/{name}/language-model": {http.MethodPost},
	}, operations(version.OpenAPI.Paths))
}

func TestManifest_DeclaredAccessChecks(t *testing.T) {
	paths := LocalManifest().ManifestData.Versions[0].OpenAPI.Paths
	for _, tc := range []struct {
		path        string
		operation   func(spec3.PathProps) *spec3.Operation
		subresource string
	}{
		{path: "/namespaces/{namespace}/modelcatalogs/{name}/config", operation: func(p spec3.PathProps) *spec3.Operation { return p.Get }, subresource: "config"},
		{path: "/namespaces/{namespace}/modelcatalogs/{name}/language-model", operation: func(p spec3.PathProps) *spec3.Operation { return p.Post }, subresource: "language-model"},
	} {
		t.Run(tc.subresource, func(t *testing.T) {
			operation := tc.operation(paths[tc.path])
			require.NotNil(t, operation)
			assert.Equal(t, "modelcatalogs", operation.Extensions["x-grafana-declared-authz-resource"])
			assert.Equal(t, tc.subresource, operation.Extensions["x-grafana-declared-authz-subresource"])
			assert.Equal(t, "get", operation.Extensions["x-grafana-declared-authz-verb"])
		})
	}
	for _, path := range []string{"/namespaces/{namespace}/config", "/namespaces/{namespace}/language-model"} {
		for _, operation := range []*spec3.Operation{paths[path].Get, paths[path].Post} {
			if operation == nil {
				continue
			}
			for key := range operation.Extensions {
				assert.NotContains(t, key, "x-grafana-declared-authz", path)
			}
		}
	}
}

func TestManifest_Roles(t *testing.T) {
	data := LocalManifest().ManifestData
	rolePattern := regexp.MustCompile(`^` + data.AppName + `:[a-z0-9]+$`)
	permissionSets := map[string]string{}
	fixedIDs := []string{}
	for name, role := range data.Roles {
		assert.Regexp(t, rolePattern, name)
		fixedIDs = append(fixedIDs, "fixed:"+data.Group+":"+name)
		if name == "aigateway:modelinvoker" {
			assert.Equal(t, []app.ManifestRoleKind{
				{Kind: "ModelCatalog", Verbs: []string{"get"}},
				{Kind: "ModelCatalog/language-model", Verbs: []string{"get"}},
				{Kind: "ModelCatalog/config", Verbs: []string{"get"}},
			}, role.Kinds)
			continue
		}
		require.Len(t, role.Kinds, 1)
		assert.Equal(t, "ModelCatalog", role.Kinds[0].Kind)
		assert.Empty(t, role.Kinds[0].Verbs)
		require.NotNil(t, role.Kinds[0].PermissionSet)
		permissionSets[name] = *role.Kinds[0].PermissionSet
	}
	assert.Equal(t, map[string]string{
		"aigateway:catalogreader": app.ManifestRolePermissionSetViewer,
		"aigateway:catalogwriter": app.ManifestRolePermissionSetEditor,
	}, permissionSets)
	assert.ElementsMatch(t, []string{
		"fixed:aigateway.ext.grafana.app:aigateway:catalogreader",
		"fixed:aigateway.ext.grafana.app:aigateway:catalogwriter",
		"fixed:aigateway.ext.grafana.app:aigateway:modelinvoker",
	}, fixedIDs)
	require.NotNil(t, data.RoleBindings)
	assert.Equal(t, []string{"aigateway:catalogreader"}, data.RoleBindings.Viewer)
	assert.Equal(t, []string{"aigateway:modelinvoker"}, data.RoleBindings.Editor)
	assert.Equal(t, []string{"aigateway:catalogwriter"}, data.RoleBindings.Admin)
	assert.Empty(t, data.RoleBindings.Additional)
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
