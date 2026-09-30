package anthropic

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/anthropics/anthropic-sdk-go/option"
)

func captureRequestBody(body *json.RawMessage) option.Middleware {
	return func(req *http.Request, next option.MiddlewareNext) (*http.Response, error) {
		*body = nil
		if req.Body != nil {
			payload, err := io.ReadAll(req.Body)
			closeErr := req.Body.Close()
			if err != nil {
				return nil, fmt.Errorf("anthropic: capturing request body: %w", err)
			}
			if closeErr != nil {
				return nil, fmt.Errorf("anthropic: closing request body: %w", closeErr)
			}
			req.Body = io.NopCloser(bytes.NewReader(payload))
			if req.GetBody != nil {
				req.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(payload)), nil }
			}
			if json.Valid(payload) {
				*body = append(json.RawMessage(nil), payload...)
			}
		}
		return next(req)
	}
}

func normalizeBetaHeaders(req *http.Request, next option.MiddlewareNext) (*http.Response, error) {
	var betas []string
	seen := make(map[string]bool)
	for _, value := range req.Header.Values("anthropic-beta") {
		for token := range strings.SplitSeq(value, ",") {
			beta := strings.ToLower(strings.TrimSpace(token))
			if beta != "" && !seen[beta] {
				seen[beta] = true
				betas = append(betas, beta)
			}
		}
	}
	if len(betas) > 0 {
		req.Header.Set("anthropic-beta", strings.Join(betas, ","))
	} else {
		req.Header.Del("anthropic-beta")
	}
	return next(req)
}

func flattenHeaders(headers http.Header) map[string]string {
	if len(headers) == 0 {
		return nil
	}
	out := make(map[string]string, len(headers))
	for key, values := range headers {
		if len(values) > 0 {
			out[key] = strings.Join(values, ", ")
		}
	}
	return out
}
