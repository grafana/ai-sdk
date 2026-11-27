package responses

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"strings"

	"github.com/grafana/ai-sdk/provider"
	"github.com/grafana/ai-sdk/schema"
)

type itemType string
type partType string

const (
	itemMessage            itemType = "message"
	itemFunctionCall       itemType = "function_call"
	itemFunctionCallOutput itemType = "function_call_output"
	itemReasoning          itemType = "reasoning"

	partInputText   partType = "input_text"
	partOutputText  partType = "output_text"
	partInputImage  partType = "input_image"
	partSummaryText partType = "summary_text"

	toolFunction = "function"

	formatText       = "text"
	formatJSONObject = "json_object"
	formatJSONSchema = "json_schema"
)

var namePattern = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)
var errRequest = errors.New("responses: invalid request")

type request struct {
	Model          string          `json:"model"`
	Input          json.RawMessage `json:"input"`
	Stream         *bool           `json:"stream"`
	Tools          []tool          `json:"tools"`
	ToolChoice     json.RawMessage `json:"tool_choice"`
	Parallel       *bool           `json:"parallel_tool_calls"`
	Reasoning      *reasoning      `json:"reasoning"`
	Text           *textConfig     `json:"text"`
	Store          *bool           `json:"store"`
	Include        []string        `json:"include"`
	PromptCacheKey *string         `json:"prompt_cache_key"`
	ClientMetadata json.RawMessage `json:"client_metadata"`
}

type reasoning struct {
	Effort  *provider.ReasoningEffort `json:"effort"`
	Summary *string                   `json:"summary"`
}

type textConfig struct {
	Format *struct {
		Type        string          `json:"type"`
		Name        string          `json:"name"`
		Description *string         `json:"description"`
		Schema      json.RawMessage `json:"schema"`
		Strict      *bool           `json:"strict"`
	} `json:"format"`
	Verbosity *string `json:"verbosity"`
}

type tool struct {
	Type        string          `json:"type"`
	Name        string          `json:"name"`
	Description *string         `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
	Strict      *bool           `json:"strict"`
}

type messageItem struct {
	Type    itemType        `json:"type"`
	ID      string          `json:"id"`
	Status  string          `json:"status"`
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

type contentPart struct {
	Type        partType          `json:"type"`
	Text        *string           `json:"text"`
	ImageURL    *string           `json:"image_url"`
	Detail      *string           `json:"detail"`
	Annotations []json.RawMessage `json:"annotations"`
	Logprobs    []json.RawMessage `json:"logprobs"`
}

type functionCallItem struct {
	Type      itemType `json:"type"`
	ID        string   `json:"id"`
	Status    string   `json:"status"`
	CallID    string   `json:"call_id"`
	Name      string   `json:"name"`
	Arguments string   `json:"arguments"`
}

type functionCallOutputItem struct {
	Type   itemType `json:"type"`
	ID     string   `json:"id"`
	Status string   `json:"status"`
	CallID string   `json:"call_id"`
	Output *string  `json:"output"`
}

type reasoningItem struct {
	Type             itemType      `json:"type"`
	ID               string        `json:"id"`
	Status           string        `json:"status"`
	Summary          []summaryPart `json:"summary"`
	Content          []summaryPart `json:"content"`
	EncryptedContent *string       `json:"encrypted_content"`
}

type summaryPart struct {
	Type partType `json:"type"`
	Text string   `json:"text"`
}

type mappedRequest struct {
	request
	options          provider.CallOptions
	validator        *schema.CompiledSchema
	toolValidators   map[string]*schema.CompiledSchema
	jsonOutput       bool
	strictOutput     bool
	reasoningSummary string
	verbosity        string
}

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

func jsonObject(raw []byte) bool {
	var object map[string]json.RawMessage
	return json.Unmarshal(raw, &object) == nil && object != nil
}

func oneOf(value string, allowed ...string) bool {
	for _, a := range allowed {
		if value == a {
			return true
		}
	}
	return false
}

func mapRequest(data []byte) (mappedRequest, error) {
	var r mappedRequest
	if decodeStrict(data, &r.request) != nil || r.Model == "" || len(r.Model) > 256 || len(r.Tools) > 128 || boolValue(r.Store) {
		return r, errRequest
	}
	for _, include := range r.Include {
		if include != "reasoning.encrypted_content" {
			return r, errRequest
		}
	}
	if present(r.ClientMetadata) && !jsonObject(r.ClientMetadata) {
		return r, errRequest
	}
	if r.Reasoning != nil {
		if e := r.Reasoning.Effort; e != nil && !oneOf(string(*e), "none", "minimal", "low", "medium", "high", "xhigh") {
			return r, errRequest
		}
		if s := r.Reasoning.Summary; s != nil {
			if !oneOf(*s, "auto", "concise", "detailed") {
				return r, errRequest
			}
			r.reasoningSummary = *s
		}
		if r.Reasoning.Effort != nil {
			r.options.Reasoning = *r.Reasoning.Effort
		}
	}
	prompt, err := mapInput(r.Input)
	if err != nil {
		return r, err
	}
	r.options.Prompt = prompt
	if err := r.mapTools(); err != nil {
		return r, err
	}
	if err := r.mapText(); err != nil {
		return r, err
	}
	return r, nil
}

func mapInput(raw json.RawMessage) ([]provider.Message, error) {
	var text string
	if !present(raw) {
		return nil, errRequest
	}
	if json.Unmarshal(raw, &text) == nil {
		return []provider.Message{{Role: provider.RoleUser, Content: []provider.ContentPart{provider.TextPart(text)}}}, nil
	}
	var items []json.RawMessage
	if decodeStrict(raw, &items) != nil || len(items) == 0 || len(items) > 4096 {
		return nil, errRequest
	}
	var prompt []provider.Message
	calls := map[string]string{}
	answered := map[string]bool{}
	appendAssistant := func(part provider.ContentPart) {
		if n := len(prompt); n > 0 && prompt[n-1].Role == provider.RoleAssistant {
			prompt[n-1].Content = append(prompt[n-1].Content, part)
			return
		}
		prompt = append(prompt, provider.Message{Role: provider.RoleAssistant, Content: []provider.ContentPart{part}})
	}
	for _, rawItem := range items {
		var head struct {
			Type itemType `json:"type"`
		}
		if json.Unmarshal(rawItem, &head) != nil {
			return nil, errRequest
		}
		switch head.Type {
		case "", itemMessage:
			var m messageItem
			if decodeStrict(rawItem, &m) != nil {
				return nil, errRequest
			}
			message, err := mapMessage(m)
			if err != nil {
				return nil, err
			}
			if message.Role == provider.RoleAssistant {
				for _, part := range message.Content {
					appendAssistant(part)
				}
				continue
			}
			prompt = append(prompt, message)
		case itemFunctionCall:
			var c functionCallItem
			if decodeStrict(rawItem, &c) != nil || c.CallID == "" || len(c.CallID) > 256 || calls[c.CallID] != "" || !namePattern.MatchString(c.Name) || !jsonObject([]byte(c.Arguments)) {
				return nil, errRequest
			}
			calls[c.CallID] = c.Name
			appendAssistant(provider.ToolCallPart(c.CallID, c.Name, json.RawMessage(c.Arguments)))
		case itemFunctionCallOutput:
			var o functionCallOutputItem
			if decodeStrict(rawItem, &o) != nil || o.Output == nil || calls[o.CallID] == "" || answered[o.CallID] {
				return nil, errRequest
			}
			answered[o.CallID] = true
			result := provider.ToolResultPart(o.CallID, calls[o.CallID], &provider.ToolResultOutput{Type: provider.ToolOutputText, Text: *o.Output})
			if n := len(prompt); n > 0 && prompt[n-1].Role == provider.RoleTool {
				prompt[n-1].Content = append(prompt[n-1].Content, result)
			} else {
				prompt = append(prompt, provider.Message{Role: provider.RoleTool, Content: []provider.ContentPart{result}})
			}
		case itemReasoning:
			var reasoning reasoningItem
			if decodeStrict(rawItem, &reasoning) != nil || len(reasoning.Content) > 0 {
				return nil, errRequest
			}
			for _, s := range reasoning.Summary {
				if s.Type != partSummaryText {
					return nil, errRequest
				}
				appendAssistant(provider.ReasoningPart(s.Text))
			}
		default:
			return nil, errRequest
		}
	}
	if len(answered) != len(calls) {
		return nil, errRequest
	}
	return prompt, nil
}

func mapMessage(m messageItem) (provider.Message, error) {
	var role provider.Role
	switch m.Role {
	case "system", "developer":
		role = provider.RoleSystem
	case "user":
		role = provider.RoleUser
	case "assistant":
		role = provider.RoleAssistant
	default:
		return provider.Message{}, errRequest
	}
	message := provider.Message{Role: role}
	var text string
	if !present(m.Content) {
		return message, errRequest
	}
	if json.Unmarshal(m.Content, &text) == nil {
		message.Content = []provider.ContentPart{provider.TextPart(text)}
		return message, nil
	}
	var parts []contentPart
	if decodeStrict(m.Content, &parts) != nil || len(parts) == 0 || len(parts) > 1024 {
		return message, errRequest
	}
	for _, p := range parts {
		var part provider.ContentPart
		switch {
		case p.Type == partInputText && role != provider.RoleAssistant && p.Text != nil && p.ImageURL == nil && p.Detail == nil && p.Annotations == nil && p.Logprobs == nil:
			part = provider.TextPart(*p.Text)
		case p.Type == partOutputText && role == provider.RoleAssistant && p.Text != nil && p.ImageURL == nil && p.Detail == nil && len(p.Annotations) == 0 && len(p.Logprobs) == 0:
			part = provider.TextPart(*p.Text)
		case p.Type == partInputImage && role == provider.RoleUser && p.Text == nil && p.ImageURL != nil && (p.Detail == nil || *p.Detail == "auto") && p.Annotations == nil && p.Logprobs == nil:
			image, ok := imagePart(*p.ImageURL)
			if !ok {
				return message, errRequest
			}
			part = image
		default:
			return message, errRequest
		}
		if role == provider.RoleSystem && part.Type != provider.ContentPartTypeText {
			return message, errRequest
		}
		message.Content = append(message.Content, part)
	}
	if role == provider.RoleSystem {
		var joined strings.Builder
		for _, part := range message.Content {
			joined.WriteString(part.Text)
		}
		message.Content = []provider.ContentPart{provider.TextPart(joined.String())}
	}
	return message, nil
}

func imagePart(url string) (provider.ContentPart, bool) {
	if rest, ok := strings.CutPrefix(url, "data:"); ok {
		meta, data, ok := strings.Cut(rest, ",")
		mediaType, encoding, _ := strings.Cut(meta, ";")
		if !ok || encoding != "base64" || !strings.HasPrefix(mediaType, "image/") || data == "" {
			return provider.ContentPart{}, false
		}
		return provider.FilePart(mediaType, provider.Base64DataContent(data)), true
	}
	if !strings.HasPrefix(url, "https://") && !strings.HasPrefix(url, "http://") {
		return provider.ContentPart{}, false
	}
	return provider.FilePart("image/*", provider.URLDataContent(url)), true
}

func (r *mappedRequest) mapTools() error {
	r.toolValidators = map[string]*schema.CompiledSchema{}
	for _, t := range r.Tools {
		if t.Type != toolFunction || !namePattern.MatchString(t.Name) || r.toolValidators[t.Name] != nil {
			return errRequest
		}
		parameters := t.Parameters
		if !present(parameters) {
			parameters = json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`)
		}
		compiled, err := compileSchema(parameters)
		if err != nil {
			return errRequest
		}
		r.toolValidators[t.Name] = compiled
		description := ""
		if t.Description != nil {
			description = *t.Description
		}
		r.options.Tools = append(r.options.Tools, provider.Tool{Type: provider.ToolTypeFunction, Name: t.Name, Description: description, InputSchema: parameters, Strict: ptr(boolValue(t.Strict))})
	}
	if !present(r.ToolChoice) {
		if len(r.Tools) > 0 {
			r.options.ToolChoice = &provider.ToolChoice{Type: provider.ToolChoiceAuto}
		}
		return nil
	}
	var choice string
	if json.Unmarshal(r.ToolChoice, &choice) == nil {
		if !oneOf(choice, "auto", "none", "required") || (choice == "required" && len(r.Tools) == 0) {
			return errRequest
		}
		r.options.ToolChoice = &provider.ToolChoice{Type: provider.ToolChoiceType(choice)}
		return nil
	}
	var named struct {
		Type string `json:"type"`
		Name string `json:"name"`
	}
	if decodeStrict(r.ToolChoice, &named) != nil || named.Type != toolFunction || r.toolValidators[named.Name] == nil {
		return errRequest
	}
	r.options.ToolChoice = &provider.ToolChoice{Type: provider.ToolChoiceTool, ToolName: named.Name}
	return nil
}

func (r *mappedRequest) mapText() error {
	if r.Text == nil {
		return nil
	}
	if v := r.Text.Verbosity; v != nil {
		if !oneOf(*v, "low", "medium", "high") {
			return errRequest
		}
		r.verbosity = *v
	}
	f := r.Text.Format
	if f == nil {
		return nil
	}
	switch f.Type {
	case formatText, formatJSONObject:
		if f.Name != "" || f.Description != nil || present(f.Schema) || f.Strict != nil {
			return errRequest
		}
		r.jsonOutput = f.Type == formatJSONObject
		if r.jsonOutput {
			r.options.ResponseFormat = &provider.ResponseFormat{Type: provider.ResponseFormatJSON}
		}
	case formatJSONSchema:
		if !namePattern.MatchString(f.Name) {
			return errRequest
		}
		compiled, err := compileSchema(f.Schema)
		if err != nil {
			return errRequest
		}
		r.validator = compiled
		r.jsonOutput = true
		r.strictOutput = boolValue(f.Strict)
		r.options.ResponseFormat = &provider.ResponseFormat{Type: provider.ResponseFormatJSON, Name: ptr(f.Name), Schema: f.Schema}
		if f.Description != nil {
			r.options.ResponseFormat.Description = *f.Description
		}
	default:
		return errRequest
	}
	return nil
}

func compileSchema(raw json.RawMessage) (*schema.CompiledSchema, error) {
	if !jsonObject(raw) {
		return nil, errRequest
	}
	compiled, err := schema.CompileSelfContainedSchema(raw)
	if err != nil {
		return nil, errRequest
	}
	return compiled, nil
}
