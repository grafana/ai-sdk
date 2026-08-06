package providerwirev4

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/grafana/ai-sdk/provider"
)

// SSEReader incrementally reads bounded complete SSE events.
type SSEReader struct {
	reader *bufio.Reader
	limit  int64
}

// NewSSEReader constructs a strict bounded SSE reader.
func NewSSEReader(reader io.Reader, eventLimit int64) (*SSEReader, error) {
	if reader == nil {
		return nil, fmt.Errorf("providerwirev4: nil SSE reader")
	}
	if eventLimit <= 0 {
		return nil, fmt.Errorf("providerwirev4: SSE event limit must be positive")
	}
	return &SSEReader{reader: bufio.NewReader(reader), limit: eventLimit}, nil
}

// Next reads and strictly decodes the next complete event. Final event bytes
// returned with io.EOF are processed before clean completion.
func (reader *SSEReader) Next() (provider.StreamPart, error) {
	var event bytes.Buffer
	var data strings.Builder
	hasData := false

	for {
		line, readErr := reader.readLine(&event)
		atEOF := errors.Is(readErr, io.EOF)
		if readErr != nil && !atEOF {
			return provider.StreamPart{}, readErr
		}

		trimmed := strings.TrimRight(string(line), "\r\n")
		if trimmed == "" {
			if hasData {
				return decodeStrictSSEData(data.String())
			}
			event.Reset()
		} else if !strings.HasPrefix(trimmed, ":") {
			field, value, ok := strings.Cut(trimmed, ":")
			if ok && field == "data" {
				value = strings.TrimPrefix(value, " ")
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
			return decodeStrictSSEData(data.String())
		}
	}
}

func (reader *SSEReader) readLine(event *bytes.Buffer) ([]byte, error) {
	var line bytes.Buffer
	for {
		fragment, err := reader.reader.ReadSlice('\n')
		if int64(event.Len()+len(fragment)) > reader.limit {
			return nil, errSSEEventTooLarge
		}
		event.Write(fragment)
		line.Write(fragment)
		if errors.Is(err, bufio.ErrBufferFull) {
			continue
		}
		return line.Bytes(), err
	}
}

func decodeStrictSSEData(data string) (provider.StreamPart, error) {
	if data == "" {
		return provider.StreamPart{}, fmt.Errorf("providerwirev4: empty SSE data event")
	}
	return DecodeStreamPart([]byte(data))
}
