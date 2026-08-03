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
	client      responses.ResponseService
	modelID     string
	provider    string
	requestOpts []option.RequestOption
	generateID  func() string
}

// NewResponses creates a [provider.LanguageModel] for the OpenAI Responses API.
func NewResponses(apiKey, modelID string, opts ...Option) provider.LanguageModel {
	m := &model{
		modelID:    modelID,
		provider:   providerName,
		generateID: defaultGenerateID,
	}
	for _, o := range opts {
		o(m)
	}
	clientOpts := append([]option.RequestOption{option.WithAPIKey(apiKey)}, m.requestOpts...)
	client := openaisdk.NewClient(clientOpts...)
	m.client = client.Responses
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
	body, warnings, br, err := buildParams(m.modelID, params)
	if err != nil {
		return nil, err
	}
	var rawResponse *http.Response
	requestOpts := append(m.requestOptions(params.Headers), option.WithResponseInto(&rawResponse))
	resp, err := m.client.New(ctx, body, requestOpts...)
	if err != nil {
		return nil, wrapAPIError(err, body)
	}
	if resp.JSON.Error.Valid() {
		return nil, responseBodyError(resp, rawResponse, body, http.StatusBadRequest, resp.Error.Message)
	}
	if !resp.JSON.Output.Valid() {
		return nil, missingOutputError(resp, rawResponse, body)
	}
	result, err := convertResponse(resp, br, m.generateID, m.provider)
	if err != nil {
		return nil, err
	}
	result.Warnings = append(result.Warnings, warnings...)
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
	body, warnings, br, err := buildParams(m.modelID, params)
	if err != nil {
		return nil, err
	}
	var rawResponse *http.Response
	requestOptions := append(m.requestOptions(params.Headers), option.WithResponseInto(&rawResponse))
	stream := m.client.NewStreaming(ctx, body, requestOptions...)
	items := pumpResponseStream(ctx, stream)
	buffered, err := preflightResponseStream(ctx, items, body, rawResponse)
	if err != nil {
		return nil, err
	}

	ch := make(chan provider.StreamPart, 64)
	go func() {
		defer close(ch)
		consumeStream(ctx, items, buffered, ch, warnings, br, body, rawResponse, m.generateID, m.provider)
	}()
	return &provider.StreamResult{Stream: ch}, nil
}

func (m *model) requestOptions(headers map[string]string) []option.RequestOption {
	opts := append([]option.RequestOption(nil), m.requestOpts...)
	for key, value := range headers {
		opts = append(opts, option.WithHeader(key, value))
	}
	return opts
}
