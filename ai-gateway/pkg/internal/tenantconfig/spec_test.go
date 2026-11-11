package tenantconfig

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/grafana-app-sdk/resource"
)

func TestDecodeSpec(t *testing.T) {
	spec, err := DecodeSpec(json.RawMessage(`{"type":"model-access","enabled":true,"modelAccess":{ "rules" : [{"teams":["b"],"models":["a"]}], "limit": 9007199254740993, "frac": 1.50, "html": "<a&b>" }}`))
	require.NoError(t, err)
	assert.Equal(t, "model-access", spec.Type)
	assert.True(t, spec.Enabled)
	assert.Equal(t, map[string]json.RawMessage{
		"modelAccess": json.RawMessage(`{"frac":1.50,"html":"<a&b>","limit":9007199254740993,"rules":[{"models":["a"],"teams":["b"]}]}`),
	}, spec.Members, "members are canonical: compact, sorted, numbers as written, no HTML escaping")

	spec, err = DecodeSpec(json.RawMessage(`{"enabled":false,"type":"marker"}`))
	require.NoError(t, err)
	assert.Empty(t, spec.Members)

	const sentinel = "sentinel-spec-value"
	deep := strings.Repeat(`{"a":`, 20000) + `1` + strings.Repeat(`}`, 20000)
	for name, raw := range map[string]string{
		"not JSON":               `{"type":`,
		"not an object":          `["` + sentinel + `"]`,
		"no type":                `{"enabled":true}`,
		"empty type":             `{"type":"","enabled":true}`,
		"type not a string":      `{"type":1,"enabled":true}`,
		"no enabled":             `{"type":"marker"}`,
		"enabled not a boolean":  `{"type":"marker","enabled":"` + sentinel + `"}`,
		"repeated type":          `{"type":"marker","type":"other","enabled":true}`,
		"repeated member":        `{"type":"marker","enabled":true,"marker":{"a":1},"marker":{"a":"` + sentinel + `"}}`,
		"trailing data":          `{"type":"marker","enabled":true} {}`,
		"member nested too deep": `{"type":"marker","enabled":true,"marker":` + deep + `}`,
		"missing":                ``,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := DecodeSpec(json.RawMessage(raw))
			require.Error(t, err)
			assert.Equal(t, "middleware: spec is not a valid Middleware spec", err.Error())
		})
	}
}

func TestMiddleware_DecodesMetadataAndKeepsTheSpec(t *testing.T) {
	raw := `{"apiVersion":"aigateway.ext.grafana.app/v1alpha1","kind":"Middleware","metadata":{"name":"marker","namespace":"stacks-1","resourceVersion":"7"},"spec":{"type":"marker","enabled":true,"marker":{"limit":9223372036854775808}}}`
	k := Kind()
	object, err := k.Read(bytes.NewReader([]byte(raw)), resource.KindEncodingJSON)
	require.NoError(t, err)
	middleware, ok := object.(*Middleware)
	require.True(t, ok)
	assert.Equal(t, "marker", middleware.Name)
	assert.Equal(t, "stacks-1", middleware.Namespace)
	assert.Equal(t, "7", middleware.ResourceVersion)
	assert.JSONEq(t, `{"type":"marker","enabled":true,"marker":{"limit":9223372036854775808}}`, string(middleware.RawSpec))
	spec, err := DecodeSpec(middleware.RawSpec)
	require.NoError(t, err)
	assert.Equal(t, `{"limit":9223372036854775808}`, string(spec.Members["marker"]), "a number above int64 stays exact")

	copied, ok := middleware.Copy().(*Middleware)
	require.True(t, ok)
	copied.RawSpec[0] = 'x'
	copied.Name = "other"
	assert.Equal(t, byte('{'), middleware.RawSpec[0], "Copy is deep")
	assert.Equal(t, "marker", middleware.Name)

	_, err = k.Read(bytes.NewReader([]byte(`{"metadata":"`+"sentinel-metadata"+`"}`)), resource.KindEncodingJSON)
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "sentinel-metadata")
}
