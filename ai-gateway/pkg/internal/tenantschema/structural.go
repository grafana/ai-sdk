package tenantschema

import (
	"encoding/json"
	"fmt"
	"maps"
	"reflect"
	"slices"

	"k8s.io/apiextensions-apiserver/pkg/apis/apiextensions"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	structuralschema "k8s.io/apiextensions-apiserver/pkg/apiserver/schema"
	structuralpruning "k8s.io/apiextensions-apiserver/pkg/apiserver/schema/pruning"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/kube-openapi/pkg/common"
	"k8s.io/kube-openapi/pkg/validation/spec"

	"github.com/grafana/grafana-app-sdk/app"
)

// serverOwnedFields are the object fields the API server validates itself and
// a kind store leaves out of a kind's structural schema.
var serverOwnedFields = []string{"metadata", "kind", "apiVersion"}

// Structural returns the structural schema of a Middleware object whose spec
// has the given members, as a kind store would build it from a manifest
// generated from them.
func Structural(members []Member) (*structuralschema.Structural, error) {
	properties := map[string]any{
		"type":    map[string]any{"type": "string"},
		"enabled": map[string]any{"type": "boolean"},
	}
	for _, member := range members {
		properties[member.Name] = member.Schema.JSONSchema()
	}
	root := map[string]any{
		"type": "object",
		"properties": map[string]any{"spec": map[string]any{
			"type":                 "object",
			"properties":           properties,
			"required":             []string{"type", "enabled"},
			"additionalProperties": false,
		}},
		"required": []string{"spec"},
	}
	raw, err := json.Marshal(root)
	if err != nil {
		return nil, fmt.Errorf("encoding the projected schema: %w", err)
	}
	return structural(raw)
}

// ManifestKind returns the structural schema a kind store builds for kind in
// version of manifest: the manifest schema converted to kube-openapi, with
// the server-owned fields removed and references expanded.
func ManifestKind(manifest app.ManifestData, version, kind string) (*structuralschema.Structural, error) {
	for _, manifestVersion := range manifest.Versions {
		if manifestVersion.Name != version {
			continue
		}
		for _, manifestKind := range manifestVersion.Kinds {
			if manifestKind.Kind != kind || manifestKind.Schema == nil {
				continue
			}
			gvk := schema.GroupVersionKind{Group: manifest.Group, Version: version, Kind: kind}
			prefix := manifest.Group + "." + version
			definitions, err := manifestKind.Schema.AsKubeOpenAPI(gvk, func(path string) spec.Ref { return spec.MustCreateRef(path) }, prefix)
			if err != nil {
				return nil, fmt.Errorf("converting the %s schema: %w", kind, err)
			}
			definition, ok := definitions[prefix+"."+kind]
			if !ok {
				return nil, fmt.Errorf("the %s schema has no root definition", kind)
			}
			root := definition.Schema
			root.Properties = maps.Clone(root.Properties)
			for _, name := range serverOwnedFields {
				delete(root.Properties, name)
			}
			root.Required = slices.DeleteFunc(slices.Clone(root.Required), func(name string) bool { return slices.Contains(serverOwnedFields, name) })
			raw, err := json.Marshal(expand(root, definitions, map[string]bool{}))
			if err != nil {
				return nil, fmt.Errorf("encoding the %s schema: %w", kind, err)
			}
			return structural(raw)
		}
	}
	return nil, fmt.Errorf("the manifest declares no %s kind with a schema in %s", kind, version)
}

func expand(s spec.Schema, definitions map[string]common.OpenAPIDefinition, expanding map[string]bool) spec.Schema {
	if ref := s.Ref.String(); ref != "" {
		definition, ok := definitions[ref]
		if !ok || expanding[ref] {
			return spec.Schema{}
		}
		expanding[ref] = true
		defer delete(expanding, ref)
		return expand(definition.Schema, definitions, expanding)
	}
	out := s
	if len(s.Properties) > 0 {
		out.Properties = make(map[string]spec.Schema, len(s.Properties))
		for name, property := range s.Properties {
			out.Properties[name] = expand(property, definitions, expanding)
		}
	}
	if s.AdditionalProperties != nil && s.AdditionalProperties.Schema != nil {
		values := *s.AdditionalProperties
		child := expand(*values.Schema, definitions, expanding)
		values.Schema = &child
		out.AdditionalProperties = &values
	}
	if s.Items != nil && s.Items.Schema != nil {
		items := *s.Items
		child := expand(*items.Schema, definitions, expanding)
		items.Schema = &child
		out.Items = &items
	}
	return out
}

func structural(raw []byte) (*structuralschema.Structural, error) {
	external := &apiextensionsv1.JSONSchemaProps{}
	if err := json.Unmarshal(raw, external); err != nil {
		return nil, fmt.Errorf("reading the schema: %w", err)
	}
	internal := &apiextensions.JSONSchemaProps{}
	if err := apiextensionsv1.Convert_v1_JSONSchemaProps_To_apiextensions_JSONSchemaProps(external, internal, nil); err != nil {
		return nil, fmt.Errorf("converting the schema: %w", err)
	}
	built, err := structuralschema.NewStructural(internal)
	if err != nil {
		return nil, fmt.Errorf("the schema is not structural: %w", err)
	}
	if errs := structuralschema.ValidateStructural(field.NewPath(""), built); len(errs) > 0 {
		return nil, fmt.Errorf("the schema is not structural: %w", errs.ToAggregate())
	}
	return built, nil
}

// RoundTrip checks that pruning a Middleware object whose spec holds
// configuration in member leaves it unchanged.
func RoundTrip(s *structuralschema.Structural, tenantType, member string, configuration json.RawMessage) error {
	var decoded any
	if err := json.Unmarshal(configuration, &decoded); err != nil {
		return fmt.Errorf("tenant type %q: the example is not JSON", tenantType)
	}
	object := map[string]any{"spec": map[string]any{"type": tenantType, "enabled": true, member: decoded}}
	pruned := runtime.DeepCopyJSON(object)
	structuralpruning.Prune(pruned, s, true)
	if !reflect.DeepEqual(object, pruned) {
		return fmt.Errorf("tenant type %q: the platform's pruning changes the example", tenantType)
	}
	return nil
}
