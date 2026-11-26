package openai

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"regexp"

	"github.com/grafana/ai-sdk/provider"
	openaisdk "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/responses"
)

const (
	specVersion  = "v4"
	providerName = "openai"
)

type model struct {
	client                           responses.ResponseService
	modelID                          string
	provider                         string
	providerOptionsName              string
	requestOpts                      []option.RequestOption
	generateID                       func() string
	webSearchSourcesIncludeSupported bool
	defaultReasoningSummary          string
}

// NewResponses creates a [provider.LanguageModel] for the OpenAI Responses API.
func NewResponses(apiKey, modelID string, opts ...Option) provider.LanguageModel {
	m := newModel(modelID, opts...)
	clientOpts := append([]option.RequestOption{option.WithAPIKey(apiKey)}, m.requestOpts...)
	client := openaisdk.NewClient(clientOpts...)
	m.client = client.Responses
	return m
}

// NewResponsesWithClient creates a [provider.LanguageModel] using a
// preconfigured OpenAI client. It is intended for provider integrations that
// reuse the Responses model implementation with provider-owned transport and
// authentication.
func NewResponsesWithClient(client openaisdk.Client, modelID string, opts ...Option) provider.LanguageModel {
	m := newModel(modelID, opts...)
	m.client = client.Responses
	return m
}

func newModel(modelID string, opts ...Option) *model {
	m := &model{
		modelID:                          modelID,
		provider:                         providerName,
		generateID:                       defaultGenerateID,
		webSearchSourcesIncludeSupported: true,
		defaultReasoningSummary:          "detailed",
	}
	for _, o := range opts {
		o(m)
	}
	return m
}

func (m *model) SpecificationVersion() string               { return specVersion }
func (m *model) Provider() string                           { return m.provider }
func (m *model) ModelID() string                            { return m.modelID }
func (m *model) SupportedURLs() map[string][]*regexp.Regexp { return supportedURLs }

var supportedURLs = map[string][]*regexp.Regexp{
	"image/*":         {regexp.MustCompile(`^https?://.*$`)},
	"application/pdf": {regexp.MustCompile(`^https?://.*$`)},
}

// defaultGenerateID produces a random hex identifier for synthesized IDs.
func defaultGenerateID() string {
	var b [12]byte
	_, _ = rand.Read(b[:])
	return "aitxt-" + hex.EncodeToString(b[:])
}

var _ provider.LanguageModel = (*model)(nil)

// DoGenerate performs a non-streaming Responses call.
func (m *model) DoGenerate(ctx context.Context, params provider.CallOptions) (*provider.GenerateResult, error) {
	body, warnings, br, err := m.buildParams(params)
	if err != nil {
		return nil, err
	}
	var rawResponse *http.Response
	var requestBody json.RawMessage
	diagnostics := newHostedToolDiagnostics(body)
	requestOpts := append(m.requestOptions(params.Headers), option.WithResponseInto(&rawResponse), option.WithMiddleware(captureRequestBody(&requestBody, diagnostics)))
	resp, err := m.client.New(ctx, body, requestOpts...)
	protectCredentials := diagnostics.protected()
	if protectCredentials {
		requestBody = nil
	}
	if err != nil || resp.JSON.Error.Valid() || !resp.JSON.Output.Valid() {
		var resultErr error
		switch {
		case err != nil:
			resultErr = wrapAPIError(err, body)
		case resp.JSON.Error.Valid():
			resultErr = responseBodyError(resp, rawResponse, body, http.StatusBadRequest, resp.Error.Message)
		default:
			resultErr = missingOutputError(resp, rawResponse, body)
		}
		if protectCredentials {
			resultErr = diagnostics.safeError(resultErr)
		}
		return nil, resultErr
	}
	result, err := convertResponse(resp, br, m.generateID, m.provider)
	if err != nil {
		if protectCredentials {
			err = diagnostics.safeError(err)
		}
		return nil, err
	}
	headers := rawResponse.Header
	if protectCredentials {
		result.Response.Body = nil
		result.Usage.Raw = nil
		result.ProviderMetadata = nil
		result.FinishReason.Raw = ""
		headers = diagnostics.safeHeaders(headers)
	}
	result.Warnings = append(result.Warnings, warnings...)
	result.Request = &provider.RequestMetadata{Body: requestBody}
	result.Response.Headers = flattenHeaders(headers)
	return result, nil
}

func missingOutputError(resp *responses.Response, rawResponse *http.Response, body responses.ResponseNewParams) *provider.APICallError {
	message := "Responses API returned no output"
	if reason := string(resp.IncompleteDetails.Reason); reason != "" {
		message += " (" + reason + ")"
	}
	return responseBodyError(resp, rawResponse, body, http.StatusInternalServerError, message)
}

func responseBodyError(resp *responses.Response, rawResponse *http.Response, body responses.ResponseNewParams, statusCode int, message string) *provider.APICallError {
	var requestBody json.RawMessage
	if encoded, err := json.Marshal(body); err == nil {
		requestBody = encoded
	}
	var url string
	var headers map[string][]string
	if rawResponse != nil {
		headers = rawResponse.Header
		if rawResponse.Request != nil {
			url = rawResponse.Request.URL.String()
		}
	}
	retryable := false
	return provider.NewAPICallError(provider.APICallErrorOptions{
		Message:           message,
		URL:               url,
		RequestBodyValues: requestBody,
		StatusCode:        statusCode,
		ResponseHeaders:   headers,
		ResponseBody:      resp.RawJSON(),
		IsRetryable:       &retryable,
	})
}

// DoStream performs a streaming Responses call.
func (m *model) DoStream(ctx context.Context, params provider.CallOptions) (*provider.StreamResult, error) {
	body, warnings, br, err := m.buildParams(params)
	if err != nil {
		return nil, err
	}
	var rawResponse *http.Response
	var requestBody json.RawMessage
	diagnostics := newHostedToolDiagnostics(body)
	requestOptions := append(m.requestOptions(params.Headers), option.WithResponseBodyInto(&rawResponse), option.WithJSONSet("stream", true), option.WithMiddleware(captureRequestBody(&requestBody, diagnostics)))
	streamCtx, cancel := context.WithCancel(ctx)
	_, err = m.client.New(streamCtx, body, requestOptions...)
	protectCredentials := diagnostics.protected()
	var headers http.Header
	if rawResponse != nil {
		headers = rawResponse.Header
	}
	if protectCredentials {
		requestBody = nil
		headers = diagnostics.safeHeaders(headers)
	}
	items := pumpResponseStream(streamCtx, rawResponse, err, params.IncludeRawChunks && !protectCredentials)
	buffered, err := preflightResponseStream(streamCtx, items, body, rawResponse)
	if err != nil {
		cancel()
		if protectCredentials {
			err = diagnostics.safeError(err)
		}
		return nil, err
	}

	ch := make(chan provider.StreamPart, 64)
	go func() {
		defer close(ch)
		defer cancel()
		consumeStream(streamCtx, items, buffered, ch, warnings, br, body, rawResponse, m.generateID, m.provider, diagnostics)
	}()
	return &provider.StreamResult{Stream: ch, Request: &provider.RequestMetadata{Body: requestBody}, Response: &provider.ResponseHeaders{Headers: flattenHeaders(headers)}}, nil
}

func (m *model) buildParams(params provider.CallOptions) (responses.ResponseNewParams, []provider.Warning, buildResult, error) {
	return buildParamsWithConfig(m.modelID, params, m.providerOptionsName, m.webSearchSourcesIncludeSupported, m.defaultReasoningSummary)
}

func (m *model) requestOptions(headers map[string]string) []option.RequestOption {
	opts := append([]option.RequestOption(nil), m.requestOpts...)
	for key, value := range headers {
		opts = append(opts, option.WithHeader(key, value))
	}
	return opts
}
