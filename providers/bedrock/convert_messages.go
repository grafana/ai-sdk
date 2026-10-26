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
var invalidBedrockDocumentNameCharacters = regexp.MustCompile(`[^a-zA-Z0-9 ()\[\]-]`)

var imageMediaTypeFormat = map[string]string{
	"image/jpeg": "jpeg",
	"image/png":  "png",
	"image/gif":  "gif",
	"image/webp": "webp",
}

var videoMediaTypeFormat = map[string]string{
	"video/x-matroska": "mkv",
	"video/quicktime":  "mov",
	"video/mp4":        "mp4",
	"video/webm":       "webm",
	"video/x-flv":      "flv",
	"video/mpeg":       "mpeg",
	"video/mpg":        "mpg",
	"video/wmv":        "wmv",
	"video/x-ms-wmv":   "wmv",
	"video/3gpp":       "three_gp",
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
	if !hasAnyTools {
		prompt, warnings = filterToolContentFromPrompt(prompt, warnings)
	}

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
				contentStart := len(content)
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
				if cp != nil && len(content) > contentStart {
					content = append(content, contentBlock{CachePoint: cp})
				}
			}
			if len(content) > 0 {
				out.Messages = appendUserMessage(out.Messages, content)
			}

		case promptBlockAssistant:
			blockStart := len(out.Messages)
			for messageIndex, msg := range block.messages {
				trimLastText := blockIndex == len(blocks)-1 && messageIndex == len(block.messages)-1
				segments, err := convertAssistantContent(msg.Content, &documentCounter, &warnings, isMistral, trimLastText)
				if err != nil {
					return convertedPrompt{}, warnings, err
				}
				cp, err := extractCachePoint(msg.ProviderOptions)
				if err != nil {
					return convertedPrompt{}, warnings, err
				}
				if last := len(segments) - 1; cp != nil && last >= 0 && segments[last].Role == assistantCachePointRole(msg.Content) {
					segments[last].Content = append(segments[last].Content, contentBlock{CachePoint: cp})
				}
				for _, segment := range segments {
					if segment.Role == converseRoleUser {
						out.Messages = appendUserMessage(out.Messages, segment.Content)
						continue
					}
					if last := len(out.Messages) - 1; last >= blockStart && out.Messages[last].Role == converseRoleAssistant {
						out.Messages[last].Content = append(out.Messages[last].Content, segment.Content...)
						continue
					}
					out.Messages = append(out.Messages, segment)
				}
			}
		}
	}

	return out, warnings, nil
}

func appendUserMessage(messages []converseMessage, content []contentBlock) []converseMessage {
	if last := len(messages) - 1; last >= 0 && messages[last].Role == converseRoleUser && containsToolResult(messages[last].Content) {
		messages[last].Content = append(messages[last].Content, content...)
		return messages
	}
	return append(messages, converseMessage{Role: converseRoleUser, Content: content})
}

func containsToolResult(content []contentBlock) bool {
	for _, block := range content {
		if block.ToolResult != nil {
			return true
		}
	}
	return false
}

func assistantCachePointRole(parts []provider.ContentPart) converseRole {
	if len(parts) > 0 && parts[len(parts)-1].Type == provider.ContentPartTypeToolResult {
		return converseRoleUser
	}
	return converseRoleAssistant
}

func convertSystemBlock(messages []provider.Message, warnings *[]provider.Warning) ([]systemContentBlock, error) {
	var out []systemContentBlock
	for _, msg := range messages {
		contentStart := len(out)
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
		if cp != nil && len(out) > contentStart {
			out = append(out, systemContentBlock{CachePoint: cp})
		}
	}
	return out, nil
}

func convertUserContent(parts []provider.ContentPart, documentCounter *int, warnings *[]provider.Warning, isMistral bool) ([]contentBlock, error) {
	var out []contentBlock
	for _, p := range parts {
		start := len(out)
		switch p.Type {
		case provider.ContentPartTypeText:
			guarded, qualifiers, err := readTextGuardContent(p.ProviderOptions)
			if err != nil {
				return nil, err
			}
			if guarded {
				out = append(out, contentBlock{GuardContent: &guardContentBlock{Text: &guardContentText{Text: p.Text, Qualifiers: qualifiers}}})
			} else {
				out = append(out, contentBlock{Text: p.Text})
			}

		case provider.ContentPartTypeFile:
			blocks, err := convertUserFilePart(p, documentCounter)
			if err != nil {
				return nil, err
			}
			out = append(out, blocks...)

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
		if len(out) > start {
			var err error
			if out, err = appendPartCachePoint(out, p.ProviderOptions); err != nil {
				return nil, err
			}
		}
	}
	return out, nil
}

func appendPartCachePoint(content []contentBlock, opts provider.ProviderOptions) ([]contentBlock, error) {
	cp, err := extractPartCachePoint(opts)
	if err != nil {
		return nil, err
	}
	if cp != nil {
		content = append(content, contentBlock{CachePoint: cp})
	}
	return content, nil
}

func convertUserFilePart(p provider.ContentPart, documentCounter *int) ([]contentBlock, error) {
	var out []contentBlock
	if p.Data == nil {
		return out, nil
	}
	switch {
	case p.Data.IsReference():
		return nil, unsupportedFunctionality("bedrock: file parts with provider references are not supported")
	case p.Data.IsURL():
		if !isS3URL(p.Data.URL) {
			return nil, unsupportedFunctionality("bedrock: file URL data is not supported")
		}
		mediaType := p.MediaType
		if !isFullMediaType(mediaType) {
			return nil, fmt.Errorf("bedrock: file URL data requires a full media type")
		}
		switch topLevelMediaType(mediaType) {
		case "image":
			format, ok := imageMediaTypeFormat[mediaType]
			if !ok {
				return nil, unsupportedFunctionality("bedrock: image media type %q is not supported", mediaType)
			}
			out = append(out, contentBlock{Image: &imageBlock{
				Format: format,
				Source: imageSource{S3Location: &s3LocationBlock{URI: p.Data.URL}},
			}})
		case "video":
			format, ok := videoMediaTypeFormat[mediaType]
			if !ok {
				return nil, unsupportedFunctionality("bedrock: video media type %q is not supported", mediaType)
			}
			out = append(out, contentBlock{Video: &videoBlock{
				Format: format,
				Source: videoSource{S3Location: &s3LocationBlock{URI: p.Data.URL}},
			}})
		default:
			return nil, unsupportedFunctionality("bedrock: file URL data is only supported for images and videos")
		}
		return out, nil
	case p.Data.IsText():
		mediaType := p.MediaType
		if !isFullMediaType(mediaType) {
			mediaType = "text/plain"
		}
		block, err := buildDocumentContentBlock(p, mediaType, base64.StdEncoding.EncodeToString([]byte(p.Data.Text)), documentCounter)
		if err != nil {
			return nil, err
		}
		out = append(out, block)
		return out, nil
	}

	b64 := p.Data.Base64
	if p.Data.Bytes != nil {
		b64 = base64.StdEncoding.EncodeToString(p.Data.Bytes)
	}
	if !p.Data.IsData() {
		return out, nil
	}
	mediaType, err := resolveFullMediaType(p)
	if err != nil {
		return nil, err
	}
	switch topLevelMediaType(mediaType) {
	case "image":
		format, ok := imageMediaTypeFormat[mediaType]
		if !ok {
			return nil, unsupportedFunctionality("bedrock: image media type %q is not supported", mediaType)
		}
		image := &imageBlock{Format: format, Source: imageSource{Bytes: b64}}
		guarded, err := readImageGuardContent(p.ProviderOptions)
		if err != nil {
			return nil, err
		}
		if guarded {
			out = append(out, contentBlock{GuardContent: &guardContentBlock{Image: image}})
		} else {
			out = append(out, contentBlock{Image: image})
		}
		return out, nil
	case "video":
		format, ok := videoMediaTypeFormat[mediaType]
		if !ok {
			return nil, unsupportedFunctionality("bedrock: video media type %q is not supported", mediaType)
		}
		out = append(out, contentBlock{
			Video: &videoBlock{Format: format, Source: videoSource{Bytes: b64}},
		})
		return out, nil
	}
	block, err := buildDocumentContentBlock(p, mediaType, b64, documentCounter)
	if err != nil {
		return nil, err
	}
	return append(out, block), nil
}

func unsupportedFunctionality(format string, args ...any) error {
	return fmt.Errorf(format+": %w", append(args, provider.ErrUnsupportedFunctionality)...)
}

func buildDocumentContentBlock(p provider.ContentPart, mediaType, b64 string, documentCounter *int) (contentBlock, error) {
	document, err := buildDocumentBlock(mediaType, filenameValue(p.Filename), b64, p.ProviderOptions, documentCounter)
	if err != nil {
		return contentBlock{}, err
	}
	return contentBlock{Document: document}, nil
}

func filenameValue(filename *string) string {
	if filename == nil {
		return ""
	}
	return *filename
}

func buildDocumentBlock(mediaType, filename, b64 string, providerOptions provider.ProviderOptions, documentCounter *int) (*documentBlock, error) {
	enableCitations, err := shouldEnableCitations(providerOptions)
	if err != nil {
		return nil, err
	}
	format, ok := documentMediaTypeFormat[mediaType]
	if !ok {
		return nil, unsupportedFunctionality("bedrock: file media type %q is not supported", mediaType)
	}
	name := sanitizeDocumentName(filename)
	if name == "" {
		*documentCounter++
		name = fmt.Sprintf("document-%d", *documentCounter)
	}
	document := &documentBlock{
		Format: format,
		Name:   name,
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
	case "video":
		signatures = []mediaTypeSignature{
			{mediaType: "video/mp4", prefix: []int{0x00, 0x00, 0x00, -1, 0x66, 0x74, 0x79, 0x70}},
			{mediaType: "video/webm", prefix: []int{0x1a, 0x45, 0xdf, 0xa3}},
			{mediaType: "video/quicktime", prefix: []int{0x00, 0x00, 0x00, 0x14, 0x66, 0x74, 0x79, 0x70, 0x71, 0x74}},
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
			if out, err = appendPartCachePoint(append(out, contentBlock{ToolResult: toolResult}), p.ProviderOptions); err != nil {
				return nil, err
			}
		case provider.ContentPartTypeToolApprovalResponse:
			// Same skip behavior as in user-role conversion.
		}
	}
	return out, nil
}

func convertAssistantContent(parts []provider.ContentPart, documentCounter *int, warnings *[]provider.Warning, isMistral bool, trimLastText bool) ([]converseMessage, error) {
	var out []converseMessage
	emit := func(role converseRole, block contentBlock) {
		out = append(out, converseMessage{Role: role, Content: []contentBlock{block}})
	}
	for partIndex, p := range parts {
		start := len(out)
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
			emit(converseRoleAssistant, contentBlock{Text: text})

		case provider.ContentPartTypeReasoning:
			meta, _, err := readReasoningMetadata(p.ProviderOptions)
			if err != nil {
				return nil, err
			}
			rc := &reasoningContentBlock{}
			switch {
			case meta.Signature != nil:
				rc.ReasoningText = &reasoningText{Text: p.Text, Signature: *meta.Signature}
			case meta.RedactedContent != nil:
				rc.RedactedContent = meta.RedactedContent
			case meta.RedactedData != nil:
				rc.RedactedReasoning = &redactedReasoning{Data: *meta.RedactedData}
			default:
				continue
			}
			emit(converseRoleAssistant, contentBlock{ReasoningContent: rc})

		case provider.ContentPartTypeToolCall:
			emit(converseRoleAssistant, contentBlock{
				ToolUse: &toolUseBlock{
					ToolUseID: normalizeToolCallID(p.ToolCallID, isMistral),
					Name:      sanitizeToolName(p.ToolName),
					Input:     toBedrockToolInput(p.Input),
				},
			})

		case provider.ContentPartTypeToolResult:
			toolResult, err := buildToolResult(p, documentCounter, isMistral, warnings)
			if err != nil {
				return nil, err
			}
			emit(converseRoleUser, contentBlock{ToolResult: toolResult})

		case provider.ContentPartTypeToolApprovalRequest:
			// Tool approval requests are local conversation bookkeeping.
			// Bedrock does not accept them; skip silently.

		case provider.ContentPartTypeCustom:
			// Custom parts are provider-owned history markers; upstream skips
			// them for Converse.

		default:
			*warnings = append(*warnings, provider.Warning{
				Type:    provider.WarnUnsupported,
				Feature: "assistantContentPart",
				Details: fmt.Sprintf("Bedrock assistant messages do not support %q", p.Type),
			})
		}
		if len(out) > start {
			last := len(out) - 1
			content, err := appendPartCachePoint(out[last].Content, p.ProviderOptions)
			if err != nil {
				return nil, err
			}
			out[last].Content = content
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
		out.Content = []toolResultContent{{Text: new("")}}
		return out, nil
	}
	switch p.Output.Type {
	case provider.ToolOutputText, provider.ToolOutputErrorText:
		out.Content = []toolResultContent{{Text: new(p.Output.Text)}}
	case provider.ToolOutputJSON, provider.ToolOutputErrorJSON:
		out.Content = []toolResultContent{{Text: new(string(p.Output.JSON))}}
	case provider.ToolOutputExecutionDenied:
		reason := "Tool call execution denied."
		if p.Output.Reason != nil {
			reason = *p.Output.Reason
		}
		out.Content = []toolResultContent{{Text: new(reason)}}
	case provider.ToolOutputContent:
		for _, c := range p.Output.Content {
			switch c.Type {
			case provider.ToolContentText:
				out.Content = append(out.Content, toolResultContent{Text: new(c.Text)})
			case provider.ToolContentFile:
				if c.Data == nil {
					*warnings = append(*warnings, provider.Warning{
						Type:    provider.WarnUnsupported,
						Feature: "toolResultContentPart",
						Details: "Bedrock tool result file has no data",
					})
					continue
				}
				if c.Data.IsURL() {
					url := c.Data.URL
					if !isS3URL(url) {
						*warnings = append(*warnings, provider.Warning{
							Type:    provider.WarnUnsupported,
							Feature: "toolResultFileURL",
							Details: fmt.Sprintf("Bedrock tool results only accept S3 image or video URLs; got %q with media type %q", url, c.MediaType),
						})
						continue
					}
					if format, ok := imageMediaTypeFormat[c.MediaType]; ok {
						out.Content = append(out.Content, toolResultContent{
							Image: &imageBlock{Format: format, Source: imageSource{S3Location: &s3LocationBlock{URI: url}}},
						})
					} else if format, ok := videoMediaTypeFormat[c.MediaType]; ok {
						out.Content = append(out.Content, toolResultContent{
							Video: &videoBlock{Format: format, Source: videoSource{S3Location: &s3LocationBlock{URI: url}}},
						})
					} else {
						*warnings = append(*warnings, provider.Warning{
							Type:    provider.WarnUnsupported,
							Feature: "toolResultFileURL",
							Details: fmt.Sprintf("Bedrock tool results only accept supported image or video media types; got %q", c.MediaType),
						})
					}
					continue
				}
				base64Data := c.Data.Base64
				if base64Data == "" && len(c.Data.Bytes) > 0 {
					base64Data = base64.StdEncoding.EncodeToString(c.Data.Bytes)
				}
				if !c.Data.IsData() {
					*warnings = append(*warnings, provider.Warning{
						Type:    provider.WarnUnsupported,
						Feature: "toolResultContentPart",
						Details: "Bedrock tool result files support only inline data",
					})
					continue
				}
				filePart := provider.ContentPart{
					MediaType:       c.MediaType,
					Filename:        c.Filename,
					Data:            c.Data,
					ProviderOptions: c.ProviderOptions,
				}
				mediaType, err := resolveFullMediaType(filePart)
				if err != nil {
					return nil, err
				}
				switch topLevelMediaType(mediaType) {
				case "image":
					format, ok := imageMediaTypeFormat[mediaType]
					if !ok {
						return nil, unsupportedFunctionality("bedrock: image media type %q is not supported", mediaType)
					}
					out.Content = append(out.Content, toolResultContent{
						Image: &imageBlock{Format: format, Source: imageSource{Bytes: base64Data}},
					})
					continue
				case "video":
					format, ok := videoMediaTypeFormat[mediaType]
					if !ok {
						return nil, unsupportedFunctionality("bedrock: video media type %q is not supported", mediaType)
					}
					out.Content = append(out.Content, toolResultContent{
						Video: &videoBlock{Format: format, Source: videoSource{Bytes: base64Data}},
					})
					continue
				}
				document, err := buildDocumentBlock(mediaType, filenameValue(c.Filename), base64Data, c.ProviderOptions, documentCounter)
				if err != nil {
					return nil, err
				}
				out.Content = append(out.Content, toolResultContent{Document: document})
			case provider.ToolContentCustom:
				// Deliberate exception to the warning convention: upstream throws
				// UnsupportedFunctionalityError, and silently dropping the entry
				// would change continuation semantics.
				return nil, unsupportedFunctionality("bedrock: unsupported tool content part type: %s", provider.ToolContentCustom)
			default:
				*warnings = append(*warnings, provider.Warning{
					Type:    provider.WarnUnsupported,
					Feature: "toolResultContentPart",
					Details: fmt.Sprintf("Bedrock tool results do not support %q", c.Type),
				})
			}
		}
		if len(out.Content) == 0 {
			out.Content = []toolResultContent{{Text: new("")}}
		}
	default:
		*warnings = append(*warnings, provider.Warning{
			Type:    provider.WarnUnsupported,
			Feature: "toolResultOutputType",
			Details: fmt.Sprintf("Bedrock tool results do not support %q", p.Output.Type),
		})
		out.Content = []toolResultContent{{Text: new("")}}
	}
	return out, nil
}

func filterToolContentFromPrompt(prompt []provider.Message, warnings []provider.Warning) ([]provider.Message, []provider.Warning) {
	removed := false
	out := make([]provider.Message, 0, len(prompt))
	for _, message := range prompt {
		if message.Role == provider.RoleSystem {
			out = append(out, message)
			continue
		}
		content := make([]provider.ContentPart, 0, len(message.Content))
		for _, part := range message.Content {
			if part.Type == provider.ContentPartTypeToolCall || part.Type == provider.ContentPartTypeToolResult {
				removed = true
				continue
			}
			content = append(content, part)
		}
		if len(content) > 0 {
			message.Content = content
			out = append(out, message)
		}
	}
	if !removed {
		return prompt, warnings
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
func sanitizeDocumentName(filename string) string {
	name := strings.Join(strings.Fields(stripFileExtension(filename)), " ")
	name = strings.TrimSpace(invalidBedrockDocumentNameCharacters.ReplaceAllString(name, ""))
	if len(name) > 200 {
		name = strings.TrimSpace(name[:200])
	}
	return name
}

func stripFileExtension(name string) string {
	if dot := strings.Index(name, "."); dot >= 0 {
		return name[:dot]
	}
	return name
}
