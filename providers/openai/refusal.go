package openai

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/grafana/ai-sdk/provider"
	"github.com/openai/openai-go/v3/responses"
)

// ResponsesMetadata preserves native output fields missing from LanguageModelV4.
// Refusal is a delta on openai.responses-refusal custom parts and the complete
// refusal on generate results and finish metadata. Nil means no refusal.
type ResponsesMetadata struct {
	Refusal *string `json:"refusal,omitempty"`
}

// Validate checks the typed native metadata without changing it.
func (m ResponsesMetadata) Validate() error {
	if m.Refusal != nil && !utf8.ValidString(*m.Refusal) {
		return fmt.Errorf("openai: invalid refusal text")
	}
	return nil
}

// ParseResponsesMetadata reads namespace.responses (normally openai or azure).
func ParseResponsesMetadata(metadata provider.ProviderMetadata, namespace string) (ResponsesMetadata, error) {
	var envelope struct {
		Responses ResponsesMetadata `json:"responses"`
	}
	if raw := metadata[namespace]; len(raw) > 0 {
		if err := json.Unmarshal(raw, &envelope); err != nil {
			return ResponsesMetadata{}, fmt.Errorf("openai: invalid responses metadata: %w", err)
		}
	}
	return envelope.Responses, envelope.Responses.Validate()
}

func withResponsesMetadata(metadata provider.ProviderMetadata, namespace string, m ResponsesMetadata) provider.ProviderMetadata {
	if m.Refusal == nil {
		return metadata
	}
	fields := map[string]json.RawMessage{}
	if raw := metadata[namespace]; len(raw) > 0 {
		_ = json.Unmarshal(raw, &fields)
	}
	fields["responses"], _ = json.Marshal(m)
	encoded, _ := json.Marshal(fields)
	copied := make(provider.ProviderMetadata, len(metadata)+1)
	for key, value := range metadata {
		copied[key] = value
	}
	copied[namespace] = encoded
	return copied
}

func responseRefusal(resp *responses.Response) ResponsesMetadata {
	var text strings.Builder
	found := false
	for _, item := range resp.Output {
		if message, ok := item.AsAny().(responses.ResponseOutputMessage); ok {
			for _, content := range message.Content {
				if refusal, ok := content.AsAny().(responses.ResponseOutputRefusal); ok {
					found = true
					text.WriteString(refusal.Refusal)
				}
			}
		}
	}
	if !found {
		return ResponsesMetadata{}
	}
	refusal := text.String()
	return ResponsesMetadata{Refusal: &refusal}
}

type refusalKey struct{ output, content int64 }

func (a *streamAdapter) appendRefusal(key refusalKey, delta string, ch chan<- provider.StreamPart) {
	if a.refusals == nil {
		a.refusals = map[refusalKey]*strings.Builder{}
	}
	if _, ok := a.refusals[key]; !ok {
		a.refusalOrder = append(a.refusalOrder, key)
		a.refusals[key] = &strings.Builder{}
	}
	a.refusals[key].WriteString(delta)
	ch <- provider.StreamPart{Type: provider.PartCustom, Kind: "openai.responses-refusal", ProviderMetadata: withResponsesMetadata(nil, a.providerOptionsName, ResponsesMetadata{Refusal: &delta})}
}

func (a *streamAdapter) finishRefusal(key refusalKey, refusal string, ch chan<- provider.StreamPart) {
	builder, exists := a.refusals[key]
	previous := ""
	if exists {
		previous = builder.String()
	}
	if exists && previous == refusal {
		return
	}
	if !strings.HasPrefix(refusal, previous) {
		a.recordStreamError("invalid_refusal")
		ch <- provider.StreamPart{Type: provider.PartError, APICallError: provider.NewAPICallError(provider.APICallErrorOptions{Message: "openai: inconsistent refusal stream", StatusCode: 502})}
		return
	}
	a.appendRefusal(key, refusal[len(previous):], ch)
}

func (a *streamAdapter) observeMessageRefusals(index int64, message responses.ResponseOutputMessage, ch chan<- provider.StreamPart) {
	for contentIndex, content := range message.Content {
		if refusal, ok := content.AsAny().(responses.ResponseOutputRefusal); ok {
			a.finishRefusal(refusalKey{index, int64(contentIndex)}, refusal.Refusal, ch)
		}
	}
}

func (a *streamAdapter) refusalMetadata() ResponsesMetadata {
	if len(a.refusalOrder) == 0 {
		return ResponsesMetadata{}
	}
	slices.SortFunc(a.refusalOrder, func(a, b refusalKey) int {
		if a.output < b.output || a.output == b.output && a.content < b.content {
			return -1
		}
		if a == b {
			return 0
		}
		return 1
	})
	var text strings.Builder
	for _, key := range a.refusalOrder {
		text.WriteString(a.refusals[key].String())
	}
	refusal := text.String()
	return ResponsesMetadata{Refusal: &refusal}
}
