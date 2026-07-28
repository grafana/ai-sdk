package providerwire

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/grafana/ai-sdk/provider"
)

// WriteSSEStreamPart writes a single [provider.StreamPart] to w as one SSE
// event in the form `data: <JSON>\n\n`. The Type discriminator lives entirely
// in the JSON body; the SSE event:-name field is not used.
//
// Callers MUST flush w (e.g. http.Flusher) after returning if they need
// real-time delivery; this helper only writes bytes.
func WriteSSEStreamPart(w io.Writer, part provider.StreamPart) error {
	body, err := json.Marshal(part)
	if err != nil {
		return fmt.Errorf("wire: marshaling StreamPart: %w", err)
	}
	var compacted bytes.Buffer
	if err := json.Compact(&compacted, body); err != nil {
		return fmt.Errorf("wire: compacting StreamPart JSON: %w", err)
	}
	if _, err := io.WriteString(w, "data: "); err != nil {
		return err
	}
	if _, err := w.Write(compacted.Bytes()); err != nil {
		return err
	}
	if _, err := io.WriteString(w, "\n\n"); err != nil {
		return err
	}
	return nil
}

// WriteSSEStreamPartTo writes a single [provider.StreamPart] to w and flushes
// it when w implements [http.Flusher]. Use [WriteSSEStreamPart] when explicit
// flush control is required.
func WriteSSEStreamPartTo(w http.ResponseWriter, part provider.StreamPart) error {
	if err := WriteSSEStreamPart(w, part); err != nil {
		return err
	}
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
	return nil
}

// SSEReader reads SSE-framed [provider.StreamPart] events from an io.Reader.
// Use [NewSSEReader] to construct one and call [SSEReader.Next] in a loop
// until io.EOF or another error is returned.
type SSEReader struct {
	br *bufio.Reader
}

// NewSSEReader wraps r as an SSE event source for [provider.StreamPart] events.
func NewSSEReader(r io.Reader) *SSEReader {
	return &SSEReader{br: bufio.NewReader(r)}
}

// Next reads the next event from the stream and decodes it into a
// [provider.StreamPart]. Returns [io.EOF] on clean stream completion. A final
// data line is decoded even when the source ends without a trailing newline.
//
// Lines that are not data: lines (e.g. comments, retry:, event:, id:) are
// ignored; this matches conservative SSE consumers and keeps the on-wire
// shape compatible with proxies that may inject keepalive comments.
func (r *SSEReader) Next() (provider.StreamPart, error) {
	var data strings.Builder
	hasData := false
	for {
		line, readErr := r.br.ReadString('\n')
		atEOF := errors.Is(readErr, io.EOF)
		if readErr != nil && !atEOF {
			return provider.StreamPart{}, readErr
		}

		// Trim only the trailing newline characters; keep leading whitespace
		// since "data: " is the SSE field-name prefix.
		line = strings.TrimRight(line, "\r\n")

		if line == "" {
			if hasData {
				return decodeSSEEvent(data.String())
			}
		} else if !strings.HasPrefix(line, ":") {
			field, value, ok := strings.Cut(line, ":")
			if ok && field == "data" {
				// SSE allows an optional single space after the colon.
				value = strings.TrimPrefix(value, " ")
				// Per SSE spec, multiple `data:` lines in a single event are
				// joined with a literal newline between them, not concatenated.
				// (https://html.spec.whatwg.org/multipage/server-sent-events.html#dispatchMessage)
				if hasData {
					data.WriteByte('\n')
				}
				hasData = true
				data.WriteString(value)
			}
		}

		if atEOF {
			if !hasData {
				return provider.StreamPart{}, io.EOF
			}
			return decodeSSEEvent(data.String())
		}
	}
}

func decodeSSEEvent(payload string) (provider.StreamPart, error) {
	if payload == "" {
		return provider.StreamPart{}, fmt.Errorf("wire: empty SSE event")
	}
	var part provider.StreamPart
	if err := json.Unmarshal([]byte(payload), &part); err != nil {
		return provider.StreamPart{}, fmt.Errorf("wire: decoding StreamPart: %w", err)
	}
	return part, nil
}
