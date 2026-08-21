package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/grafana/ai-sdk/gateway/catalog"
	"github.com/grafana/ai-sdk/gateway/failure"
	providerv4 "github.com/grafana/ai-sdk/gateway/providerwire/v4"
	"github.com/grafana/ai-sdk/provider"
)

const (
	providerWireV4Prefix             = "/api/v1/aisdk"
	providerWireV4UnaryID            = "providerwire-v4/unary"
	providerWireV4UnaryAlias         = "providerwire-v4/unary-alias"
	providerWireV4StreamStartID      = "providerwire-v4/stream-with-start"
	providerWireV4StreamNoStartID    = "providerwire-v4/stream-without-start"
	providerWireV4StreamErrorID      = "providerwire-v4/stream-error"
	providerWireV4AuthenticationID   = "providerwire-v4/authentication"
	providerWireV4AuthenticationText = "safe integration authentication failure"
	providerWireV4FailurePrefix      = "providerwire-v4/failure/"
)

type providerWireV4ModelMode string

const (
	providerWireV4UnaryMode         providerWireV4ModelMode = "unary"
	providerWireV4StreamStartMode   providerWireV4ModelMode = "stream-with-start"
	providerWireV4StreamNoStartMode providerWireV4ModelMode = "stream-without-start"
	providerWireV4StreamErrorMode   providerWireV4ModelMode = "stream-error"
)

type providerWireV4Model struct {
	mode providerWireV4ModelMode
}

func (m *providerWireV4Model) SpecificationVersion() string               { return "v4" }
func (m *providerWireV4Model) Provider() string                           { return "private-integration-provider" }
func (m *providerWireV4Model) ModelID() string                            { return "private-integration-model" }
func (m *providerWireV4Model) SupportedURLs() map[string][]*regexp.Regexp { return nil }

func (m *providerWireV4Model) DoGenerate(_ context.Context, options provider.CallOptions) (*provider.GenerateResult, error) {
	if m.mode != providerWireV4UnaryMode {
		return nil, errors.New("integration model does not support unary calls")
	}
	if err := validateProviderWireV4Options(options); err != nil {
		return nil, err
	}
	return &provider.GenerateResult{
		Content: []provider.GenerateContentPart{
			{Type: provider.ContentText, Text: "mapped-unary"},
			{Type: provider.ContentText, Text: ""},
		},
		FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop, Raw: "integration-stop"},
		Usage: provider.Usage{
			InputTokens: provider.InputTokenUsage{
				Total:      intPtr(7),
				NoCache:    intPtr(4),
				CacheRead:  intPtr(2),
				CacheWrite: intPtr(1),
			},
			OutputTokens: provider.OutputTokenUsage{
				Total:     intPtr(3),
				Text:      intPtr(3),
				Reasoning: intPtr(0),
			},
		},
		Response: &provider.GenerateResponse{ResponseMetadata: provider.ResponseMetadata{
			ModelID:  "private-integration-model",
			Provider: "private-integration-provider",
		}},
	}, nil
}

func (m *providerWireV4Model) DoStream(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
	parts := make(chan provider.StreamPart, 8)
	if m.mode == providerWireV4UnaryMode {
		close(parts)
		return nil, errors.New("integration model does not support streaming calls")
	}
	if m.mode == providerWireV4StreamStartMode {
		parts <- provider.StreamPart{Type: provider.PartStreamStart, Warnings: []provider.Warning{}}
	}
	if m.mode == providerWireV4StreamErrorMode {
		retryable := false
		parts <- provider.StreamPart{Type: provider.PartError, APICallError: provider.NewAPICallError(provider.APICallErrorOptions{
			Message:     "private integration provider error",
			StatusCode:  http.StatusUnauthorized,
			IsRetryable: &retryable,
		})}
		close(parts)
		return &provider.StreamResult{Stream: parts}, nil
	}
	parts <- provider.StreamPart{
		Type:       provider.PartResponseMeta,
		ResponseID: "integration-response",
		ModelID:    "private-integration-model",
		Provider:   "private-integration-provider",
	}
	parts <- provider.StreamPart{Type: provider.PartTextStart, ID: "integration-text"}
	parts <- provider.StreamPart{Type: provider.PartTextDelta, ID: "integration-text", Delta: ""}
	parts <- provider.StreamPart{Type: provider.PartTextDelta, ID: "integration-text", Delta: "streamed-text"}
	parts <- provider.StreamPart{Type: provider.PartTextEnd, ID: "integration-text"}
	parts <- provider.StreamPart{
		Type:         provider.PartFinish,
		FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop, Raw: "integration-stop"},
		Usage: &provider.Usage{
			InputTokens:  provider.InputTokenUsage{Total: intPtr(2)},
			OutputTokens: provider.OutputTokenUsage{Total: intPtr(1), Text: intPtr(1)},
		},
		Warnings: []provider.Warning{},
	}
	close(parts)
	return &provider.StreamResult{Stream: parts}, nil
}

func validateProviderWireV4Options(options provider.CallOptions) error {
	if len(options.Prompt) != 3 || options.Prompt[0].Role != provider.RoleSystem || len(options.Prompt[0].Content) != 1 || options.Prompt[0].Content[0].Text != "" {
		return errors.New("unexpected system prompt mapping")
	}
	if options.Prompt[1].Role != provider.RoleUser || len(options.Prompt[1].Content) != 2 || options.Prompt[1].Content[0].Text != "hello" || options.Prompt[1].Content[1].Text != "" {
		return errors.New("unexpected user prompt mapping")
	}
	if options.Prompt[2].Role != provider.RoleAssistant || len(options.Prompt[2].Content) != 1 || options.Prompt[2].Content[0].Text != "reply" {
		return errors.New("unexpected assistant prompt mapping")
	}
	if !languageModelNumberEquals(options.MaxOutputTokens, 0) || !languageModelNumberEquals(options.TopK, 7) || !languageModelNumberEquals(options.Seed, 42) {
		return errors.New("unexpected integer setting mapping")
	}
	if options.Temperature == nil || *options.Temperature != 0 || options.TopP == nil || *options.TopP != 0.5 || options.PresencePenalty == nil || *options.PresencePenalty != 0 || options.FrequencyPenalty == nil || *options.FrequencyPenalty != -0.5 {
		return errors.New("unexpected floating setting mapping")
	}
	if options.StopSequences == nil || len(options.StopSequences) != 0 || options.Reasoning == nil || *options.Reasoning != provider.ReasoningHigh {
		return errors.New("unexpected collection or reasoning mapping")
	}
	if options.Tools != nil || options.ToolChoice != nil || options.ResponseFormat != nil || options.IncludeRawChunks != nil || options.Headers != nil || options.ProviderOptions != nil {
		return errors.New("unexpected deferred option mapping")
	}
	return nil
}

func languageModelNumberEquals(value *provider.LanguageModelNumber, expected int64) bool {
	if value == nil {
		return false
	}
	actual, ok := value.Int64()
	return ok && actual == expected
}

func newProviderWireV4Handler() (*providerv4.Handler, error) {
	entries := []catalog.StaticEntry{
		{Info: catalog.ModelInfo{ID: providerWireV4UnaryID, Aliases: []string{providerWireV4UnaryAlias}}, Model: &providerWireV4Model{mode: providerWireV4UnaryMode}},
		{Info: catalog.ModelInfo{ID: providerWireV4StreamStartID}, Model: &providerWireV4Model{mode: providerWireV4StreamStartMode}},
		{Info: catalog.ModelInfo{ID: providerWireV4StreamNoStartID}, Model: &providerWireV4Model{mode: providerWireV4StreamNoStartMode}},
		{Info: catalog.ModelInfo{ID: providerWireV4StreamErrorID}, Model: &providerWireV4Model{mode: providerWireV4StreamErrorMode}},
	}
	modelCatalog, err := catalog.NewStatic(entries)
	if err != nil {
		return nil, fmt.Errorf("constructing ProviderWire V4 integration catalog: %w", err)
	}
	failures := make(map[string]failure.Failure, len(providerWireV4FailureCategories))
	for name, category := range providerWireV4FailureCategories {
		message := "safe integration " + name + " failure"
		if category == failure.CategoryAuthentication {
			message = providerWireV4AuthenticationText
		}
		value, err := failure.New(category, message)
		if err != nil {
			return nil, fmt.Errorf("constructing ProviderWire V4 integration failure %q: %w", name, err)
		}
		failures[name] = value
	}
	return providerv4.NewHandler(modelCatalog, providerv4.WithPolicy(providerv4.PolicyFunc(func(_ context.Context, request providerv4.PolicyRequest) *failure.Failure {
		name := ""
		if request.ModelID == providerWireV4AuthenticationID {
			name = "authentication"
		} else if strings.HasPrefix(request.ModelID, providerWireV4FailurePrefix) {
			name = strings.TrimPrefix(request.ModelID, providerWireV4FailurePrefix)
		}
		value, ok := failures[name]
		if !ok {
			return nil
		}
		return &value
	})))
}

var providerWireV4FailureCategories = map[string]failure.Category{
	"invalid-request":   failure.CategoryInvalidRequest,
	"authentication":    failure.CategoryAuthentication,
	"permission":        failure.CategoryPermission,
	"not-found":         failure.CategoryNotFound,
	"rate-limit":        failure.CategoryRateLimit,
	"overload":          failure.CategoryOverload,
	"failed-dependency": failure.CategoryFailedDependency,
	"upstream-failure":  failure.CategoryUpstreamFailure,
	"timeout":           failure.CategoryTimeout,
	"cancellation":      failure.CategoryCancellation,
	"internal-failure":  failure.CategoryInternalFailure,
}

func mountProviderWireV4(mux *http.ServeMux) error {
	handler, err := newProviderWireV4Handler()
	if err != nil {
		return err
	}
	mux.Handle("POST "+providerWireV4Prefix+providerv4.PathLanguageModel, handler)
	return nil
}
