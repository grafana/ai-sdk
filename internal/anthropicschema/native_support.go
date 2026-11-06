package anthropicschema

import (
	"net/url"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"
)

// maxNativeOptionalProperties is the optional-property budget for native
// structured outputs. Anthropic rejects schemas above 24 optional properties
// and grammar compilation fails earlier (from 13 on Bedrock and the Anthropic
// API, from 11 on Vertex).
const maxNativeOptionalProperties = 10

var (
	statsSubschemaKeywords     = []string{"additionalProperties", "contains", "else", "if", "items", "not", "then"}
	statsSubschemaListKeywords = []string{"allOf", "anyOf", "oneOf", "prefixItems"}
	schemaMapKeywords          = []string{"dependentSchemas", "patternProperties", "properties"}
	schemaDataKeywords         = []string{"const", "default", "enum", "examples"}
)

// SupportsNativeStructuredOutput reports whether Anthropic native structured
// outputs (`output_config.format`) can represent schema without rejecting it
// or losing data. Self-referencing schemas and schemas with more than ten
// optional properties are rejected; open objects (maps, free-form objects,
// pattern properties) are lossy because native output closes them. Mirrors
// upstream supports-native-structured-output-schema.ts.
func SupportsNativeStructuredOutput(schema map[string]any) bool {
	if HasCircularReference(schema) {
		return false
	}
	stats := nativeSchemaStats(schema, schema, map[string]schemaStats{})
	return !stats.hasOpenObject && stats.optionalProperties <= maxNativeOptionalProperties
}

type schemaStats struct {
	optionalProperties int
	hasOpenObject      bool
}

func (s *schemaStats) add(other schemaStats) {
	s.optionalProperties += other.optionalProperties
	s.hasOpenObject = s.hasOpenObject || other.hasOpenObject
}

func nativeSchemaStats(node any, root map[string]any, refStats map[string]schemaStats) schemaStats {
	var stats schemaStats
	record, ok := node.(map[string]any)
	if !ok {
		return stats
	}
	addChild := func(child any) { stats.add(nativeSchemaStats(child, root, refStats)) }

	if ref, ok := record["$ref"].(string); ok && strings.HasPrefix(ref, "#") {
		resolved, seen := refStats[ref]
		if !seen {
			resolved = nativeSchemaStats(resolveSchemaPointer(root, ref), root, refStats)
			refStats[ref] = resolved
		}
		stats.add(resolved)
	}

	if isOpenObject(record) {
		stats.hasOpenObject = true
	}

	if properties, ok := record["properties"].(map[string]any); ok {
		required := map[string]bool{}
		if list, ok := record["required"].([]any); ok {
			for _, name := range list {
				if s, ok := name.(string); ok {
					required[s] = true
				}
			}
		}
		for name := range properties {
			if !required[name] {
				stats.optionalProperties++
			}
		}
	}

	for _, keyword := range statsSubschemaKeywords {
		if list, ok := record[keyword].([]any); ok {
			for _, child := range list {
				addChild(child)
			}
		} else {
			addChild(record[keyword])
		}
	}
	for _, keyword := range statsSubschemaListKeywords {
		if list, ok := record[keyword].([]any); ok {
			for _, child := range list {
				addChild(child)
			}
		}
	}
	for _, keyword := range schemaMapKeywords {
		if children, ok := record[keyword].(map[string]any); ok {
			for _, child := range children {
				addChild(child)
			}
		}
	}
	return stats
}

func isOpenObject(node map[string]any) bool {
	additional, hasAdditional := node["additionalProperties"]
	if b, ok := additional.(bool); ok && b {
		return true
	}
	if _, ok := additional.(map[string]any); ok {
		return true
	}
	if patterns, ok := node["patternProperties"].(map[string]any); ok && len(patterns) > 0 {
		return true
	}

	isObjectType := false
	switch t := node["type"].(type) {
	case string:
		isObjectType = t == "object"
	case []any:
		for _, v := range t {
			if v == "object" {
				isObjectType = true
			}
		}
	}
	closed := false
	if b, ok := additional.(bool); ok && hasAdditional && !b {
		closed = true
	}
	return isObjectType && node["properties"] == nil && !closed
}

// HasCircularReference reports whether schema contains a cycle of local
// `$ref`s reachable from the root, such as a tree node whose children
// reference the node itself. Mirrors upstream has-circular-schema-reference.ts.
func HasCircularReference(schema map[string]any) bool {
	visiting := map[string]bool{}
	visited := map[string]bool{}

	var visit func(pointer string) bool
	visit = func(pointer string) bool {
		if visiting[pointer] {
			return true
		}
		if visited[pointer] {
			return false
		}
		target := resolveSchemaPointer(schema, pointer)
		if target == nil {
			return false
		}
		visiting[pointer] = true
		circular := false
		for _, ref := range collectLocalRefs(target, nil) {
			if visit(ref) {
				circular = true
				break
			}
		}
		delete(visiting, pointer)
		visited[pointer] = true
		return circular
	}
	return visit("#")
}

func collectLocalRefs(node any, refs []string) []string {
	switch n := node.(type) {
	case []any:
		for _, item := range n {
			refs = collectLocalRefs(item, refs)
		}
	case map[string]any:
		for key, value := range n {
			switch {
			case key == "$ref":
				if ref, ok := value.(string); ok && strings.HasPrefix(ref, "#") {
					refs = append(refs, ref)
				}
			case slices.Contains(schemaMapKeywords, key):
				if children, ok := value.(map[string]any); ok {
					for _, child := range children {
						refs = collectLocalRefs(child, refs)
					}
				} else {
					refs = collectLocalRefs(value, refs)
				}
			case key != "$defs" && key != "definitions" && !slices.Contains(schemaDataKeywords, key):
				refs = collectLocalRefs(value, refs)
			}
		}
	}
	return refs
}

func resolveSchemaPointer(schema map[string]any, pointer string) any {
	if pointer == "#" {
		return schema
	}
	if !strings.HasPrefix(pointer, "#/") {
		return nil
	}
	decoded, err := url.PathUnescape(pointer[2:])
	if err != nil || !utf8.ValidString(decoded) {
		return nil
	}

	var current any = schema
	for _, raw := range strings.Split(decoded, "/") {
		segment := strings.ReplaceAll(strings.ReplaceAll(raw, "~1", "/"), "~0", "~")
		switch c := current.(type) {
		case []any:
			index, err := strconv.Atoi(segment)
			if err != nil || index < 0 || index >= len(c) {
				return nil
			}
			current = c[index]
		case map[string]any:
			current = c[segment]
		default:
			return nil
		}
	}
	return current
}
