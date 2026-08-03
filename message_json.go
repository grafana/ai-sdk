package aisdk

import (
	"encoding/json"
	"fmt"
	"strings"
)

var _ json.Marshaler = UIMessage{}

type uiMessageJSON struct {
	ID       string            `json:"id"`
	Role     Role              `json:"role"`
	Parts    []json.RawMessage `json:"parts"`
	Metadata json.RawMessage   `json:"metadata,omitempty"`
}

type partEnvelope struct {
	Type string `json:"type"`
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
		ID:       m.ID,
		Role:     m.Role,
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
	m.ID = raw.ID
	m.Role = raw.Role
	m.Metadata = raw.Metadata
	m.Parts = make([]Part, len(raw.Parts))
	for i, partData := range raw.Parts {
		p, err := unmarshalPart(partData)
		if err != nil {
			return fmt.Errorf("unmarshaling part %d: %w", i, err)
		}
		m.Parts[i] = p
	}
	return nil
}

func unmarshalPart(data json.RawMessage) (Part, error) {
	var env partEnvelope
	if err := json.Unmarshal(data, &env); err != nil {
		return nil, err
	}

	partType := UIPartType(env.Type)

	switch {
	case partType == UIPartText:
		var p TextPart
		if err := json.Unmarshal(data, &p); err != nil {
			return nil, err
		}
		return p, nil

	case partType == UIPartReasoning:
		var p ReasoningPart
		if err := json.Unmarshal(data, &p); err != nil {
			return nil, err
		}
		return p, nil

	case partType == UIPartFile:
		var p FilePart
		if err := json.Unmarshal(data, &p); err != nil {
			return nil, err
		}
		return p, nil

	case partType == UIPartReasoningFile:
		var p ReasoningFilePart
		if err := json.Unmarshal(data, &p); err != nil {
			return nil, err
		}
		return p, nil

	case partType == UIPartSourceURL:
		var p SourceURLPart
		if err := json.Unmarshal(data, &p); err != nil {
			return nil, err
		}
		return p, nil

	case partType == UIPartSourceDocument:
		var p SourceDocumentPart
		if err := json.Unmarshal(data, &p); err != nil {
			return nil, err
		}
		return p, nil

	case partType == UIPartCustom:
		var p CustomPart
		if err := json.Unmarshal(data, &p); err != nil {
			return nil, err
		}
		return p, nil

	case partType == UIPartStepStart:
		return StepStartPart{}, nil

	case partType == UIPartDynamicTool:
		var p DynamicToolUIPart
		if err := json.Unmarshal(data, &p); err != nil {
			return nil, err
		}
		return p, nil

	case strings.HasPrefix(env.Type, "tool-"):
		toolName := strings.TrimPrefix(env.Type, "tool-")
		var p ToolInvocationPart
		if err := json.Unmarshal(data, &p); err != nil {
			return nil, err
		}
		if p.ToolName == "" {
			p.ToolName = toolName
		} else if p.ToolName != toolName {
			return nil, fmt.Errorf("tool type prefix %q conflicts with toolName field %q", env.Type, p.ToolName)
		}
		return p, nil

	case strings.HasPrefix(env.Type, "data-"):
		dataName := strings.TrimPrefix(env.Type, "data-")
		var p DataPart
		if err := json.Unmarshal(data, &p); err != nil {
			return nil, err
		}
		p.DataName = dataName
		return p, nil

	default:
		return rawPart{Type_: env.Type, Raw: data}, nil
	}
}

type rawPart struct {
	Type_ string
	Raw   json.RawMessage
}

func (r rawPart) PartType() string { return r.Type_ }
