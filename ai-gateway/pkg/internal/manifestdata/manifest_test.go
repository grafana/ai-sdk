package manifestdata

import (
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"testing"

	"github.com/grafana/grafana-app-sdk/app"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/kube-openapi/pkg/spec3"

	"github.com/grafana/ai-sdk/ai-gateway/pkg/extension/tenanttype"
	"github.com/grafana/ai-sdk/schema"
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

	want := []string{"type", "enabled"}
	for _, tenantType := range tenanttype.All() {
		if tenantType.ConfigSchema.JSON() != nil {
			want = append(want, tenantType.Member())
		}
	}
	assert.ElementsMatch(t, want, slices.Collect(maps.Keys(spec.Value.Properties)), "one member per tenant type with configuration")
	assert.ElementsMatch(t, []string{"type", "enabled", "defaultInstructions", "defaultSettings"}, want, "the shipped members")

	settings := spec.Value.Properties["defaultSettings"].Value
	assert.ElementsMatch(t, []string{"temperature", "topP", "topK", "maxOutputTokens", "stopSequences", "seed", "presencePenalty", "frequencyPenalty"}, slices.Collect(maps.Keys(settings.Properties)), "default settings declare only sampling and limit fields")
	require.NotNil(t, settings.AdditionalProperties.Has)
	assert.False(t, *settings.AdditionalProperties.Has)
	instructions := spec.Value.Properties["defaultInstructions"].Value
	assert.ElementsMatch(t, []string{"messages"}, slices.Collect(maps.Keys(instructions.Properties)))
	message := instructions.Properties["messages"].Value.Items.Value
	assert.ElementsMatch(t, []string{"content"}, slices.Collect(maps.Keys(message.Properties)), "messages carry no provider options")
	require.NotNil(t, message.AdditionalProperties.Has)
	assert.False(t, *message.AdditionalProperties.Has)
}

func TestManifest_MiddlewareMembersMirrorTheirTypes(t *testing.T) {
	kind := manifestKind(t, LocalManifest().ManifestData.Versions[0], "Middleware")
	raw, err := json.Marshal(kind.Schema)
	require.NoError(t, err)
	members := specMembers(t, raw)
	require.Contains(t, members, "enabled", "the manifest's spec properties are read")
	for _, tenantType := range tenanttype.All() {
		if tenantType.ConfigSchema.JSON() == nil {
			continue
		}
		t.Run(tenantType.Name, func(t *testing.T) {
			assert.Empty(t, memberProblems(t, tenantType, members[tenantType.Member()]))
		})
	}
}

func TestMemberProblems(t *testing.T) {
	const modelAccessSchema = `{"type":"object","properties":{
		"rules":{"type":"array","minItems":1,"items":{"type":"object","properties":{
			"models":{"type":"array","items":{"type":"string","pattern":"^[a-z]+$"}},
			"teams":{"type":"array","items":{"type":"string"}},
			"limits":{"type":"object","properties":{"rpm":{"type":"integer","minimum":1,"maximum":9007199254740991}},"additionalProperties":false}
		},"required":["models","teams"],"additionalProperties":false}},
		"mode":{"type":"string","enum":["allow","deny"],"description":"Whether rules allow or deny.\nApplies to every rule."},
		"temperature":{"type":"number","maximum":2},
		"strict":{"type":"boolean"}
	},"required":["rules"],"additionalProperties":false}`
	configSchema, err := schema.SchemaFromJSON(json.RawMessage(modelAccessSchema))
	require.NoError(t, err)
	modelAccess := tenanttype.Type{Name: "model-access", Description: "Controls which models teams may use.", ConfigSchema: configSchema, Example: json.RawMessage(`{"rules":[{"models":["balanced"],"teams":["platform"],"limits":{"rpm":10}}],"mode":"deny","temperature":0.5,"strict":true}`)}

	raw, err := os.ReadFile(filepath.Join("testdata", "generated-middleware-schemas.json"))
	require.NoError(t, err)
	generated := specMembers(t, raw)["modelAccess"]
	require.NotNil(t, generated)
	assert.Empty(t, memberProblems(t, modelAccess, generated), "the app-sdk generator's member for a hand-written mirror agrees with the type")

	change := func(edit func(member map[string]any)) map[string]any {
		copied := map[string]any{}
		encoded, err := json.Marshal(generated)
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(encoded, &copied))
		edit(copied)
		return copied
	}
	properties := func(node map[string]any) map[string]any { return node["properties"].(map[string]any) }
	rule := func(member map[string]any) map[string]any {
		return properties(member)["rules"].(map[string]any)["items"].(map[string]any)
	}
	for _, tc := range []struct {
		name    string
		member  map[string]any
		problem string
	}{
		{name: "missing member", problem: "the manifest declares no member"},
		{name: "missing property", member: change(func(m map[string]any) { delete(properties(m), "strict") }), problem: "/ declares properties [mode rules temperature], the type [mode rules strict temperature]"},
		{name: "extra nested property", member: change(func(m map[string]any) { properties(rule(m))["extra"] = map[string]any{"type": "string"} }), problem: "/properties/rules/items declares properties [extra limits models teams], the type [limits models teams]"},
		{name: "different JSON type", member: change(func(m map[string]any) { properties(m)["temperature"] = map[string]any{"type": "integer"} }), problem: `/properties/temperature has type "integer", the type "number"`},
		{name: "different item type", member: change(func(m map[string]any) {
			properties(rule(m))["models"] = map[string]any{"type": "array", "items": map[string]any{"type": "integer"}}
		}), problem: `/properties/rules/items/properties/models/items has type "integer", the type "string"`},
		{name: "different required fields", member: change(func(m map[string]any) { rule(m)["required"] = []any{"models"} }), problem: "/properties/rules/items requires [models], the type [models teams]"},
		{name: "open object", member: change(func(m map[string]any) { delete(rule(m), "additionalProperties") }), problem: "/properties/rules/items must set additionalProperties to false"},
		{name: "different description", member: change(func(m map[string]any) { properties(m)["mode"].(map[string]any)["description"] = "Other." }), problem: "/properties/mode has another description than the type"},
		{name: "different member description", member: change(func(m map[string]any) { m["description"] = "Other." }), problem: "/ has another description than the type"},
		{name: "value constraint", member: change(func(m map[string]any) { properties(m)["mode"].(map[string]any)["enum"] = []any{"allow"} }), problem: `/properties/mode declares "enum", which the manifest leaves to admission`},
		{name: "example the member refuses", member: change(func(m map[string]any) { properties(m)["strict"] = map[string]any{"type": "string"} }), problem: "the member refuses the type's example"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Contains(t, memberProblems(t, modelAccess, tc.member), tc.problem)
		})
	}
}

// specMembers returns the Middleware spec properties of a manifest version
// schema, as JSON objects.
func specMembers(t *testing.T, raw []byte) map[string]map[string]any {
	t.Helper()
	var schemas struct {
		Spec struct {
			Properties map[string]map[string]any `json:"properties"`
		} `json:"spec"`
	}
	require.NoError(t, json.Unmarshal(raw, &schemas))
	return schemas.Spec.Properties
}

// structureKeywords are the keywords a manifest member may declare.
var structureKeywords = []string{"type", "description", "properties", "required", "items", "additionalProperties"}

// memberProblems compares a type's schema with its manifest member: the same
// properties, JSON types, required fields, items and descriptions, closed
// objects, no value constraints, and an example both accept. The member's own
// description is the type's Description.
func memberProblems(t *testing.T, tenantType tenanttype.Type, member map[string]any) []string {
	t.Helper()
	if member == nil {
		return []string{"the manifest declares no member"}
	}
	var typeSchema map[string]any
	require.NoError(t, json.Unmarshal(tenantType.ConfigSchema.JSON(), &typeSchema))
	typeSchema["description"] = tenantType.Description
	if tenantType.Description == "" {
		delete(typeSchema, "description")
	}
	found := structureProblems(typeSchema, member, "")
	encoded, err := json.Marshal(member)
	require.NoError(t, err)
	if compiled, err := schema.SchemaFromJSON(encoded); err != nil || compiled.Validate(tenantType.Example) != nil {
		found = append(found, "the member refuses the type's example")
	}
	return found
}

func structureProblems(want, got map[string]any, at string) []string {
	where := at
	if where == "" {
		where = "/"
	}
	var found []string
	for keyword := range got {
		if !slices.Contains(structureKeywords, keyword) {
			found = append(found, fmt.Sprintf("%s declares %q, which the manifest leaves to admission", where, keyword))
		}
	}
	if got["type"] != want["type"] {
		return append(found, fmt.Sprintf("%s has type %q, the type %q", where, got["type"], want["type"]))
	}
	if got["description"] != want["description"] {
		found = append(found, where+" has another description than the type")
	}
	switch want["type"] {
	case "object":
		if got["additionalProperties"] != false {
			found = append(found, where+" must set additionalProperties to false")
		}
		wantProperties, _ := want["properties"].(map[string]any)
		gotProperties, _ := got["properties"].(map[string]any)
		if names, wantNames := slices.Sorted(maps.Keys(gotProperties)), slices.Sorted(maps.Keys(wantProperties)); !slices.Equal(names, wantNames) {
			return append(found, fmt.Sprintf("%s declares properties %v, the type %v", where, names, wantNames))
		}
		if required, wantRequired := sortedStrings(got["required"]), sortedStrings(want["required"]); !slices.Equal(required, wantRequired) {
			found = append(found, fmt.Sprintf("%s requires %v, the type %v", where, required, wantRequired))
		}
		for name, property := range wantProperties {
			gotProperty, _ := gotProperties[name].(map[string]any)
			wantProperty, _ := property.(map[string]any)
			found = append(found, structureProblems(wantProperty, gotProperty, at+"/properties/"+name)...)
		}
	case "array":
		wantItems, _ := want["items"].(map[string]any)
		gotItems, _ := got["items"].(map[string]any)
		found = append(found, structureProblems(wantItems, gotItems, at+"/items")...)
	}
	return found
}

func sortedStrings(value any) []string {
	list, _ := value.([]any)
	names := []string{}
	for _, entry := range list {
		name, _ := entry.(string)
		names = append(names, name)
	}
	slices.Sort(names)
	return names
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
