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

func (m *model) DoStream(ctx context.Context, opts provider.CallOptions) (*provider.StreamResult, error) {
	resp, body, err := m.doRequest(ctx, opts, true)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		defer func() { _ = resp.Body.Close() }()
		return nil, surfaceHTTPError(resp)
	}
	if !isEventStreamContentType(resp.Header.Get("Content-Type")) {
		defer func() { _ = resp.Body.Close() }()
		return nil, newInvalidStreamContentTypeError(resp)
	}

	ch := make(chan provider.StreamPart, streamBufferSize)
	go m.readStream(ctx, resp, ch, opts.IncludeRawChunks != nil && *opts.IncludeRawChunks)

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
		return nil, surfaceHTTPError(resp)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, newGenerateAPICallError(resp, nil, err)
	}
	result, err := providerwire.DecodeGenerateResult(body)
	if err != nil {
		return nil, newGenerateAPICallError(resp, body, err)
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

func (m *model) doRequest(ctx context.Context, opts provider.CallOptions, streaming bool) (*http.Response, []byte, error) {
	body, err := providerwire.EncodeCallOptions(opts)
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

	reader := providerwire.NewSSEReader(resp.Body)
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

// surfaceHTTPError decodes a non-2xx HTTP response into an *provider.APICallError
// and, as the gateway analog of the Vercel AI SDK gateway, runs the provider
// normalizer to surface a *provider.GatewayError when a normalized category is
// identified. When no category is identified the plain *provider.APICallError is
// surfaced. Either way the decoded APICallError (with its Data, status, headers,
// body, and retryability) remains reachable via errors.As.
func surfaceHTTPError(resp *http.Response) error {
	apiErr := decodeOrSynthesizeHTTPError(resp)
	if gw := NormalizeAPICallError(apiErr); gw != nil && gw.Type != GatewayErrorInternalServer {
		return gw
	}
	return apiErr
}

func decodeOrSynthesizeHTTPError(resp *http.Response) *provider.APICallError {
	body, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return provider.NewAPICallError(provider.APICallErrorOptions{
			Message:         fmt.Sprintf("grafana: reading error response body: %v", readErr),
			URL:             responseURL(resp),
			StatusCode:      resp.StatusCode,
			ResponseHeaders: cloneHeader(resp.Header),
			Cause:           readErr,
		})
	}

	clone := *resp
	clone.Body = io.NopCloser(bytes.NewReader(body))
	apiErr, err := providerwire.DecodeErrorResponse(&clone)
	if err == nil {
		return apiErr
	}

	return provider.NewAPICallError(provider.APICallErrorOptions{
		Message:         fmt.Sprintf("grafana: decoding error response: %v", err),
		URL:             responseURL(resp),
		StatusCode:      resp.StatusCode,
		ResponseHeaders: cloneHeader(resp.Header),
		ResponseBody:    string(body),
		Cause:           err,
	})
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

func newInvalidStreamContentTypeError(resp *http.Response) *provider.APICallError {
	body, _ := io.ReadAll(resp.Body)
	retryable := false
	contentType := resp.Header.Get("Content-Type")
	return provider.NewAPICallError(provider.APICallErrorOptions{
		Message:         fmt.Sprintf("grafana: expected stream response content type %q, got %q", providerwire.MIMESSE, contentType),
		URL:             responseURL(resp),
		StatusCode:      resp.StatusCode,
		ResponseHeaders: cloneHeader(resp.Header),
		ResponseBody:    string(body),
		IsRetryable:     &retryable,
		Cause:           fmt.Errorf("grafana: invalid stream response content type %q", contentType),
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

func isProtocolGenerateError(err error) bool {
	var syntaxErr *json.SyntaxError
	if errors.As(err, &syntaxErr) {
		return true
	}
	var typeErr *json.UnmarshalTypeError
	return errors.As(err, &typeErr)
}

func isProtocolStreamError(err error) bool {
	var syntaxErr *json.SyntaxError
	if errors.As(err, &syntaxErr) {
		return true
	}
	var typeErr *json.UnmarshalTypeError
	if errors.As(err, &typeErr) {
		return true
	}
	return strings.HasPrefix(err.Error(), "wire: empty SSE event")
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
