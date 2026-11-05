package openaicompatible

import (
	"encoding/json"
	"errors"
	"maps"
	"slices"
	"unicode/utf8"

	"github.com/grafana/ai-sdk/provider"
)

// ChatCustomTool is a native text or grammar-constrained tool declaration.
type ChatCustomTool struct {
	Name        string                `json:"name"`
	Description *string               `json:"description,omitempty"`
	Format      *ChatCustomToolFormat `json:"format,omitempty"`
}

// ChatCustomToolFormat describes free text or a provider-validated grammar.
type ChatCustomToolFormat struct {
	Type    string                 `json:"type"`
	Grammar *ChatCustomToolGrammar `json:"grammar,omitempty"`
}

// ChatCustomToolGrammar contains a native Lark or regular-expression grammar.
type ChatCustomToolGrammar struct {
	Syntax     string `json:"syntax"`
	Definition string `json:"definition"`
}

type chatCustomCall struct {
	Name  string `json:"name"`
	Input string `json:"input"`
}
type chatCustomCallDelta struct {
	Name  *string `json:"name"`
	Input *string `json:"input"`
}

// Validate checks the declaration shape. Grammar compilation is provider-owned.
func (t ChatCustomTool) Validate() error {
	invalid := errors.New("openai: invalid custom tool")
	if t.Name == "" || !utf8.ValidString(t.Name) {
		return invalid
	}
	if t.Description != nil && !utf8.ValidString(*t.Description) {
		return invalid
	}
	if t.Format != nil {
		switch t.Format.Type {
		case "text":
			if t.Format.Grammar != nil {
				return invalid
			}
		case "grammar":
			if t.Format.Grammar == nil || (t.Format.Grammar.Syntax != "lark" && t.Format.Grammar.Syntax != "regex") || t.Format.Grammar.Definition == "" || !utf8.ValidString(t.Format.Grammar.Definition) {
				return invalid
			}
		default:
			return invalid
		}
	}
	return nil
}

func customToolDefinition(t provider.Tool) (*ChatCustomTool, error) {
	out := &ChatCustomTool{Name: t.Name}
	if raw := t.Args["description"]; len(raw) > 0 {
		if json.Unmarshal(raw, &out.Description) != nil {
			return nil, errors.New("openai: invalid custom description")
		}
	}
	if raw := t.Args["format"]; len(raw) > 0 {
		if json.Unmarshal(raw, &out.Format) != nil {
			return nil, errors.New("openai: invalid custom format")
		}
	}
	return out, out.Validate()
}

func validateCustomTools(tools []provider.Tool) error {
	for _, tool := range tools {
		if tool.Type == provider.ToolTypeProvider && tool.ID == "openai.custom" {
			if _, err := customToolDefinition(tool); err != nil {
				return err
			}
		}
	}
	return nil
}

func customHistoryCall(part provider.ContentPart) (*chatToolCall, error) {
	extra, err := openAICompatibleMetadata(part.ProviderOptions)
	if err != nil {
		return nil, err
	}
	marker, ok := extra["customTool"]
	if !ok {
		return nil, nil
	}
	custom, ok := marker.(bool)
	if !ok {
		return nil, errors.New("openai: invalid custom tool marker")
	}
	if !custom {
		return nil, nil
	}
	var input string
	if json.Unmarshal(part.Input, &input) != nil {
		return nil, errors.New("openai: custom tool input must encode text")
	}
	return &chatToolCall{ID: part.ToolCallID, Type: "custom", Custom: &chatCustomCall{Name: part.ToolName, Input: input}}, nil
}

func toolEncoding(custom bool) provider.ToolInputEncoding {
	if custom {
		return provider.ToolInputEncodingText
	}
	return ""
}

func markDeclaredCustomHistory(prompt []provider.Message, tools []provider.Tool) []provider.Message {
	names := map[string]bool{}
	for _, tool := range tools {
		if tool.Type == provider.ToolTypeProvider && tool.ID == "openai.custom" {
			names[tool.Name] = true
		}
	}
	if len(names) == 0 {
		return prompt
	}
	out := slices.Clone(prompt)
	for i, message := range out {
		out[i].Content = slices.Clone(message.Content)
		for j, part := range out[i].Content {
			if part.Type != provider.ContentPartTypeToolCall || !names[part.ToolName] {
				continue
			}
			extra, err := openAICompatibleMetadata(part.ProviderOptions)
			if err != nil {
				continue
			}
			extra = maps.Clone(extra)
			if extra == nil {
				extra = map[string]any{}
			}
			extra["customTool"] = true
			raw, _ := json.Marshal(extra)
			part.ProviderOptions = maps.Clone(part.ProviderOptions)
			if part.ProviderOptions == nil {
				part.ProviderOptions = provider.ProviderOptions{}
			}
			part.ProviderOptions["openaiCompatible"] = provider.RawProviderOption{Key: "openaiCompatible", Raw: raw}
			out[i].Content[j] = part
		}
	}
	return out
}
