package extensionhost

import (
	"fmt"
	"regexp"
	"slices"

	"github.com/grafana/ai-sdk/ai-gateway/pkg/extension"
)

var (
	namePattern       = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)
	secretNamePattern = regexp.MustCompile(`^[a-z][A-Za-z0-9]{0,62}$`)
)

// ValidateDefinitions checks a distribution's definitions and its mandatory
// extension names without reading any setting.
func ValidateDefinitions(definitions []extension.Definition, mandatory []string) error {
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
