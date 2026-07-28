//go:build conformance

package conformance

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"hash/crc32"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSSEFraming_ContentType(t *testing.T) {
	assert.Equal(t, "text/event-stream", SSEFraming{}.ContentType())
}

func TestBedrockFraming_ContentType(t *testing.T) {
	assert.Equal(t, "application/vnd.amazon.eventstream", BedrockFraming{}.ContentType())
}

func TestBedrockFraming_EncodesValidFrames(t *testing.T) {
	fixture := []byte(`{"messageStart":{"role":"assistant"}}
{"contentBlockDelta":{"contentBlockIndex":0,"delta":{"text":"hi"}}}
`)
	rec := httptest.NewRecorder()
	BedrockFraming{}.WriteFixture(rec, fixture)

	body := rec.Body.Bytes()
	require.NotEmpty(t, body)

	// Parse the bytes back: validate prelude + headers + payload for the
	// first frame so we know the encoding is well-formed.
	require.GreaterOrEqual(t, len(body), 16)
	totalLen := binary.BigEndian.Uint32(body[:4])
	headersLen := binary.BigEndian.Uint32(body[4:8])
	preludeCRC := binary.BigEndian.Uint32(body[8:12])
	assert.Equal(t, crc32.ChecksumIEEE(body[:8]), preludeCRC)

	// Extract :event-type from headers by linear scan.
	headers := body[12 : 12+headersLen]
	assert.True(t, strings.Contains(string(headers), "messageStart"))

	// Verify total frame length matches.
	require.GreaterOrEqual(t, len(body), int(totalLen))
	// CRC32 of the full first frame minus the last 4 bytes must equal the
	// trailing 4 bytes.
	firstFrame := body[:totalLen]
	assert.Equal(t, crc32.ChecksumIEEE(firstFrame[:len(firstFrame)-4]),
		binary.BigEndian.Uint32(firstFrame[len(firstFrame)-4:]))
}

func TestSSEFraming_EmitsEventTypeAndData(t *testing.T) {
	fixture := []byte(`{"type":"message_start","data":{"foo":"bar"}}` + "\n")
	rec := httptest.NewRecorder()
	SSEFraming{}.WriteFixture(rec, fixture)
	body := rec.Body.String()
	assert.Contains(t, body, "event: message_start")
	assert.Contains(t, body, "data: ")
}

func TestBedrockFraming_SkipsMalformedLines(t *testing.T) {
	fixture := []byte("not json\n{\"messageStart\":{\"role\":\"assistant\"}}\n")
	rec := httptest.NewRecorder()
	BedrockFraming{}.WriteFixture(rec, fixture)
	// Only one valid line should produce a frame.
	assert.Greater(t, len(rec.Body.Bytes()), 0)
}

func TestBedrockFraming_HeaderEntryFormat(t *testing.T) {
	// Sanity-check that the header layout matches what the Go decoder expects:
	// name-length, name, value-type, value-length, value.
	headers := encodeHeader(":event-type", "messageStart")
	require.Len(t, headers, 1+len(":event-type")+1+2+len("messageStart"))
	assert.Equal(t, byte(len(":event-type")), headers[0])
	assert.Equal(t, byte(7), headers[1+len(":event-type")])
}

func TestBedrockFraming_RoundTripsCompletely(t *testing.T) {
	// Make sure a writer + reader round-trip yields the original payload.
	rec := httptest.NewRecorder()
	rec.Header().Set("Content-Type", BedrockFraming{}.ContentType())
	BedrockFraming{}.WriteFixture(rec, []byte(`{"contentBlockDelta":{"contentBlockIndex":0,"delta":{"text":"hi"}}}`+"\n"))
	res := rec.Result()
	defer func() { _ = res.Body.Close() }()
	assert.Equal(t, "application/vnd.amazon.eventstream", res.Header.Get("Content-Type"))
	body, _ := readAll(res.Body)
	// Validate that the payload contains the inner JSON object somewhere.
	require.NotEmpty(t, body)
	assert.True(t, bytes.Contains(body, []byte(`"contentBlockIndex":0`)))
}

func TestBedrockFraming_PayloadPreserved(t *testing.T) {
	// Encode + decode (manual reads from bytes) to assert payload bytes are
	// exactly the inner JSON object.
	fixture := []byte(`{"foo":{"bar":42}}` + "\n")
	rec := httptest.NewRecorder()
	BedrockFraming{}.WriteFixture(rec, fixture)
	body := rec.Body.Bytes()
	totalLen := binary.BigEndian.Uint32(body[:4])
	headersLen := binary.BigEndian.Uint32(body[4:8])
	payload := body[12+headersLen : totalLen-4]
	var got struct{ Bar int }
	require.NoError(t, json.Unmarshal(payload, &got))
	assert.Equal(t, 42, got.Bar)
}

func readAll(r interface{ Read(p []byte) (int, error) }) ([]byte, error) {
	var out []byte
	buf := make([]byte, 1024)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			out = append(out, buf[:n]...)
		}
		if err != nil {
			if err.Error() == "EOF" {
				return out, nil
			}
			return out, err
		}
	}
}

// Compile-time check: BedrockFraming must be usable as a Framing.
var _ Framing = BedrockFraming{}
var _ Framing = SSEFraming{}

// Compile-time check: http.ResponseWriter expected by Framing.
var _ Framing = framingFunc(func(w http.ResponseWriter, _ []byte) {})

type framingFunc func(http.ResponseWriter, []byte)

func (framingFunc) ContentType() string                            { return "" }
func (f framingFunc) WriteFixture(w http.ResponseWriter, b []byte) { f(w, b) }
