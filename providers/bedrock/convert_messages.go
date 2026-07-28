package bedrock

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/grafana/ai-sdk/provider"
)

// imageMediaTypeFormat maps `image/<X>` media types to the Bedrock image
// format string (`jpeg`, `png`, `gif`, `webp`).
var invalidBedrockToolNameCharacters = regexp.MustCompile(`[^a-zA-Z0-9_-]`)

var imageMediaTypeFormat = map[string]string{
	"image/jpeg": "jpeg",
	"image/png":  "png",
	"image/gif":  "gif",
	"image/webp": "webp",
}

// documentMediaTypeFormat maps document MIME types to Bedrock's `format`
// strings. Mirrors upstream BEDROCK_DOCUMENT_MIME_TYPES.
var documentMediaTypeFormat = map[string]string{
	"application/pdf":    "pdf",
	"text/csv":           "csv",
	"application/msword": "doc",
	"application/vnd.openxmlformats-officedocument.wordprocessingml.document": "docx",
	"application/vnd.ms-excel": "xls",
	"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet": "xlsx",
	"text/html":     "html",
	"text/plain":    "txt",
	"text/markdown": "md",
}

// convertedPrompt is the output of [convertPrompt]. Tool results live under
// user-role messages per Converse contract, so we keep system blocks
// separate.
type convertedPrompt struct {
	System   []systemContentBlock
	Messages []converseMessage
}

// promptBlockKind groups consecutive messages by role family. Bedrock
// Converse expects strictly alternating user/assistant after the system
// block, so adjacent user+tool messages must collapse into a single user
// message.
type promptBlockKind int

const (
	promptBlockSystem promptBlockKind = iota
	promptBlockUser
	promptBlockAssistant
)

type promptBlock struct {
	kind     promptBlockKind
	messages []provider.Message
}

// groupPromptBlocks merges adjacent messages by role family. System messages
// form a system block, user+tool messages a user block, assistant messages
// an assistant block. Mirrors upstream `groupIntoBlocks`.
func groupPromptBlocks(prompt []provider.Message) []promptBlock {
	var blocks []promptBlock
	for _, msg := range prompt {
		var kind promptBlockKind
		switch msg.Role {
		case provider.RoleSystem:
			kind = promptBlockSystem
		case provider.RoleAssistant:
			kind = promptBlockAssistant
		case provider.RoleUser, provider.RoleTool:
			kind = promptBlockUser
		default:
			continue
		}
		if n := len(blocks); n > 0 && blocks[n-1].kind == kind {
			blocks[n-1].messages = append(blocks[n-1].messages, msg)
			continue
		}
		blocks = append(blocks, promptBlock{kind: kind, messages: []provider.Message{msg}})
	}
	return blocks
}

// convertPrompt translates `provider.CallOptions.Prompt` into Bedrock
// Converse `system` and `messages` arrays. Reports warnings inline via
// the warnings slice (passed by reference style; appended into ctx).
//
// Returns the converted prompt, a list of warnings collected during
// translation, and an error when prompt content is unsupported or provider
// options are malformed.
func convertPrompt(prompt []provider.Message, isMistral bool, hasAnyTools bool) (convertedPrompt, []provider.Warning, error) {
	var warnings []provider.Warning
	var out convertedPrompt
	documentCounter := 0

	blocks := groupPromptBlocks(prompt)
	for blockIndex, block := range blocks {
		switch block.kind {
		case promptBlockSystem:
			// Bedrock requires all system messages to precede any
			// user/assistant message. Upstream throws an
			// UnsupportedFunctionalityError here; per this port's convention
			// we emit a warning and still forward the system text rather than
			// fail the request.
			if len(out.Messages) > 0 {
				warnings = append(warnings, provider.Warning{
					Type:    provider.WarnUnsupported,
					Feature: "systemMessage",
					Details: "Multiple system messages that are separated by user/assistant messages are not supported by Bedrock; the later system message is forwarded but may be rejected.",
				})
			}
			blocks, err := convertSystemBlock(block.messages, &warnings)
			if err != nil {
				return convertedPrompt{}, warnings, err
			}
			out.System = append(out.System, blocks...)

		case promptBlockUser:
			// Merge all user+tool messages within this block into one user
			// message per Converse contract.
			var content []contentBlock
			for _, msg := range block.messages {
				switch msg.Role {
				case provider.RoleUser:
					blocks, err := convertUserContent(msg.Content, &documentCounter, &warnings, isMistral)
					if err != nil {
						return convertedPrompt{}, warnings, err
					}
					content = append(content, blocks...)
				case provider.RoleTool:
					blocks, err := convertToolContent(msg.Content, &documentCounter, &warnings, isMistral)
					if err != nil {
						return convertedPrompt{}, warnings, err
					}
					content = append(content, blocks...)
				}
				cp, err := extractCachePoint(msg.ProviderOptions)
				if err != nil {
					return convertedPrompt{}, warnings, err
				}
				if cp != nil {
					content = append(content, contentBlock{CachePoint: cp})
				}
			}
			if len(content) > 0 {
				out.Messages = append(out.Messages, converseMessage{Role: "user", Content: content})
			}

		case promptBlockAssistant:
			var content []contentBlock
			for messageIndex, msg := range block.messages {
				trimLastText := blockIndex == len(blocks)-1 && messageIndex == len(block.messages)-1
				contentBlocks, err := convertAssistantContent(msg.Content, &warnings, isMistral, trimLastText)
				if err != nil {
					return convertedPrompt{}, warnings, err
				}
				content = append(content, contentBlocks...)
				cp, err := extractCachePoint(msg.ProviderOptions)
				if err != nil {
					return convertedPrompt{}, warnings, err
				}
				if cp != nil {
					content = append(content, contentBlock{CachePoint: cp})
				}
			}
			if len(content) > 0 {
				out.Messages = append(out.Messages, converseMessage{Role: "assistant", Content: content})
			}
		}
	}

	// Filter tool content when no tools are active. Matches upstream behavior
	// that prevents 400s from Bedrock when a tool-result block is sent
	// without active tools.
	if !hasAnyTools {
		out.Messages, warnings = filterToolContentFromMessages(out.Messages, warnings)
	}

	return out, warnings, nil
}

func convertSystemBlock(messages []provider.Message, warnings *[]provider.Warning) ([]systemContentBlock, error) {
	var out []systemContentBlock
	for _, msg := range messages {
		for _, part := range msg.Content {
			if part.Type != provider.ContentPartTypeText {
				*warnings = append(*warnings, provider.Warning{
					Type:    provider.WarnUnsupported,
					Feature: "systemContentPart",
					Details: fmt.Sprintf("Bedrock system messages only support text; ignoring %q", part.Type),
				})
				continue
			}
			out = append(out, systemContentBlock{Text: part.Text})
		}
		cp, err := extractCachePoint(msg.ProviderOptions)
		if err != nil {
			return nil, err
		}
		if cp != nil {
			out = append(out, systemContentBlock{CachePoint: cp})
		}
	}
	return out, nil
}

func convertUserContent(parts []provider.ContentPart, documentCounter *int, warnings *[]provider.Warning, isMistral bool) ([]contentBlock, error) {
	var out []contentBlock
	for _, p := range parts {
		switch p.Type {
		case provider.ContentPartTypeText:
			out = append(out, contentBlock{Text: p.Text})

		case provider.ContentPartTypeFile:
			if p.Data == nil {
				continue
			}
			switch {
			case len(p.Data.Reference) > 0:
				return nil, fmt.Errorf("bedrock: file parts with provider references are not supported")
			case p.Data.URL != "":
				if !isS3URL(p.Data.URL) {
					return nil, fmt.Errorf("bedrock: file URL data is not supported")
				}
				mediaType := p.MediaType
				if !isFullMediaType(mediaType) || topLevelMediaType(mediaType) != "image" {
					return nil, fmt.Errorf("bedrock: file URL data is only supported for images with a full media type")
				}
				format, ok := imageMediaTypeFormat[mediaType]
				if !ok {
					return nil, fmt.Errorf("bedrock: image media type %q is not supported", mediaType)
				}
				out = append(out, contentBlock{Image: &imageBlock{
					Format: format,
					Source: imageSource{S3Location: &s3LocationBlock{URI: p.Data.URL}},
				}})
				continue
			case p.Data.Text != "":
				mediaType := p.MediaType
				if !isFullMediaType(mediaType) {
					mediaType = "text/plain"
				}
				block, err := buildDocumentContentBlock(p, mediaType, base64.StdEncoding.EncodeToString([]byte(p.Data.Text)), documentCounter)
				if err != nil {
					return nil, err
				}
				out = append(out, block)
				continue
			}

			b64 := p.Data.Base64
			if b64 == "" && len(p.Data.Bytes) > 0 {
				b64 = base64.StdEncoding.EncodeToString(p.Data.Bytes)
			}
			if b64 == "" {
				continue
			}
			mediaType, err := resolveFullMediaType(p)
			if err != nil {
				return nil, err
			}
			if topLevelMediaType(mediaType) == "image" {
				format, ok := imageMediaTypeFormat[mediaType]
				if !ok {
					return nil, fmt.Errorf("bedrock: image media type %q is not supported", mediaType)
				}
				out = append(out, contentBlock{
					Image: &imageBlock{Format: format, Source: imageSource{Bytes: b64}},
				})
				continue
			}
			block, err := buildDocumentContentBlock(p, mediaType, b64, documentCounter)
			if err != nil {
				return nil, err
			}
			out = append(out, block)

		case provider.ContentPartTypeToolResult:
			toolResult, err := buildToolResult(p, documentCounter, isMistral, warnings)
			if err != nil {
				return nil, err
			}
			out = append(out, contentBlock{ToolResult: toolResult})

		case provider.ContentPartTypeToolApprovalResponse:
			// Approval responses are local-only conversation bookkeeping; skip
			// silently, matching upstream behavior for user-role approval
			// responses.
		}
	}
	return out, nil
}

func buildDocumentContentBlock(p provider.ContentPart, mediaType, b64 string, documentCounter *int) (contentBlock, error) {
	document, err := buildDocumentBlock(mediaType, p.Filename, b64, p.ProviderOptions, documentCounter)
	if err != nil {
		return contentBlock{}, err
	}
	return contentBlock{Document: document}, nil
}

func buildDocumentBlock(mediaType, filename, b64 string, providerOptions provider.ProviderOptions, documentCounter *int) (*documentBlock, error) {
	enableCitations, err := shouldEnableCitations(providerOptions)
	if err != nil {
		return nil, err
	}
	format, ok := documentMediaTypeFormat[mediaType]
	if !ok {
		return nil, fmt.Errorf("bedrock: file media type %q is not supported", mediaType)
	}
	name := filename
	if name == "" {
		*documentCounter++
		name = fmt.Sprintf("document-%d", *documentCounter)
	}
	document := &documentBlock{
		Format: format,
		Name:   stripFileExtension(name),
		Source: documentSource{Bytes: b64},
	}
	if enableCitations {
		document.Citations = &documentCitations{Enabled: true}
	}
	return document, nil
}

func resolveFullMediaType(p provider.ContentPart) (string, error) {
	if isFullMediaType(p.MediaType) {
		return p.MediaType, nil
	}
	var data []byte
	if len(p.Data.Bytes) > 0 {
		data = p.Data.Bytes
	} else {
		var err error
		data, err = decodeBase64MediaPrefix(p.Data.Base64)
		if err != nil {
			return "", fmt.Errorf("bedrock: decoding file data: %w", err)
		}
	}
	// Match upstream by restricting signature detection to the declared top-level
	// type instead of reclassifying data whose bytes contradict that declaration.
	if detected := detectMediaType(data, topLevelMediaType(p.MediaType)); detected != "" {
		return detected, nil
	}
	return "", fmt.Errorf("bedrock: file of media type %q must specify subtype since it could not be auto-detected", p.MediaType)
}

const mediaTypeDetectionBase64PrefixLength = 24

// decodeBase64MediaPrefix decodes only the prefix used for media signature
// detection. It intentionally matches upstream's browser atob behavior by
// accepting URL-safe, padded, unpadded, mixed-alphabet, and whitespace forms.
func decodeBase64MediaPrefix(value string) ([]byte, error) {
	if len(value) > mediaTypeDetectionBase64PrefixLength {
		value = value[:mediaTypeDetectionBase64PrefixLength]
	}
	var normalized strings.Builder
	normalized.Grow(len(value))
	for _, char := range value {
		switch char {
		case '-':
			normalized.WriteByte('+')
		case '_':
			normalized.WriteByte('/')
		case '\t', '\n', '\f', '\r', ' ':
			continue
		default:
			normalized.WriteRune(char)
		}
	}
	decoded, err := base64.StdEncoding.DecodeString(normalized.String())
	if err == nil {
		return decoded, nil
	}
	return base64.RawStdEncoding.DecodeString(normalized.String())
}

func isFullMediaType(mediaType string) bool {
	_, subtype, ok := strings.Cut(mediaType, "/")
	return ok && subtype != "" && subtype != "*"
}

func topLevelMediaType(mediaType string) string {
	topLevel, _, _ := strings.Cut(mediaType, "/")
	return topLevel
}

func isS3URL(value string) bool {
	return strings.HasPrefix(value, "s3://")
}

func sanitizeToolName(name string) string {
	sanitized := invalidBedrockToolNameCharacters.ReplaceAllString(name, "")
	if sanitized == "" {
		return "_"
	}
	return sanitized
}

type mediaTypeSignature struct {
	mediaType string
	prefix    []int
}

func detectMediaType(data []byte, topLevelType string) string {
	var signatures []mediaTypeSignature
	switch topLevelType {
	case "image":
		signatures = []mediaTypeSignature{
			{mediaType: "image/gif", prefix: []int{0x47, 0x49, 0x46}},
			{mediaType: "image/png", prefix: []int{0x89, 0x50, 0x4e, 0x47}},
			{mediaType: "image/jpeg", prefix: []int{0xff, 0xd8}},
			{mediaType: "image/webp", prefix: []int{0x52, 0x49, 0x46, 0x46, -1, -1, -1, -1, 0x57, 0x45, 0x42, 0x50}},
			{mediaType: "image/bmp", prefix: []int{0x42, 0x4d}},
			{mediaType: "image/tiff", prefix: []int{0x49, 0x49, 0x2a, 0x00}},
			{mediaType: "image/tiff", prefix: []int{0x4d, 0x4d, 0x00, 0x2a}},
			{mediaType: "image/avif", prefix: []int{0x00, 0x00, 0x00, 0x20, 0x66, 0x74, 0x79, 0x70, 0x61, 0x76, 0x69, 0x66}},
			{mediaType: "image/heic", prefix: []int{0x00, 0x00, 0x00, 0x20, 0x66, 0x74, 0x79, 0x70, 0x68, 0x65, 0x69, 0x63}},
		}
	case "application":
		signatures = []mediaTypeSignature{
			{mediaType: "application/pdf", prefix: []int{0x25, 0x50, 0x44, 0x46}},
		}
	}
	for _, signature := range signatures {
		if hasSignature(data, signature.prefix) {
			return signature.mediaType
		}
	}
	return ""
}

func hasSignature(data []byte, prefix []int) bool {
	if len(data) < len(prefix) {
		return false
	}
	for i, value := range prefix {
		if value >= 0 && data[i] != byte(value) {
			return false
		}
	}
	return true
}

func convertToolContent(parts []provider.ContentPart, documentCounter *int, warnings *[]provider.Warning, isMistral bool) ([]contentBlock, error) {
	var out []contentBlock
	for _, p := range parts {
		switch p.Type {
		case provider.ContentPartTypeToolResult:
			toolResult, err := buildToolResult(p, documentCounter, isMistral, warnings)
			if err != nil {
				return nil, err
			}
			out = append(out, contentBlock{ToolResult: toolResult})
		case provider.ContentPartTypeToolApprovalResponse:
			// Same skip behavior as in user-role conversion.
		}
	}
	return out, nil
}

func convertAssistantContent(parts []provider.ContentPart, warnings *[]provider.Warning, isMistral bool, trimLastText bool) ([]contentBlock, error) {
	var out []contentBlock
	for partIndex, p := range parts {
		switch p.Type {
		case provider.ContentPartTypeText:
			// Skip empty text parts unless they coexist with reasoning blocks
			// (matches upstream `trimIfLast`/empty-text behavior).
			if trimECMAScriptWhitespace(p.Text) == "" && !containsReasoning(parts) {
				continue
			}
			text := p.Text
			if trimLastText && partIndex == len(parts)-1 {
				text = trimECMAScriptWhitespace(text)
			}
			out = append(out, contentBlock{Text: text})

		case provider.ContentPartTypeReasoning:
			meta, _, err := readReasoningMetadata(p.ProviderOptions)
			if err != nil {
				return nil, err
			}
			rc := &reasoningContentBlock{}
			switch {
			case meta.Signature != "":
				rc.ReasoningText = &reasoningText{Text: p.Text, Signature: meta.Signature}
			case meta.RedactedData != "":
				rc.RedactedReasoning = &redactedReasoning{Data: meta.RedactedData}
			default:
				continue
			}
			out = append(out, contentBlock{ReasoningContent: rc})

		case provider.ContentPartTypeToolCall:
			input := toBedrockToolInput(p.Input)
			out = append(out, contentBlock{
				ToolUse: &toolUseBlock{
					ToolUseID: normalizeToolCallID(p.ToolCallID, isMistral),
					Name:      sanitizeToolName(p.ToolName),
					Input:     input,
				},
			})

		case provider.ContentPartTypeToolApprovalRequest:
			// Tool approval requests are local conversation bookkeeping.
			// Bedrock does not accept them; skip silently.

		default:
			*warnings = append(*warnings, provider.Warning{
				Type:    provider.WarnUnsupported,
				Feature: "assistantContentPart",
				Details: fmt.Sprintf("Bedrock assistant messages do not support %q", p.Type),
			})
		}
	}
	return out, nil
}

func toBedrockToolInput(input json.RawMessage) json.RawMessage {
	if len(input) == 0 {
		return json.RawMessage(`{}`)
	}

	var value any
	if err := json.Unmarshal(input, &value); err != nil {
		wrapped, _ := json.Marshal(map[string]any{"rawInvalidInput": string(input)})
		return json.RawMessage(wrapped)
	}
	if _, ok := value.(map[string]any); ok {
		return input
	}
	wrapped, _ := json.Marshal(map[string]any{"rawInvalidInput": value})
	return json.RawMessage(wrapped)
}

func containsReasoning(parts []provider.ContentPart) bool {
	for _, p := range parts {
		if p.Type == provider.ContentPartTypeReasoning {
			return true
		}
	}
	return false
}

func trimECMAScriptWhitespace(text string) string {
	return strings.TrimFunc(text, func(r rune) bool {
		switch r {
		case '\t', '\n', '\v', '\f', '\r', ' ', '\u00a0', '\u1680', '\u2028', '\u2029', '\u202f', '\u205f', '\u3000', '\ufeff':
			return true
		default:
			return r >= '\u2000' && r <= '\u200a'
		}
	})
}

// buildToolResult converts a tool-result part into a Converse toolResult
// block. Supported content outputs are mapped to text, image, and document
// blocks; unsupported variants degrade to a single empty text block and emit
// a warning.
func buildToolResult(p provider.ContentPart, documentCounter *int, isMistral bool, warnings *[]provider.Warning) (*toolResultBlock, error) {
	out := &toolResultBlock{
		ToolUseID: normalizeToolCallID(p.ToolCallID, isMistral),
	}
	if p.Output == nil {
		out.Content = []toolResultContent{{Text: ""}}
		return out, nil
	}
	switch p.Output.Type {
	case provider.ToolOutputText, provider.ToolOutputErrorText:
		out.Content = []toolResultContent{{Text: p.Output.Text}}
	case provider.ToolOutputJSON, provider.ToolOutputErrorJSON:
		out.Content = []toolResultContent{{Text: string(p.Output.JSON)}}
	case provider.ToolOutputExecutionDenied:
		reason := p.Output.Reason
		if reason == "" {
			reason = "Tool call execution denied."
		}
		out.Content = []toolResultContent{{Text: reason}}
	case provider.ToolOutputContent:
		for _, c := range p.Output.Content {
			switch c.Type {
			case provider.ToolContentText:
				out.Content = append(out.Content, toolResultContent{Text: c.Text})
			case provider.ToolContentFileData:
				filePart := provider.ContentPart{
					MediaType:       c.MediaType,
					Filename:        c.Filename,
					Data:            &provider.DataContent{Base64: c.Data},
					ProviderOptions: c.ProviderOptions,
				}
				mediaType, err := resolveFullMediaType(filePart)
				if err != nil {
					return nil, err
				}
				if topLevelMediaType(mediaType) == "image" {
					format, ok := imageMediaTypeFormat[mediaType]
					if !ok {
						return nil, fmt.Errorf("bedrock: image media type %q is not supported", mediaType)
					}
					out.Content = append(out.Content, toolResultContent{
						Image: &imageBlock{Format: format, Source: imageSource{Bytes: c.Data}},
					})
					continue
				}
				document, err := buildDocumentBlock(mediaType, c.Filename, c.Data, c.ProviderOptions, documentCounter)
				if err != nil {
					return nil, err
				}
				out.Content = append(out.Content, toolResultContent{Document: document})
			case provider.ToolContentFileURL:
				if format, ok := imageMediaTypeFormat[c.MediaType]; ok && isS3URL(c.URL) {
					out.Content = append(out.Content, toolResultContent{
						Image: &imageBlock{Format: format, Source: imageSource{S3Location: &s3LocationBlock{URI: c.URL}}},
					})
				} else {
					*warnings = append(*warnings, provider.Warning{
						Type:    provider.WarnUnsupported,
						Feature: "toolResultFileURL",
						Details: fmt.Sprintf("Bedrock tool results only accept S3 image URLs; got %q with media type %q", c.URL, c.MediaType),
					})
				}
			default:
				*warnings = append(*warnings, provider.Warning{
					Type:    provider.WarnUnsupported,
					Feature: "toolResultContentPart",
					Details: fmt.Sprintf("Bedrock tool results do not support %q", c.Type),
				})
			}
		}
		if len(out.Content) == 0 {
			out.Content = []toolResultContent{{Text: ""}}
		}
	default:
		*warnings = append(*warnings, provider.Warning{
			Type:    provider.WarnUnsupported,
			Feature: "toolResultOutputType",
			Details: fmt.Sprintf("Bedrock tool results do not support %q", p.Output.Type),
		})
		out.Content = []toolResultContent{{Text: ""}}
	}
	return out, nil
}

// filterToolContentFromMessages strips tool-call and tool-result blocks from
// every message when no tools are active. Bedrock 400s otherwise.
func filterToolContentFromMessages(messages []converseMessage, warnings []provider.Warning) ([]converseMessage, []provider.Warning) {
	hasToolContent := false
	for _, m := range messages {
		for _, b := range m.Content {
			if b.ToolUse != nil || b.ToolResult != nil {
				hasToolContent = true
				break
			}
		}
		if hasToolContent {
			break
		}
	}
	if !hasToolContent {
		return messages, warnings
	}
	out := make([]converseMessage, 0, len(messages))
	for _, m := range messages {
		filtered := make([]contentBlock, 0, len(m.Content))
		for _, b := range m.Content {
			if b.ToolUse != nil || b.ToolResult != nil {
				continue
			}
			filtered = append(filtered, b)
		}
		if len(filtered) > 0 {
			out = append(out, converseMessage{Role: m.Role, Content: filtered})
		}
	}
	warnings = append(warnings, provider.Warning{
		Type:    provider.WarnUnsupported,
		Feature: "toolContent",
		Details: "Tool calls and results removed from conversation because Bedrock does not support tool content without active tools.",
	})
	return out, warnings
}

// stripFileExtension trims the suffix starting at the first `.` so document
// names don't include extension segments (matches upstream's behavior of
// `stripFileExtension`).
func stripFileExtension(name string) string {
	if dot := strings.Index(name, "."); dot >= 0 {
		return name[:dot]
	}
	return name
}
