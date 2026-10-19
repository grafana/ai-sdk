package manifestdata

import (
	"net/http"
	"os"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/kube-openapi/pkg/spec3"
)

func TestManifest_DeclaresOnlyGatewayRoutes(t *testing.T) {
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
	assert.Empty(t, version.Kinds)
	assert.Equal(t, map[string][]string{
		"/namespaces/{namespace}/config":         {http.MethodGet},
		"/namespaces/{namespace}/language-model": {http.MethodPost},
	}, operations(version.OpenAPI.Paths))
}

func TestManifest_GeneratorMatchesRuntimeVersion(t *testing.T) {
	script, err := os.ReadFile("../../../../.config/app-sdk/generate-kinds.mjs")
	require.NoError(t, err)
	generator := regexp.MustCompile(`(?m)^const VERSION = '(v\S+)';$`).FindSubmatch(script)
	require.Len(t, generator, 2)

	goMod, err := os.ReadFile("../../../../go.mod")
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
