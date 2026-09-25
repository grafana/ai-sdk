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

func (c *rawFrameCapture) record(data []byte) {
	data = bytes.TrimSpace(data)
	if bytes.Equal(data, []byte("[DONE]")) {
		return
	}
	var raw json.RawMessage
	var compact bytes.Buffer
	if json.Compact(&compact, data) == nil {
		raw = compact.Bytes()
	}
	c.pending = append(c.pending, provider.StreamPart{Type: provider.PartRaw, RawValue: raw})
}

type rawFrameBody struct {
	io.ReadCloser
	reader     *bufio.Reader
	capture    *rawFrameCapture
	chunk      []byte
	line       []byte
	frameData  []byte
	ignoreLine bool
	hasData    bool
	frameDone  bool
	offset     int
	lastErr    error
}

func (b *rawFrameBody) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if len(b.chunk) == 0 {
		if b.lastErr != nil {
			return 0, b.lastErr
		}
		chunk, err := b.reader.ReadSlice('\n')
		b.chunk = chunk
		if err != nil && err != bufio.ErrBufferFull {
			b.lastErr = err
		}
		if err == bufio.ErrBufferFull && len(b.line) == 0 && !bytes.HasPrefix(chunk, []byte("data:")) {
			b.ignoreLine = true
		}
		if !b.ignoreLine {
			b.line = append(b.line, chunk...)
		}
		if err == nil {
			if !b.ignoreLine {
				line := bytes.TrimSuffix(b.line, []byte{'\n'})
				line = bytes.TrimSuffix(line, []byte{'\r'})
				if len(line) == 0 {
					b.frameDone = true
				} else {
					key, value, _ := bytes.Cut(line, []byte{':'})
					if bytes.Equal(key, []byte("data")) {
						if b.hasData {
							b.frameData = append(b.frameData, '\n')
						}
						b.frameData = append(b.frameData, bytes.TrimPrefix(value, []byte{' '})...)
						b.hasData = true
					}
				}
			}
			b.line = nil
			b.ignoreLine = false
		}
		if len(b.chunk) == 0 {
			return 0, b.lastErr
		}
	}
	n := copy(p, b.chunk[b.offset:])
	b.offset += n
	if b.offset == len(b.chunk) {
		if b.frameDone {
			if b.hasData {
				b.capture.record(b.frameData)
			}
			b.frameData = nil
			b.hasData = false
			b.frameDone = false
		}
		b.chunk = nil
		b.offset = 0
	}
	return n, nil
}
