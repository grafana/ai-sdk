package openai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/openai/openai-go/v3/option"
)

func captureRequestBody(body *json.RawMessage) option.Middleware {
	return func(req *http.Request, next option.MiddlewareNext) (*http.Response, error) {
		*body = nil
		if req.Body != nil {
			payload, err := io.ReadAll(req.Body)
			closeErr := req.Body.Close()
			if err != nil {
				return nil, fmt.Errorf("openai: capturing request body: %w", err)
			}
			if closeErr != nil {
				return nil, fmt.Errorf("openai: closing request body: %w", closeErr)
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
