package chatcompletions

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"sync"

	"github.com/grafana/ai-sdk/provider"
	"github.com/grafana/ai-sdk/schema"
)

type role string
type kind string

const (
	roleSystem    role = "system"
	roleUser      role = "user"
	roleAssistant role = "assistant"
	roleTool      role = "tool"
	kindFunction  kind = "function"
	kindText      kind = "text"
	kindJSON      kind = "json_object"
	kindSchema    kind = "json_schema"
)

var namePattern = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)
var errRequest = errors.New("chat completions: invalid request")

type request struct {
	Model               string                   `json:"model"`
	Messages            []message                `json:"messages"`
	Stream              *bool                    `json:"stream"`
	N                   *int                     `json:"n"`
	Store               *bool                    `json:"store"`
	Temperature         *float64                 `json:"temperature"`
	TopP                *float64                 `json:"top_p"`
	FrequencyPenalty    *float64                 `json:"frequency_penalty"`
	PresencePenalty     *float64                 `json:"presence_penalty"`
	Seed                *int                     `json:"seed"`
	MaxTokens           *int                     `json:"max_tokens"`
	MaxCompletionTokens *int                     `json:"max_completion_tokens"`
	Stop                json.RawMessage          `json:"stop"`
	Tools               []tool                   `json:"tools"`
	ToolChoice          json.RawMessage          `json:"tool_choice"`
	Parallel            *bool                    `json:"parallel_tool_calls"`
	Reasoning           provider.ReasoningEffort `json:"reasoning_effort"`
	ResponseFormat      *responseFormat          `json:"response_format"`
	StreamOptions       *struct {
		IncludeUsage *bool `json:"include_usage"`
	} `json:"stream_options"`
}
type message struct {
	Role       role            `json:"role"`
	Content    json.RawMessage `json:"content"`
	ToolCalls  []toolCall      `json:"tool_calls"`
	ToolCallID *string         `json:"tool_call_id"`
}
type function struct {
	Name        string          `json:"name"`
	Description *string         `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
	Strict      *bool           `json:"strict"`
}
type tool struct {
	Type     kind      `json:"type"`
	Function *function `json:"function"`
}
type callFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}
type toolCall struct {
	ID       string       `json:"id"`
	Type     kind         `json:"type"`
	Function callFunction `json:"function"`
}
type responseFormat struct {
	Type       kind `json:"type"`
	JSONSchema *struct {
		Name        string          `json:"name"`
		Description *string         `json:"description"`
		Schema      json.RawMessage `json:"schema"`
		Strict      *bool           `json:"strict"`
	} `json:"json_schema"`
}
type mappedRequest struct {
	request
	options        provider.CallOptions
	validator      *schema.CompiledSchema
	toolValidators map[string]*schema.CompiledSchema
	jsonOutput     bool
	strictOutput   bool
}

//go:embed request_schema.json
var requestSchemaJSON []byte
var compileRequestSchema = sync.OnceValues(func() (*schema.CompiledSchema, error) {
	return schema.CompileSchema(requestSchemaJSON)
})

func decodeStrict(data []byte, dst any) error {
	if !present(data) {
		return errRequest
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(dst); err != nil {
		return errRequest
	}
	if d.Decode(new(any)) != io.EOF {
		return errRequest
	}
	return nil
}

func present(raw json.RawMessage) bool {
	return len(raw) > 0 && !bytes.Equal(bytes.TrimSpace(raw), []byte("null"))
}
func boolValue(v *bool) bool { return v != nil && *v }
func ptr[T any](v T) *T      { return &v }

func mapRequest(data []byte) (mappedRequest, error) {
	var r mappedRequest
	var document any
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if decoder.Decode(&document) != nil || decoder.Decode(new(any)) != io.EOF {
		return r, errRequest
	}
	data, err := json.Marshal(document)
	if err != nil {
		return r, errRequest
	}
	validator, err := compileRequestSchema()
	if err != nil || validator.Validate(data) != nil {
		return r, errRequest
	}
	if decodeStrict(data, &r.request) != nil || r.Model == "" || len(r.Model) > 256 || len(r.Messages) == 0 || len(r.Messages) > 1024 || len(r.Tools) > 128 || (r.N != nil && *r.N != 1) || boolValue(r.Store) || (r.StreamOptions != nil && !boolValue(r.Stream)) {
		return r, errRequest
	}
	if r.MaxTokens != nil && r.MaxCompletionTokens != nil {
		return r, errRequest
	}
	maxTokens := r.MaxCompletionTokens
	if maxTokens == nil {
		maxTokens = r.MaxTokens
	}
	if maxTokens != nil && (*maxTokens <= 0 || *maxTokens > 2147483647) {
		return r, errRequest
	}
	for _, v := range []struct {
		value     *float64
		low, high float64
	}{{r.Temperature, 0, 2}, {r.TopP, 0, 1}, {r.FrequencyPenalty, -2, 2}, {r.PresencePenalty, -2, 2}} {
		if v.value != nil && (*v.value < v.low || *v.value > v.high) {
			return r, errRequest
		}
	}
	r.options = provider.CallOptions{MaxOutputTokens: maxTokens, Temperature: r.Temperature, TopP: r.TopP, FrequencyPenalty: r.FrequencyPenalty, PresencePenalty: r.PresencePenalty, Seed: r.Seed, Reasoning: r.Reasoning}
	if present(r.Stop) {
		var single string
		if json.Unmarshal(r.Stop, &single) == nil {
			r.options.StopSequences = []string{single}
		} else if decodeStrict(r.Stop, &r.options.StopSequences) != nil {
			return r, errRequest
		}
		if len(r.options.StopSequences) == 0 || len(r.options.StopSequences) > 4 {
			return r, errRequest
		}
	}
	pending := map[string]string{}
	used := map[string]bool{}
	seenNonSystem := false
	for _, m := range r.Messages {
		if m.Role == roleSystem && seenNonSystem {
			return r, errRequest
		}
		if m.Role != roleSystem {
			seenNonSystem = true
		}
		if m.Role != roleTool && len(pending) > 0 {
			return r, errRequest
		}
		if m.Role != roleAssistant && len(m.ToolCalls) > 0 || m.Role != roleTool && m.ToolCallID != nil {
			return r, errRequest
		}
		p := provider.Message{Role: provider.Role(m.Role)}
		switch m.Role {
		case roleSystem, roleUser, roleAssistant:
			if present(m.Content) {
				var text string
				if json.Unmarshal(m.Content, &text) == nil {
					p.Content = []provider.ContentPart{provider.TextPart(text)}
				} else {
					var parts []struct {
						Type kind    `json:"type"`
						Text *string `json:"text"`
					}
					if decodeStrict(m.Content, &parts) != nil || len(parts) > 1024 {
						return r, errRequest
					}
					for _, part := range parts {
						if part.Type != kindText || part.Text == nil {
							return r, errRequest
						}
						p.Content = append(p.Content, provider.TextPart(*part.Text))
					}
				}
			} else if m.Role != roleAssistant || len(m.ToolCalls) == 0 {
				return r, errRequest
			}
			for _, call := range m.ToolCalls {
				if call.Type != kindFunction || call.ID == "" || len(call.ID) > 256 || used[call.ID] || !namePattern.MatchString(call.Function.Name) || !jsonObject([]byte(call.Function.Arguments)) {
					return r, errRequest
				}
				used[call.ID] = true
				pending[call.ID] = call.Function.Name
				p.Content = append(p.Content, provider.ToolCallPart(call.ID, call.Function.Name, json.RawMessage(call.Function.Arguments)))
			}
		case roleTool:
			if m.ToolCallID == nil || pending[*m.ToolCallID] == "" {
				return r, errRequest
			}
			var text string
			if !present(m.Content) || json.Unmarshal(m.Content, &text) != nil {
				return r, errRequest
			}
			p.Content = []provider.ContentPart{provider.ToolResultPart(*m.ToolCallID, pending[*m.ToolCallID], &provider.ToolResultOutput{Type: provider.ToolOutputText, Text: text})}
			delete(pending, *m.ToolCallID)
		default:
			return r, errRequest
		}
		r.options.Prompt = append(r.options.Prompt, p)
	}
	if len(pending) > 0 {
		return r, errRequest
	}
	names := map[string]bool{}
	r.toolValidators = map[string]*schema.CompiledSchema{}
	for _, t := range r.Tools {
		if t.Type != kindFunction || t.Function == nil || !namePattern.MatchString(t.Function.Name) || names[t.Function.Name] {
			return r, errRequest
		}
		f := t.Function
		compiled, err := compileSchema(f.Parameters)
		if err != nil {
			return r, errRequest
		}
		r.toolValidators[f.Name] = compiled
		names[f.Name] = true
		description := ""
		if f.Description != nil {
			description = *f.Description
		}
		r.options.Tools = append(r.options.Tools, provider.Tool{Type: provider.ToolTypeFunction, Name: f.Name, Description: description, InputSchema: f.Parameters, Strict: ptr(boolValue(f.Strict))})
	}
	if present(r.ToolChoice) {
		var choice string
		if json.Unmarshal(r.ToolChoice, &choice) == nil {
			switch choice {
			case "auto", "none", "required":
				r.options.ToolChoice = &provider.ToolChoice{Type: provider.ToolChoiceType(choice)}
			default:
				return r, errRequest
			}
		} else {
			var named struct {
				Type     kind `json:"type"`
				Function struct {
					Name string `json:"name"`
				} `json:"function"`
			}
			if decodeStrict(r.ToolChoice, &named) != nil || named.Type != kindFunction || !names[named.Function.Name] {
				return r, errRequest
			}
			r.options.ToolChoice = &provider.ToolChoice{Type: provider.ToolChoiceTool, ToolName: named.Function.Name}
		}
	} else if len(r.Tools) > 0 {
		r.options.ToolChoice = &provider.ToolChoice{Type: provider.ToolChoiceAuto}
	}
	if r.options.ToolChoice != nil && r.options.ToolChoice.Type == provider.ToolChoiceRequired && len(r.Tools) == 0 {
		return r, errRequest
	}
	if r.ResponseFormat != nil {
		f := r.ResponseFormat
		switch f.Type {
		case kindText, kindJSON:
			if f.JSONSchema != nil {
				return r, errRequest
			}
			r.jsonOutput = f.Type == kindJSON
		case kindSchema:
			if f.JSONSchema == nil || !namePattern.MatchString(f.JSONSchema.Name) {
				return r, errRequest
			}
			var err error
			r.validator, err = compileSchema(f.JSONSchema.Schema)
			if err != nil {
				return r, errRequest
			}
			r.jsonOutput = true
			r.strictOutput = boolValue(f.JSONSchema.Strict)
		default:
			return r, errRequest
		}
		if r.jsonOutput {
			r.options.ResponseFormat = &provider.ResponseFormat{Type: provider.ResponseFormatJSON}
			if f.JSONSchema != nil {
				r.options.ResponseFormat.Name = f.JSONSchema.Name
				r.options.ResponseFormat.Schema = f.JSONSchema.Schema
				if f.JSONSchema.Description != nil {
					r.options.ResponseFormat.Description = *f.JSONSchema.Description
				}
			}
		}
	}
	return r, nil
}

func jsonObject(raw []byte) bool {
	var object map[string]json.RawMessage
	return json.Unmarshal(raw, &object) == nil && object != nil
}
func compileSchema(raw []byte) (*schema.CompiledSchema, error) {
	if len(raw) > 32768 || !jsonObject(raw) {
		return nil, errRequest
	}
	var value any
	if json.Unmarshal(raw, &value) != nil {
		return nil, errRequest
	}
	nodes := 0
	var walk func(any, int) bool
	walk = func(v any, depth int) bool {
		nodes++
		if depth > 16 || nodes > 512 {
			return false
		}
		switch obj := v.(type) {
		case map[string]any:
			for key, child := range obj {
				if key == "$ref" || key == "$dynamicRef" || key == "$id" || key == "$schema" {
					return false
				}
				if !walk(child, depth+1) {
					return false
				}
			}
		case []any:
			for _, child := range obj {
				if !walk(child, depth+1) {
					return false
				}
			}
		}
		return true
	}
	if !walk(value, 0) {
		return nil, errRequest
	}
	root, ok := value.(map[string]any)
	if !ok || root["type"] != "object" || !schemaVocabulary(root) {
		return nil, errRequest
	}
	return schema.CompileSchema(raw)
}

func schemaVocabulary(obj map[string]any) bool {
	for key, value := range obj {
		switch key {
		case "type", "description", "title", "enum", "const", "required", "minimum", "maximum", "minLength", "maxLength", "minItems", "maxItems":
		case "additionalProperties":
			if _, ok := value.(bool); !ok {
				return false
			}
		case "properties":
			properties, ok := value.(map[string]any)
			if !ok {
				return false
			}
			for _, property := range properties {
				child, ok := property.(map[string]any)
				if !ok || !schemaVocabulary(child) {
					return false
				}
			}
		case "items":
			child, ok := value.(map[string]any)
			if !ok || !schemaVocabulary(child) {
				return false
			}
		default:
			return false
		}
	}
	return true
}
