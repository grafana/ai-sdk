package bedrock

import (
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
)

// AWS Smithy event stream binary frame format:
//
//	+--------+--------+--------+--------+
//	|     total length (4)     |
//	+--------+--------+--------+--------+
//	|    headers length (4)    |
//	+--------+--------+--------+--------+
//	|     prelude CRC (4)      |  ; CRC32 of total + headers lengths
//	+--------+--------+--------+--------+
//	|   headers (headersLen bytes)     |  ; sequence of header entries
//	+----------------------------------+
//	|   payload (totalLen - 16 -       |
//	|             headersLen bytes)     |
//	+----------------------------------+
//	|     message CRC (4)              |  ; CRC32 of all bytes except last 4
//	+----------------------------------+
//
// Header entry:
//   1 byte name length
//   N bytes name
//   1 byte value type (only string types 7 and 6 used by Bedrock)
//   2 bytes value length (big-endian, for string types)
//   M bytes value
//
// We extract `:event-type`, `:message-type`, `:exception-type`, and
// `:content-type` and the JSON payload. Other header types are tolerated
// (skipped) so the decoder remains forward-compatible.

const (
	maxFrameLength = 16 * 1024 * 1024 // safety bound to avoid runaway allocation
	preludeSize    = 8
	minFrameLen    = preludeSize + 4 + 4 // prelude + prelude CRC + msg CRC
)

// frameHeader is the set of metadata headers we care about. Keys are
// canonicalised by the decoder (lower-cased) before dispatch.
type frameHeader struct {
	MessageType   string
	EventType     string
	ExceptionType string
	ContentType   string
	// Other carries unknown headers so the caller can log or surface them.
	Other map[string]string
}

// decodeEventStream reads AWS event-stream frames from r and invokes cb with
// each decoded frame. It returns when r yields io.EOF after a complete frame,
// or when cb returns a non-nil error, or when ctx is cancelled.
//
// Partial frames are buffered across Read calls; the underlying reader is
// drained in chunks. CRC validation is enabled by default; mismatches return
// ErrCRCMismatch.
//
// The function does not close r.
func decodeEventStream(r io.Reader, cb func(hdr frameHeader, payload []byte) error) error {
	dec := &eventStreamDecoder{src: r}
	return dec.run(cb)
}

// ErrCRCMismatch is returned when a frame fails CRC validation. Indicates
// transport corruption or a decoder bug.
var ErrCRCMismatch = errors.New("event stream: CRC mismatch")

type eventStreamDecoder struct {
	src io.Reader
	buf []byte
}

func (d *eventStreamDecoder) run(cb func(frameHeader, []byte) error) error {
	for {
		// Ensure we have at least 4 bytes for the length prefix.
		if err := d.fill(4); err != nil {
			if err == io.EOF && len(d.buf) == 0 {
				return nil
			}
			if err == io.EOF {
				return io.ErrUnexpectedEOF
			}
			return err
		}
		totalLen := int(binary.BigEndian.Uint32(d.buf[:4]))
		if totalLen < minFrameLen {
			return fmt.Errorf("event stream: invalid frame length %d", totalLen)
		}
		if totalLen > maxFrameLength {
			return fmt.Errorf("event stream: frame length %d exceeds limit %d", totalLen, maxFrameLength)
		}
		if err := d.fill(totalLen); err != nil {
			if err == io.EOF {
				return io.ErrUnexpectedEOF
			}
			return err
		}
		frame := d.buf[:totalLen]
		hdr, payload, err := parseFrame(frame)
		if err != nil {
			return err
		}
		if cbErr := cb(hdr, payload); cbErr != nil {
			return cbErr
		}
		// Drop the consumed frame from the buffer.
		d.buf = append(d.buf[:0], d.buf[totalLen:]...)
	}
}

// fill ensures d.buf has at least n bytes, reading from d.src as needed.
func (d *eventStreamDecoder) fill(n int) error {
	if len(d.buf) >= n {
		return nil
	}
	// Read up to 4KB increments until the buffer holds n bytes.
	tmp := make([]byte, 4096)
	for len(d.buf) < n {
		nr, err := d.src.Read(tmp)
		if nr > 0 {
			d.buf = append(d.buf, tmp[:nr]...)
		}
		if err == io.EOF {
			if len(d.buf) >= n {
				return nil
			}
			return io.EOF
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// parseFrame validates CRCs and extracts header + payload bytes.
func parseFrame(frame []byte) (frameHeader, []byte, error) {
	if len(frame) < minFrameLen {
		return frameHeader{}, nil, fmt.Errorf("event stream: short frame (%d bytes)", len(frame))
	}
	totalLen := int(binary.BigEndian.Uint32(frame[:4]))
	headersLen := int(binary.BigEndian.Uint32(frame[4:8]))
	preludeCRC := binary.BigEndian.Uint32(frame[8:12])
	messageCRC := binary.BigEndian.Uint32(frame[totalLen-4:])

	if totalLen != len(frame) {
		return frameHeader{}, nil, fmt.Errorf("event stream: length mismatch (got %d, want %d)", len(frame), totalLen)
	}
	if expectedPreludeCRC := crc32.ChecksumIEEE(frame[:8]); expectedPreludeCRC != preludeCRC {
		return frameHeader{}, nil, fmt.Errorf("%w: prelude (got %x, want %x)", ErrCRCMismatch, preludeCRC, expectedPreludeCRC)
	}
	if expectedMessageCRC := crc32.ChecksumIEEE(frame[:totalLen-4]); expectedMessageCRC != messageCRC {
		return frameHeader{}, nil, fmt.Errorf("%w: message (got %x, want %x)", ErrCRCMismatch, messageCRC, expectedMessageCRC)
	}

	headersEnd := 12 + headersLen
	if headersEnd > totalLen-4 {
		return frameHeader{}, nil, fmt.Errorf("event stream: headers length %d exceeds frame size", headersLen)
	}
	headers, err := parseHeaders(frame[12:headersEnd])
	if err != nil {
		return frameHeader{}, nil, err
	}
	payload := frame[headersEnd : totalLen-4]
	// Return a copy of the payload so the caller can retain it past the next
	// frame decode (we reuse d.buf in-place).
	payloadCopy := make([]byte, len(payload))
	copy(payloadCopy, payload)
	return headers, payloadCopy, nil
}

// parseHeaders walks the headers byte range, recognizing only the string-type
// headers (types 6 = uuid/byte-array string and 7 = utf-8 string) used by
// Bedrock. Other types are skipped after consuming the appropriate value
// length so we stay forward-compatible.
func parseHeaders(raw []byte) (frameHeader, error) {
	var hdr frameHeader
	i := 0
	for i < len(raw) {
		if i+1 > len(raw) {
			return hdr, fmt.Errorf("event stream: truncated header name length")
		}
		nameLen := int(raw[i])
		i++
		if i+nameLen > len(raw) {
			return hdr, fmt.Errorf("event stream: truncated header name")
		}
		name := string(raw[i : i+nameLen])
		i += nameLen
		if i >= len(raw) {
			return hdr, fmt.Errorf("event stream: missing header value type")
		}
		valueType := raw[i]
		i++

		var valueStr string
		var consumed int
		var err error
		valueStr, consumed, err = readHeaderValue(valueType, raw[i:])
		if err != nil {
			return hdr, err
		}
		i += consumed

		dispatchHeader(&hdr, name, valueStr)
	}
	return hdr, nil
}

func dispatchHeader(hdr *frameHeader, name, value string) {
	switch name {
	case ":message-type":
		hdr.MessageType = value
	case ":event-type":
		hdr.EventType = value
	case ":exception-type":
		hdr.ExceptionType = value
	case ":content-type":
		hdr.ContentType = value
	default:
		// Skip non-string typed headers (which decode to empty value) so the
		// Other map only carries truly string-valued metadata.
		if value == "" {
			return
		}
		if hdr.Other == nil {
			hdr.Other = map[string]string{}
		}
		hdr.Other[name] = value
	}
}

// readHeaderValue consumes the value bytes for a single header entry. Only
// string-like types (6, 7) are decoded; other types are skipped after reading
// the documented number of bytes. Returns the decoded string (empty when not
// a string type), the number of bytes consumed, and any error.
func readHeaderValue(valueType byte, rest []byte) (string, int, error) {
	switch valueType {
	case 0: // true
		return "true", 0, nil
	case 1: // false
		return "false", 0, nil
	case 2: // int8 (1 byte)
		if len(rest) < 1 {
			return "", 0, fmt.Errorf("event stream: truncated int8")
		}
		return "", 1, nil
	case 3: // int16 (2 bytes)
		if len(rest) < 2 {
			return "", 0, fmt.Errorf("event stream: truncated int16")
		}
		return "", 2, nil
	case 4: // int32 (4 bytes)
		if len(rest) < 4 {
			return "", 0, fmt.Errorf("event stream: truncated int32")
		}
		return "", 4, nil
	case 5: // int64 (8 bytes)
		if len(rest) < 8 {
			return "", 0, fmt.Errorf("event stream: truncated int64")
		}
		return "", 8, nil
	case 6: // byte_array (length-prefixed 2-byte length)
		if len(rest) < 2 {
			return "", 0, fmt.Errorf("event stream: truncated byte_array length")
		}
		l := int(binary.BigEndian.Uint16(rest[:2]))
		if 2+l > len(rest) {
			return "", 0, fmt.Errorf("event stream: truncated byte_array value")
		}
		return string(rest[2 : 2+l]), 2 + l, nil
	case 7: // string (length-prefixed 2-byte length)
		if len(rest) < 2 {
			return "", 0, fmt.Errorf("event stream: truncated string length")
		}
		l := int(binary.BigEndian.Uint16(rest[:2]))
		if 2+l > len(rest) {
			return "", 0, fmt.Errorf("event stream: truncated string value")
		}
		return string(rest[2 : 2+l]), 2 + l, nil
	case 8: // timestamp (8 bytes)
		if len(rest) < 8 {
			return "", 0, fmt.Errorf("event stream: truncated timestamp")
		}
		return "", 8, nil
	case 9: // uuid (16 bytes)
		if len(rest) < 16 {
			return "", 0, fmt.Errorf("event stream: truncated uuid")
		}
		return "", 16, nil
	default:
		return "", 0, fmt.Errorf("event stream: unknown header value type %d", valueType)
	}
}

// encodeFrame serializes a header + payload pair into the AWS event-stream
// binary format. Used by the conformance harness Bedrock-framing replay
// server and by tests of the decoder.
//
// All headers in hdr are written as string-typed entries (type 7). Empty
// fields are skipped. The caller's Other map (if any) is written first;
// known fields follow in a stable order so encoded output is byte-stable.
func encodeFrame(hdr frameHeader, payload []byte) []byte {
	headerBytes := encodeHeaders(hdr)
	headersLen := len(headerBytes)
	totalLen := preludeSize + 4 + headersLen + len(payload) + 4

	frame := make([]byte, 0, totalLen)
	prelude := make([]byte, 8)
	binary.BigEndian.PutUint32(prelude[:4], uint32(totalLen))
	binary.BigEndian.PutUint32(prelude[4:], uint32(headersLen))
	frame = append(frame, prelude...)
	preludeCRC := crc32.ChecksumIEEE(prelude)
	preludeCRCBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(preludeCRCBytes, preludeCRC)
	frame = append(frame, preludeCRCBytes...)
	frame = append(frame, headerBytes...)
	frame = append(frame, payload...)
	messageCRC := crc32.ChecksumIEEE(frame)
	messageCRCBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(messageCRCBytes, messageCRC)
	frame = append(frame, messageCRCBytes...)
	return frame
}

func encodeHeaders(hdr frameHeader) []byte {
	var buf []byte
	add := func(name, value string) {
		if name == "" || value == "" {
			return
		}
		buf = append(buf, byte(len(name)))
		buf = append(buf, name...)
		buf = append(buf, 7) // string type
		lenBytes := make([]byte, 2)
		binary.BigEndian.PutUint16(lenBytes, uint16(len(value)))
		buf = append(buf, lenBytes...)
		buf = append(buf, value...)
	}
	// Known headers first for deterministic ordering.
	add(":message-type", hdr.MessageType)
	add(":event-type", hdr.EventType)
	add(":exception-type", hdr.ExceptionType)
	add(":content-type", hdr.ContentType)
	for k, v := range hdr.Other {
		add(k, v)
	}
	return buf
}
