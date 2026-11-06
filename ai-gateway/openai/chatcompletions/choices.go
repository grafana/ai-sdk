package chatcompletions

import (
	"context"
	"errors"
	"net/http"

	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
	"github.com/grafana/ai-sdk/provider"
	compatible "github.com/grafana/ai-sdk/providers/openai-compatible"
)

func mapGenerateChoices(result *provider.GenerateResult, r mappedRequest, id, model string, created int64) (completion, error) {
	var out completion
	if result == nil || len(result.Choices) != r.options.ChoiceCount {
		return out, errOutput
	}
	count := r.options.ChoiceCount
	r.options.ChoiceCount = 1
	seen := make([]bool, count)
	for _, alternative := range result.Choices {
		if alternative.Index < 0 || alternative.Index >= count || seen[alternative.Index] {
			return out, errOutput
		}
		seen[alternative.Index] = true
		single, err := mapGenerate(&provider.GenerateResult{Content: alternative.Content, FinishReason: alternative.FinishReason, ProviderMetadata: alternative.ProviderMetadata, Response: result.Response}, r, id, model, created)
		if err != nil {
			return out, err
		}
		if len(out.Choices) == 0 {
			out = single
			out.Choices = nil
			out.Grafana = nil
		}
		mapped := single.Choices[0]
		mapped.Index = alternative.Index
		mapped.Grafana = single.Grafana
		if len(alternative.ProviderMetadata) > 0 {
			if mapped.Grafana == nil {
				mapped.Grafana = &diagnostics{}
			}
			mapped.Grafana.ProviderMetadata = alternative.ProviderMetadata
		}
		out.Choices = append(out.Choices, mapped)
	}
	var err error
	out.Usage, err = mapUsage(result.Usage)
	if err != nil {
		return out, err
	}
	metadata, err := chatMetadata(result.ProviderMetadata)
	if err != nil || nativeUsage(out.Usage, metadata.Usage) != nil {
		return out, errOutput
	}
	out.Moderation = metadata.Moderation
	out.Metadata = metadata.Metadata
	if metadata.ServiceTier != nil {
		out.ServiceTier = metadata.ServiceTier
	}
	if metadata.SystemFingerprint != nil {
		out.SystemFingerprint = metadata.SystemFingerprint
	}
	warnings, err := mapWarnings(result.Warnings)
	if err != nil {
		return out, err
	}
	if len(warnings) > 0 {
		out.Grafana = &diagnostics{Warnings: warnings}
	}
	return out, nil
}

func (h *handler) serveChoicesStream(ctx context.Context, w http.ResponseWriter, call *execution.Call, r mappedRequest, id, model string, created int64) {
	stream, err := call.Stream()
	if err != nil {
		if !errors.Is(err, context.Canceled) {
			writeError(w, categoryStatus(execution.ClassifyProvider(err)), executionDiagnostics(nil, call.View(err), nil))
		}
		return
	}
	defer stream.Close()
	write, closeWriter, err := newStreamWriter(ctx, w)
	if err != nil {
		WriteError(w, 500)
		return
	}
	defer closeWriter()
	count := r.options.ChoiceCount
	states := make([]streamState, count)
	opened := make([]bool, count)
	final := make([]*string, count)
	metadata := make([]provider.ProviderMetadata, count)
	for i := range states {
		states[i] = streamState{textIDs: map[string]bool{}, tools: map[string]*toolState{}}
	}
	var pending *diagnostics
	contentDiagnostics := make([]*diagnostics, count)
	var view execution.View
	viewed, anyOpened, complete := false, false, false
	var total *usage
	var serviceTier, fingerprint *string
	var moderation *compatible.ChatModeration
	var finalMetadata provider.ProviderMetadata
	emit := func(index int, d delta, finish *string, probabilities any, usage *usage, empty bool) bool {
		if r.legacyFunctions {
			var err error
			d, err = legacyDelta(d)
			if err != nil {
				return false
			}
			if finish != nil {
				finish = ptr(legacyFinish(*finish))
			}
		}
		detail := pending
		if !empty && contentDiagnostics[index] != nil {
			if detail == nil {
				detail = &diagnostics{}
			} else {
				copy := *detail
				detail = &copy
			}
			detail.Sources = contentDiagnostics[index].Sources
			detail.Citations = contentDiagnostics[index].Citations
		}
		c := chunk{Moderation: moderation, ServiceTier: serviceTier, SystemFingerprint: fingerprint, ID: id, Object: "chat.completion.chunk", Created: created, Model: model, Choices: []chunkChoice{{Index: index, Delta: d, Finish: finish, Logprobs: probabilities}}, Usage: usage, Grafana: detail}
		if empty {
			c.Choices = []chunkChoice{}
		}
		data, err := r.marshalChunk(c)
		if err != nil || !write(data) {
			return false
		}
		pending = nil
		if !empty {
			contentDiagnostics[index] = nil
		}
		return true
	}
	open := func(index int) bool {
		if opened[index] {
			return true
		}
		opened[index] = true
		anyOpened = true
		return emit(index, delta{Role: roleAssistant}, nil, nil, nil, false)
	}
	fail := func(status int) { write(errorBody(status, executionDiagnostics(nil, view, nil))) }
	for {
		if ctx.Err() != nil {
			return
		}
		part, wait := stream.Next()
		if !viewed {
			view = call.View(wait.Err())
			viewed = true
		}
		if wait == execution.WaitFinished && complete {
			pending = executionDiagnostics(pending, view, finalMetadata)
			for i := range states {
				if final[i] == nil {
					fail(502)
					return
				}
				if len(metadata[i]) > 0 {
					pending = executionDiagnostics(pending, view, metadata[i])
				}
				if !open(i) || !emit(i, delta{}, final[i], nil, nil, false) {
					return
				}
			}
			if r.StreamOptions != nil && boolValue(r.StreamOptions.IncludeUsage) {
				if !emit(0, delta{}, nil, nil, total, true) {
					return
				}
			}
			write([]byte("[DONE]"))
			return
		}
		if wait != execution.WaitPart {
			if wait != execution.WaitCanceled {
				fail(categoryStatus(execution.ClassifyProvider(wait.Err())))
			}
			return
		}
		if part.ChoiceIndex == nil {
			switch part.Type {
			case provider.PartStreamStart:
				warnings, err := mapWarnings(part.Warnings)
				if err != nil {
					fail(502)
					return
				}
				if len(warnings) > 0 {
					pending = &diagnostics{Warnings: warnings}
				}
			case provider.PartResponseMeta:
				native, err := mapNativeResponse(part.ResponseID, part.ModelID, part.Timestamp)
				if err != nil {
					fail(502)
					return
				}
				if !anyOpened {
					native.apply(&id, &model, &created)
				} else {
					fail(502)
					return
				}
			case provider.PartRaw:
			case provider.PartError:
				detail := executionDiagnostics(nil, view, nil)
				if current := view.Current(part.APICallError); current != nil {
					if detail == nil {
						detail = &diagnostics{}
					}
					detail.NativeError = current
				}
				write(errorBody(categoryStatus(execution.ClassifyStreamPart(part.APICallError)), detail))
				return
			case provider.PartChoicesFinish:
				if part.Usage == nil {
					fail(502)
					return
				}
				total, err = mapUsage(*part.Usage)
				if err != nil {
					fail(502)
					return
				}
				native, err := chatMetadata(part.ProviderMetadata)
				if err != nil || nativeUsage(total, native.Usage) != nil {
					fail(502)
					return
				}
				if native.Moderation != nil {
					moderation = native.Moderation
				}
				if native.ServiceTier != nil {
					serviceTier = native.ServiceTier
				}
				if native.SystemFingerprint != nil {
					fingerprint = native.SystemFingerprint
				}
				finalMetadata = part.ProviderMetadata
				complete = true
			default:
				fail(502)
				return
			}
			continue
		}
		index := *part.ChoiceIndex
		if index < 0 || index >= count {
			fail(502)
			return
		}
		state := &states[index]
		if part.Type == provider.PartFinish {
			native, err := chatMetadata(part.ProviderMetadata)
			if err != nil {
				fail(502)
				return
			}
			if native.Refusal != nil && !state.refused {
				state.refused = true
				if !open(index) || !emit(index, delta{Refusal: native.Refusal}, nil, nil, nil, false) {
					return
				}
			}
			f, _, err := state.finish(part, r)
			if err != nil {
				fail(502)
				return
			}
			final[index] = &f
			metadata[index] = part.ProviderMetadata
			continue
		}
		if part.Type == provider.PartCustom && part.Kind == "openai-compatible.chat-completion" {
			d, probabilities, err := state.nativeDelta(r, part.ProviderMetadata)
			if err != nil {
				fail(502)
				return
			}
			native, _ := chatMetadata(part.ProviderMetadata)
			if native.Moderation != nil {
				moderation = native.Moderation
			}
			if native.ServiceTier != nil {
				serviceTier = native.ServiceTier
			}
			if native.SystemFingerprint != nil {
				fingerprint = native.SystemFingerprint
			}
			if !open(index) || !emit(index, d, nil, probabilities, nil, false) {
				return
			}
			continue
		}
		if !validCitations(part.Citations) || part.Type == provider.PartSource && (part.Source == nil || !validSource(*part.Source)) {
			fail(502)
			return
		}
		contentDiagnostics[index] = sourceDiagnostics(contentDiagnostics[index], part.Source, part.Citations)
		d, err := state.consume(part, r)
		if err != nil {
			fail(502)
			return
		}
		if d != nil {
			if !open(index) || !emit(index, *d, nil, nil, nil, false) {
				return
			}
		}
	}
}
