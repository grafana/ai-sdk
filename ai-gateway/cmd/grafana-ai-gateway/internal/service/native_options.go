package service

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/grafana/ai-sdk/ai-gateway/catalog"
	"github.com/grafana/ai-sdk/provider"
	anthropicprovider "github.com/grafana/ai-sdk/providers/anthropic"
)

type nativeOptionsModel struct {
	provider.LanguageModel
	validate func(provider.CallOptions) error
}

func (m nativeOptionsModel) DoGenerate(ctx context.Context, options provider.CallOptions) (*provider.GenerateResult, error) {
	if err := m.validate(options); err != nil {
		return nil, err
	}
	return m.LanguageModel.DoGenerate(ctx, options)
}

func (m nativeOptionsModel) DoStream(ctx context.Context, options provider.CallOptions) (*provider.StreamResult, error) {
	if err := m.validate(options); err != nil {
		return nil, err
	}
	return m.LanguageModel.DoStream(ctx, options)
}

func validateAnthropicOptions(options provider.CallOptions) error {
	call, _, err := provider.ResolveOption[anthropicprovider.AnthropicOptions](options.ProviderOptions, "anthropic")
	if err != nil {
		return catalog.ErrUnsupportedRequest
	}
	if (call.Container != nil && len(call.Container.Skills) > 0) ||
		(call.Fallbacks != nil && (call.Fallbacks.Default || len(call.Fallbacks.Chain) > 0)) {
		return catalog.ErrUnsupportedRequest
	}
	servers, err := validateMCPServers(call.MCPServers)
	if err != nil {
		return err
	}
	for _, message := range options.Prompt {
		if message.Role != provider.RoleAssistant {
			continue
		}
		for _, part := range message.Content {
			if part.Type != provider.ContentPartTypeToolCall || !part.ProviderExecuted {
				continue
			}
			if raw, ok := part.ProviderOptions["anthropic"].(provider.RawProviderOption); ok {
				var history struct {
					Type string `json:"type"`
				}
				if json.Unmarshal(raw.Raw, &history) == nil && history.Type == "mcp-tool-use" {
					var fields map[string]json.RawMessage
					var serverName string
					if json.Unmarshal(raw.Raw, &fields) != nil || json.Unmarshal(fields["serverName"], &serverName) != nil || !servers[serverName] {
						return catalog.ErrUnsupportedRequest
					}
				}
			}
		}
	}
	return nil
}

func validateCompatibleOptions(options provider.CallOptions, providerName string) error {
	name, _, _ := strings.Cut(providerName, ".")
	name = strings.TrimSpace(name)
	for _, namespace := range []string{name, compatibleCamelName(name)} {
		if err := refuseNativeFields(options.ProviderOptions, namespace, "model", "response_format", "functions", "function_call"); err != nil {
			return err
		}
	}
	for _, message := range options.Prompt {
		switch message.Role {
		case provider.RoleSystem, provider.RoleAssistant:
			if err := refuseCompatibleMessageFields(message.ProviderOptions); err != nil {
				return err
			}
		case provider.RoleUser:
			if len(message.Content) == 1 && message.Content[0].Type == provider.ContentPartTypeText {
				if err := refuseCompatibleMessageFields(message.Content[0].ProviderOptions); err != nil {
					return err
				}
				continue
			}
			if err := refuseCompatibleMessageFields(message.ProviderOptions); err != nil {
				return err
			}
		}
		for _, part := range message.Content {
			switch {
			case message.Role == provider.RoleUser:
				if err := refuseNativeFields(part.ProviderOptions, "openaiCompatible", "type", "text", "image_url", "input_audio", "file"); err != nil {
					return err
				}
			case message.Role == provider.RoleAssistant && part.Type == provider.ContentPartTypeToolCall:
				if err := refuseNativeFields(part.ProviderOptions, "openaiCompatible", "id", "type", "function"); err != nil {
					return err
				}
			case message.Role == provider.RoleTool && part.Type == provider.ContentPartTypeToolResult:
				if err := refuseCompatibleMessageFields(part.ProviderOptions); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func refuseCompatibleMessageFields(options provider.ProviderOptions) error {
	return refuseNativeFields(options, "openaiCompatible", "role", "content", "tool_calls", "tool_call_id", "function_call", "reasoning_content")
}

func refuseNativeFields(options provider.ProviderOptions, namespace string, fields ...string) error {
	value, exists := options[namespace]
	if !exists {
		return nil
	}
	var encoded []byte
	switch value := value.(type) {
	case provider.RawProviderOption:
		encoded = value.Raw
	default:
		var err error
		encoded, err = json.Marshal(value)
		if err != nil {
			return err
		}
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &object); err != nil {
		return err
	}
	for _, field := range fields {
		if _, exists := object[field]; exists {
			return catalog.ErrUnsupportedRequest
		}
	}
	return nil
}

func compatibleCamelName(name string) string {
	var result strings.Builder
	upperNext := false
	for _, r := range name {
		if r == '-' || r == '_' {
			upperNext = true
			continue
		}
		if upperNext && r >= 'a' && r <= 'z' {
			r -= 'a' - 'A'
		}
		result.WriteRune(r)
		upperNext = false
	}
	return result.String()
}
