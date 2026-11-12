package extensionhost

import (
	"bytes"
	"fmt"
	"regexp"
	"slices"

	"github.com/grafana/ai-sdk/ai-gateway/pkg/extension"
	"github.com/grafana/ai-sdk/ai-gateway/pkg/extension/tenanttype"
)

var (
	namePattern       = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)
	secretNamePattern = regexp.MustCompile(`^[a-z][A-Za-z0-9]{0,62}$`)
)

// reservedMembers are the Middleware spec fields a tenant type's member must
// not shadow.
var reservedMembers = []string{"type", "enabled"}

// ValidateDefinitions checks the public tenant types, a distribution's
// definitions and its mandatory extension names without reading any setting.
// Every tenant-scope definition must bind to a public type: same name,
// configuration schema and default state.
func ValidateDefinitions(definitions []extension.Definition, mandatory []string, types []tenanttype.Type) error {
	typesByName, err := validateTypes(types)
	if err != nil {
		return err
	}
	byName := make(map[string]extension.Definition, len(definitions))
	for _, definition := range definitions {
		if err := validateDefinition(definition); err != nil {
			return err
		}
		if _, duplicate := byName[definition.Name]; duplicate {
			return fmt.Errorf("extensions: extension name %q is registered twice", definition.Name)
		}
		byName[definition.Name] = definition
	}
	for i, name := range mandatory {
		definition, ok := byName[name]
		if !ok {
			return fmt.Errorf("extensions: mandatory extension %q is not registered", name)
		}
		if definition.Scope != extension.ScopeOperator {
			return fmt.Errorf("extensions: mandatory extension %q must have operator scope", name)
		}
		if slices.Contains(mandatory[:i], name) {
			return fmt.Errorf("extensions: mandatory extension %q is listed twice", name)
		}
	}
	for _, definition := range definitions {
		if definition.Scope != extension.ScopeTenant {
			continue
		}
		if err := checkBinding(definition, typesByName); err != nil {
			return err
		}
	}
	return nil
}

func validateTypes(types []tenanttype.Type) (map[string]tenanttype.Type, error) {
	byName := make(map[string]tenanttype.Type, len(types))
	members := make(map[string]string, len(types))
	for _, tenantType := range types {
		if !namePattern.MatchString(tenantType.Name) {
			return nil, fmt.Errorf("extensions: tenant type name %q must be 1 to 63 lowercase letters, digits or hyphens, starting and ending with a letter or digit", tenantType.Name)
		}
		if _, duplicate := byName[tenantType.Name]; duplicate {
			return nil, fmt.Errorf("extensions: tenant type %q is declared twice", tenantType.Name)
		}
		byName[tenantType.Name] = tenantType
		if tenantType.ConfigSchema.JSON() == nil {
			if tenantType.Example != nil {
				return nil, fmt.Errorf("extensions: tenant type %q has an example but no configuration schema", tenantType.Name)
			}
			continue
		}
		member := tenantType.Member()
		if slices.Contains(reservedMembers, member) {
			return nil, fmt.Errorf("extensions: tenant type %q has the reserved member %q", tenantType.Name, member)
		}
		if other, taken := members[member]; taken {
			return nil, fmt.Errorf("extensions: tenant types %q and %q share the member %q", other, tenantType.Name, member)
		}
		members[member] = tenantType.Name
		if tenantType.ConfigSchema.Compiled() == nil {
			return nil, fmt.Errorf("extensions: tenant type %q has an invalid configuration schema", tenantType.Name)
		}
		if tenantType.Example == nil || CheckConfig(extension.Definition{ConfigSchema: tenantType.ConfigSchema}, tenantType.Example) != nil {
			return nil, fmt.Errorf("extensions: tenant type %q needs an example its configuration schema accepts", tenantType.Name)
		}
	}
	return byName, nil
}

func checkBinding(definition extension.Definition, types map[string]tenanttype.Type) error {
	tenantType, ok := types[definition.Name]
	if !ok {
		return fmt.Errorf("extensions: tenant extension %q has no public tenant type", definition.Name)
	}
	if !bytes.Equal(definition.ConfigSchema.JSON(), tenantType.ConfigSchema.JSON()) {
		return fmt.Errorf("extensions: tenant extension %q differs from its public tenant type in its configuration schema", definition.Name)
	}
	if definition.EnabledByDefault != tenantType.EnabledByDefault {
		return fmt.Errorf("extensions: tenant extension %q differs from its public tenant type in its default state", definition.Name)
	}
	return nil
}

func validateDefinition(definition extension.Definition) error {
	if !namePattern.MatchString(definition.Name) {
		return fmt.Errorf("extensions: extension name %q must be 1 to 63 lowercase letters, digits or hyphens, starting and ending with a letter or digit", definition.Name)
	}
	invalid := func(field string) error {
		return fmt.Errorf("extensions: extension %q has an invalid %s", definition.Name, field)
	}
	switch definition.Scope {
	case extension.ScopeOperator, extension.ScopeTenant:
	default:
		return invalid("scope")
	}
	switch definition.FailurePolicy {
	case extension.FailClosed, extension.FailSkip:
	default:
		return invalid("failure policy")
	}
	if definition.New == nil {
		return invalid("factory")
	}
	for i, keys := range definition.Keys {
		if (keys != extension.KeysOperator && keys != extension.KeysCustomer) || slices.Contains(definition.Keys[:i], keys) {
			return invalid("key ownership")
		}
	}
	for i, secret := range definition.Secrets {
		if !secretNamePattern.MatchString(secret) || slices.Contains(definition.Secrets[:i], secret) {
			return invalid("secret name")
		}
	}
	if definition.ConfigSchema.JSON() != nil && definition.ConfigSchema.Compiled() == nil {
		return invalid("configuration schema")
	}
	return nil
}

// applies reports whether definition applies to calls whose keys are owned by
// keys.
func applies(definition extension.Definition, keys extension.KeyOwnership) bool {
	return len(definition.Keys) == 0 || slices.Contains(definition.Keys, keys)
}
