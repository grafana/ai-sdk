package extensionhost

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/grafana/ai-sdk/ai-gateway/pkg/extension"
	"github.com/grafana/ai-sdk/ai-gateway/pkg/internal/config"
)

var emptyConfig = json.RawMessage(`{}`)

// Plan is the validated configuration of every extension to construct.
// Secrets are read separately, after every other setting is validated.
type Plan struct {
	enabled []planned
	// tenant holds every tenant-scope definition, enabled or not, in name
	// order.
	tenant []extension.Definition
}

type planned struct {
	definition extension.Definition
	config     json.RawMessage
	secretEnv  map[string]string
	secrets    map[string]string
}

// NewPlan applies the operator extension setting's entries to valid
// definitions. Operator extensions without an entry use their default state.
// Every tenant extension is planned with an empty configuration, since any
// namespace may enable it.
// Mandatory extensions must have an enabled entry. Failures name the setting,
// the extension and the field, never their values.
func NewPlan(definitions []extension.Definition, mandatory []string, entries map[string]config.ExtensionEntry) (*Plan, error) {
	setting := config.SettingName(config.ExtensionsSetting)
	byName := make(map[string]extension.Definition, len(definitions))
	for _, definition := range definitions {
		byName[definition.Name] = definition
	}
	for _, name := range slices.Sorted(maps.Keys(entries)) {
		definition, ok := byName[name]
		if !ok {
			return nil, fmt.Errorf("config: %s.%s is not a registered extension", setting, name)
		}
		if definition.Scope != extension.ScopeOperator {
			return nil, fmt.Errorf("config: %s.%s is a tenant extension, which this setting does not configure", setting, name)
		}
		for _, secret := range slices.Sorted(maps.Keys(entries[name].Secrets)) {
			if !slices.Contains(definition.Secrets, secret) {
				return nil, fmt.Errorf("config: %s.%s.secrets.%s is not a declared secret", setting, name, secret)
			}
		}
	}
	plan := &Plan{}
	for _, definition := range definitions {
		if definition.Scope == extension.ScopeTenant {
			plan.tenant = append(plan.tenant, definition)
		}
	}
	slices.SortFunc(plan.tenant, func(a, b extension.Definition) int { return strings.Compare(a.Name, b.Name) })
	for _, definition := range definitions {
		if definition.Scope == extension.ScopeTenant {
			plan.enabled = append(plan.enabled, planned{definition: definition, config: emptyConfig})
			continue
		}
		entry, configured := entries[definition.Name]
		enabled := definition.EnabledByDefault
		if configured {
			enabled = entry.Enabled == nil || *entry.Enabled
		}
		if slices.Contains(mandatory, definition.Name) && (!configured || !enabled) {
			return nil, fmt.Errorf("config: %s.%s is mandatory and must be configured and enabled", setting, definition.Name)
		}
		if !enabled {
			continue
		}
		path := setting + "." + definition.Name
		configuration := emptyConfig
		if entry.Config != nil {
			encoded, err := json.Marshal(entry.Config)
			if err != nil {
				return nil, fmt.Errorf("config: %s.config is not representable as JSON", path)
			}
			configuration = encoded
		}
		if err := CheckConfig(definition, configuration); err != nil {
			return nil, fmt.Errorf("config: %s.config %w", path, err)
		}
		for _, secret := range definition.Secrets {
			if strings.TrimSpace(entry.Secrets[secret]) == "" {
				return nil, fmt.Errorf("config: %s.secrets.%s must name an environment variable", path, secret)
			}
		}
		plan.enabled = append(plan.enabled, planned{definition: definition, config: configuration, secretEnv: entry.Secrets})
	}
	return plan, nil
}

// CheckConfig checks configuration, a JSON object, against definition's
// schema. A definition without a schema accepts only an empty object. The
// error names the deepest failing instance location, never a value or the
// schema validator's message, so callers can prefix it with their own field
// path. A location names object keys: with closed schemas, as tenant types
// use, only declared properties and array indexes; an operator schema that
// allows maps can surface the operator's own keys.
func CheckConfig(definition extension.Definition, configuration json.RawMessage) error {
	if definition.ConfigSchema.JSON() == nil {
		if string(configuration) != string(emptyConfig) {
			return errors.New("is not accepted: the extension takes no configuration")
		}
		return nil
	}
	err := definition.ConfigSchema.Validate(configuration)
	if err == nil {
		return nil
	}
	location := "/"
	var validation *jsonschema.ValidationError
	if errors.As(err, &validation) {
		location = "/" + strings.Join(deepestLocation(validation), "/")
	}
	return fmt.Errorf("does not match the extension's schema at %s", location)
}

func deepestLocation(validation *jsonschema.ValidationError) []string {
	for len(validation.Causes) != 0 {
		validation = validation.Causes[0]
	}
	return validation.InstanceLocation
}

// ResolveSecrets reads every enabled extension's declared secrets. A missing
// or empty variable fails naming the variable, not its value.
func (p *Plan) ResolveSecrets(lookupEnv config.LookupEnv) error {
	setting := config.SettingName(config.ExtensionsSetting)
	for i := range p.enabled {
		planned := &p.enabled[i]
		planned.secrets = make(map[string]string, len(planned.definition.Secrets))
		for _, secret := range planned.definition.Secrets {
			variable := planned.secretEnv[secret]
			value, ok := lookupEnv(variable)
			if !ok || strings.TrimSpace(value) == "" {
				return fmt.Errorf("config: %s.%s.secrets.%s names %q, which is unset or empty", setting, planned.definition.Name, secret, variable)
			}
			planned.secrets[secret] = value
		}
	}
	return nil
}

// Secrets returns every resolved secret value, for redaction.
func (p *Plan) Secrets() []string {
	var secrets []string
	for _, planned := range p.enabled {
		for _, secret := range planned.definition.Secrets {
			if value, ok := planned.secrets[secret]; ok {
				secrets = append(secrets, value)
			}
		}
	}
	return secrets
}
