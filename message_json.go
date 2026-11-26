package aisdk

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/grafana/ai-sdk/provider"
)

var _ json.Marshaler = UIMessage{}

type uiMessageJSON struct {
	ID       *string           `json:"id"`
	Role     *Role             `json:"role"`
	Parts    []json.RawMessage `json:"parts"`
	Metadata json.RawMessage   `json:"metadata,omitempty"`
}

func (m UIMessage) MarshalJSON() ([]byte, error) {
	parts := make([]json.RawMessage, len(m.Parts))
	for i, p := range m.Parts {
		b, err := marshalPart(p)
		if err != nil {
			return nil, fmt.Errorf("marshaling part %d: %w", i, err)
		}
		parts[i] = b
	}
	return json.Marshal(uiMessageJSON{
		ID:       &m.ID,
		Role:     &m.Role,
		Parts:    parts,
		Metadata: m.Metadata,
	})
}

func marshalPart(p Part) (json.RawMessage, error) {
	var typeName string
	switch v := p.(type) {
	case TextPart:
		typeName = string(UIPartText)
		return marshalWithType(typeName, v)
	case ReasoningPart:
		typeName = string(UIPartReasoning)
		return marshalWithType(typeName, v)
	case ToolInvocationPart:
		typeName = "tool-" + v.ToolName
		return marshalWithType(typeName, v)
	case DynamicToolUIPart:
		typeName = string(UIPartDynamicTool)
		return marshalWithType(typeName, v)
	case FilePart:
		typeName = string(UIPartFile)
		return marshalWithType(typeName, v)
	case ReasoningFilePart:
		typeName = string(UIPartReasoningFile)
		return marshalWithType(typeName, v)
	case SourceURLPart:
		typeName = string(UIPartSourceURL)
		return marshalWithType(typeName, v)
	case SourceDocumentPart:
		typeName = string(UIPartSourceDocument)
		return marshalWithType(typeName, v)
	case DataPart:
		typeName = "data-" + v.DataName
		return marshalDataPart(typeName, v)
	case CustomPart:
		typeName = string(UIPartCustom)
		return marshalWithType(typeName, v)
	case StepStartPart:
		return json.Marshal(map[string]string{"type": string(UIPartStepStart)})
	case rawPart:
		return v.Raw, nil
	default:
		return nil, fmt.Errorf("unknown part type: %T", p)
	}
}

func marshalWithType(typeName string, v any) (json.RawMessage, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	typeBytes, _ := json.Marshal(typeName)
	m["type"] = typeBytes
	return json.Marshal(m)
}

func marshalDataPart(typeName string, v DataPart) (json.RawMessage, error) {
	m := map[string]any{
		"type": typeName,
		"data": json.RawMessage(v.Data),
	}
	if v.ID != "" {
		m["id"] = v.ID
	}
	return json.Marshal(m)
}

func (m *UIMessage) UnmarshalJSON(data []byte) error {
	var raw uiMessageJSON
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if raw.ID == nil {
		return missingUIField("id")
	}
	if raw.Role == nil {
		return missingUIField("role")
	}
	if raw.Parts == nil {
		return missingUIField("parts")
	}
	parts := make([]Part, len(raw.Parts))
	for i, partData := range raw.Parts {
		p, err := unmarshalPart(partData)
		if err != nil {
			return fmt.Errorf("unmarshaling part %d: %w", i, err)
		}
		parts[i] = p
	}
	*m = UIMessage{ID: *raw.ID, Role: *raw.Role, Metadata: raw.Metadata, Parts: parts}
	return nil
}

type partEnvelope struct {
	Type       *UIPartType     `json:"type"`
	Text       nonNullUIField  `json:"text"`
	Citations  json.RawMessage `json:"citations"`
	Kind       nonNullUIField  `json:"kind"`
	MediaType  nonNullUIField  `json:"mediaType"`
	URL        nonNullUIField  `json:"url"`
	SourceID   nonNullUIField  `json:"sourceId"`
	Title      nonNullUIField  `json:"title"`
	ToolCallID nonNullUIField  `json:"toolCallId"`
	ToolName   nonNullUIField  `json:"toolName"`
	State      nonNullUIField  `json:"state"`
	Approval   json.RawMessage `json:"approval"`
}

type nonNullUIField bool

func (present *nonNullUIField) UnmarshalJSON(data []byte) error {
	*present = nonNullUIField(!isJSONNull(data))
	return nil
}

type uiRequiredField struct {
	name    string
	present nonNullUIField
}

func missingUIField(name string) error {
	return fmt.Errorf("aisdk: missing or null required UI field %q", name)
}

func requireUIFields(fields ...uiRequiredField) error {
	for _, field := range fields {
		if !field.present {
			return missingUIField(field.name)
		}
	}
	return nil
}

func decodeUIValue[T Part](data json.RawMessage, fields ...uiRequiredField) (T, error) {
	var part T
	if err := requireUIFields(fields...); err != nil {
		return part, err
	}
	if err := json.Unmarshal(data, &part); err != nil {
		return part, err
	}
	return part, nil
}

func unmarshalPart(data json.RawMessage) (Part, error) {
	var env partEnvelope
	if err := json.Unmarshal(data, &env); err != nil {
		return nil, err
	}
	if env.Type == nil {
		return nil, missingUIField("type")
	}
	if *env.Type == "" {
		return nil, fmt.Errorf("aisdk: UI part type must not be empty")
	}

	switch *env.Type {
	case UIPartText:
		p, err := decodeUIValue[TextPart](data, uiRequiredField{"text", env.Text})
		if err != nil {
			return nil, err
		}
		if err := requireUICitationFields(env.Citations); err != nil {
			return nil, err
		}
		return p, nil
	case UIPartReasoning:
		return decodeUIValue[ReasoningPart](data, uiRequiredField{"text", env.Text})
	case UIPartFile:
		return decodeUIValue[FilePart](data, uiRequiredField{"mediaType", env.MediaType}, uiRequiredField{"url", env.URL})
	case UIPartReasoningFile:
		return decodeUIValue[ReasoningFilePart](data, uiRequiredField{"mediaType", env.MediaType}, uiRequiredField{"url", env.URL})
	case UIPartSourceURL:
		return decodeUIValue[SourceURLPart](data, uiRequiredField{"sourceId", env.SourceID}, uiRequiredField{"url", env.URL})
	case UIPartSourceDocument:
		return decodeUIValue[SourceDocumentPart](data, uiRequiredField{"sourceId", env.SourceID}, uiRequiredField{"mediaType", env.MediaType}, uiRequiredField{"title", env.Title})
	case UIPartCustom:
		return decodeUIValue[CustomPart](data, uiRequiredField{"kind", env.Kind})
	case UIPartStepStart:
		return StepStartPart{}, nil
	case UIPartDynamicTool:
		p, err := decodeUIValue[DynamicToolUIPart](data, uiRequiredField{"toolName", env.ToolName})
		if err != nil {
			return nil, err
		}
		return p, requireUIToolFields(env, toolPartFields(p))
	}

	typeName := string(*env.Type)
	if toolName, ok := strings.CutPrefix(typeName, "tool-"); ok {
		p, err := decodeUIValue[ToolInvocationPart](data)
		if err != nil {
			return nil, err
		}
		if p.ToolName == "" {
			p.ToolName = toolName
		} else if p.ToolName != toolName {
			return nil, fmt.Errorf("tool type prefix %q conflicts with toolName field %q", typeName, p.ToolName)
		}
		return p, requireUIToolFields(env, toolPartFields(p))
	}
	if dataName, ok := strings.CutPrefix(typeName, "data-"); ok {
		var p DataPart
		if err := json.Unmarshal(data, &p); err != nil {
			return nil, err
		}
		if p.Data == nil {
			return nil, missingUIField("data")
		}
		p.DataName = dataName
		return p, nil
	}
	return rawPart{Type_: typeName, Raw: data}, nil
}

func requireUICitationFields(raw json.RawMessage) error {
	if raw == nil {
		return nil
	}
	var citations []struct {
		Source *struct {
			Type       *provider.ContentPartType `json:"type"`
			SourceType *provider.SourceType      `json:"sourceType"`
			ID         nonNullUIField            `json:"id"`
			URL        nonNullUIField            `json:"url"`
			MediaType  nonNullUIField            `json:"mediaType"`
			Title      nonNullUIField            `json:"title"`
		} `json:"source"`
	}
	if err := json.Unmarshal(raw, &citations); err != nil {
		return err
	}
	for i, citation := range citations {
		source := citation.Source
		if source == nil {
			return fmt.Errorf("citation %d: %w", i, missingUIField("source"))
		}
		if source.Type == nil {
			return fmt.Errorf("citation %d: %w", i, missingUIField("source.type"))
		}
		if *source.Type != provider.ContentPartTypeSource {
			return fmt.Errorf("citation %d: invalid source.type, expected %q", i, provider.ContentPartTypeSource)
		}
		if source.SourceType == nil {
			return fmt.Errorf("citation %d: %w", i, missingUIField("source.sourceType"))
		}
		fields := []uiRequiredField{{"source.id", source.ID}}
		switch *source.SourceType {
		case provider.SourceTypeURL:
			fields = append(fields, uiRequiredField{"source.url", source.URL})
		case provider.SourceTypeDocument:
			fields = append(fields, uiRequiredField{"source.mediaType", source.MediaType}, uiRequiredField{"source.title", source.Title})
		}
		if err := requireUIFields(fields...); err != nil {
			return fmt.Errorf("citation %d: %w", i, err)
		}
	}
	return nil
}

func requireUIToolFields(env partEnvelope, p toolPartFields) error {
	if err := requireUIFields(uiRequiredField{"toolCallId", env.ToolCallID}, uiRequiredField{"state", env.State}); err != nil {
		return err
	}
	if toolInvocationStateRequiresInput(p.State) && p.Input == nil {
		return missingUIField("input")
	}
	switch p.State {
	case ToolStateOutputAvailable:
		if p.Output == nil {
			return missingUIField("output")
		}
	case ToolStateOutputError:
		if p.ErrorText == nil {
			return missingUIField("errorText")
		}
	case ToolStateApprovalRequested, ToolStateApprovalResponded, ToolStateOutputDenied:
		if p.Approval == nil {
			return missingUIField("approval")
		}
	default:
		return nil
	}
	if p.Approval != nil {
		var approval struct {
			ID *string `json:"id"`
		}
		if err := json.Unmarshal(env.Approval, &approval); err != nil {
			return err
		}
		if approval.ID == nil {
			return missingUIField("approval.id")
		}
		if p.State != ToolStateApprovalRequested && p.Approval.Approved == nil {
			return missingUIField("approval.approved")
		}
	}
	return nil
}

type rawPart struct {
	Type_ string
	Raw   json.RawMessage
}

func (r rawPart) PartType() string { return r.Type_ }
