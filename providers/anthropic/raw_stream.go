package anthropic

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/grafana/ai-sdk/provider"
)

type rawFrameCapture struct {
	pending []provider.StreamPart
}

func (c *rawFrameCapture) option() option.RequestOption {
	return option.WithMiddleware(func(req *http.Request, next option.MiddlewareNext) (*http.Response, error) {
		response, err := next(req)
		if err == nil && response != nil && response.StatusCode < http.StatusBadRequest && response.Body != nil &&
			strings.HasPrefix(strings.ToLower(response.Header.Get("Content-Type")), "text/event-stream") {
			response.Body = &rawFrameBody{ReadCloser: response.Body, reader: bufio.NewReader(response.Body), capture: c}
		}
		return response, err
	})
}

func (c *rawFrameCapture) take() []provider.StreamPart {
	parts := c.pending
	c.pending = nil
	return parts
}

func (c *rawFrameCapture) record(frame []byte) {
	var lines [][]byte
	for _, line := range bytes.Split(frame, []byte{'\n'}) {
		line = bytes.TrimSuffix(line, []byte{'\r'})
		key, value, ok := bytes.Cut(line, []byte{':'})
		if !ok || !bytes.Equal(key, []byte("data")) {
			continue
		}
		value = bytes.TrimPrefix(value, []byte{' '})
		lines = append(lines, value)
	}
	if len(lines) == 0 {
		return
	}
	data := bytes.TrimSpace(bytes.Join(lines, []byte{'\n'}))
	if bytes.Equal(data, []byte("[DONE]")) {
		return
	}
	var raw json.RawMessage
	if json.Valid(data) {
		var compact bytes.Buffer
		if json.Compact(&compact, data) == nil {
			raw = append(json.RawMessage(nil), compact.Bytes()...)
		}
	}
	c.pending = append(c.pending, provider.StreamPart{Type: provider.PartRaw, RawValue: raw})
}

type rawFrameBody struct {
	io.ReadCloser
	reader  *bufio.Reader
	capture *rawFrameCapture
	frame   []byte
	offset  int
	lastErr error
}

func (b *rawFrameBody) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if len(b.frame) == 0 {
		if b.lastErr != nil {
			return 0, b.lastErr
		}
		for {
			line, err := b.reader.ReadBytes('\n')
			b.frame = append(b.frame, line...)
			if err != nil {
				b.lastErr = err
				break
			}
			if bytes.Equal(line, []byte{'\n'}) || bytes.Equal(line, []byte{'\r', '\n'}) {
				break
			}
		}
		if len(b.frame) == 0 {
			return 0, b.lastErr
		}
	}
	n := copy(p, b.frame[b.offset:])
	b.offset += n
	if b.offset == len(b.frame) {
		if b.lastErr == nil {
			b.capture.record(b.frame)
		}
		b.frame = nil
		b.offset = 0
	}
	return n, nil
}
