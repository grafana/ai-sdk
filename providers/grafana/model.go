package grafana

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strings"

	"github.com/grafana/ai-sdk/gateway/providerwire"
	"github.com/grafana/ai-sdk/provider"
	"github.com/grafana/authlib/authn"
)

const (
	specVersion      = "v4"
	streamBufferSize = 64
)

type model struct {
	provider *Provider
	modelID  string
}

var _ provider.LanguageModel = (*model)(nil)

func (m *model) SpecificationVersion() string               { return specVersion }
func (m *model) Provider() string                           { return providerName }
func (m *model) ModelID() string                            { return m.modelID }
func (m *model) SupportedURLs() map[string][]*regexp.Regexp { return nil }

func (m *model) codec() wireCodec {
	if m.provider != nil && m.provider.wireCodec != nil {
		return m.provider.wireCodec
	}
	return legacyWireCodec{}
}

func (m *model) strictMode() bool {
	_, strict := m.codec().(strictWireCodec)
	return strict
}

func (m *model) maxUnaryResponseBytes() int64 {
	if m.provider != nil && m.provider.maxUnaryResponseBytes > 0 {
		return m.provider.maxUnaryResponseBytes
	}
	return DefaultMaxUnaryResponseBytes
}

func (m *model) maxErrorResponseBytes() int64 {
	if m.provider != nil && m.provider.maxErrorResponseBytes > 0 {
		return m.provider.maxErrorResponseBytes
	}
	return DefaultMaxErrorResponseBytes
}

func (m *model) maxSSEEventBytes() int64 {
	if m.provider != nil && m.provider.maxSSEEventBytes > 0 {
		return m.provider.maxSSEEventBytes
	}
	return DefaultMaxSSEEventBytes
}

func (m *model) DoStream(ctx context.Context, opts provider.CallOptions) (*provider.StreamResult, error) {
	resp, body, err := m.doRequest(ctx, opts, true)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		defer func() { _ = resp.Body.Close() }()
		return nil, m.surfaceHTTPError(resp)
	}
	if !isEventStreamContentType(resp.Header.Get("Content-Type")) {
		defer func() { _ = resp.Body.Close() }()
		return nil, m.newInvalidStreamContentTypeError(resp)
	}

	ch := make(chan provider.StreamPart, streamBufferSize)
	go m.readStream(ctx, resp, ch, opts.IncludeRawChunks)

	return &provider.StreamResult{
		Stream:   ch,
		Request:  &provider.RequestMetadata{Body: body},
		Response: &provider.ResponseHeaders{Headers: singleValueHeaders(resp.Header)},
	}, nil
}

func (m *model) DoGenerate(ctx context.Context, opts provider.CallOptions) (*provider.GenerateResult, error) {
	resp, requestBody, err := m.doRequest(ctx, opts, false)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, m.surfaceHTTPError(resp)
	}
	if !isJSONContentType(resp.Header.Get("Content-Type")) {
		return nil, m.newInvalidGenerateContentTypeError(resp)
	}

	body, err := readResponseWithinLimit(resp.Body, m.maxUnaryResponseBytes())
	if err != nil {
		return nil, newGenerateAPICallError(resp, body, err)
	}
	result, err := m.codec().decodeGenerateResult(body)
	if err != nil {
		return nil, newGenerateAPICallError(resp, body, errors.Join(errProtocolResponse, err))
	}
	if result.Request == nil {
		result.Request = &provider.RequestMetadata{Body: json.RawMessage(requestBody)}
	}
	if result.Response == nil {
		result.Response = &provider.GenerateResponse{
			Headers: singleValueHeaders(resp.Header),
			Body:    json.RawMessage(body),
		}
	} else {
		if result.Response.Headers == nil {
			result.Response.Headers = singleValueHeaders(resp.Header)
		}
		if result.Response.Body == nil {
			result.Response.Body = json.RawMessage(body)
		}
	}
	return result, nil
}

func isEventStreamContentType(value string) bool {
	mediaType, _, err := mime.ParseMediaType(value)
	return err == nil && mediaType == providerwire.MIMESSE
}

func isJSONContentType(value string) bool {
	mediaType, _, err := mime.ParseMediaType(value)
	return err == nil && mediaType == providerwire.MIMEJSON
}

func (m *model) doRequest(ctx context.Context, opts provider.CallOptions, streaming bool) (*http.Response, []byte, error) {
	body, err := m.codec().encodeCallOptions(opts)
	if err != nil {
		return nil, nil, fmt.Errorf("grafana: encoding call options: %w", err)
	}

	accessToken, err := m.accessToken(ctx)
	if err != nil {
		return nil, nil, err
	}

	endpoint := languageModelEndpoint(m.provider.baseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, nil, fmt.Errorf("grafana: creating model call request: %w", err)
	}
	m.setHeaders(req, accessToken, streaming, opts.Headers)

	resp, err := m.provider.httpClient.Do(req)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, nil, ctxErr
		}
		return nil, nil, newTransportAPICallError(endpoint, err)
	}
	return resp, body, nil
}

func languageModelEndpoint(baseURL string) string {
	u, err := url.Parse(baseURL)
	if err != nil {
		return baseURL + providerwire.PathLanguageModel
	}
	u.Path = path.Join(u.Path, providerwire.PathLanguageModel)
	return u.String()
}

func (m *model) accessToken(ctx context.Context) (string, error) {
	resp, err := m.provider.tokenExchanger.Exchange(ctx, authn.TokenExchangeRequest{
		Namespace: m.provider.namespace,
		Audiences: []string{m.provider.audience},
	})
	if err != nil {
		return "", fmt.Errorf("grafana: exchanging access token: %w", err)
	}
	if resp == nil || resp.Token == "" {
		return "", fmt.Errorf("grafana: token exchange returned empty access token")
	}
	return resp.Token, nil
}

func (m *model) setHeaders(req *http.Request, accessToken string, streaming bool, headers map[string]string) {
	for name, value := range headers {
		req.Header.Set(name, value)
	}
	req.Header.Set("Content-Type", providerwire.MIMEJSON)
	req.Header.Set(accessTokenHeader, accessToken)
	req.Header.Set(providerwire.HeaderModelID, m.modelID)
	req.Header.Set(providerwire.HeaderSpecVersion, providerwire.SpecVersionV4)
	if streaming {
		req.Header.Set(providerwire.HeaderStreaming, "true")
		req.Header.Set("Accept", providerwire.MIMESSE)
	} else {
		req.Header.Set(providerwire.HeaderStreaming, "false")
		req.Header.Set("Accept", providerwire.MIMEJSON)
	}
	if userToken := userIDTokenFromContext(req.Context()); userToken != "" {
		req.Header.Set(userIDHeader, userToken)
	}
}

func (m *model) readStream(ctx context.Context, resp *http.Response, ch chan<- provider.StreamPart, includeRawChunks bool) {
	defer close(ch)
	defer func() { _ = resp.Body.Close() }()

	reader, err := m.codec().newStreamReader(resp.Body, m.maxSSEEventBytes())
	if err != nil {
		if ctx.Err() == nil {
			_ = sendStreamPart(ctx, ch, provider.StreamPart{Type: provider.PartError, APICallError: newStreamAPICallError(resp, err)})
		}
		return
	}
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		part, err := reader.Next()
		if errors.Is(err, io.EOF) {
			return
		}
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			if !sendStreamPart(ctx, ch, provider.StreamPart{
				Type:         provider.PartError,
				APICallError: newStreamAPICallError(resp, err),
			}) {
				return
			}
			return
		}
		if part.Type == provider.PartRaw && !includeRawChunks {
			continue
		}
		if !sendStreamPart(ctx, ch, part) {
			return
		}
	}
}

func sendStreamPart(ctx context.Context, ch chan<- provider.StreamPart, part provider.StreamPart) bool {
	select {
	case <-ctx.Done():
		return false
	default:
	}

	select {
	case ch <- part:
		return true
	case <-ctx.Done():
		return false
	}
}

// surfaceHTTPError decodes a bounded non-2xx response and normalizes recognized
// gateway categories while preserving the APICallError as the cause.
func (m *model) surfaceHTTPError(resp *http.Response) error {
	apiErr, decoded := m.decodeOrSynthesizeHTTPError(resp)
	if m.strictMode() && !decoded {
		return apiErr
	}
	if gatewayErr := NormalizeAPICallError(apiErr); gatewayErr != nil {
		if m.strictMode() || gatewayErr.Type != GatewayErrorInternalServer {
			return gatewayErr
		}
	}
	return apiErr
}

func (m *model) decodeOrSynthesizeHTTPError(resp *http.Response) (*provider.APICallError, bool) {
	body, readErr := readResponseWithinLimit(resp.Body, m.maxErrorResponseBytes())
	if readErr != nil {
		retryable := !errors.Is(readErr, errResponseTooLarge)
		return provider.NewAPICallError(provider.APICallErrorOptions{
			Message:         fmt.Sprintf("grafana: reading error response body: %v", readErr),
			URL:             responseURL(resp),
			StatusCode:      resp.StatusCode,
			ResponseHeaders: cloneHeader(resp.Header),
			ResponseBody:    string(body),
			IsRetryable:     &retryable,
			Cause:           readErr,
		}), false
	}

	apiErr, err := m.codec().decodeErrorResponse(resp, body)
	if err == nil {
		if apiErr.URL == "" {
			apiErr.URL = responseURL(resp)
		}
		if apiErr.ResponseHeaders == nil {
			apiErr.ResponseHeaders = cloneHeader(resp.Header)
		}
		if apiErr.ResponseBody == "" {
			apiErr.ResponseBody = string(body)
		}
		return apiErr, true
	}

	options := provider.APICallErrorOptions{
		Message:         fmt.Sprintf("grafana: decoding error response: %v", err),
		URL:             responseURL(resp),
		StatusCode:      resp.StatusCode,
		ResponseHeaders: cloneHeader(resp.Header),
		ResponseBody:    string(body),
		Cause:           errors.Join(errProtocolResponse, err),
	}
	if m.strictMode() {
		retryable := false
		options.IsRetryable = &retryable
	}
	return provider.NewAPICallError(options), false
}

func newTransportAPICallError(endpoint string, err error) *provider.APICallError {
	retryable := true
	return provider.NewAPICallError(provider.APICallErrorOptions{
		Message:     fmt.Sprintf("grafana: model call request failed: %v", err),
		URL:         endpoint,
		IsRetryable: &retryable,
		Cause:       err,
	})
}

func (m *model) newInvalidGenerateContentTypeError(resp *http.Response) *provider.APICallError {
	return m.newInvalidContentTypeError(resp, providerwire.MIMEJSON, "generate")
}

func (m *model) newInvalidStreamContentTypeError(resp *http.Response) *provider.APICallError {
	return m.newInvalidContentTypeError(resp, providerwire.MIMESSE, "stream")
}

func (m *model) newInvalidContentTypeError(resp *http.Response, expected, operation string) *provider.APICallError {
	body, readErr := readResponseWithinLimit(resp.Body, m.maxErrorResponseBytes())
	retryable := false
	contentType := resp.Header.Get("Content-Type")
	cause := error(fmt.Errorf("grafana: invalid %s response content type %q", operation, contentType))
	if readErr != nil {
		cause = errors.Join(cause, readErr)
	}
	return provider.NewAPICallError(provider.APICallErrorOptions{
		Message:         fmt.Sprintf("grafana: expected %s response content type %q, got %q", operation, expected, contentType),
		URL:             responseURL(resp),
		StatusCode:      resp.StatusCode,
		ResponseHeaders: cloneHeader(resp.Header),
		ResponseBody:    string(body),
		IsRetryable:     &retryable,
		Cause:           cause,
	})
}

func newStreamAPICallError(resp *http.Response, err error) *provider.APICallError {
	retryable := !isProtocolStreamError(err)
	return provider.NewAPICallError(provider.APICallErrorOptions{
		Message:         fmt.Sprintf("grafana: reading stream response: %v", err),
		URL:             responseURL(resp),
		StatusCode:      resp.StatusCode,
		ResponseHeaders: cloneHeader(resp.Header),
		IsRetryable:     &retryable,
		Cause:           err,
	})
}

func newGenerateAPICallError(resp *http.Response, body []byte, err error) *provider.APICallError {
	retryable := !isProtocolGenerateError(err)
	return provider.NewAPICallError(provider.APICallErrorOptions{
		Message:         fmt.Sprintf("grafana: processing generate response: %v", err),
		URL:             responseURL(resp),
		StatusCode:      resp.StatusCode,
		ResponseHeaders: cloneHeader(resp.Header),
		ResponseBody:    string(body),
		IsRetryable:     &retryable,
		Cause:           err,
	})
}

var errProtocolResponse = errors.New("grafana: invalid provider-wire response")

func isProtocolGenerateError(err error) bool {
	if errors.Is(err, errProtocolResponse) || errors.Is(err, errResponseTooLarge) {
		return true
	}
	var syntaxErr *json.SyntaxError
	if errors.As(err, &syntaxErr) {
		return true
	}
	var typeErr *json.UnmarshalTypeError
	return errors.As(err, &typeErr)
}

func isProtocolStreamError(err error) bool {
	if errors.Is(err, errProtocolResponse) || errors.Is(err, errSSEEventTooLarge) {
		return true
	}
	var syntaxErr *json.SyntaxError
	if errors.As(err, &syntaxErr) {
		return true
	}
	var typeErr *json.UnmarshalTypeError
	if errors.As(err, &typeErr) {
		return true
	}
	return strings.Contains(err.Error(), "empty SSE data event") || strings.HasPrefix(err.Error(), "wire: empty SSE event")
}

func responseURL(resp *http.Response) string {
	if resp != nil && resp.Request != nil && resp.Request.URL != nil {
		return resp.Request.URL.String()
	}
	return ""
}

func cloneHeader(header http.Header) map[string][]string {
	if len(header) == 0 {
		return nil
	}
	clone := make(map[string][]string, len(header))
	for key, values := range header {
		clone[key] = append([]string(nil), values...)
	}
	return clone
}

func singleValueHeaders(header http.Header) map[string]string {
	if len(header) == 0 {
		return nil
	}
	clone := make(map[string]string, len(header))
	for key, values := range header {
		if len(values) > 0 {
			clone[key] = values[0]
		}
	}
	return clone
}
