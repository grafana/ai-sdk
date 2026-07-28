package bedrock

import (
	"bytes"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEncodeFrame_RoundTrip(t *testing.T) {
	hdr := frameHeader{
		MessageType: "event",
		EventType:   "contentBlockDelta",
		ContentType: "application/json",
	}
	payload := []byte(`{"contentBlockIndex":0,"delta":{"text":"hi"}}`)
	frame := encodeFrame(hdr, payload)

	var got []frameHeader
	var payloads [][]byte
	err := decodeEventStream(bytes.NewReader(frame), func(h frameHeader, p []byte) error {
		got = append(got, h)
		payloads = append(payloads, p)
		return nil
	})
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "event", got[0].MessageType)
	assert.Equal(t, "contentBlockDelta", got[0].EventType)
	assert.Equal(t, "application/json", got[0].ContentType)
	assert.Equal(t, payload, payloads[0])
}

func TestEncodeFrame_MultipleFramesSerial(t *testing.T) {
	var buf bytes.Buffer
	for i, msg := range []string{"first", "second", "third"} {
		hdr := frameHeader{MessageType: "event", EventType: "contentBlockDelta"}
		buf.Write(encodeFrame(hdr, []byte(msg)))
		_ = i
	}
	var got []string
	err := decodeEventStream(&buf, func(_ frameHeader, p []byte) error {
		got = append(got, string(p))
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"first", "second", "third"}, got)
}

func TestEncodeFrame_PartialReads(t *testing.T) {
	// Build several frames, then feed them through a reader that returns one
	// byte at a time to exercise the buffering path.
	var buf bytes.Buffer
	for _, msg := range []string{"a", "bb", "ccc"} {
		hdr := frameHeader{MessageType: "event", EventType: "contentBlockDelta"}
		buf.Write(encodeFrame(hdr, []byte(msg)))
	}
	src := &slowReader{buf: buf.Bytes()}
	var got []string
	err := decodeEventStream(src, func(_ frameHeader, p []byte) error {
		got = append(got, string(p))
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"a", "bb", "ccc"}, got)
}

func TestEncodeFrame_ExceptionEvent(t *testing.T) {
	hdr := frameHeader{
		MessageType:   "exception",
		ExceptionType: "throttlingException",
		ContentType:   "application/json",
	}
	payload := []byte(`{"message":"rate limit"}`)
	frame := encodeFrame(hdr, payload)

	err := decodeEventStream(bytes.NewReader(frame), func(h frameHeader, p []byte) error {
		assert.Equal(t, "exception", h.MessageType)
		assert.Equal(t, "throttlingException", h.ExceptionType)
		assert.Equal(t, payload, p)
		return nil
	})
	require.NoError(t, err)
}

func TestEncodeFrame_TruncatedFrameReturnsUnexpectedEOF(t *testing.T) {
	hdr := frameHeader{MessageType: "event", EventType: "contentBlockDelta"}
	frame := encodeFrame(hdr, []byte("hi"))
	truncated := frame[:len(frame)-5]
	err := decodeEventStream(bytes.NewReader(truncated), func(_ frameHeader, _ []byte) error {
		return nil
	})
	require.Error(t, err)
	assert.Equal(t, io.ErrUnexpectedEOF, err)
}

func TestEncodeFrame_BadPreludeCRC(t *testing.T) {
	hdr := frameHeader{MessageType: "event", EventType: "contentBlockDelta"}
	frame := encodeFrame(hdr, []byte("hi"))
	// Corrupt the prelude CRC (bytes 8-11) so the decoder rejects the frame.
	binary.BigEndian.PutUint32(frame[8:12], 0xdeadbeef)
	err := decodeEventStream(bytes.NewReader(frame), func(_ frameHeader, _ []byte) error {
		return nil
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrCRCMismatch), "got: %v", err)
}

func TestEncodeFrame_BadMessageCRC(t *testing.T) {
	hdr := frameHeader{MessageType: "event", EventType: "contentBlockDelta"}
	frame := encodeFrame(hdr, []byte("hi"))
	// Corrupt the payload so the message CRC fails.
	frame[len(frame)-7] ^= 0xff
	err := decodeEventStream(bytes.NewReader(frame), func(_ frameHeader, _ []byte) error {
		return nil
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrCRCMismatch), "got: %v", err)
}

func TestEncodeFrame_OtherHeaders(t *testing.T) {
	hdr := frameHeader{
		MessageType: "event",
		EventType:   "contentBlockDelta",
		Other:       map[string]string{"x-custom": "value"},
	}
	frame := encodeFrame(hdr, []byte("{}"))
	err := decodeEventStream(bytes.NewReader(frame), func(h frameHeader, _ []byte) error {
		assert.Equal(t, "value", h.Other["x-custom"])
		return nil
	})
	require.NoError(t, err)
}

func TestEncodeFrame_CallbackErrorStopsDecoding(t *testing.T) {
	var buf bytes.Buffer
	for i := 0; i < 3; i++ {
		hdr := frameHeader{MessageType: "event", EventType: "contentBlockDelta"}
		buf.Write(encodeFrame(hdr, []byte("x")))
	}
	want := errors.New("stop")
	count := 0
	err := decodeEventStream(&buf, func(_ frameHeader, _ []byte) error {
		count++
		if count == 2 {
			return want
		}
		return nil
	})
	assert.Equal(t, want, err)
	assert.Equal(t, 2, count)
}

// slowReader returns one byte per Read call. Used to ensure the decoder
// buffers across partial reads correctly.
type slowReader struct {
	buf []byte
	off int
}

func (s *slowReader) Read(p []byte) (int, error) {
	if s.off >= len(s.buf) {
		return 0, io.EOF
	}
	p[0] = s.buf[s.off]
	s.off++
	return 1, nil
}

func TestParseHeaders_KnownTypes(t *testing.T) {
	// Construct a raw header block with various value types.
	var buf bytes.Buffer
	writeHeader := func(name string, valueType byte, value []byte) {
		buf.WriteByte(byte(len(name)))
		buf.WriteString(name)
		buf.WriteByte(valueType)
		buf.Write(value)
	}
	writeString := func(name, value string) {
		l := make([]byte, 2)
		binary.BigEndian.PutUint16(l, uint16(len(value)))
		writeHeader(name, 7, append(l, []byte(value)...))
	}
	writeString(":event-type", "messageStart")
	writeString(":content-type", "application/json")
	writeHeader("intval", 4, []byte{0, 0, 0, 7}) // int32 skipped
	writeString(":message-type", "event")

	hdr, err := parseHeaders(buf.Bytes())
	require.NoError(t, err)
	assert.Equal(t, "messageStart", hdr.EventType)
	assert.Equal(t, "application/json", hdr.ContentType)
	assert.Equal(t, "event", hdr.MessageType)
	assert.NotContains(t, hdr.Other, "intval", "non-string headers are dropped from Other")
}

func TestParseHeaders_TruncatedReturnsError(t *testing.T) {
	_, err := parseHeaders([]byte{0x05, 'a', 'b'})
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "truncated"), "got: %v", err)
}

func TestEncodeFrame_KnownGoodFrame(t *testing.T) {
	// Construct a known-good frame manually and verify our decoder reads it.
	headers := []byte{}
	add := func(name, value string) {
		headers = append(headers, byte(len(name)))
		headers = append(headers, name...)
		headers = append(headers, 7)
		lenBytes := []byte{0, 0}
		binary.BigEndian.PutUint16(lenBytes, uint16(len(value)))
		headers = append(headers, lenBytes...)
		headers = append(headers, value...)
	}
	add(":event-type", "messageStart")
	payload := []byte(`{"role":"assistant"}`)

	totalLen := 12 + len(headers) + len(payload) + 4
	headersLen := len(headers)

	frame := make([]byte, 0, totalLen)
	prelude := make([]byte, 8)
	binary.BigEndian.PutUint32(prelude[:4], uint32(totalLen))
	binary.BigEndian.PutUint32(prelude[4:], uint32(headersLen))
	frame = append(frame, prelude...)
	preludeCRC := crc32.ChecksumIEEE(prelude)
	preludeCRCBytes := []byte{0, 0, 0, 0}
	binary.BigEndian.PutUint32(preludeCRCBytes, preludeCRC)
	frame = append(frame, preludeCRCBytes...)
	frame = append(frame, headers...)
	frame = append(frame, payload...)
	msgCRC := crc32.ChecksumIEEE(frame)
	msgCRCBytes := []byte{0, 0, 0, 0}
	binary.BigEndian.PutUint32(msgCRCBytes, msgCRC)
	frame = append(frame, msgCRCBytes...)

	err := decodeEventStream(bytes.NewReader(frame), func(h frameHeader, p []byte) error {
		assert.Equal(t, "messageStart", h.EventType)
		assert.Equal(t, payload, p)
		return nil
	})
	require.NoError(t, err)
}
