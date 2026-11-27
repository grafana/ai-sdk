package openai

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/grafana/ai-sdk/provider"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"
)

// OpenAIAdditionalToolType identifies a definition loaded from result history.
type OpenAIAdditionalToolType string

const (
	// OpenAIAdditionalToolFunction declares a function tool.
	OpenAIAdditionalToolFunction OpenAIAdditionalToolType = "function"
	// OpenAIAdditionalToolCustom declares a tool with free-form or grammar input.
	OpenAIAdditionalToolCustom OpenAIAdditionalToolType = "custom"
	// OpenAIAdditionalToolNamespace groups function tools.
	OpenAIAdditionalToolNamespace OpenAIAdditionalToolType = "namespace"
)

// OpenAIAdditionalTool is a native Responses definition, not an executable Go tool.
// Function tools require Parameters. Custom tools can specify Format. Namespaces
// require Description and at least one function in Tools.
type OpenAIAdditionalTool struct {
	Type         OpenAIAdditionalToolType    `json:"type"`
	Name         string                      `json:"name"`
	Description  *string                     `json:"description,omitempty"`
	Parameters   json.RawMessage             `json:"parameters,omitempty"`
	Strict       *bool                       `json:"strict,omitempty"`
	DeferLoading *bool                       `json:"defer_loading,omitempty"`
	Format       *OpenAIAdditionalToolFormat `json:"format,omitempty"`
	Tools        []OpenAIAdditionalTool      `json:"tools,omitzero"`
}

// OpenAIAdditionalToolFormatType identifies the input format of a custom tool.
type OpenAIAdditionalToolFormatType string

const (
	// OpenAIAdditionalToolFormatText accepts unconstrained text.
	OpenAIAdditionalToolFormatText OpenAIAdditionalToolFormatType = "text"
	// OpenAIAdditionalToolFormatGrammar uses a grammar definition.
	OpenAIAdditionalToolFormatGrammar OpenAIAdditionalToolFormatType = "grammar"
)

// OpenAIAdditionalToolGrammarSyntax identifies the grammar language.
type OpenAIAdditionalToolGrammarSyntax string

const (
	// OpenAIAdditionalToolGrammarRegex selects a regular expression.
	OpenAIAdditionalToolGrammarRegex OpenAIAdditionalToolGrammarSyntax = "regex"
	// OpenAIAdditionalToolGrammarLark selects a Lark grammar.
	OpenAIAdditionalToolGrammarLark OpenAIAdditionalToolGrammarSyntax = "lark"
)

// OpenAIAdditionalToolFormat describes the accepted input for a custom tool.
// A grammar format requires both Syntax and Definition.
type OpenAIAdditionalToolFormat struct {
	Type       OpenAIAdditionalToolFormatType    `json:"type"`
	Syntax     OpenAIAdditionalToolGrammarSyntax `json:"syntax,omitempty"`
	Definition *string                           `json:"definition,omitempty"`
}

func appendAdditionalTools(items []responses.ResponseInputItemUnionParam, part provider.ContentPart, namespace string) ([]responses.ResponseInputItemUnionParam, error) {
	raw, err := resultAdditionalToolsOption(part.ProviderOptions, namespace)
	if err == nil && raw == nil && namespace != providerName {
		namespace = providerName
		raw, err = resultAdditionalToolsOption(part.ProviderOptions, namespace)
	}
	if err != nil {
		return nil, fmt.Errorf("openai: %s.additionalTools: %w", namespace, err)
	}
	if raw == nil {
		return items, nil
	}
	var definitions []json.RawMessage
	if err := json.Unmarshal(raw, &definitions); err != nil || definitions == nil {
		return nil, fmt.Errorf("openai: %s.additionalTools must be an array", namespace)
	}
	if len(definitions) == 0 {
		return items, nil
	}
	tools := make([]responses.ToolUnionParam, len(definitions))
	for i, definition := range definitions {
		validated, err := validateAdditionalTool(definition, false)
		if err != nil {
			return nil, fmt.Errorf("openai: %s.additionalTools[%d]: %w", namespace, i, err)
		}
		tools[i] = param.Override[responses.ToolUnionParam](validated)
	}
	return append(items, responses.ResponseInputItemParamOfAdditionalTools(tools)), nil
}

func resultAdditionalToolsOption(opts provider.ProviderOptions, namespace string) (json.RawMessage, error) {
	var tools []OpenAIAdditionalTool
	switch opt := opts[namespace].(type) {
	case OpenAIPartOptions:
		tools = opt.AdditionalTools
	case *OpenAIPartOptions:
		if opt != nil {
			tools = opt.AdditionalTools
		}
	case provider.RawProviderOption:
		if len(opt.Raw) == 0 {
			return nil, nil
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(opt.Raw, &fields); err != nil {
			return nil, fmt.Errorf("expected a provider options object")
		}
		return fields["additionalTools"], nil
	}
	if tools == nil {
		return nil, nil
	}
	return json.Marshal(tools)
}

type additionalToolFields map[string]json.RawMessage

func (f additionalToolFields) matches(key, prefixes string, optional bool) bool {
	value := f[key]
	if len(value) == 0 {
		return optional
	}
	return strings.IndexByte(prefixes, value[0]) >= 0
}

func (f additionalToolFields) string(key string) string {
	var value string
	_ = json.Unmarshal(f[key], &value)
	return value
}

func (f additionalToolFields) allows(keys ...string) bool {
	for field := range f {
		if !slices.Contains(keys, field) {
			return false
		}
	}
	return true
}

func validateAdditionalTool(raw json.RawMessage, functionOnly bool) (additionalToolFields, error) {
	var fields additionalToolFields
	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
		return nil, fmt.Errorf("definition must be an object")
	}
	if fields.string("name") == "" || !fields.matches("description", `"`, true) {
		return nil, fmt.Errorf("expected a nonempty name and a string description if present")
	}
	typ := OpenAIAdditionalToolType(fields.string("type"))
	if functionOnly && typ != OpenAIAdditionalToolFunction {
		return nil, fmt.Errorf("namespace children must be function definitions")
	}
	switch typ {
	case OpenAIAdditionalToolFunction:
		if !fields.allows("type", "name", "description", "parameters", "strict", "defer_loading") ||
			!fields.matches("parameters", "{", false) || !fields.matches("strict", "tf", true) || !fields.matches("defer_loading", "tf", true) {
			return nil, fmt.Errorf("invalid function definition")
		}
	case OpenAIAdditionalToolCustom:
		if !fields.allows("type", "name", "description", "format") {
			return nil, fmt.Errorf("invalid custom definition")
		}
		if raw := fields["format"]; raw != nil {
			format, err := validateAdditionalToolFormat(raw)
			if err != nil {
				return nil, err
			}
			fields["format"], err = json.Marshal(format)
			if err != nil {
				return nil, err
			}
		}
	case OpenAIAdditionalToolNamespace:
		var children []json.RawMessage
		if !fields.allows("type", "name", "description", "tools") || !fields.matches("description", `"`, false) ||
			json.Unmarshal(fields["tools"], &children) != nil || len(children) == 0 {
			return nil, fmt.Errorf("expected a namespace description and a nonempty tools list")
		}
		validated := make([]additionalToolFields, len(children))
		for i, child := range children {
			var err error
			validated[i], err = validateAdditionalTool(child, true)
			if err != nil {
				return nil, fmt.Errorf("tools[%d]: %w", i, err)
			}
		}
		var err error
		fields["tools"], err = json.Marshal(validated)
		if err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("expected a function, custom or namespace definition")
	}
	return fields, nil
}

func validateAdditionalToolFormat(raw json.RawMessage) (additionalToolFields, error) {
	var fields additionalToolFields
	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
		return nil, fmt.Errorf("custom format must be an object")
	}
	switch OpenAIAdditionalToolFormatType(fields.string("type")) {
	case OpenAIAdditionalToolFormatText:
		if fields.allows("type") {
			return fields, nil
		}
	case OpenAIAdditionalToolFormatGrammar:
		syntax := OpenAIAdditionalToolGrammarSyntax(fields.string("syntax"))
		if fields.allows("type", "syntax", "definition") && fields.matches("definition", `"`, false) &&
			(syntax == OpenAIAdditionalToolGrammarRegex || syntax == OpenAIAdditionalToolGrammarLark) {
			return fields, nil
		}
	}
	return nil, fmt.Errorf("invalid custom format")
}
