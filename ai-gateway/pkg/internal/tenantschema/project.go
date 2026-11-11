package tenantschema

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/grafana/ai-sdk/ai-gateway/pkg/extension/tenanttype"
)

// JSONType is a JSON Schema type a projected node may have.
type JSONType string

// The JSON types a projection keeps. null is refused.
const (
	TypeString  JSONType = "string"
	TypeInteger JSONType = "integer"
	TypeNumber  JSONType = "number"
	TypeBoolean JSONType = "boolean"
	TypeArray   JSONType = "array"
	TypeObject  JSONType = "object"
)

var jsonTypes = []JSONType{TypeString, TypeInteger, TypeNumber, TypeBoolean, TypeArray, TypeObject}

// droppedKeywords are value constraints, defaults and annotations that
// admission checks or that the manifest doesn't need.
var droppedKeywords = []string{
	"enum", "const", "pattern", "format",
	"minimum", "maximum", "exclusiveMinimum", "exclusiveMaximum", "multipleOf",
	"minLength", "maxLength", "minItems", "maxItems", "minProperties", "maxProperties", "uniqueItems",
	"default", "examples", "title", "deprecated", "readOnly", "writeOnly",
	"contentEncoding", "contentMediaType", "$schema", "$id", "$comment", "$defs", "definitions",
}

var keptKeywords = []string{"type", "description", "properties", "required", "items", "additionalProperties"}

// Node is a projected schema node: exactly one JSON type, and for objects
// either declared properties or a typed value schema.
type Node struct {
	Type        JSONType
	Description string
	Properties  map[string]*Node
	Required    []string
	Items       *Node
	Values      *Node
}

// Member is a tenant type's projected spec member.
type Member struct {
	Type        string
	Name        string
	Description string
	Schema      *Node
}

// Project returns the member of a type with a configuration schema. A type
// without one has no member, and Project returns false.
func Project(tenantType tenanttype.Type) (Member, bool, error) {
	raw := tenantType.ConfigSchema.JSON()
	if raw == nil {
		return Member{}, false, nil
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var root map[string]any
	if err := decoder.Decode(&root); err != nil {
		return Member{}, false, fmt.Errorf("tenant type %q: the configuration schema is not a JSON object", tenantType.Name)
	}
	p := projector{root: root, resolving: map[string]bool{}}
	node, err := p.node(root, "")
	if err != nil {
		return Member{}, false, fmt.Errorf("tenant type %q: the configuration schema %w", tenantType.Name, err)
	}
	if node.Type != TypeObject {
		return Member{}, false, fmt.Errorf("tenant type %q: the configuration schema must be an object", tenantType.Name)
	}
	return Member{Type: tenantType.Name, Name: tenantType.Member(), Description: tenantType.Description, Schema: node}, true, nil
}

type projector struct {
	root      map[string]any
	resolving map[string]bool
}

func (p projector) node(schema map[string]any, location string) (*Node, error) {
	at := func(format string, args ...any) error {
		return fmt.Errorf("at %s "+format, append([]any{locationOrRoot(location)}, args...)...)
	}
	if ref, ok := schema["$ref"]; ok {
		return p.reference(schema, ref, location)
	}
	for _, keyword := range slices.Sorted(maps.Keys(schema)) {
		if !slices.Contains(keptKeywords, keyword) && !slices.Contains(droppedKeywords, keyword) {
			return nil, at("uses %q, which a tenant type schema can't use", keyword)
		}
	}
	name, ok := schema["type"].(string)
	if !ok || !slices.Contains(jsonTypes, JSONType(name)) {
		return nil, at("must have exactly one type among string, integer, number, boolean, array and object")
	}
	node := &Node{Type: JSONType(name)}
	if description, ok := schema["description"].(string); ok {
		node.Description = description
	}
	switch node.Type {
	case TypeArray:
		items, ok := schema["items"].(map[string]any)
		if !ok {
			return nil, at("must declare items as one schema")
		}
		child, err := p.node(items, location+"/items")
		if err != nil {
			return nil, err
		}
		node.Items = child
	case TypeObject:
		if err := p.object(node, schema, location); err != nil {
			return nil, err
		}
	default:
		for _, keyword := range []string{"properties", "required", "items", "additionalProperties"} {
			if _, ok := schema[keyword]; ok {
				return nil, at("declares %q on a %s", keyword, node.Type)
			}
		}
	}
	return node, nil
}

func (p projector) object(node *Node, schema map[string]any, location string) error {
	at := func(format string, args ...any) error {
		return fmt.Errorf("at %s "+format, append([]any{locationOrRoot(location)}, args...)...)
	}
	properties, hasProperties := schema["properties"].(map[string]any)
	if _, declared := schema["properties"]; declared && !hasProperties {
		return at("must declare properties as an object")
	}
	additional, hasAdditional := schema["additionalProperties"]
	if _, isItems := schema["items"]; isItems {
		return at("declares \"items\" on an object")
	}
	switch {
	case len(properties) > 0:
		if closed, ok := additional.(bool); !hasAdditional || !ok || closed {
			return at("must set additionalProperties to false")
		}
		node.Properties = make(map[string]*Node, len(properties))
		for _, name := range slices.Sorted(maps.Keys(properties)) {
			property, ok := properties[name].(map[string]any)
			if !ok {
				return at("must declare property %q as a schema", name)
			}
			child, err := p.node(property, location+"/properties/"+name)
			if err != nil {
				return err
			}
			node.Properties[name] = child
		}
		required, err := requiredFields(schema["required"], properties)
		if err != nil {
			return at("%s", err)
		}
		node.Required = required
	case hasAdditional:
		values, ok := additional.(map[string]any)
		if !ok {
			return at("must declare properties, or additionalProperties as a schema")
		}
		if _, ok := schema["required"]; ok {
			return at("declares required without properties")
		}
		child, err := p.node(values, location+"/additionalProperties")
		if err != nil {
			return err
		}
		node.Values = child
	default:
		return at("must declare properties, or additionalProperties as a schema")
	}
	return nil
}

func requiredFields(value any, properties map[string]any) ([]string, error) {
	if value == nil {
		return nil, nil
	}
	list, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("must declare required as a list")
	}
	required := make([]string, 0, len(list))
	for _, entry := range list {
		name, ok := entry.(string)
		if _, declared := properties[name]; !ok || !declared {
			return nil, fmt.Errorf("requires a field it doesn't declare")
		}
		required = append(required, name)
	}
	slices.Sort(required)
	return slices.Compact(required), nil
}

func (p projector) reference(schema map[string]any, value any, location string) (*Node, error) {
	ref, ok := value.(string)
	for keyword := range schema {
		if keyword != "$ref" && keyword != "description" {
			ok = false
		}
	}
	if !ok {
		return nil, fmt.Errorf("at %s must use $ref alone", locationOrRoot(location))
	}
	target, found := p.resolve(ref)
	if !found {
		return nil, fmt.Errorf("at %s references %q, which isn't a local definition", locationOrRoot(location), ref)
	}
	if p.resolving[ref] {
		return nil, fmt.Errorf("at %s references %q recursively", locationOrRoot(location), ref)
	}
	p.resolving[ref] = true
	defer delete(p.resolving, ref)
	node, err := p.node(target, location)
	if err != nil {
		return nil, err
	}
	if description, ok := schema["description"].(string); ok {
		node.Description = description
	}
	return node, nil
}

func (p projector) resolve(ref string) (map[string]any, bool) {
	for _, prefix := range []string{"#/$defs/", "#/definitions/"} {
		name, ok := strings.CutPrefix(ref, prefix)
		if !ok {
			continue
		}
		definitions, _ := p.root[strings.TrimSuffix(strings.TrimPrefix(prefix, "#/"), "/")].(map[string]any)
		target, ok := definitions[name].(map[string]any)
		return target, ok
	}
	return nil, false
}

func locationOrRoot(location string) string {
	if location == "" {
		return "/"
	}
	return location
}

// JSONSchema returns the node as a JSON Schema object.
func (n *Node) JSONSchema() map[string]any {
	schema := map[string]any{"type": string(n.Type)}
	if n.Description != "" {
		schema["description"] = n.Description
	}
	switch {
	case n.Items != nil:
		schema["items"] = n.Items.JSONSchema()
	case n.Values != nil:
		schema["additionalProperties"] = n.Values.JSONSchema()
	case n.Properties != nil:
		properties := make(map[string]any, len(n.Properties))
		for name, property := range n.Properties {
			properties[name] = property.JSONSchema()
		}
		schema["properties"] = properties
		schema["additionalProperties"] = false
		if len(n.Required) > 0 {
			schema["required"] = n.Required
		}
	}
	return schema
}
