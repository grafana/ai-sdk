package tenanttype

import (
	"encoding/json"
	"slices"
	"strings"

	"github.com/grafana/ai-sdk/ai-gateway/pkg/extension"
	"github.com/grafana/ai-sdk/schema"
)

// Type is a public tenant extension type.
type Type struct {
	// Name is the extension name, the Middleware resource's spec.type and
	// metadata.name. It follows the extension name rule.
	Name string
	// Description describes the type's member in the manifest.
	Description string
	// ConfigSchema is the JSON Schema of the type's configuration. The zero
	// value means the type takes no configuration and has no member.
	ConfigSchema schema.Schema
	// EnabledByDefault is the extension's state in a namespace that stores no
	// resource for it.
	EnabledByDefault bool
	// Example is a configuration the schema accepts. It documents the type
	// and proves that the manifest member stores it unchanged. It is nil when
	// the type takes no configuration.
	Example json.RawMessage
}

// Implementation is how a build implements a type.
type Implementation struct {
	// Keys lists the key ownerships of the calls the extension applies to. Nil
	// applies to both.
	Keys []extension.KeyOwnership
	// Sources narrows the calls the extension applies to. Nil applies to every
	// source whose ownership is in Keys.
	Sources []extension.Source
	// Priority orders request transforms among tenant-scope extensions.
	Priority      int
	FailurePolicy extension.FailurePolicy
	// New constructs an instance from a validated configuration, as
	// [extension.Definition.New] does.
	New func(config json.RawMessage, deps extension.Dependencies) (any, error)
}

var registry = []Type{}

// All returns the public tenant types in name order. It returns a copy.
func All() []Type {
	types := slices.Clone(registry)
	slices.SortFunc(types, func(a, b Type) int { return strings.Compare(a.Name, b.Name) })
	return types
}

// Member returns the name of the spec member that holds the type's
// configuration: the lowerCamel form of the name, so "default-instructions"
// becomes "defaultInstructions".
func (t Type) Member() string {
	var member strings.Builder
	for i, part := range strings.Split(t.Name, "-") {
		if i > 0 && part != "" && part[0] >= 'a' && part[0] <= 'z' {
			part = string(part[0]-'a'+'A') + part[1:]
		}
		member.WriteString(part)
	}
	return member.String()
}

// Bind returns the tenant-scope definition of impl for t, with t's name,
// configuration schema and default state.
func (t Type) Bind(impl Implementation) extension.Definition {
	return extension.Definition{
		Name:             t.Name,
		Scope:            extension.ScopeTenant,
		Keys:             impl.Keys,
		Sources:          impl.Sources,
		Priority:         impl.Priority,
		FailurePolicy:    impl.FailurePolicy,
		ConfigSchema:     t.ConfigSchema,
		EnabledByDefault: t.EnabledByDefault,
		New:              impl.New,
	}
}
