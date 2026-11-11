package extensionhost

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/grafana/ai-sdk/ai-gateway/pkg/extension"
	"github.com/grafana/ai-sdk/ai-gateway/pkg/internal/config"
)

var emptyConfig = json.RawMessage(`{}`)

// Plan is the validated configuration of every enabled extension. Secrets are
// read separately, after every other setting is validated.
type Plan struct {
	enabled []planned
}

type planned struct {
	definition extension.Definition
	config     json.RawMessage
	secretEnv  map[string]string
	secrets    map[string]string
}

// NewPlan applies the operator extension setting's entries to valid
// definitions. Operator extensions without an entry use their default state;
// tenant extensions run when enabled by default, with an empty configuration.
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
		for _, secret := range slices.Sorted(maps.Keys(entries[name].SecretEnv)) {
			if !slices.Contains(definition.Secrets, secret) {
				return nil, fmt.Errorf("config: %s.%s.%sEnv is not a declared secret", setting, name, secret)
			}
		}
	}
	plan := &Plan{}
	for _, definition := range definitions {
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
		configuration := entry.Config
		if configuration == nil {
			configuration = emptyConfig
		}
		if err := CheckConfig(definition, configuration); err != nil {
			return nil, fmt.Errorf("config: %s.config %w", path, err)
		}
		for _, secret := range definition.Secrets {
			if _, ok := entry.SecretEnv[secret]; !ok {
				return nil, fmt.Errorf("config: %s.%sEnv is required", path, secret)
			}
		}
		plan.enabled = append(plan.enabled, planned{definition: definition, config: configuration, secretEnv: entry.SecretEnv})
	}
	return plan, nil
}

// CheckConfig checks configuration, a JSON object, against definition's
// schema. A definition without a schema accepts only an empty object. The
// error names the deepest failing instance location, never a value or the
// schema validator's message, so callers can prefix it with their own field
// path.
func CheckConfig(definition extension.Definition, configuration json.RawMessage) error {
	if definition.ConfigSchema.JSON() == nil {
		if string(configuration) != string(emptyConfig) {
			return errors.New("is not accepted: the extension takes no configuration")
		}
		return nil
	}
	compiled, err := exactSchema(definition.ConfigSchema.JSON())
	if err != nil {
		return errors.New("does not match the extension's schema at /")
	}
	instance, err := jsonschema.UnmarshalJSON(bytes.NewReader(configuration))
	if err != nil {
		return errors.New("does not match the extension's schema at /")
	}
	err = compiled.Validate(instance)
	if err == nil {
		return nil
	}
	location := "/"
	var validation *jsonschema.ValidationError
	if errors.As(err, &validation) {
		location = "/" + strings.Join(redactKeys(definition.ConfigSchema.JSON(), instance, deepestLocation(validation)), "/")
	}
	return fmt.Errorf("does not match the extension's schema at %s", location)
}

// exactSchemas caches configuration schemas compiled for exact validation, by
// their JSON. Definitions are fixed for the process, so it stays bounded.
var exactSchemas sync.Map

// exactSchema compiles raw as the schema package does. Validating against it
// with an instance decoded by [jsonschema.UnmarshalJSON] compares numbers
// exactly, so bounds, enums and multiples above 2^53 are checked against the
// value that is stored.
func exactSchema(raw json.RawMessage) (*jsonschema.Schema, error) {
	if compiled, ok := exactSchemas.Load(string(raw)); ok {
		return compiled.(*jsonschema.Schema), nil
	}
	document, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	if err := compiler.AddResource("schema.json", document); err != nil {
		return nil, err
	}
	compiled, err := compiler.Compile("schema.json")
	if err != nil {
		return nil, err
	}
	exactSchemas.Store(string(raw), compiled)
	return compiled, nil
}

// redactKeys replaces the location segments that aren't declared by the
// schema with "*": keys of a typed map, and every segment the walk can't
// follow. Indexes into arrays and declared property names of objects are
// kept, so a location never names configuration data.
func redactKeys(raw json.RawMessage, instance any, location []string) []string {
	var root map[string]any
	_ = json.Unmarshal(raw, &root)
	node := root
	redacted := make([]string, len(location))
	for i, segment := range location {
		node = resolveReference(root, node)
		properties, _ := node["properties"].(map[string]any)
		items, _ := node["items"].(map[string]any)
		property, declared := properties[segment].(map[string]any)
		switch value := instance.(type) {
		case map[string]any:
			instance = value[segment]
			if declared {
				redacted[i], node = segment, property
				continue
			}
		case []any:
			if index, err := strconv.Atoi(segment); err == nil && index >= 0 && index < len(value) && strconv.Itoa(index) == segment {
				redacted[i], node, instance = segment, items, value[index]
				continue
			}
			instance = nil
		default:
			instance = nil
		}
		values, _ := node["additionalProperties"].(map[string]any)
		redacted[i], node = "*", values
	}
	return redacted
}

func resolveReference(root, node map[string]any) map[string]any {
	ref, _ := node["$ref"].(string)
	for _, key := range []string{"$defs", "definitions"} {
		if name, ok := strings.CutPrefix(ref, "#/"+key+"/"); ok {
			definitions, _ := root[key].(map[string]any)
			target, _ := definitions[name].(map[string]any)
			return target
		}
	}
	return node
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
				return fmt.Errorf("config: %s.%s.%sEnv %q is unset or empty", setting, planned.definition.Name, secret, variable)
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
