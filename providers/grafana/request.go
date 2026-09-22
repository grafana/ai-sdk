package grafana

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"reflect"
	"strings"
	"unicode/utf8"

	"github.com/grafana/ai-sdk/provider"
)

type requestObject map[string]any

type requestCallOptionsFields struct {
	Prompt           []provider.Message       `json:"prompt,omitempty"`
	Tools            []provider.Tool          `json:"tools,omitempty"`
	ToolChoice       *provider.ToolChoice     `json:"toolChoice,omitempty"`
	MaxOutputTokens  *int                     `json:"maxOutputTokens,omitempty"`
	Temperature      *float64                 `json:"temperature,omitempty"`
	TopP             *float64                 `json:"topP,omitempty"`
	TopK             *int                     `json:"topK,omitempty"`
	PresencePenalty  *float64                 `json:"presencePenalty,omitempty"`
	FrequencyPenalty *float64                 `json:"frequencyPenalty,omitempty"`
	StopSequences    []string                 `json:"stopSequences,omitempty"`
	ResponseFormat   *provider.ResponseFormat `json:"responseFormat,omitempty"`
	Seed             *int                     `json:"seed,omitempty"`
	Reasoning        provider.ReasoningEffort `json:"reasoning,omitempty"`
	IncludeRawChunks bool                     `json:"includeRawChunks,omitempty"`
	Headers          map[string]string        `json:"headers,omitempty"`
	ProviderOptions  provider.ProviderOptions `json:"providerOptions,omitempty"`
}

var _ = requestCallOptionsFields(provider.CallOptions{})

var errRequest = errors.New("grafana: invalid or unrepresentable request")

func encodeRequest(opts provider.CallOptions) ([]byte, error) {
	if err := validateRequestValue(reflect.ValueOf(opts), 0); err != nil {
		return nil, err
	}
	out := requestObject{}
	if opts.Prompt != nil {
		messages := make([]requestObject, len(opts.Prompt))
		for i, message := range opts.Prompt {
			m, err := projectMessage(message)
			if err != nil {
				return nil, err
			}
			messages[i] = m
		}
		out["prompt"] = messages
	}
	if opts.Tools != nil {
		tools := make([]requestObject, len(opts.Tools))
		for i, tool := range opts.Tools {
			value, err := projectTool(tool)
			if err != nil {
				return nil, err
			}
			tools[i] = value
		}
		out["tools"] = tools
	}
	if t := opts.ToolChoice; t != nil {
		v := requestObject{"type": t.Type}
		switch t.Type {
		case provider.ToolChoiceAuto, provider.ToolChoiceNone, provider.ToolChoiceRequired:
			if t.ToolName != "" {
				return nil, errRequest
			}
		case provider.ToolChoiceTool:
			v["toolName"] = t.ToolName
		default:
			return nil, errRequest
		}
		out["toolChoice"] = v
	}
	for key, value := range map[string]any{"maxOutputTokens": opts.MaxOutputTokens, "temperature": opts.Temperature, "topP": opts.TopP, "topK": opts.TopK, "presencePenalty": opts.PresencePenalty, "frequencyPenalty": opts.FrequencyPenalty, "seed": opts.Seed} {
		if !reflect.ValueOf(value).IsNil() {
			out[key] = value
		}
	}
	if opts.StopSequences != nil {
		out["stopSequences"] = opts.StopSequences
	}
	if f := opts.ResponseFormat; f != nil {
		v := requestObject{"type": f.Type}
		switch f.Type {
		case provider.ResponseFormatText:
			if f.Schema != nil || f.Name != "" || f.Description != "" {
				return nil, errRequest
			}
		case provider.ResponseFormatJSON:
			if f.Schema != nil {
				if !validJSONObject(f.Schema) {
					return nil, errRequest
				}
				v["schema"] = f.Schema
			}
			optionalString(v, "name", f.Name)
			optionalString(v, "description", f.Description)
		default:
			return nil, errRequest
		}
		out["responseFormat"] = v
	}
	switch opts.Reasoning {
	case provider.ReasoningProviderDefault:
	case provider.ReasoningNone, provider.ReasoningMinimal, provider.ReasoningLow, provider.ReasoningMedium, provider.ReasoningHigh, provider.ReasoningXHigh:
		out["reasoning"] = opts.Reasoning
	default:
		return nil, errRequest
	}
	if opts.IncludeRawChunks {
		out["includeRawChunks"] = true
	}
	if opts.Headers != nil {
		seen := map[string]bool{}
		for key, value := range opts.Headers {
			name := http.CanonicalHeaderKey(key)
			if !validHeaderName(key) || !validHeaderValue(value) || seen[name] {
				return nil, errRequest
			}
			seen[name] = true
		}
		out["headers"] = opts.Headers
	}
	if err := projectOptions(out, opts.ProviderOptions); err != nil {
		return nil, err
	}
	b, err := json.Marshal(out)
	if err != nil {
		return nil, errRequest
	}
	return b, nil
}

func projectMessage(message provider.Message) (requestObject, error) {
	out := requestObject{"role": message.Role}
	if err := projectOptions(out, message.ProviderOptions); err != nil {
		return nil, err
	}
	switch message.Role {
	case provider.RoleSystem:
		var text strings.Builder
		for _, p := range message.Content {
			if p.Type != provider.ContentPartTypeText || p.ProviderOptions != nil {
				return nil, errRequest
			}
			if _, err := projectPart(p); err != nil {
				return nil, err
			}
			text.WriteString(p.Text)
		}
		out["content"] = text.String()
	case provider.RoleUser, provider.RoleAssistant, provider.RoleTool:
		if message.Content == nil {
			return nil, errRequest
		}
		var parts []requestObject
		if message.Content != nil {
			parts = make([]requestObject, len(message.Content))
		}
		for i, p := range message.Content {
			if message.Role == provider.RoleUser && p.Type != provider.ContentPartTypeText && p.Type != provider.ContentPartTypeFile {
				return nil, errRequest
			}
			if message.Role == provider.RoleTool && p.Type != provider.ContentPartTypeToolResult && p.Type != provider.ContentPartTypeToolApprovalResponse {
				return nil, errRequest
			}
			if message.Role == provider.RoleAssistant && p.Type == provider.ContentPartTypeToolApprovalResponse {
				return nil, errRequest
			}
			value, err := projectPart(p)
			if err != nil {
				return nil, err
			}
			parts[i] = value
		}
		out["content"] = parts
	default:
		return nil, errRequest
	}
	return out, nil
}

func projectPart(p provider.ContentPart) (requestObject, error) {
	out := requestObject{"type": p.Type}
	if err := projectOptions(out, p.ProviderOptions); err != nil {
		return nil, err
	}
	rest := p
	rest.Type = ""
	rest.ProviderOptions = nil
	switch p.Type {
	case provider.ContentPartTypeText, provider.ContentPartTypeReasoning:
		out["text"] = p.Text
		rest.Text = ""
	case provider.ContentPartTypeFile, provider.ContentPartTypeReasoningFile:
		data, err := projectData(p.Data)
		if err != nil {
			return nil, err
		}
		if p.Type == provider.ContentPartTypeReasoningFile && !p.Data.IsData() && !p.Data.IsURL() {
			return nil, errRequest
		}
		out["data"] = data
		out["mediaType"] = p.MediaType
		rest.Data = nil
		rest.MediaType = ""
		if p.Type == provider.ContentPartTypeFile {
			optionalString(out, "filename", p.Filename)
			rest.Filename = ""
		}
	case provider.ContentPartTypeCustom:
		if !strings.Contains(p.Kind, ".") {
			return nil, errRequest
		}
		out["kind"] = p.Kind
		rest.Kind = ""
	case provider.ContentPartTypeToolCall:
		if p.Input == nil {
			return nil, errRequest
		}
		out["toolCallId"] = p.ToolCallID
		out["toolName"] = p.ToolName
		out["input"] = p.Input
		if p.ProviderExecuted {
			out["providerExecuted"] = true
		}
		rest.ToolCallID = ""
		rest.ToolName = ""
		rest.Input = nil
		rest.ProviderExecuted = false
	case provider.ContentPartTypeToolResult:
		value, err := projectOutput(p.Output)
		if err != nil {
			return nil, err
		}
		out["output"] = value
		out["toolCallId"] = p.ToolCallID
		out["toolName"] = p.ToolName
		rest.Output = nil
		rest.ToolCallID = ""
		rest.ToolName = ""
	case provider.ContentPartTypeToolApprovalResponse:
		if p.Approved == nil {
			return nil, errRequest
		}
		out["approvalId"] = p.ApprovalID
		out["approved"] = *p.Approved
		optionalString(out, "reason", p.Reason)
		rest.ApprovalID = ""
		rest.Approved = nil
		rest.Reason = ""
	default:
		return nil, errRequest
	}
	if !reflect.ValueOf(rest).IsZero() {
		return nil, errRequest
	}
	return out, nil
}

func projectData(d *provider.DataContent) (requestObject, error) {
	if d == nil || d.Validate() != nil {
		return nil, errRequest
	}
	switch {
	case d.IsData():
		value := d.Base64
		if d.Bytes != nil {
			value = base64.StdEncoding.EncodeToString(d.Bytes)
		}
		return requestObject{"type": "data", "data": value}, nil
	case d.IsURL():
		return requestObject{"type": "url", "url": d.URL}, nil
	case d.Reference != nil:
		var reference map[string]string
		if json.Unmarshal(d.Reference, &reference) != nil || reference == nil {
			return nil, errRequest
		}
		if _, reserved := reference["type"]; reserved {
			return nil, errRequest
		}
		return requestObject{"type": "reference", "reference": d.Reference}, nil
	default:
		return requestObject{"type": "text", "text": d.Text}, nil
	}
}

func projectTool(t provider.Tool) (requestObject, error) {
	out := requestObject{"type": t.Type, "name": t.Name}
	if err := projectOptions(out, t.ProviderOptions); err != nil {
		return nil, err
	}
	switch t.Type {
	case provider.ToolTypeFunction:
		if t.ID != "" || t.Args != nil || !validJSONObject(t.InputSchema) {
			return nil, errRequest
		}
		optionalString(out, "description", t.Description)
		out["inputSchema"] = t.InputSchema
		if t.Strict != nil {
			out["strict"] = *t.Strict
		}
		if t.InputExamples != nil {
			examples := make([]requestObject, len(t.InputExamples))
			for i, e := range t.InputExamples {
				if !validJSONObject(e.Input) {
					return nil, errRequest
				}
				examples[i] = requestObject{"input": e.Input}
			}
			out["inputExamples"] = examples
		}
	case provider.ToolTypeProvider:
		if t.Description != "" || t.InputSchema != nil || t.InputExamples != nil || t.Strict != nil || t.ProviderOptions != nil || t.Args == nil || !strings.Contains(t.ID, ".") {
			return nil, errRequest
		}
		out["id"] = t.ID
		out["args"] = t.Args
	default:
		return nil, errRequest
	}
	return out, nil
}

func projectOutput(p *provider.ToolResultOutput) (requestObject, error) {
	if p == nil {
		return nil, errRequest
	}
	out := requestObject{"type": p.Type}
	if err := projectOptions(out, p.ProviderOptions); err != nil {
		return nil, err
	}
	rest := *p
	rest.Type = ""
	rest.ProviderOptions = nil
	switch p.Type {
	case provider.ToolOutputText, provider.ToolOutputErrorText:
		out["value"] = p.Text
		rest.Text = ""
	case provider.ToolOutputJSON, provider.ToolOutputErrorJSON:
		out["value"] = p.JSON
		rest.JSON = nil
	case provider.ToolOutputExecutionDenied:
		optionalString(out, "reason", p.Reason)
		rest.Reason = ""
	case provider.ToolOutputContent:
		if p.ProviderOptions != nil || p.Content == nil {
			return nil, errRequest
		}
		var values []requestObject
		if p.Content != nil {
			values = make([]requestObject, len(p.Content))
		}
		for i, v := range p.Content {
			value, err := projectResultContent(v)
			if err != nil {
				return nil, err
			}
			values[i] = value
		}
		out["value"] = values
		rest.Content = nil
	default:
		return nil, errRequest
	}
	if !reflect.ValueOf(rest).IsZero() {
		return nil, errRequest
	}
	return out, nil
}

func projectResultContent(p provider.ToolResultContentValue) (requestObject, error) {
	out := requestObject{"type": p.Type}
	if err := projectOptions(out, p.ProviderOptions); err != nil {
		return nil, err
	}
	switch p.Type {
	case provider.ToolContentText:
		if p.Data != nil || p.MediaType != "" || p.Filename != "" {
			return nil, errRequest
		}
		out["text"] = p.Text
	case provider.ToolContentFile:
		if p.Text != "" {
			return nil, errRequest
		}
		data, err := projectData(p.Data)
		if err != nil {
			return nil, err
		}
		out["data"] = data
		out["mediaType"] = p.MediaType
		optionalString(out, "filename", p.Filename)
	case provider.ToolContentCustom:
		if p.Text != "" || p.Data != nil || p.MediaType != "" || p.Filename != "" {
			return nil, errRequest
		}
	default:
		return nil, errRequest
	}
	return out, nil
}

func optionalString(out requestObject, key, value string) {
	if value != "" {
		out[key] = value
	}
}

func projectOptions(out requestObject, options provider.ProviderOptions) error {
	if options == nil {
		return nil
	}
	values := make(map[string]json.RawMessage, len(options))
	for key, value := range options {
		if value == nil {
			return errRequest
		}
		var raw []byte
		var err error
		switch v := value.(type) {
		case provider.RawProviderOption:
			raw = v.Raw
		case *provider.RawProviderOption:
			if v == nil {
				raw = []byte("null")
			} else {
				raw = v.Raw
			}
		default:
			raw, err = json.Marshal(value)
		}
		if err != nil || !validJSONObject(raw) {
			return errRequest
		}
		values[key] = raw
	}
	out["providerOptions"] = values
	return nil
}

func validJSONObject(raw []byte) bool {
	if !validJSON(raw) {
		return false
	}
	var object map[string]json.RawMessage
	return json.Unmarshal(raw, &object) == nil && object != nil
}

func validateRequestValue(v reflect.Value, depth int) error {
	if depth > 100 {
		return errRequest
	}
	if !v.IsValid() {
		return nil
	}
	if v.Type() == reflect.TypeFor[json.RawMessage]() {
		if v.IsNil() {
			return nil
		}
		raw := v.Bytes()
		if !validJSON(raw) {
			return errRequest
		}
		return nil
	}
	switch v.Kind() {
	case reflect.Interface, reflect.Pointer:
		if !v.IsNil() {
			return validateRequestValue(v.Elem(), depth+1)
		}
	case reflect.String:
		if !utf8.ValidString(v.String()) {
			return errRequest
		}
	case reflect.Float32, reflect.Float64:
		if math.IsNaN(v.Float()) || math.IsInf(v.Float(), 0) {
			return errRequest
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if v.Int() < -9007199254740991 || v.Int() > 9007199254740991 {
			return errRequest
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if v.Uint() > 9007199254740991 {
			return errRequest
		}
	case reflect.Struct:
		for i := range v.NumField() {
			if err := validateRequestValue(v.Field(i), depth+1); err != nil {
				return err
			}
		}
	case reflect.Map:
		iter := v.MapRange()
		for iter.Next() {
			if err := validateRequestValue(iter.Key(), depth+1); err != nil {
				return err
			}
			if err := validateRequestValue(iter.Value(), depth+1); err != nil {
				return err
			}
		}
	case reflect.Slice, reflect.Array:
		if v.Type().Elem().Kind() == reflect.Uint8 {
			return nil
		}
		for i := 0; i < v.Len(); i++ {
			if err := validateRequestValue(v.Index(i), depth+1); err != nil {
				return err
			}
		}
	}
	return nil
}
