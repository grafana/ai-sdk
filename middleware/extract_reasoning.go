package middleware

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/grafana/ai-sdk/provider"
)

// ExtractReasoningOptions configures the ExtractReasoning middleware.
type ExtractReasoningOptions struct {
	TagName            string
	Separator          string
	StartWithReasoning bool
}

// ExtractReasoning returns a Middleware that extracts XML-tagged reasoning
// sections from text output and converts them to reasoning content parts.
//
// For generate calls, it parses <tag>...</tag> from text content and produces
// separate reasoning and text content parts.
//
// For stream calls, it intercepts text deltas and emits reasoning-start/delta/end
// events for tagged content and text-delta events for non-tagged content,
// handling partial tags across chunk boundaries.
func ExtractReasoning(opts ExtractReasoningOptions) Middleware {
	if opts.Separator == "" {
		opts.Separator = "\n"
	}

	openingTag := fmt.Sprintf("<%s>", opts.TagName)
	closingTag := fmt.Sprintf("</%s>", opts.TagName)

	return Middleware{
		WrapGenerate: extractReasoningGenerate(openingTag, closingTag, opts),
		WrapStream:   extractReasoningStream(openingTag, closingTag, opts),
	}
}

func extractReasoningGenerate(openingTag, closingTag string, opts ExtractReasoningOptions) func(ctx context.Context, params WrapGenerateParams) (*provider.GenerateResult, error) {
	re := regexp.MustCompile(fmt.Sprintf(`(?s)%s(.*?)%s`, regexp.QuoteMeta(openingTag), regexp.QuoteMeta(closingTag)))

	return func(ctx context.Context, params WrapGenerateParams) (*provider.GenerateResult, error) {
		result, err := params.DoGenerate(ctx)
		if err != nil {
			return nil, err
		}

		var transformed []provider.GenerateContentPart
		for _, part := range result.Content {
			if part.Type != provider.ContentText {
				transformed = append(transformed, part)
				continue
			}

			text := part.Text
			if opts.StartWithReasoning {
				text = openingTag + text
			}

			matches := re.FindAllStringSubmatchIndex(text, -1)
			if len(matches) == 0 {
				transformed = append(transformed, part)
				continue
			}

			var reasoningParts []string
			for _, m := range matches {
				reasoningParts = append(reasoningParts, text[m[2]:m[3]])
			}
			reasoningText := strings.Join(reasoningParts, opts.Separator)

			textWithoutReasoning := text
			for i := len(matches) - 1; i >= 0; i-- {
				m := matches[i]
				before := textWithoutReasoning[:m[0]]
				after := textWithoutReasoning[m[1]:]
				sep := ""
				if len(before) > 0 && len(after) > 0 {
					sep = opts.Separator
				}
				textWithoutReasoning = before + sep + after
			}

			transformed = append(transformed, provider.GenerateContentPart{
				Type: provider.ContentReasoning,
				Text: reasoningText,
			})
			transformed = append(transformed, provider.GenerateContentPart{
				Type: provider.ContentText,
				Text: textWithoutReasoning,
			})
		}

		result.Content = transformed
		return result, nil
	}
}

func extractReasoningStream(openingTag, closingTag string, opts ExtractReasoningOptions) func(ctx context.Context, params WrapStreamParams) (*provider.StreamResult, error) {
	return func(ctx context.Context, params WrapStreamParams) (*provider.StreamResult, error) {
		result, err := params.DoStream(ctx)
		if err != nil {
			return nil, err
		}

		state := &extractionState{
			openingTag:  openingTag,
			closingTag:  closingTag,
			separator:   opts.Separator,
			isReasoning: opts.StartWithReasoning,
			extractions: make(map[string]*perIDExtraction),
		}

		return TransformStream(ctx, result, state.transform, nil), nil
	}
}

type perIDExtraction struct {
	isFirstReasoning bool
	isFirstText      bool
	afterSwitch      bool
	isReasoning      bool
	buffer           string
	idCounter        int
	textID           string
}

type extractionState struct {
	openingTag       string
	closingTag       string
	separator        string
	isReasoning      bool
	extractions      map[string]*perIDExtraction
	delayedTextStart *provider.StreamPart
}

func (s *extractionState) transform(part provider.StreamPart, emit func(provider.StreamPart)) {
	if part.Type == provider.PartTextStart {
		p := part
		s.delayedTextStart = &p
		return
	}

	if part.Type == provider.PartTextEnd && s.delayedTextStart != nil {
		emit(*s.delayedTextStart)
		s.delayedTextStart = nil
	}

	if part.Type != provider.PartTextDelta {
		emit(part)
		return
	}

	ext, ok := s.extractions[part.ID]
	if !ok {
		ext = &perIDExtraction{
			isFirstReasoning: true,
			isFirstText:      true,
			isReasoning:      s.isReasoning,
			textID:           part.ID,
		}
		s.extractions[part.ID] = ext
	}

	ext.buffer += part.Delta

	for {
		var nextTag string
		if ext.isReasoning {
			nextTag = s.closingTag
		} else {
			nextTag = s.openingTag
		}

		startIndex := getPotentialStartIndex(ext.buffer, nextTag)
		if startIndex < 0 {
			s.publish(ext, ext.buffer, emit)
			ext.buffer = ""
			break
		}

		s.publish(ext, ext.buffer[:startIndex], emit)

		foundFullMatch := startIndex+len(nextTag) <= len(ext.buffer)
		if foundFullMatch {
			ext.buffer = ext.buffer[startIndex+len(nextTag):]

			if ext.isReasoning {
				if ext.isFirstReasoning {
					emit(provider.StreamPart{
						Type: provider.PartReasoningStart,
						ID:   fmt.Sprintf("reasoning-%d", ext.idCounter),
					})
				}
				emit(provider.StreamPart{
					Type: provider.PartReasoningEnd,
					ID:   fmt.Sprintf("reasoning-%d", ext.idCounter),
				})
				ext.idCounter++
			}

			ext.isReasoning = !ext.isReasoning
			ext.afterSwitch = true
		} else {
			ext.buffer = ext.buffer[startIndex:]
			break
		}
	}
}

func (s *extractionState) publish(ext *perIDExtraction, text string, emit func(provider.StreamPart)) {
	if len(text) == 0 {
		return
	}

	prefix := ""
	if ext.afterSwitch {
		if ext.isReasoning && !ext.isFirstReasoning {
			prefix = s.separator
		} else if !ext.isReasoning && !ext.isFirstText {
			prefix = s.separator
		}
	}

	if ext.isReasoning {
		if ext.afterSwitch || ext.isFirstReasoning {
			emit(provider.StreamPart{
				Type: provider.PartReasoningStart,
				ID:   fmt.Sprintf("reasoning-%d", ext.idCounter),
			})
		}
		emit(provider.StreamPart{
			Type:  provider.PartReasoningDelta,
			Delta: prefix + text,
			ID:    fmt.Sprintf("reasoning-%d", ext.idCounter),
		})
		ext.isFirstReasoning = false
	} else {
		if s.delayedTextStart != nil {
			emit(*s.delayedTextStart)
			s.delayedTextStart = nil
		}
		emit(provider.StreamPart{
			Type:  provider.PartTextDelta,
			Delta: prefix + text,
			ID:    ext.textID,
		})
		ext.isFirstText = false
	}
	ext.afterSwitch = false
}

// getPotentialStartIndex finds where searchedText could begin in text.
// Returns the index of a complete match, or the index of a partial match
// at the end of text (where a suffix of text matches a prefix of searchedText).
// Returns -1 if no match or potential match is found.
func getPotentialStartIndex(text, searchedText string) int {
	if len(searchedText) == 0 {
		return -1
	}

	idx := strings.Index(text, searchedText)
	if idx >= 0 {
		return idx
	}

	for i := len(text) - 1; i >= 0; i-- {
		suffix := text[i:]
		if strings.HasPrefix(searchedText, suffix) {
			return i
		}
	}

	return -1
}
