package tenantschema

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strconv"

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

// MaxSafeInteger is the largest integer a float64, and a JavaScript number,
// represents exactly. A bound an integer field declares must be within
// ±MaxSafeInteger; values beyond it aren't compared exactly, as upstream.
const MaxSafeInteger = 1<<53 - 1

// droppedKeywords are value constraints, defaults and annotations that
// admission checks or that the manifest doesn't need.
var droppedKeywords = []string{
	"enum", "const", "pattern", "format",
	"minimum", "maximum", "exclusiveMinimum", "exclusiveMaximum", "multipleOf",
	"minLength", "maxLength", "minItems", "maxItems", "minProperties", "maxProperties", "uniqueItems",
	"default", "examples", "title", "deprecated", "readOnly", "writeOnly",
	"contentEncoding", "contentMediaType", "$schema", "$id", "$comment",
}

var keptKeywords = []string{"type", "description", "properties", "required", "items", "additionalProperties"}

// Node is a projected schema node: exactly one JSON type, and for objects
// declared properties.
type Node struct {
	Type        JSONType
	Description string
	Properties  map[string]*Node
	Required    []string
	Items       *Node
}

// Member is a tenant type's projected spec member. Its schema's description
// is the type's.
type Member struct {
	Type   string
	Name   string
	Schema *Node
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
	node, err := project(root, "")
	if err != nil {
		return Member{}, false, fmt.Errorf("tenant type %q: the configuration schema %w", tenantType.Name, err)
	}
	if node.Type != TypeObject {
		return Member{}, false, fmt.Errorf("tenant type %q: the configuration schema must be an object", tenantType.Name)
	}
	if tenantType.Description != "" {
		node.Description = tenantType.Description
	}
	return Member{Type: tenantType.Name, Name: tenantType.Member(), Schema: node}, true, nil
}

func project(schema map[string]any, location string) (*Node, error) {
	at := func(format string, args ...any) error {
		return fmt.Errorf("at %s "+format, append([]any{locationOrRoot(location)}, args...)...)
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
	if node.Type != TypeObject {
		for _, keyword := range []string{"properties", "required", "additionalProperties"} {
			if _, ok := schema[keyword]; ok {
				return nil, at("declares %q on a %s", keyword, node.Type)
			}
		}
	}
	if _, ok := schema["items"]; ok && node.Type != TypeArray {
		return nil, at("declares \"items\" on a %s", node.Type)
	}
	switch node.Type {
	case TypeInteger:
		for _, bound := range []string{"minimum", "maximum"} {
			if value, declared := schema[bound]; declared && !safeInteger(value) {
				return nil, at("must set %s to an integer within ±%d", bound, MaxSafeInteger)
			}
		}
	case TypeArray:
		items, ok := schema["items"].(map[string]any)
		if !ok {
			return nil, at("must declare items as one schema")
		}
		child, err := project(items, location+"/items")
		if err != nil {
			return nil, err
		}
		node.Items = child
	case TypeObject:
		if err := object(node, schema, location); err != nil {
			return nil, err
		}
	}
	return node, nil
}

func object(node *Node, schema map[string]any, location string) error {
	at := func(format string, args ...any) error {
		return fmt.Errorf("at %s "+format, append([]any{locationOrRoot(location)}, args...)...)
	}
	properties, _ := schema["properties"].(map[string]any)
	if closed, ok := schema["additionalProperties"].(bool); len(properties) == 0 || !ok || closed {
		return at("must declare properties and set additionalProperties to false")
	}
	node.Properties = make(map[string]*Node, len(properties))
	for _, name := range slices.Sorted(maps.Keys(properties)) {
		property, ok := properties[name].(map[string]any)
		if !ok {
			return at("must declare property %q as a schema", name)
		}
		child, err := project(property, location+"/properties/"+name)
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
	return nil
}

func safeInteger(value any) bool {
	number, ok := value.(json.Number)
	if !ok {
		return false
	}
	bound, err := strconv.ParseInt(number.String(), 10, 64)
	return err == nil && bound >= -MaxSafeInteger && bound <= MaxSafeInteger
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
