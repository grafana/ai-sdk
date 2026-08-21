package provider

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// Role identifies the author of a [Message].
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// Message is a single message in a [CallOptions.Prompt]. It is a flat
// discriminated struct: the [Message.Role] field selects the variant.
//
// This shape mirrors LanguageModelV4Message from upstream (Vercel AI SDK).
// TypeScript discriminated unions serialize directly to JSON; in Go we model
// the union as one struct with a Role discriminator and a slice of flat
// [ContentPart] values. Constructor helpers ([NewSystemMessage] et al.)
// preserve ergonomic construction at producer call sites.
//
// Producer-side rules (e.g. "user messages contain only text and file parts")
// are enforced by orchestration and direct provider request boundaries, not by
// the flat type itself.
type Message struct {
	Role            Role            `json:"role"`
	Content         []ContentPart   `json:"content"`
	ProviderOptions ProviderOptions `json:"providerOptions,omitempty"`
}

// MarshalJSON emits the compatibility Vercel AI SDK LanguageModelV4 message shape. A
// system message's content is emitted as a plain JSON string (the concatenation
// of its text parts), matching `LanguageModelV4Message` where system content is
// a `string`. All other roles emit content as the canonical part array.
// Decoding remains tolerant of both shapes (see [Message.UnmarshalJSON]).
// See openspec change provider-wire-upstream-full-compat.
func (m Message) MarshalJSON() ([]byte, error) {
	type alias Message
	if m.Role != RoleSystem {
		return json.Marshal(alias(m))
	}
	var sb strings.Builder
	for _, part := range m.Content {
		if part.Type == ContentPartTypeText {
			sb.WriteString(part.Text)
		}
	}
	return json.Marshal(struct {
		Role            Role            `json:"role"`
		Content         string          `json:"content"`
		ProviderOptions ProviderOptions `json:"providerOptions,omitempty"`
	}{Role: m.Role, Content: sb.String(), ProviderOptions: m.ProviderOptions})
}

// UnmarshalJSON decodes a [Message] from the canonical Go-to-Go wire form and
// additionally tolerates the upstream Vercel AI SDK LanguageModelV4 shape where
// a system message's content is a bare JSON string
// (`{"role":"system","content":"..."}`). A string content is wrapped into a
// single [ContentPartTypeText] part so the in-memory representation is uniform.
//
// This is decode-only tolerance; marshaling continues to emit the canonical
// array form. See openspec change provider-wire-upstream-decode-compat.
//
// String content is accepted only for the system role, matching the upstream
// dialect (`LanguageModelV4Message` types system content as `string` and all
// other roles as content arrays). A string content on any other role fails
// closed rather than silently widening the accepted input.
func (m *Message) UnmarshalJSON(data []byte) error {
	var raw struct {
		Role            Role            `json:"role"`
		Content         json.RawMessage `json:"content"`
		ProviderOptions ProviderOptions `json:"providerOptions"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	m.Role = raw.Role
	m.ProviderOptions = raw.ProviderOptions
	m.Content = nil

	trimmed := bytes.TrimSpace(raw.Content)
	if len(trimmed) == 0 || string(trimmed) == "null" {
		return nil
	}
	// Upstream system messages carry content as a JSON string.
	if trimmed[0] == '"' {
		if raw.Role != RoleSystem {
			return fmt.Errorf("provider: string message content is only valid for the system role, got role %q", raw.Role)
		}
		var s string
		if err := json.Unmarshal(trimmed, &s); err != nil {
			return err
		}
		m.Content = []ContentPart{{Type: ContentPartTypeText, Text: s}}
		return nil
	}
	var parts []ContentPart
	if err := json.Unmarshal(trimmed, &parts); err != nil {
		return err
	}
	m.Content = parts
	return nil
}

// NewSystemMessage creates a system-role [Message] containing a single
// [ContentPartTypeText] part with the given text.
func NewSystemMessage(text string) Message {
	return Message{
		Role:    RoleSystem,
		Content: []ContentPart{{Type: ContentPartTypeText, Text: text}},
	}
}

// NewUserMessage creates a user-role [Message] with the given content parts.
func NewUserMessage(parts ...ContentPart) Message {
	return Message{Role: RoleUser, Content: parts}
}

// NewAssistantMessage creates an assistant-role [Message] with the given content parts.
func NewAssistantMessage(parts ...ContentPart) Message {
	return Message{Role: RoleAssistant, Content: parts}
}

// NewToolMessage creates a tool-role [Message] with the given content parts.
func NewToolMessage(parts ...ContentPart) Message {
	return Message{Role: RoleTool, Content: parts}
}

// TextParts returns a single-element slice containing a [ContentPartTypeText]
// part wrapping the given string. Convenience for constructing user messages.
//
// Deprecated: use [TextPart] together with [NewUserMessage] et al., or the
// role-text shortcuts [UserText] / [AssistantText].
func TextParts(text string) []ContentPart {
	return []ContentPart{TextPart(text)}
}

// UserText is a shortcut for `NewUserMessage(TextPart(text))`.
func UserText(text string) Message {
	return NewUserMessage(TextPart(text))
}

// AssistantText is a shortcut for `NewAssistantMessage(TextPart(text))`.
func AssistantText(text string) Message {
	return NewAssistantMessage(TextPart(text))
}
