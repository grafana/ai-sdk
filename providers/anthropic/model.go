package anthropic

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/anthropics/anthropic-sdk-go/packages/param"
	vertexsdk "github.com/anthropics/anthropic-sdk-go/vertex"
	"github.com/grafana/ai-sdk/provider"
)

const specVersion = "v4"

const vertexCloudPlatformScope = "https://www.googleapis.com/auth/cloud-platform"

var vertexGoogleAuth = vertexsdk.WithGoogleAuth

type model struct {
	client       anthropic.Client
	modelID      string
	providerName string
	resolveModel func(string) string
	requestOpts  []option.RequestOption
	generateID   func() string
	capabilities providerCapabilities
}

// New creates a LanguageModel for the direct Anthropic API.
func New(apiKey, modelID string, opts ...Option) provider.LanguageModel {
	m := &model{
		client: anthropic.NewClient(
			option.WithoutEnvironmentDefaults(),
			option.WithAPIKey(apiKey),
		),
		modelID:      modelID,
		providerName: "anthropic",
		resolveModel: func(id string) string { return id },
		generateID:   defaultGenerateID,
		capabilities: directProviderCapabilities,
	}
	for _, o := range opts {
		o(m)
	}
	return m
}

// NewVertex creates a LanguageModel for Anthropic models on Google Vertex AI.
//
// The underlying anthropic-sdk-go Vertex helper panics when the region is
// empty, when Application Default Credentials cannot be loaded, or when the
// authenticated HTTP client cannot be created. Those panics are recovered and
// returned as ordinary errors so callers can handle authentication failures
// without crashing the process.
func NewVertex(ctx context.Context, location, projectID, modelID string, opts ...Option) (provider.LanguageModel, error) {
	authOpt, err := vertexAuth(ctx, location, projectID)
	if err != nil {
		return nil, err
	}
	m := &model{
		client:       anthropic.NewClient(authOpt),
		modelID:      modelID,
		providerName: "anthropic.vertex",
		resolveModel: ResolveVertexModelID,
		generateID:   defaultGenerateID,
		capabilities: vertexProviderCapabilities,
	}
	for _, o := range opts {
		o(m)
	}
	return m, nil
}

// vertexAuth invokes vertexsdk.WithGoogleAuth and converts any panic it raises
// into a returned error. The upstream SDK panics on misconfiguration (empty
// region, missing credentials, HTTP client setup failure); we honor the
// ai-sdk rule of never propagating panics across our API boundary.
func vertexAuth(ctx context.Context, location, projectID string) (opt option.RequestOption, err error) {
	defer func() {
		if r := recover(); r != nil {
			switch v := r.(type) {
			case error:
				err = fmt.Errorf("anthropic vertex auth: %w", v)
			default:
				err = fmt.Errorf("anthropic vertex auth: %v", v)
			}
			opt = nil
		}
	}()
	return vertexGoogleAuth(ctx, location, projectID, vertexCloudPlatformScope), nil
}

func (m *model) SpecificationVersion() string               { return specVersion }
func (m *model) Provider() string                           { return m.providerName }
func (m *model) ModelID() string                            { return m.modelID }
func (m *model) SupportedURLs() map[string][]*regexp.Regexp { return nil }

func (m *model) DoStream(ctx context.Context, params provider.CallOptions) (*provider.StreamResult, error) {
	p, mapping, warnings, br, err := buildParamsWithCapabilities(m.resolveModel(m.modelID), params, true, m.capabilities)
	if err != nil {
		return nil, err
	}

	citDocs := extractCitationDocuments(params.Prompt)
	var requestBody json.RawMessage
	var response *http.Response
	requestOpts := m.requestOptions(br, params.Headers)
	for _, beta := range p.Betas {
		requestOpts = append(requestOpts, option.WithHeaderAdd("anthropic-beta", string(beta)))
	}
	if !param.IsOmitted(p.UserProfileID) {
		requestOpts = append(requestOpts, option.WithHeader("anthropic-user-profile-id", p.UserProfileID.Value))
	}
	if !param.IsOmitted(p.WorkspaceID) {
		requestOpts = append(requestOpts, option.WithHeader("anthropic-workspace-id", p.WorkspaceID.Value))
	}
	requestOpts = append(requestOpts, option.WithMiddleware(captureRequestBody(&requestBody)), option.WithJSONSet("stream", true))
	streamCtx, cancel := context.WithCancel(ctx)
	if err := m.client.Post(streamCtx, "v1/messages?beta=true", p, &response, requestOpts...); err != nil {
		cancel()
		return nil, wrapInitialStreamError(err, p)
	}
	items := pumpMessageStream(streamCtx, response, params.IncludeRawChunks)
	buffered, err := preflightMessageStream(streamCtx, items, p)
	if err != nil {
		cancel()
		return nil, err
	}

	ch := make(chan provider.StreamPart, 64)
	go func() {
		defer close(ch)
		defer cancel()
		consumeStream(ctx, items, buffered, ch, mapping, warnings, br.usesJsonResponseTool, citDocs, m.generateID, m.providerName, br.markCodeExecutionDynamic, flattenHeaders(response.Header))
	}()

	return &provider.StreamResult{Stream: ch, Request: &provider.RequestMetadata{Body: requestBody}, Response: &provider.ResponseHeaders{Headers: flattenHeaders(response.Header)}}, nil
}

func (m *model) DoGenerate(ctx context.Context, params provider.CallOptions) (*provider.GenerateResult, error) {
	p, mapping, warnings, br, err := buildParamsWithCapabilities(m.resolveModel(m.modelID), params, false, m.capabilities)
	if err != nil {
		return nil, err
	}

	citDocs := extractCitationDocuments(params.Prompt)
	var requestBody json.RawMessage
	var response *http.Response
	requestOpts := m.requestOptions(br, params.Headers)
	requestOpts = append(requestOpts, option.WithMiddleware(captureRequestBody(&requestBody)), option.WithResponseInto(&response))

	msg, err := m.client.Beta.Messages.New(ctx, p, requestOpts...)
	if err != nil {
		return nil, wrapAPIError(err, "", p)
	}

	result, err := convertResponse(msg, mapping, br.usesJsonResponseTool, citDocs, m.generateID, m.providerName, br.markCodeExecutionDynamic)
	if err != nil {
		return nil, fmt.Errorf("converting response: %w", err)
	}
	result.Warnings = append(result.Warnings, warnings...)
	result.Request = &provider.RequestMetadata{Body: requestBody}
	result.Response.Headers = flattenHeaders(response.Header)
	result.Response.Body = json.RawMessage(msg.RawJSON())
	return result, nil
}

func (m *model) requestOptions(br buildResult, headers map[string]string) []option.RequestOption {
	opts := append([]option.RequestOption(nil), br.requestOptions...)
	opts = append(opts, m.requestOpts...)
	for key, value := range headers {
		if strings.EqualFold(key, "anthropic-beta") {
			opts = append(opts, option.WithHeaderAdd(key, value))
		} else {
			opts = append(opts, option.WithHeader(key, value))
		}
	}
	return append(opts, option.WithMiddleware(normalizeBetaHeaders))
}

func consumeStream(ctx context.Context, items <-chan messageStreamItem, buffered []messageStreamItem, ch chan<- provider.StreamPart, mapping toolNameMapping, warnings []provider.Warning, usesJsonResponseTool bool, citDocs []citationDocument, generateID func() string, providerName string, markCodeExecutionDynamic bool, responseHeaders map[string]string) {
	parts := make(chan provider.StreamPart, 64)
	go func() {
		defer close(parts)
		consumeStreamParts(ctx, items, buffered, parts, mapping, warnings, usesJsonResponseTool, citDocs, generateID, providerName, markCodeExecutionDynamic, responseHeaders)
	}()
	for {
		select {
		case <-ctx.Done():
			drainProviderStreamParts(parts)
			return
		case part, ok := <-parts:
			if !ok {
				return
			}
			select {
			case ch <- part:
			case <-ctx.Done():
				drainProviderStreamParts(parts)
				return
			}
		}
	}
}

func drainProviderStreamParts(parts <-chan provider.StreamPart) {
	go func() {
		for range parts {
		}
	}()
}

func consumeStreamParts(ctx context.Context, items <-chan messageStreamItem, buffered []messageStreamItem, ch chan<- provider.StreamPart, mapping toolNameMapping, warnings []provider.Warning, usesJsonResponseTool bool, citDocs []citationDocument, generateID func() string, providerName string, markCodeExecutionDynamic bool, responseHeaders map[string]string) {
	ch <- provider.StreamPart{Type: provider.PartStreamStart, Warnings: warnings}

	adapter := &streamAdapter{
		blocks:                   make(map[int64]*blockState),
		mapping:                  mapping,
		serverToolCalls:          make(map[string]string),
		mcpToolCalls:             make(map[string]mcpToolCallInfo),
		usesJsonResponseTool:     usesJsonResponseTool,
		markCodeExecutionDynamic: markCodeExecutionDynamic,
		citationDocuments:        citDocs,
		generateID:               generateID,
		providerName:             providerName,
		responseHeaders:          responseHeaders,
	}

	handle := func(item messageStreamItem) bool {
		if item.hasRaw {
			ch <- provider.StreamPart{Type: provider.PartRaw, RawValue: item.rawValue}
		}
		if item.err != nil {
			ch <- provider.StreamPart{Type: provider.PartError, APICallError: wrapAsAPICallError(item.err, "", nil)}
			return false
		}
		if item.event == nil {
			return true
		}
		if err := adapter.handleEvent(*item.event, ch); err != nil {
			ch <- provider.StreamPart{
				Type: provider.PartError,
				APICallError: provider.NewAPICallError(provider.APICallErrorOptions{
					Message: fmt.Sprintf("handling stream event: %v", err),
					Cause:   err,
				}),
			}
			return false
		}
		return true
	}

	for _, item := range buffered {
		if !handle(item) {
			return
		}
	}
	for {
		select {
		case <-ctx.Done():
			return
		case item, ok := <-items:
			if !ok || !handle(item) {
				return
			}
		}
	}
}
