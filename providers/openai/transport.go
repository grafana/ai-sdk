package openai

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/grafana/ai-sdk/provider"
	"github.com/openai/openai-go/v3/option"
)

type requestCapture struct {
	body json.RawMessage
}

type capturingBody struct {
	io.ReadCloser
	buffer bytes.Buffer
	failed bool
}

func (b *capturingBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if n > 0 {
		_, _ = b.buffer.Write(p[:n])
	}
	if err != nil && err != io.EOF {
		b.failed = true
	}
	return n, err
}

func (c *requestCapture) option() option.RequestOption {
	return option.WithMiddleware(func(req *http.Request, next option.MiddlewareNext) (*http.Response, error) {
		if req.Body == nil {
			return next(req)
		}
		body := &capturingBody{ReadCloser: req.Body}
		req.Body = body
		response, err := next(req)
		if err == nil && response != nil && response.StatusCode < http.StatusBadRequest && !body.failed &&
			(req.ContentLength < 0 || int64(body.buffer.Len()) == req.ContentLength) && json.Valid(body.buffer.Bytes()) {
			c.body = append(json.RawMessage(nil), body.buffer.Bytes()...)
		} else {
			c.body = nil
		}
		return response, err
	})
}

func transportHeaders(response *http.Response) map[string]string {
	if response == nil {
		return nil
	}
	headers := make(map[string]string, len(response.Header))
	for name, values := range response.Header {
		headers[strings.ToLower(name)] = strings.Join(values, ", ")
	}
	return headers
}

func (c *requestCapture) request() *provider.RequestMetadata {
	if len(c.body) == 0 {
		return nil
	}
	return &provider.RequestMetadata{Body: c.body}
}
