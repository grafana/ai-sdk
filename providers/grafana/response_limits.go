package grafana

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"
)

var (
	errResponseTooLarge = errors.New("grafana: response body exceeds configured limit")
	errSSEEventTooLarge = errors.New("grafana: SSE event exceeds configured limit")
)

func readResponseWithinLimit(reader io.Reader, limit int64) ([]byte, error) {
	limited := &io.LimitedReader{R: reader, N: limit}
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	var extra [1]byte
	count, extraErr := io.ReadFull(reader, extra[:])
	if count > 0 {
		return data, errResponseTooLarge
	}
	if extraErr != nil && !errors.Is(extraErr, io.EOF) && !errors.Is(extraErr, io.ErrUnexpectedEOF) {
		return data, extraErr
	}
	return data, nil
}

type ssePayloadReader struct {
	reader *bufio.Reader
	limit  int64
}

func newSSEPayloadReader(reader io.Reader, limit int64) (*ssePayloadReader, error) {
	if reader == nil {
		return nil, fmt.Errorf("grafana: nil SSE reader")
	}
	if limit <= 0 {
		return nil, fmt.Errorf("grafana: SSE event limit must be positive")
	}
	return &ssePayloadReader{reader: bufio.NewReader(reader), limit: limit}, nil
}

func (reader *ssePayloadReader) next() ([]byte, error) {
	var event bytes.Buffer
	var data strings.Builder
	hasData := false
	for {
		line, readErr := reader.readLine(&event)
		atEOF := errors.Is(readErr, io.EOF)
		if readErr != nil && !atEOF {
			return nil, readErr
		}
		trimmed := strings.TrimRight(string(line), "\r\n")
		if trimmed == "" {
			if hasData {
				if data.Len() == 0 {
					return nil, errors.New("grafana: empty SSE data event")
				}
				return []byte(data.String()), nil
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
				return nil, io.EOF
			}
			if data.Len() == 0 {
				return nil, errors.New("grafana: empty SSE data event")
			}
			return []byte(data.String()), nil
		}
	}
}

func (reader *ssePayloadReader) readLine(event *bytes.Buffer) ([]byte, error) {
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
