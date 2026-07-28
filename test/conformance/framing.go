//go:build conformance

package conformance

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"hash/crc32"
	"net/http"
)

// Framing serializes a fixture body (a sequence of JSON lines) onto an HTTP
// response, controlling Content-Type and per-line wire framing. Anthropic and
// Grafana fixtures use SSE; Bedrock fixtures use AWS Smithy event-stream
// binary frames.
type Framing interface {
	ContentType() string
	WriteFixture(w http.ResponseWriter, fixture []byte)
}

// SSEFraming is the default SSE wire format used by Anthropic and Grafana
// fixtures. Each fixture line is wrapped as `event: <type>\ndata: <json>\n\n`
// where the event type comes from the JSON `type` field.
type SSEFraming struct{}

func (SSEFraming) ContentType() string { return "text/event-stream" }

func (SSEFraming) WriteFixture(w http.ResponseWriter, fixture []byte) {
	flusher, _ := w.(http.Flusher)
	scanner := bufio.NewScanner(bytes.NewReader(fixture))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		eventType := extractSSEEventType(line)
		_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", eventType, line)
		if flusher != nil {
			flusher.Flush()
		}
	}
}

func extractSSEEventType(line string) string {
	var m struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal([]byte(line), &m); err != nil || m.Type == "" {
		return "unknown"
	}
	return m.Type
}

// BedrockFraming serializes Bedrock fixture lines as AWS Smithy event-stream
// binary frames. Each line is a JSON object with a single outer key (e.g.
// `messageStart`, `contentBlockDelta`); we encode the outer key as the
// `:event-type` header and the inner JSON object as the frame payload.
// Content-Type is `application/vnd.amazon.eventstream`.
type BedrockFraming struct{}

func (BedrockFraming) ContentType() string { return "application/vnd.amazon.eventstream" }

func (BedrockFraming) WriteFixture(w http.ResponseWriter, fixture []byte) {
	flusher, _ := w.(http.Flusher)
	scanner := bufio.NewScanner(bytes.NewReader(fixture))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		var wrapper map[string]json.RawMessage
		if err := json.Unmarshal([]byte(line), &wrapper); err != nil || len(wrapper) != 1 {
			continue
		}
		for eventType, payload := range wrapper {
			frame := encodeBedrockFrame(eventType, payload)
			_, _ = w.Write(frame)
			if flusher != nil {
				flusher.Flush()
			}
		}
	}
}

// encodeBedrockFrame builds a single AWS event-stream binary frame with
// `:event-type=<eventType>` and the given JSON payload. Layout:
//
//	4 bytes total length
//	4 bytes headers length
//	4 bytes prelude CRC
//	N bytes headers
//	M bytes payload
//	4 bytes message CRC
func encodeBedrockFrame(eventType string, payload []byte) []byte {
	headers := encodeHeader(":message-type", "event")
	headers = append(headers, encodeHeader(":event-type", eventType)...)
	headers = append(headers, encodeHeader(":content-type", "application/json")...)

	headersLen := len(headers)
	totalLen := 8 + 4 + headersLen + len(payload) + 4

	frame := make([]byte, 0, totalLen)
	prelude := make([]byte, 8)
	binary.BigEndian.PutUint32(prelude[:4], uint32(totalLen))
	binary.BigEndian.PutUint32(prelude[4:], uint32(headersLen))
	frame = append(frame, prelude...)
	preludeCRC := crc32.ChecksumIEEE(prelude)
	preludeCRCBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(preludeCRCBytes, preludeCRC)
	frame = append(frame, preludeCRCBytes...)
	frame = append(frame, headers...)
	frame = append(frame, payload...)
	msgCRC := crc32.ChecksumIEEE(frame)
	msgCRCBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(msgCRCBytes, msgCRC)
	frame = append(frame, msgCRCBytes...)
	return frame
}

// encodeHeader emits a single string-typed (type 7) header entry.
func encodeHeader(name, value string) []byte {
	buf := []byte{byte(len(name))}
	buf = append(buf, name...)
	buf = append(buf, 7) // string type
	lenBytes := make([]byte, 2)
	binary.BigEndian.PutUint16(lenBytes, uint16(len(value)))
	buf = append(buf, lenBytes...)
	buf = append(buf, value...)
	return buf
}
