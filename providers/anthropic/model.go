package anthropic

import (
	"context"
	"fmt"
	"regexp"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/anthropics/anthropic-sdk-go/packages/ssestream"
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
		client:       anthropic.NewClient(option.WithAPIKey(apiKey)),
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
	requestOpts := append([]option.RequestOption{}, br.requestOptions...)
	requestOpts = append(requestOpts, m.requestOpts...)
	stream := m.client.Beta.Messages.NewStreaming(ctx, p, requestOpts...)

	var firstEvent *anthropic.BetaRawMessageStreamEventUnion
	if stream.Next() {
		event := stream.Current()
		firstEvent = &event
	} else if err := stream.Err(); err != nil {
		_ = stream.Close()
		return nil, wrapInitialStreamError(err, p)
	}

	ch := make(chan provider.StreamPart, 64)
	go func() {
		defer close(ch)
		consumeStream(stream, firstEvent, ch, mapping, warnings, br.usesJsonResponseTool, citDocs, m.generateID, m.providerName, br.markCodeExecutionDynamic)
	}()

	return &provider.StreamResult{Stream: ch}, nil
}

func (m *model) DoGenerate(ctx context.Context, params provider.CallOptions) (*provider.GenerateResult, error) {
	p, mapping, warnings, br, err := buildParamsWithCapabilities(m.resolveModel(m.modelID), params, false, m.capabilities)
	if err != nil {
		return nil, err
	}

	citDocs := extractCitationDocuments(params.Prompt)
	requestOpts := append([]option.RequestOption{}, br.requestOptions...)
	requestOpts = append(requestOpts, m.requestOpts...)

	msg, err := m.client.Beta.Messages.New(ctx, p, requestOpts...)
	if err != nil {
		return nil, wrapAPIError(err, "", p)
	}

	result, err := convertResponse(msg, mapping, br.usesJsonResponseTool, citDocs, m.generateID, m.providerName, br.markCodeExecutionDynamic)
	if err != nil {
		return nil, fmt.Errorf("converting response: %w", err)
	}
	result.Warnings = append(result.Warnings, warnings...)
	return result, nil
}

func consumeStream(stream *ssestream.Stream[anthropic.BetaRawMessageStreamEventUnion], firstEvent *anthropic.BetaRawMessageStreamEventUnion, ch chan<- provider.StreamPart, mapping toolNameMapping, warnings []provider.Warning, usesJsonResponseTool bool, citDocs []citationDocument, generateID func() string, providerName string, markCodeExecutionDynamic bool) {
	defer func() { _ = stream.Close() }()

	adapter := &streamAdapter{
		blocks:                   make(map[int64]*blockState),
		warnings:                 warnings,
		mapping:                  mapping,
		serverToolCalls:          make(map[string]string),
		mcpToolCalls:             make(map[string]mcpToolCallInfo),
		usesJsonResponseTool:     usesJsonResponseTool,
		markCodeExecutionDynamic: markCodeExecutionDynamic,
		citationDocuments:        citDocs,
		generateID:               generateID,
		providerName:             providerName,
	}

	handleEvent := func(event anthropic.BetaRawMessageStreamEventUnion) bool {
		if err := adapter.handleEvent(event, ch); err != nil {
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

	if firstEvent != nil && !handleEvent(*firstEvent) {
		return
	}
	for stream.Next() {
		if !handleEvent(stream.Current()) {
			return
		}
	}

	if err := stream.Err(); err != nil {
		ch <- provider.StreamPart{
			Type:         provider.PartError,
			APICallError: wrapAsAPICallError(err, "", nil),
		}
	}
}
