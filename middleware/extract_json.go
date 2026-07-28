package middleware

import (
	"context"
	"regexp"
	"strings"

	"github.com/grafana/ai-sdk/provider"
)

// JSONTransform transforms raw text into JSON text.
type JSONTransform func(text string) string

// ExtractJSONOptions configures ExtractJSON.
type ExtractJSONOptions struct {
	Transform JSONTransform
}

// ExtractJSON returns a Middleware that strips markdown JSON fences from text
// content in generate and stream responses.
func ExtractJSON(opts ExtractJSONOptions) Middleware {
	transform := opts.Transform
	hasCustomTransform := transform != nil
	if transform == nil {
		transform = stripJSONFences
	}
	return Middleware{
		WrapGenerate: func(ctx context.Context, params WrapGenerateParams) (*provider.GenerateResult, error) {
			result, err := params.DoGenerate(ctx)
			if err != nil {
				return nil, err
			}
			for i, part := range result.Content {
				if part.Type == provider.ContentText {
					result.Content[i].Text = transform(part.Text)
				}
			}
			return result, nil
		},
		WrapStream: func(ctx context.Context, params WrapStreamParams) (*provider.StreamResult, error) {
			result, err := params.DoStream(ctx)
			if err != nil {
				return nil, err
			}
			buffers := map[string]*jsonTextBuffer{}
			return TransformStream(ctx, result, func(part provider.StreamPart, emit func(provider.StreamPart)) {
				switch part.Type {
				case provider.PartTextStart:
					phase := jsonTextPhasePrefix
					if hasCustomTransform {
						phase = jsonTextPhaseBuffering
					}
					buffers[part.ID] = &jsonTextBuffer{start: part, phase: phase}
				case provider.PartTextDelta:
					if b := buffers[part.ID]; b != nil {
						b.text.WriteString(part.Delta)
						if b.phase == jsonTextPhaseBuffering {
							return
						}
						if b.phase == jsonTextPhasePrefix {
							buffered := b.text.String()
							switch {
							case len(buffered) > 0 && !strings.HasPrefix(buffered, "`"):
								b.phase = jsonTextPhaseStreaming
								emit(b.start)
							case strings.HasPrefix(buffered, "```"):
								if strings.Contains(buffered, "\n") {
									if prefix := jsonFencePrefixPattern.FindString(buffered); prefix != "" {
										b.text.Reset()
										b.text.WriteString(buffered[len(prefix):])
										b.prefixStripped = true
									}
									b.phase = jsonTextPhaseStreaming
									emit(b.start)
								}
							case len(buffered) >= 3 && !strings.HasPrefix(buffered, "```"):
								b.phase = jsonTextPhaseStreaming
								emit(b.start)
							}
						}
						if b.phase == jsonTextPhaseStreaming && b.text.Len() > jsonSuffixBufferSize {
							buffered := b.text.String()
							toStream := buffered[:len(buffered)-jsonSuffixBufferSize]
							remaining := buffered[len(buffered)-jsonSuffixBufferSize:]
							b.text.Reset()
							b.text.WriteString(remaining)
							emit(provider.StreamPart{Type: provider.PartTextDelta, ID: part.ID, Delta: toStream})
						}
					} else {
						emit(part)
					}
				case provider.PartTextEnd:
					if b := buffers[part.ID]; b != nil {
						if b.phase == jsonTextPhasePrefix || b.phase == jsonTextPhaseBuffering {
							emit(b.start)
						}
						text := b.text.String()
						switch {
						case b.phase == jsonTextPhaseBuffering:
							text = transform(text)
						case b.prefixStripped:
							text = stripJSONFenceSuffix(text)
						case b.phase == jsonTextPhasePrefix:
							text = transform(text)
						default:
							text = stripJSONFenceSuffix(text)
						}
						if text != "" {
							emit(provider.StreamPart{Type: provider.PartTextDelta, ID: part.ID, Delta: text})
						}
						emit(part)
						delete(buffers, part.ID)
					} else {
						emit(part)
					}
				default:
					emit(part)
				}
			}, nil), nil
		},
	}
}

type jsonTextBuffer struct {
	start          provider.StreamPart
	phase          jsonTextPhase
	text           strings.Builder
	prefixStripped bool
}

type jsonTextPhase string

const (
	jsonTextPhasePrefix    jsonTextPhase = "prefix"
	jsonTextPhaseStreaming jsonTextPhase = "streaming"
	jsonTextPhaseBuffering jsonTextPhase = "buffering"
	jsonSuffixBufferSize                 = 12
)

var (
	jsonFencePrefixPattern = regexp.MustCompile(`^` + "```" + `(?:json)?\s*\n?`)
	jsonFenceSuffixPattern = regexp.MustCompile(`(?s)\n?` + "```" + `\s*$`)
)

func stripJSONFences(text string) string {
	trimmed := strings.TrimSpace(text)
	trimmed = jsonFencePrefixPattern.ReplaceAllString(trimmed, "")
	trimmed = jsonFenceSuffixPattern.ReplaceAllString(trimmed, "")
	return strings.TrimSpace(trimmed)
}

func stripJSONFenceSuffix(text string) string {
	return strings.TrimRight(jsonFenceSuffixPattern.ReplaceAllString(text, ""), " \t\r\n")
}
